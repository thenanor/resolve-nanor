package tickets

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"
)

// fakeRepository is an in-memory Repository used to unit test TicketsService
// without a database, mirroring the NestJS suite's use of in-memory SQLite
// but staying idiomatic Go (interface + fake, no embedded DB).
type fakeRepository struct {
	byID map[string]*Ticket
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{byID: map[string]*Ticket{}}
}

func (f *fakeRepository) CreateTicket(_ context.Context, t *Ticket) error {
	cp := *t
	cp.Comments = append([]Comment{}, t.Comments...)
	f.byID[t.ID] = &cp
	return nil
}

func (f *fakeRepository) UpdateTicket(_ context.Context, t *Ticket) error {
	existing, ok := f.byID[t.ID]
	if !ok {
		return errors.New("not found")
	}
	comments := existing.Comments
	cp := *t
	cp.Comments = comments
	f.byID[t.ID] = &cp
	return nil
}

func (f *fakeRepository) SetTriage(_ context.Context, id string, category Category, priority Priority, updatedAt string) error {
	t, ok := f.byID[id]
	if !ok {
		return errors.New("not found")
	}
	cat := category
	t.Category = &cat
	t.Priority = priority
	t.PendingCategory = nil
	t.PendingPriority = nil
	t.UpdatedAt = updatedAt
	return nil
}

func (f *fakeRepository) SetPendingTriage(_ context.Context, id string, category Category, priority Priority, updatedAt string) error {
	t, ok := f.byID[id]
	if !ok {
		return errors.New("not found")
	}
	cat, pri := category, priority
	t.PendingCategory = &cat
	t.PendingPriority = &pri
	t.UpdatedAt = updatedAt
	return nil
}

func (f *fakeRepository) AcceptPendingTriage(_ context.Context, id string, updatedAt string) error {
	t, ok := f.byID[id]
	if !ok {
		return errors.New("not found")
	}
	t.Category = t.PendingCategory
	if t.PendingPriority != nil {
		t.Priority = *t.PendingPriority
	}
	t.PendingCategory = nil
	t.PendingPriority = nil
	t.UpdatedAt = updatedAt
	return nil
}

func (f *fakeRepository) RejectPendingTriage(_ context.Context, id string, updatedAt string) error {
	t, ok := f.byID[id]
	if !ok {
		return errors.New("not found")
	}
	t.PendingCategory = nil
	t.PendingPriority = nil
	t.UpdatedAt = updatedAt
	return nil
}

func (f *fakeRepository) AddComment(_ context.Context, ticketID string, c Comment) error {
	t, ok := f.byID[ticketID]
	if !ok {
		return errors.New("not found")
	}
	c.Seq = int64(len(t.Comments) + 1)
	t.Comments = append(t.Comments, c)
	return nil
}

func (f *fakeRepository) FindByID(_ context.Context, id string) (*Ticket, error) {
	t, ok := f.byID[id]
	if !ok {
		return nil, nil
	}
	cp := *t
	cp.Comments = append([]Comment{}, t.Comments...)
	return &cp, nil
}

func (f *fakeRepository) FindAll(_ context.Context, filter Filter) ([]Ticket, error) {
	return f.findFiltered(filter), nil
}

// FindPage mirrors PostgresRepository.FindPage: filter, sort, then slice.
// HasMore is derived from whether a row exists past the requested limit,
// not from a separate count, matching the production implementation.
func (f *fakeRepository) FindPage(_ context.Context, filter Filter, page Pagination) (Page, error) {
	matched := f.findFiltered(filter)

	start := page.Offset
	if start > len(matched) {
		start = len(matched)
	}
	end := start + page.Limit
	hasMore := end < len(matched)
	if end > len(matched) {
		end = len(matched)
	}

	tickets := append([]Ticket{}, matched[start:end]...)
	return Page{
		Tickets: tickets,
		Limit:   page.Limit,
		Offset:  page.Offset,
		HasMore: hasMore,
	}, nil
}

// findFiltered returns matching tickets ordered like the real repository's
// "ORDER BY created_at ASC, id ASC" so pagination in tests is deterministic
// despite fakeRepository storing tickets in a map.
func (f *fakeRepository) findFiltered(filter Filter) []Ticket {
	var result []Ticket
	for _, t := range f.byID {
		if filter.Status != "" && t.Status != filter.Status {
			continue
		}
		if filter.Priority != "" && t.Priority != filter.Priority {
			continue
		}
		cp := *t
		cp.Comments = append([]Comment{}, t.Comments...)
		result = append(result, cp)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt != result[j].CreatedAt {
			return result[i].CreatedAt < result[j].CreatedAt
		}
		return result[i].ID < result[j].ID
	})
	return result
}

// fakeAudit is a capturing AuditRecorder used to assert on the audit trail
// written by TicketsService, without depending on the audit package.
type fakeAudit struct {
	entries []auditRecord
}

type auditRecord struct {
	Actor    string
	Action   string
	TicketID string
	Details  map[string]any
}

func (f *fakeAudit) Record(_ context.Context, actor, action, ticketID string, details map[string]any) error {
	f.entries = append(f.entries, auditRecord{Actor: actor, Action: action, TicketID: ticketID, Details: details})
	return nil
}

// List mirrors PostgresRepository.List's chronological (oldest-first)
// ordering, so ListAudit's newest-first reversal is exercised against
// realistic input rather than an already-reversed fake.
func (f *fakeAudit) List(_ context.Context, ticketID string) ([]AuditEntry, error) {
	entries := []AuditEntry{}
	for _, e := range f.forTicket(ticketID) {
		entries = append(entries, AuditEntry{Actor: e.Actor, Action: e.Action, TicketID: e.TicketID, Details: e.Details})
	}
	return entries, nil
}

func (f *fakeAudit) forTicket(ticketID string) []auditRecord {
	var out []auditRecord
	for _, e := range f.entries {
		if e.TicketID == ticketID {
			out = append(out, e)
		}
	}
	return out
}

func newTestService() (*Service, *fakeAudit) {
	audit := &fakeAudit{}
	svc := NewService(newFakeRepository(), audit)
	return svc, audit
}

// triageCall captures one NotifyTicketCreated invocation, for tests that
// assert on the goroutine dispatched by Create.
type triageCall struct {
	ticketID, subject, description string
}

// channelTriageNotifier is a TriageNotifier used to assert, without
// time.Sleep-based flakiness, that Create's triage notification runs in its
// own goroutine: block (if non-nil) lets a test hold the call open to prove
// Create doesn't wait for it, and calls reports what was received.
type channelTriageNotifier struct {
	calls chan triageCall
	block chan struct{}
}

func newChannelTriageNotifier() *channelTriageNotifier {
	return &channelTriageNotifier{calls: make(chan triageCall, 1)}
}

func (f *channelTriageNotifier) NotifyTicketCreated(_ context.Context, ticketID, subject, description string) error {
	if f.block != nil {
		<-f.block
	}
	f.calls <- triageCall{ticketID: ticketID, subject: subject, description: description}
	return nil
}

func newTestServiceWithTriage(triage TriageNotifier) (*Service, *fakeAudit) {
	audit := &fakeAudit{}
	svc := NewService(newFakeRepository(), audit, triage)
	return svc, audit
}

func TestCreate_DoesNotBlockOnTriageNotification(t *testing.T) {
	notifier := &channelTriageNotifier{calls: make(chan triageCall, 1), block: make(chan struct{})}
	svc, _ := newTestServiceWithTriage(notifier)

	done := make(chan struct{})
	go func() {
		if _, err := svc.Create(context.Background(), "test", validInput); err != nil {
			t.Errorf("Create returned error: %v", err)
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Create did not return promptly; it appears to be waiting on the triage notification")
	}
}

func TestCreate_DispatchesTriageNotificationWithTicketFields(t *testing.T) {
	notifier := newChannelTriageNotifier()
	svc, _ := newTestServiceWithTriage(notifier)

	ticket, err := svc.Create(context.Background(), "test", validInput)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	select {
	case call := <-notifier.calls:
		if call.ticketID != ticket.ID {
			t.Errorf("ticketID = %q, want %q", call.ticketID, ticket.ID)
		}
		if call.subject != validInput.Subject {
			t.Errorf("subject = %q, want %q", call.subject, validInput.Subject)
		}
		if call.description != validInput.Description {
			t.Errorf("description = %q, want %q", call.description, validInput.Description)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("NotifyTicketCreated was never called")
	}
}

func TestApplyTriage_HighConfidenceAppliesDirectly(t *testing.T) {
	svc, audit := newTestService()
	ticket, err := svc.Create(context.Background(), "test", validInput)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	got, err := svc.ApplyTriage(context.Background(), "triage-service", ticket.ID, "billing", "urgent", "high")
	if err != nil {
		t.Fatalf("ApplyTriage returned error: %v", err)
	}
	if got.Category == nil || *got.Category != CategoryBilling {
		t.Errorf("Category = %v, want %q", got.Category, CategoryBilling)
	}
	if got.Priority != PriorityUrgent {
		t.Errorf("Priority = %q, want %q", got.Priority, PriorityUrgent)
	}
	if got.PendingCategory != nil || got.PendingPriority != nil {
		t.Errorf("expected no pending suggestion, got PendingCategory=%v PendingPriority=%v", got.PendingCategory, got.PendingPriority)
	}

	entries := audit.forTicket(ticket.ID)
	found := false
	for _, e := range entries {
		if e.Action == "ticket.triaged" {
			found = true
			if e.Details["category"] != "billing" || e.Details["priority"] != "urgent" {
				t.Errorf("audit details = %+v, want category=billing priority=urgent", e.Details)
			}
		}
	}
	if !found {
		t.Error("expected a ticket.triaged audit entry")
	}
}

func TestApplyTriage_MediumConfidenceAppliesDirectly(t *testing.T) {
	svc, _ := newTestService()
	ticket, err := svc.Create(context.Background(), "test", validInput)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	got, err := svc.ApplyTriage(context.Background(), "triage-service", ticket.ID, "bug", "low", "medium")
	if err != nil {
		t.Fatalf("ApplyTriage returned error: %v", err)
	}
	if got.Category == nil || *got.Category != CategoryBug {
		t.Errorf("Category = %v, want %q", got.Category, CategoryBug)
	}
	if got.Priority != PriorityLow {
		t.Errorf("Priority = %q, want %q", got.Priority, PriorityLow)
	}
}

func TestApplyTriage_LowConfidenceStoresPendingSuggestionOnly(t *testing.T) {
	svc, audit := newTestService()
	ticket, err := svc.Create(context.Background(), "test", validInput)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	originalPriority := ticket.Priority

	got, err := svc.ApplyTriage(context.Background(), "triage-service", ticket.ID, "how_to", "urgent", "low")
	if err != nil {
		t.Fatalf("ApplyTriage returned error: %v", err)
	}
	if got.Category != nil {
		t.Errorf("Category = %v, want nil (low confidence must not apply directly)", got.Category)
	}
	if got.Priority != originalPriority {
		t.Errorf("Priority = %q, want unchanged %q", got.Priority, originalPriority)
	}
	if got.PendingCategory == nil || *got.PendingCategory != CategoryHowTo {
		t.Errorf("PendingCategory = %v, want %q", got.PendingCategory, CategoryHowTo)
	}
	if got.PendingPriority == nil || *got.PendingPriority != PriorityUrgent {
		t.Errorf("PendingPriority = %v, want %q", got.PendingPriority, PriorityUrgent)
	}

	entries := audit.forTicket(ticket.ID)
	found := false
	for _, e := range entries {
		if e.Action == "ticket.triage_needs_review" {
			found = true
		}
	}
	if !found {
		t.Error("expected a ticket.triage_needs_review audit entry")
	}
}

func TestApplyTriage_InvalidCategoryRejected(t *testing.T) {
	svc, _ := newTestService()
	ticket, err := svc.Create(context.Background(), "test", validInput)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	_, err = svc.ApplyTriage(context.Background(), "triage-service", ticket.ID, "not_a_category", "urgent", "high")
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected a *ValidationError, got %v", err)
	}
}

func TestApplyTriage_InvalidPriorityRejected(t *testing.T) {
	svc, _ := newTestService()
	ticket, err := svc.Create(context.Background(), "test", validInput)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	_, err = svc.ApplyTriage(context.Background(), "triage-service", ticket.ID, "billing", "not_a_priority", "high")
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected a *ValidationError, got %v", err)
	}
}

func TestApplyTriage_InvalidConfidenceRejected(t *testing.T) {
	svc, _ := newTestService()
	ticket, err := svc.Create(context.Background(), "test", validInput)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	_, err = svc.ApplyTriage(context.Background(), "triage-service", ticket.ID, "billing", "urgent", "not_a_confidence")
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected a *ValidationError, got %v", err)
	}
}

func TestReviewTriage_AcceptPromotesPendingSuggestion(t *testing.T) {
	svc, audit := newTestService()
	ticket, err := svc.Create(context.Background(), "test", validInput)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if _, err := svc.ApplyTriage(context.Background(), "triage-service", ticket.ID, "how_to", "urgent", "low"); err != nil {
		t.Fatalf("ApplyTriage returned error: %v", err)
	}

	got, err := svc.ReviewTriage(context.Background(), "test", ticket.ID, true)
	if err != nil {
		t.Fatalf("ReviewTriage returned error: %v", err)
	}
	if got.Category == nil || *got.Category != CategoryHowTo {
		t.Errorf("Category = %v, want %q", got.Category, CategoryHowTo)
	}
	if got.Priority != PriorityUrgent {
		t.Errorf("Priority = %q, want %q", got.Priority, PriorityUrgent)
	}
	if got.PendingCategory != nil || got.PendingPriority != nil {
		t.Errorf("expected pending fields cleared, got PendingCategory=%v PendingPriority=%v", got.PendingCategory, got.PendingPriority)
	}

	entries := audit.forTicket(ticket.ID)
	found := false
	for _, e := range entries {
		if e.Action == "ticket.triage_reviewed" && e.Details["decision"] == "accept" {
			found = true
		}
	}
	if !found {
		t.Error("expected a ticket.triage_reviewed audit entry with decision=accept")
	}
}

func TestReviewTriage_RejectDiscardsPendingSuggestion(t *testing.T) {
	svc, audit := newTestService()
	ticket, err := svc.Create(context.Background(), "test", validInput)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	originalPriority := ticket.Priority
	if _, err := svc.ApplyTriage(context.Background(), "triage-service", ticket.ID, "how_to", "urgent", "low"); err != nil {
		t.Fatalf("ApplyTriage returned error: %v", err)
	}

	got, err := svc.ReviewTriage(context.Background(), "test", ticket.ID, false)
	if err != nil {
		t.Fatalf("ReviewTriage returned error: %v", err)
	}
	if got.Category != nil {
		t.Errorf("Category = %v, want nil (rejected suggestion must not apply)", got.Category)
	}
	if got.Priority != originalPriority {
		t.Errorf("Priority = %q, want unchanged %q", got.Priority, originalPriority)
	}
	if got.PendingCategory != nil || got.PendingPriority != nil {
		t.Errorf("expected pending fields cleared, got PendingCategory=%v PendingPriority=%v", got.PendingCategory, got.PendingPriority)
	}

	entries := audit.forTicket(ticket.ID)
	found := false
	for _, e := range entries {
		if e.Action == "ticket.triage_reviewed" && e.Details["decision"] == "reject" {
			found = true
		}
	}
	if !found {
		t.Error("expected a ticket.triage_reviewed audit entry with decision=reject")
	}
}

func TestReviewTriage_NoPendingSuggestionIsRejected(t *testing.T) {
	svc, _ := newTestService()
	ticket, err := svc.Create(context.Background(), "test", validInput)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	_, err = svc.ReviewTriage(context.Background(), "test", ticket.ID, true)
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected a *ValidationError, got %v", err)
	}
}

func TestReviewTriage_UnknownTicketReturnsNotFound(t *testing.T) {
	svc, _ := newTestService()
	_, err := svc.ReviewTriage(context.Background(), "test", "tkt_doesnotexist", true)
	var nf *NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("expected a *NotFoundError, got %v", err)
	}
}

var validInput = CreateInput{
	Subject:       "Cannot log in",
	Description:   "Password reset email never arrives",
	CustomerEmail: "ani@example.am",
	Priority:      "high",
}

func TestCreate_ValidTicketStartsNew(t *testing.T) {
	svc, _ := newTestService()
	ticket, err := svc.Create(context.Background(), "test", validInput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ticket.Status != StatusNew {
		t.Errorf("status = %q, want %q", ticket.Status, StatusNew)
	}
	if ticket.Priority != PriorityHigh {
		t.Errorf("priority = %q, want %q", ticket.Priority, PriorityHigh)
	}
	if ticket.ResolvedAt != nil {
		t.Errorf("resolvedAt = %v, want nil", *ticket.ResolvedAt)
	}
}

func TestCreate_RejectsInvalidInputNamingField(t *testing.T) {
	cases := []struct {
		name  string
		input CreateInput
		field string
	}{
		{"blank subject", withSubject(validInput, "  "), "subject"},
		{"empty description", withDescription(validInput, ""), "description"},
		{"bad email", withEmail(validInput, "not-an-email"), "customerEmail"},
		{"bad priority", withPriority(validInput, "critical"), "priority"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			svc, _ := newTestService()
			_, err := svc.Create(context.Background(), "test", c.input)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			var ve *ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("expected ValidationError, got %T: %v", err, err)
			}
			if !strings.Contains(ve.Message, c.field) {
				t.Errorf("message %q does not mention field %q", ve.Message, c.field)
			}
		})
	}
}

func TestChangeStatus_WalksHappyPath(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()
	ticket, _ := svc.Create(ctx, "test", validInput)

	for _, to := range []string{"open", "in_progress", "resolved", "closed"} {
		if _, err := svc.ChangeStatus(ctx, "test", ticket.ID, to); err != nil {
			t.Fatalf("transition to %s: %v", to, err)
		}
	}

	final, err := svc.FindByID(ctx, ticket.ID)
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != StatusClosed {
		t.Errorf("status = %q, want closed", final.Status)
	}
	if final.ResolvedAt == nil {
		t.Error("resolvedAt is nil, want set")
	}
}

func TestChangeStatus_SupportsWaitingCustomerLoop(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()
	ticket, _ := svc.Create(ctx, "test", validInput)

	for _, to := range []string{"open", "in_progress", "waiting_customer", "in_progress"} {
		if _, err := svc.ChangeStatus(ctx, "test", ticket.ID, to); err != nil {
			t.Fatalf("transition to %s: %v", to, err)
		}
	}

	final, _ := svc.FindByID(ctx, ticket.ID)
	if final.Status != StatusInProgress {
		t.Errorf("status = %q, want in_progress", final.Status)
	}
}

func TestChangeStatus_RejectsSkippingStates(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()
	ticket, _ := svc.Create(ctx, "test", validInput)

	_, err := svc.ChangeStatus(ctx, "test", ticket.ID, "resolved")
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
}

func TestChangeStatus_RejectsReopeningClosedTicket(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()
	ticket, _ := svc.Create(ctx, "test", validInput)
	for _, to := range []string{"open", "in_progress", "resolved", "closed"} {
		if _, err := svc.ChangeStatus(ctx, "test", ticket.ID, to); err != nil {
			t.Fatalf("transition to %s: %v", to, err)
		}
	}

	_, err := svc.ChangeStatus(ctx, "test", ticket.ID, "open")
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
}

func TestChangeStatus_ErrorNamesAllowedNextStates(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()
	ticket, _ := svc.Create(ctx, "test", validInput)

	_, err := svc.ChangeStatus(ctx, "test", ticket.ID, "closed")
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	if !strings.Contains(ve.Message, "open") {
		t.Errorf("message %q does not name allowed state 'open'", ve.Message)
	}
}

func TestAddComment_OrdersPublicAndInternal(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()
	ticket, _ := svc.Create(ctx, "test", validInput)

	if _, err := svc.AddComment(ctx, "agent-1", ticket.ID, CommentInput{Author: "agent-1", Body: "Looking into it", Internal: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddComment(ctx, "ani", ticket.ID, CommentInput{Author: "ani", Body: "Any update?", Internal: false}); err != nil {
		t.Fatal(err)
	}

	final, _ := svc.FindByID(ctx, ticket.ID)
	if len(final.Comments) != 2 {
		t.Fatalf("len(comments) = %d, want 2", len(final.Comments))
	}
	if !final.Comments[0].Internal {
		t.Error("comments[0].Internal = false, want true")
	}
	if final.Comments[1].Author != "ani" {
		t.Errorf("comments[1].Author = %q, want ani", final.Comments[1].Author)
	}
}

func TestAddComment_RejectsEmptyBody(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()
	ticket, _ := svc.Create(ctx, "test", validInput)

	_, err := svc.AddComment(ctx, "x", ticket.ID, CommentInput{Author: "x", Body: " "})
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
}

func TestAuditTrail_RecordsCreationStatusChangesAndComments(t *testing.T) {
	svc, audit := newTestService()
	ctx := context.Background()
	ticket, _ := svc.Create(ctx, "nanor", validInput)
	if _, err := svc.ChangeStatus(ctx, "nanor", ticket.ID, "open"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddComment(ctx, "agent-1", ticket.ID, CommentInput{Author: "agent-1", Body: "hi"}); err != nil {
		t.Fatal(err)
	}

	entries := audit.forTicket(ticket.ID)
	if len(entries) != 3 {
		t.Fatalf("len(entries) = %d, want 3", len(entries))
	}
	wantActions := []string{"ticket.created", "ticket.status_changed", "ticket.commented"}
	for i, want := range wantActions {
		if entries[i].Action != want {
			t.Errorf("entries[%d].Action = %q, want %q", i, entries[i].Action, want)
		}
	}
	if entries[0].Actor != "nanor" {
		t.Errorf("entries[0].Actor = %q, want nanor", entries[0].Actor)
	}
	if entries[1].Details["from"] != "new" || entries[1].Details["to"] != "open" {
		t.Errorf("entries[1].Details = %v, want {from: new, to: open}", entries[1].Details)
	}
	if entries[2].Actor != "agent-1" {
		t.Errorf("entries[2].Actor = %q, want agent-1", entries[2].Actor)
	}
}

func TestListAudit_ReturnsEntriesNewestFirst(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()
	ticket, _ := svc.Create(ctx, "nanor", validInput)
	if _, err := svc.ChangeStatus(ctx, "nanor", ticket.ID, "open"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddComment(ctx, "agent-1", ticket.ID, CommentInput{Author: "agent-1", Body: "hi"}); err != nil {
		t.Fatal(err)
	}

	page, err := svc.ListAudit(ctx, ticket.ID, Pagination{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantActions := []string{"ticket.commented", "ticket.status_changed", "ticket.created"}
	if len(page.Entries) != len(wantActions) {
		t.Fatalf("len(entries) = %d, want %d", len(page.Entries), len(wantActions))
	}
	for i, want := range wantActions {
		if page.Entries[i].Action != want {
			t.Errorf("entries[%d].Action = %q, want %q", i, page.Entries[i].Action, want)
		}
	}
	if page.Limit != DefaultPageLimit {
		t.Errorf("limit = %d, want %d", page.Limit, DefaultPageLimit)
	}
	if page.Offset != 0 {
		t.Errorf("offset = %d, want 0", page.Offset)
	}
	if page.HasMore {
		t.Error("hasMore = true, want false")
	}
}

func TestListAudit_RespectsLimitAndOffset(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()
	ticket, _ := svc.Create(ctx, "nanor", validInput)
	if _, err := svc.ChangeStatus(ctx, "nanor", ticket.ID, "open"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddComment(ctx, "agent-1", ticket.ID, CommentInput{Author: "agent-1", Body: "hi"}); err != nil {
		t.Fatal(err)
	}

	// newest first: [commented, status_changed, created]
	page, err := svc.ListAudit(ctx, ticket.ID, Pagination{Limit: 1, Offset: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page.Entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(page.Entries))
	}
	if page.Entries[0].Action != "ticket.status_changed" {
		t.Errorf("entries[0].Action = %q, want ticket.status_changed", page.Entries[0].Action)
	}
	if !page.HasMore {
		t.Error("hasMore = false, want true")
	}
}

func TestListAudit_ClampsLimitToMax(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()
	ticket, _ := svc.Create(ctx, "nanor", validInput)

	page, err := svc.ListAudit(ctx, ticket.ID, Pagination{Limit: 5000})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page.Limit != MaxPageLimit {
		t.Errorf("limit = %d, want %d", page.Limit, MaxPageLimit)
	}
}

func TestListAudit_RejectsNegativeLimit(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()
	ticket, _ := svc.Create(ctx, "nanor", validInput)

	_, err := svc.ListAudit(ctx, ticket.ID, Pagination{Limit: -1})
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
}

func TestListAudit_RejectsNegativeOffset(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()
	ticket, _ := svc.Create(ctx, "nanor", validInput)

	_, err := svc.ListAudit(ctx, ticket.ID, Pagination{Offset: -1})
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
}

func TestListAudit_404sOnUnknownTicket(t *testing.T) {
	svc, _ := newTestService()
	_, err := svc.ListAudit(context.Background(), "tkt_missing", Pagination{})
	var nf *NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("expected NotFoundError, got %v", err)
	}
}

func TestFindByID_404sOnUnknownTicket(t *testing.T) {
	svc, _ := newTestService()
	_, err := svc.FindByID(context.Background(), "tkt_missing")
	var nf *NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("expected NotFoundError, got %v", err)
	}
}

func withSubject(in CreateInput, v string) CreateInput     { in.Subject = v; return in }
func withDescription(in CreateInput, v string) CreateInput { in.Description = v; return in }
func withEmail(in CreateInput, v string) CreateInput       { in.CustomerEmail = v; return in }
func withPriority(in CreateInput, v string) CreateInput    { in.Priority = v; return in }
