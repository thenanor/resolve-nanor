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

// --- REPLYGUARD-1: AddComment's synchronous reply-guard gate ---

// fakeReplyGuardClient is a ReplyGuardClient used to assert on what
// AddComment sends to reply-guard and to script what it returns, mirroring
// fakeAudit's role for AuditRecorder.
type fakeReplyGuardClient struct {
	calls  []ReplyGuardInput
	result ReplyGuardOutcome
	err    error
}

func (f *fakeReplyGuardClient) Guard(_ context.Context, input ReplyGuardInput) (ReplyGuardOutcome, error) {
	f.calls = append(f.calls, input)
	return f.result, f.err
}

func newTestServiceWithGuard(guard ReplyGuardClient) (*Service, *fakeAudit) {
	audit := &fakeAudit{}
	svc := NewService(newFakeRepository(), audit).WithReplyGuard(guard)
	return svc, audit
}

func TestAddComment_AC3_InternalNeverGuardedRegardlessOfFlags(t *testing.T) {
	guard := &fakeReplyGuardClient{result: ReplyGuardOutcome{Verdict: "escalate"}}
	svc, _ := newTestServiceWithGuard(guard)
	ctx := context.Background()
	ticket, _ := svc.Create(ctx, "test", validInput)

	c, err := svc.AddComment(ctx, "agent-1", ticket.ID, CommentInput{
		Author: "agent-1", Body: "internal note", Internal: true, OverrideReason: "",
	})
	if err != nil {
		t.Fatalf("AddComment returned error: %v", err)
	}
	if c.GuardResult != nil {
		t.Errorf("GuardResult = %+v, want nil for an internal comment", c.GuardResult)
	}
	if len(guard.calls) != 0 {
		t.Errorf("guard called %d times, want 0 for an internal comment", len(guard.calls))
	}
}

func TestAddComment_AC4_FromCannedResponseSkipsGuardAndKeepsAuditShapeUnchanged(t *testing.T) {
	guard := &fakeReplyGuardClient{result: ReplyGuardOutcome{Verdict: "escalate"}}
	svc, audit := newTestServiceWithGuard(guard)
	ctx := context.Background()
	ticket, _ := svc.Create(ctx, "test", validInput)

	c, err := svc.AddComment(ctx, "agent-1", ticket.ID, CommentInput{
		Author: "agent-1", Body: "canned text", Internal: false, FromCannedResponse: true,
	})
	if err != nil {
		t.Fatalf("AddComment returned error: %v", err)
	}
	if c.GuardResult != nil {
		t.Errorf("GuardResult = %+v, want nil for a fromCannedResponse comment", c.GuardResult)
	}
	if len(guard.calls) != 0 {
		t.Errorf("guard called %d times, want 0 for a fromCannedResponse comment", len(guard.calls))
	}
	entries := audit.forTicket(ticket.ID)
	last := entries[len(entries)-1]
	if _, ok := last.Details["verdict"]; ok {
		t.Errorf("audit details = %v, want no verdict field for an unguarded comment", last.Details)
	}
	if len(last.Details) != 2 {
		t.Errorf("audit details = %v, want exactly commentId/internal (unchanged shape)", last.Details)
	}
}

func TestAddComment_AC5_AC6_NonCannedNonInternalTriggersGuardCallWithOnlyInternalNotesAndTicketFields(t *testing.T) {
	guard := &fakeReplyGuardClient{result: ReplyGuardOutcome{Verdict: "send"}}
	svc, _ := newTestServiceWithGuard(guard)
	ctx := context.Background()
	ticket, _ := svc.Create(ctx, "test", validInput)
	if _, err := svc.AddComment(ctx, "agent-1", ticket.ID, CommentInput{Author: "agent-1", Body: "secret internal context", Internal: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddComment(ctx, "agent-1", ticket.ID, CommentInput{Author: "agent-1", Body: "Reply to the customer", Internal: false}); err != nil {
		t.Fatal(err)
	}

	if len(guard.calls) != 1 {
		t.Fatalf("guard called %d times, want 1 (only for the non-internal, non-canned reply)", len(guard.calls))
	}
	got := guard.calls[0]
	if got.TicketSubject != ticket.Subject || got.TicketStatus != string(ticket.Status) || got.TicketPriority != string(ticket.Priority) {
		t.Errorf("guard input ticket fields = %+v, want ticket's own subject/status/priority", got)
	}
	if got.Body != "Reply to the customer" {
		t.Errorf("guard input Body = %q, want the candidate reply's own body", got.Body)
	}
	if len(got.InternalNotes) != 1 || got.InternalNotes[0].Body != "secret internal context" {
		t.Errorf("guard input InternalNotes = %+v, want exactly the one internal comment", got.InternalNotes)
	}
}

func TestAddComment_AC17_SendVerdictCreatesCommentWithGuardResult(t *testing.T) {
	guard := &fakeReplyGuardClient{result: ReplyGuardOutcome{Verdict: "send", Confidence: ConfidenceHigh, Reasoning: "clean"}}
	svc, _ := newTestServiceWithGuard(guard)
	ctx := context.Background()
	ticket, _ := svc.Create(ctx, "test", validInput)

	c, err := svc.AddComment(ctx, "agent-1", ticket.ID, CommentInput{Author: "agent-1", Body: "All set, thanks!", Internal: false})
	if err != nil {
		t.Fatalf("AddComment returned error: %v", err)
	}
	if c.GuardResult == nil || c.GuardResult.Verdict != "send" {
		t.Fatalf("GuardResult = %+v, want a send verdict", c.GuardResult)
	}
	final, _ := svc.FindByID(ctx, ticket.ID)
	if len(final.Comments) != 1 {
		t.Fatalf("len(comments) = %d, want 1", len(final.Comments))
	}
}

func TestAddComment_AC18_ReviseWithoutOverrideReasonReturns409RejectionAndCreatesNoComment(t *testing.T) {
	guard := &fakeReplyGuardClient{result: ReplyGuardOutcome{
		Verdict:  "revise",
		Findings: []ReplyGuardFinding{{Policy: "tone", Severity: "medium", Issue: "a bit curt", Quote: "no"}},
	}}
	svc, _ := newTestServiceWithGuard(guard)
	ctx := context.Background()
	ticket, _ := svc.Create(ctx, "test", validInput)

	_, err := svc.AddComment(ctx, "agent-1", ticket.ID, CommentInput{Author: "agent-1", Body: "no", Internal: false})
	var gre *GuardRejectedError
	if !errors.As(err, &gre) {
		t.Fatalf("expected *GuardRejectedError, got %v", err)
	}
	if gre.Outcome.Verdict != "revise" || len(gre.Outcome.Findings) != 1 {
		t.Errorf("Outcome = %+v, want the revise verdict and its findings", gre.Outcome)
	}
	final, _ := svc.FindByID(ctx, ticket.ID)
	if len(final.Comments) != 0 {
		t.Fatalf("len(comments) = %d, want 0 — a rejected revise must not create a Comment", len(final.Comments))
	}
}

func TestAddComment_AC19_ReviseWithOverrideReasonCreatesCommentAndAuditsOverride(t *testing.T) {
	guard := &fakeReplyGuardClient{result: ReplyGuardOutcome{Verdict: "revise", Confidence: ConfidenceMedium}}
	svc, audit := newTestServiceWithGuard(guard)
	ctx := context.Background()
	ticket, _ := svc.Create(ctx, "test", validInput)

	c, err := svc.AddComment(ctx, "agent-1", ticket.ID, CommentInput{
		Author: "agent-1", Body: "sending anyway", Internal: false, OverrideReason: "lead approved by phone",
	})
	if err != nil {
		t.Fatalf("AddComment returned error: %v", err)
	}
	if c.GuardResult == nil || c.GuardResult.Verdict != "revise" {
		t.Fatalf("GuardResult = %+v, want the revise verdict preserved on the created comment", c.GuardResult)
	}
	final, _ := svc.FindByID(ctx, ticket.ID)
	if len(final.Comments) != 1 {
		t.Fatalf("len(comments) = %d, want 1 — an overridden revise must create the Comment", len(final.Comments))
	}
	entries := audit.forTicket(ticket.ID)
	last := entries[len(entries)-1]
	if last.Details["overrideReason"] != "lead approved by phone" {
		t.Errorf("audit details = %v, want overrideReason recorded", last.Details)
	}
	if last.Details["verdict"] != "revise" {
		t.Errorf("audit details = %v, want verdict=revise recorded", last.Details)
	}
}

func TestAddComment_AC20_EscalateAlwaysRejectsEvenWithOverrideReason(t *testing.T) {
	guard := &fakeReplyGuardClient{result: ReplyGuardOutcome{
		Verdict:  "escalate",
		Findings: []ReplyGuardFinding{{Policy: "disclosure", Severity: "high", Issue: "leaked internal note", Quote: "we found a bug"}},
	}}
	svc, _ := newTestServiceWithGuard(guard)
	ctx := context.Background()
	ticket, _ := svc.Create(ctx, "test", validInput)

	_, err := svc.AddComment(ctx, "agent-1", ticket.ID, CommentInput{
		Author: "agent-1", Body: "we found a bug", Internal: false, OverrideReason: "please let me send it",
	})
	var gre *GuardRejectedError
	if !errors.As(err, &gre) {
		t.Fatalf("expected *GuardRejectedError even with an overrideReason, got %v", err)
	}
	if gre.Outcome.Verdict != "escalate" {
		t.Errorf("Outcome.Verdict = %q, want escalate", gre.Outcome.Verdict)
	}
	final, _ := svc.FindByID(ctx, ticket.ID)
	if len(final.Comments) != 0 {
		t.Fatalf("len(comments) = %d, want 0 — escalate is never overridable", len(final.Comments))
	}
}

func TestAddComment_AC21_AuditDetailsIncludeGuardFieldsOnlyWhenGuarded(t *testing.T) {
	guard := &fakeReplyGuardClient{result: ReplyGuardOutcome{
		Verdict:            "send",
		Confidence:         ConfidenceHigh,
		InjectionSuspected: false,
		Findings:           []ReplyGuardFinding{{Policy: "tone", Severity: "low", Issue: "minor", Quote: "ok"}},
	}}
	svc, audit := newTestServiceWithGuard(guard)
	ctx := context.Background()
	ticket, _ := svc.Create(ctx, "test", validInput)

	if _, err := svc.AddComment(ctx, "agent-1", ticket.ID, CommentInput{Author: "agent-1", Body: "ok", Internal: false}); err != nil {
		t.Fatal(err)
	}
	entries := audit.forTicket(ticket.ID)
	last := entries[len(entries)-1]
	if last.Details["verdict"] != "send" {
		t.Errorf("Details[verdict] = %v, want send", last.Details["verdict"])
	}
	if last.Details["findingCount"] != 1 {
		t.Errorf("Details[findingCount] = %v, want 1", last.Details["findingCount"])
	}
	if last.Details["confidence"] != "high" {
		t.Errorf("Details[confidence] = %v, want high", last.Details["confidence"])
	}
	if last.Details["injectionSuspected"] != false {
		t.Errorf("Details[injectionSuspected] = %v, want false", last.Details["injectionSuspected"])
	}
	if _, ok := last.Details["overrideReason"]; ok {
		t.Errorf("Details = %v, want no overrideReason when none was used", last.Details)
	}
}

func TestAddComment_AC22_GuardCallFailureCreatesNoCommentAndFailsClosed(t *testing.T) {
	guard := &fakeReplyGuardClient{err: errors.New("anthropic unavailable")}
	svc, _ := newTestServiceWithGuard(guard)
	ctx := context.Background()
	ticket, _ := svc.Create(ctx, "test", validInput)

	_, err := svc.AddComment(ctx, "agent-1", ticket.ID, CommentInput{Author: "agent-1", Body: "hello", Internal: false})
	var gue *GuardUnavailableError
	if !errors.As(err, &gue) {
		t.Fatalf("expected *GuardUnavailableError, got %v", err)
	}
	final, _ := svc.FindByID(ctx, ticket.ID)
	if len(final.Comments) != 0 {
		t.Fatalf("len(comments) = %d, want 0 — a guard failure must fail closed", len(final.Comments))
	}
}

func TestAddComment_UnrecognizedVerdictFailsClosed(t *testing.T) {
	guard := &fakeReplyGuardClient{result: ReplyGuardOutcome{Verdict: "maybe"}}
	svc, _ := newTestServiceWithGuard(guard)
	ctx := context.Background()
	ticket, _ := svc.Create(ctx, "test", validInput)

	_, err := svc.AddComment(ctx, "agent-1", ticket.ID, CommentInput{Author: "agent-1", Body: "hello", Internal: false})
	var gue *GuardUnavailableError
	if !errors.As(err, &gue) {
		t.Fatalf("expected an unrecognized verdict to fail closed as *GuardUnavailableError, got %v", err)
	}
	final, _ := svc.FindByID(ctx, ticket.ID)
	if len(final.Comments) != 0 {
		t.Fatalf("len(comments) = %d, want 0", len(final.Comments))
	}
}

func TestAddComment_NilGuardSkipsGateEntirely(t *testing.T) {
	svc, _ := newTestService() // no WithReplyGuard call — guard is nil
	ctx := context.Background()
	ticket, _ := svc.Create(ctx, "test", validInput)

	c, err := svc.AddComment(ctx, "agent-1", ticket.ID, CommentInput{Author: "agent-1", Body: "hello", Internal: false})
	if err != nil {
		t.Fatalf("AddComment returned error with no guard configured: %v", err)
	}
	if c.GuardResult != nil {
		t.Errorf("GuardResult = %+v, want nil when no guard is configured", c.GuardResult)
	}
}
