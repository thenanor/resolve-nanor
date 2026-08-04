package tickets

import (
	"context"
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
	repo  Repository
	audit AuditRecorder
}

func NewService(repo Repository, audit AuditRecorder) *Service {
	return &Service{repo: repo, audit: audit}
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
	if page.Limit == 0 {
		page.Limit = DefaultPageLimit
	}
	if page.Limit < 0 {
		return Page{}, invalid("limit must be a positive integer")
	}
	if page.Limit > MaxPageLimit {
		page.Limit = MaxPageLimit
	}
	if page.Offset < 0 {
		return Page{}, invalid("offset must be zero or a positive integer")
	}
	return s.repo.FindPage(ctx, filter, page)
}

// ListAudit returns id's audit trail, newest first. The repository lists
// chronologically (oldest first, matching how entries are recorded), so the
// reversal happens here rather than being duplicated across repository
// implementations and test fakes.
func (s *Service) ListAudit(ctx context.Context, id string) ([]AuditEntry, error) {
	if _, err := s.FindByID(ctx, id); err != nil {
		return nil, err
	}
	entries, err := s.audit.List(ctx, id)
	if err != nil {
		return nil, err
	}
	newestFirst := make([]AuditEntry, len(entries))
	for i, e := range entries {
		newestFirst[len(entries)-1-i] = e
	}
	return newestFirst, nil
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
