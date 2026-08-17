package tickets

import (
	"context"
	"log"
	"regexp"
	"strings"
	"time"

	"resolve/internal/ids"
)

var emailRe = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

const (
	DefaultPageLimit = 50
	MaxPageLimit     = 200
)

// AuditEntry mirrors the fields of audit.Entry that tickets exposes over its
// ticket-scoped audit endpoint. It is declared here (consumer side), like
// AuditRecorder below, so tickets does not depend on the audit package.
type AuditEntry struct {
	Actor    string         `json:"actor"`
	Action   string         `json:"action"`
	TicketID string         `json:"ticketId"`
	Details  map[string]any `json:"details"`
	At       string         `json:"at"`
}

// AuditRecorder is the slice of audit.Service that tickets depends on. It is
// declared here (consumer side) to avoid an import cycle between the
// tickets and audit packages.
type AuditRecorder interface {
	Record(ctx context.Context, actor, action, ticketID string, details map[string]any) error
	List(ctx context.Context, ticketID string) ([]AuditEntry, error)
}

// TriageNotifier is the slice of the triage service's HTTP API that tickets
// depends on. Declared here (consumer side), like AuditRecorder above, so
// tickets does not need to import an HTTP-client package for triage.
type TriageNotifier interface {
	NotifyTicketCreated(ctx context.Context, ticketID, subject, description string) error
}

// triageNotifyTimeout bounds how long a fire-and-forget triage notification
// may run. It is generous rather than tuned, since it never blocks a caller
// — see the comment at its use site in Create.
const triageNotifyTimeout = 20 * time.Second

type CreateInput struct {
	Subject       string
	Description   string
	CustomerEmail string
	Priority      string
}

type CommentInput struct {
	Author   string
	Body     string
	Internal bool
}

type Service struct {
	repo   Repository
	audit  AuditRecorder
	triage TriageNotifier
}

// NewService wires a Service together. triage is variadic (0 or 1 value)
// rather than a required parameter so existing tests that predate the
// triage feature keep compiling unchanged; production wiring (cmd/api)
// always passes one explicitly. Create only dispatches a notification when
// a TriageNotifier is actually configured.
func NewService(repo Repository, audit AuditRecorder, triage ...TriageNotifier) *Service {
	var t TriageNotifier
	if len(triage) > 0 {
		t = triage[0]
	}
	return &Service{repo: repo, audit: audit, triage: t}
}

func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func (s *Service) Create(ctx context.Context, actor string, input CreateInput) (*Ticket, error) {
	if strings.TrimSpace(input.Subject) == "" {
		return nil, invalid("subject must be a non-empty string")
	}
	if strings.TrimSpace(input.Description) == "" {
		return nil, invalid("description must be a non-empty string")
	}
	if input.CustomerEmail == "" || !emailRe.MatchString(input.CustomerEmail) {
		return nil, invalid("customerEmail must be a valid email address")
	}
	priority := Priority(input.Priority)
	if !priority.Valid() {
		names := make([]string, len(AllPriorities))
		for i, p := range AllPriorities {
			names[i] = string(p)
		}
		return nil, invalid("priority must be one of: %s", strings.Join(names, ", "))
	}

	ts := now()
	t := &Ticket{
		ID:            ids.New("tkt"),
		Subject:       strings.TrimSpace(input.Subject),
		Description:   strings.TrimSpace(input.Description),
		CustomerEmail: input.CustomerEmail,
		Priority:      priority,
		Status:        StatusNew,
		Comments:      []Comment{},
		CreatedAt:     ts,
		UpdatedAt:     ts,
		ResolvedAt:    nil,
	}

	if err := s.repo.CreateTicket(ctx, t); err != nil {
		return nil, err
	}
	if err := s.audit.Record(ctx, actor, "ticket.created", t.ID, map[string]any{
		"subject":  t.Subject,
		"priority": string(t.Priority),
	}); err != nil {
		return nil, err
	}

	// Fire-and-forget: notify the triage service so it can classify the
	// ticket asynchronously. This must not use the request's ctx — chi
	// cancels it the moment this handler returns, which happens right
	// after Create does, so a goroutine inheriting it would fail with
	// context.Canceled on effectively every call. Arguments are passed by
	// value rather than closing over t, since t is a pointer the caller
	// still owns.
	if s.triage != nil {
		go func(ticketID, subject, description string) {
			ctx, cancel := context.WithTimeout(context.Background(), triageNotifyTimeout)
			defer cancel()
			if err := s.triage.NotifyTicketCreated(ctx, ticketID, subject, description); err != nil {
				log.Printf("triage notify failed for ticket %s: %v", ticketID, err)
			}
		}(t.ID, t.Subject, t.Description)
	}

	return t, nil
}

func (s *Service) ChangeStatus(ctx context.Context, actor, id, to string) (*Ticket, error) {
	t, err := s.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	allowed := AllowedTransitions[t.Status]
	if to == "" || !containsStatus(allowed, Status(to)) {
		names := make([]string, len(allowed))
		for i, a := range allowed {
			names[i] = string(a)
		}
		list := strings.Join(names, ", ")
		if list == "" {
			list = "(none — terminal state)"
		}
		return nil, invalid("cannot move ticket from '%s' to '%s'; allowed: %s", t.Status, to, list)
	}

	from := t.Status
	t.Status = Status(to)
	if t.Status == StatusResolved {
		resolvedAt := now()
		t.ResolvedAt = &resolvedAt
	}
	t.UpdatedAt = now()

	if err := s.repo.UpdateTicket(ctx, t); err != nil {
		return nil, err
	}
	if err := s.audit.Record(ctx, actor, "ticket.status_changed", t.ID, map[string]any{
		"from": string(from),
		"to":   to,
	}); err != nil {
		return nil, err
	}
	return t, nil
}

// ApplyTriage records a classification result from the triage service.
// medium/high-confidence results overwrite the ticket's category/priority
// directly; low-confidence results are stored as a pending suggestion so an
// uncertain classification never silently overrides a ticket's priority.
func (s *Service) ApplyTriage(ctx context.Context, actor, id, category, priority, confidence string) (*Ticket, error) {
	t, err := s.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	cat := Category(category)
	if !cat.Valid() {
		names := make([]string, len(AllCategories))
		for i, c := range AllCategories {
			names[i] = string(c)
		}
		return nil, invalid("category must be one of: %s", strings.Join(names, ", "))
	}
	pri := Priority(priority)
	if !pri.Valid() {
		names := make([]string, len(AllPriorities))
		for i, p := range AllPriorities {
			names[i] = string(p)
		}
		return nil, invalid("priority must be one of: %s", strings.Join(names, ", "))
	}
	conf := Confidence(confidence)
	if !conf.Valid() {
		names := make([]string, len(AllConfidence))
		for i, c := range AllConfidence {
			names[i] = string(c)
		}
		return nil, invalid("confidence must be one of: %s", strings.Join(names, ", "))
	}

	t.UpdatedAt = now()

	if conf == ConfidenceLow {
		t.PendingCategory = &cat
		t.PendingPriority = &pri
		if err := s.repo.SetPendingTriage(ctx, t.ID, cat, pri, t.UpdatedAt); err != nil {
			return nil, err
		}
		if err := s.audit.Record(ctx, actor, "ticket.triage_needs_review", t.ID, map[string]any{
			"category":   string(cat),
			"priority":   string(pri),
			"confidence": string(conf),
		}); err != nil {
			return nil, err
		}
		return t, nil
	}

	t.Category = &cat
	t.Priority = pri
	t.PendingCategory = nil
	t.PendingPriority = nil
	if err := s.repo.SetTriage(ctx, t.ID, cat, pri, t.UpdatedAt); err != nil {
		return nil, err
	}
	if err := s.audit.Record(ctx, actor, "ticket.triaged", t.ID, map[string]any{
		"category":   string(cat),
		"priority":   string(pri),
		"confidence": string(conf),
	}); err != nil {
		return nil, err
	}
	return t, nil
}

// ReviewTriage records a human decision on a pending (low-confidence)
// triage suggestion: accept promotes it to the ticket's actual
// category/priority, reject discards it. It is an error to review a ticket
// with no pending suggestion.
func (s *Service) ReviewTriage(ctx context.Context, actor, id string, accept bool) (*Ticket, error) {
	t, err := s.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if t.PendingCategory == nil {
		return nil, invalid("ticket %s has no pending triage suggestion to review", id)
	}

	t.UpdatedAt = now()
	decision := "reject"
	if accept {
		decision = "accept"
		t.Category = t.PendingCategory
		t.Priority = *t.PendingPriority
	}
	t.PendingCategory = nil
	t.PendingPriority = nil

	if accept {
		if err := s.repo.AcceptPendingTriage(ctx, t.ID, t.UpdatedAt); err != nil {
			return nil, err
		}
	} else {
		if err := s.repo.RejectPendingTriage(ctx, t.ID, t.UpdatedAt); err != nil {
			return nil, err
		}
	}
	if err := s.audit.Record(ctx, actor, "ticket.triage_reviewed", t.ID, map[string]any{
		"decision": decision,
	}); err != nil {
		return nil, err
	}
	return t, nil
}

func (s *Service) AddComment(ctx context.Context, actor, id string, input CommentInput) (*Comment, error) {
	t, err := s.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.Author) == "" {
		return nil, invalid("author must be a non-empty string")
	}
	if strings.TrimSpace(input.Body) == "" {
		return nil, invalid("body must be a non-empty string")
	}

	c := Comment{
		ID:       ids.New("cmt"),
		Author:   strings.TrimSpace(input.Author),
		Body:     strings.TrimSpace(input.Body),
		Internal: input.Internal,
		At:       now(),
	}

	if err := s.repo.AddComment(ctx, t.ID, c); err != nil {
		return nil, err
	}
	if err := s.audit.Record(ctx, actor, "ticket.commented", t.ID, map[string]any{
		"commentId": c.ID,
		"internal":  c.Internal,
	}); err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Service) FindAll(ctx context.Context, filter Filter) ([]Ticket, error) {
	return s.repo.FindAll(ctx, filter)
}

// FindPage applies filter first, then bounds the matching rows to a page.
// A zero Limit takes the default; limits above MaxPageLimit are clamped
// rather than rejected, matching common list-endpoint conventions.
func (s *Service) FindPage(ctx context.Context, filter Filter, page Pagination) (Page, error) {
	page, err := normalizePage(page)
	if err != nil {
		return Page{}, err
	}
	return s.repo.FindPage(ctx, filter, page)
}

// normalizePage applies the pagination conventions shared by FindPage and
// ListAudit: a zero Limit takes the default, limits above MaxPageLimit are
// clamped rather than rejected, and negative Limit/Offset are rejected.
func normalizePage(page Pagination) (Pagination, error) {
	if page.Limit == 0 {
		page.Limit = DefaultPageLimit
	}
	if page.Limit < 0 {
		return Pagination{}, invalid("limit must be a positive integer")
	}
	if page.Limit > MaxPageLimit {
		page.Limit = MaxPageLimit
	}
	if page.Offset < 0 {
		return Pagination{}, invalid("offset must be zero or a positive integer")
	}
	return page, nil
}

// AuditPage is one page of a ticket's audit trail, newest first. It mirrors
// Page's shape (entries + limit/offset/hasMore) for consistency with
// FindPage.
type AuditPage struct {
	Entries []AuditEntry `json:"entries"`
	Limit   int          `json:"limit"`
	Offset  int          `json:"offset"`
	HasMore bool         `json:"hasMore"`
}

// ListAudit returns id's audit trail, newest first, bounded to a page using
// the same limit/offset conventions as FindPage. The repository lists
// chronologically (oldest first, matching how entries are recorded), so the
// reversal happens here rather than being duplicated across repository
// implementations and test fakes.
//
// AuditRecorder.List has no limit/offset of its own — it always returns a
// ticket's full audit trail — so pagination is applied to the in-memory
// result here rather than pushed down to a query. AuditRecorder is backed by
// backend/internal/audit, which this package cannot modify (see the
// protect-audit hook), so adding LIMIT/OFFSET at the SQL level isn't
// available from here.
func (s *Service) ListAudit(ctx context.Context, id string, page Pagination) (AuditPage, error) {
	if _, err := s.FindByID(ctx, id); err != nil {
		return AuditPage{}, err
	}
	page, err := normalizePage(page)
	if err != nil {
		return AuditPage{}, err
	}

	entries, err := s.audit.List(ctx, id)
	if err != nil {
		return AuditPage{}, err
	}
	newestFirst := make([]AuditEntry, len(entries))
	for i, e := range entries {
		newestFirst[len(entries)-1-i] = e
	}

	start := page.Offset
	if start > len(newestFirst) {
		start = len(newestFirst)
	}
	end := start + page.Limit
	hasMore := end < len(newestFirst)
	if end > len(newestFirst) {
		end = len(newestFirst)
	}

	return AuditPage{
		Entries: append([]AuditEntry{}, newestFirst[start:end]...),
		Limit:   page.Limit,
		Offset:  page.Offset,
		HasMore: hasMore,
	}, nil
}

func (s *Service) FindByID(ctx context.Context, id string) (*Ticket, error) {
	t, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, &NotFoundError{ID: id}
	}
	return t, nil
}

func containsStatus(list []Status, s Status) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
