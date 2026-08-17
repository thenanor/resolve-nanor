package drafts

import (
	"context"
	"errors"
	"testing"
	"time"

	"resolve/internal/tickets"
)

// fakeRepository is an in-memory Repository, mirroring
// tickets/service_test.go's fakeRepository.
type fakeRepository struct {
	byID map[string]*Draft
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{byID: map[string]*Draft{}}
}

func (f *fakeRepository) CreateDraft(_ context.Context, d *Draft) error {
	cp := *d
	f.byID[d.ID] = &cp
	return nil
}

func (f *fakeRepository) FindByID(_ context.Context, draftID string) (*Draft, error) {
	d, ok := f.byID[draftID]
	if !ok {
		return nil, nil
	}
	cp := *d
	if d.GuardResult != nil {
		gr := *d.GuardResult
		cp.GuardResult = &gr
	}
	return &cp, nil
}

func (f *fakeRepository) UpdateBody(_ context.Context, draftID, body, updatedAt string) error {
	d, ok := f.byID[draftID]
	if !ok {
		return errors.New("not found")
	}
	d.Body = body
	d.Status = StatusPendingReview
	d.GuardResult = nil
	d.UpdatedAt = updatedAt
	return nil
}

func (f *fakeRepository) SetGuardResult(_ context.Context, draftID string, result GuardResult, updatedAt string) error {
	d, ok := f.byID[draftID]
	if !ok {
		return errors.New("not found")
	}
	r := result
	d.GuardResult = &r
	d.Status = StatusGuarded
	d.UpdatedAt = updatedAt
	return nil
}

func (f *fakeRepository) SetGuardFailed(_ context.Context, draftID, updatedAt string) error {
	d, ok := f.byID[draftID]
	if !ok {
		return errors.New("not found")
	}
	d.Status = StatusGuardFailed
	d.UpdatedAt = updatedAt
	return nil
}

func (f *fakeRepository) MarkSent(_ context.Context, draftID, updatedAt string) error {
	d, ok := f.byID[draftID]
	if !ok {
		return errors.New("not found")
	}
	d.Status = StatusSent
	d.UpdatedAt = updatedAt
	return nil
}

// fakeTicketReader is a TicketReader returning a fixed context/notes for
// known ticket ids, and a *NotFoundError for anything else.
type fakeTicketReader struct {
	byTicketID map[string]struct {
		ctx   TicketContext
		notes []InternalNote
	}
}

func newFakeTicketReader() *fakeTicketReader {
	return &fakeTicketReader{byTicketID: map[string]struct {
		ctx   TicketContext
		notes []InternalNote
	}{}}
}

func (f *fakeTicketReader) seed(ticketID string, ctx TicketContext, notes []InternalNote) {
	f.byTicketID[ticketID] = struct {
		ctx   TicketContext
		notes []InternalNote
	}{ctx, notes}
}

func (f *fakeTicketReader) ReadContext(_ context.Context, ticketID string) (TicketContext, []InternalNote, error) {
	v, ok := f.byTicketID[ticketID]
	if !ok {
		return TicketContext{}, nil, &NotFoundError{ID: ticketID}
	}
	return v.ctx, v.notes, nil
}

// fakeTicketCommenter is a TicketCommenter that records every call.
type fakeTicketCommenter struct {
	calls []replyCall
	err   error
}

type replyCall struct {
	actor, ticketID, author, body string
}

func (f *fakeTicketCommenter) AddReply(_ context.Context, actor, ticketID, author, body string) error {
	f.calls = append(f.calls, replyCall{actor, ticketID, author, body})
	return f.err
}

// fakeAudit is a capturing AuditRecorder, mirroring
// tickets/service_test.go's fakeAudit.
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

func (f *fakeAudit) forTicket(ticketID, action string) []auditRecord {
	var out []auditRecord
	for _, e := range f.entries {
		if e.TicketID == ticketID && e.Action == action {
			out = append(out, e)
		}
	}
	return out
}

// channelReplyGuardNotifier is a ReplyGuardNotifier used to assert, without
// time.Sleep-based flakiness, that CreateDraft/UpdateBody dispatch their
// guard notification in their own goroutine — mirrors
// tickets/service_test.go's channelTriageNotifier.
type channelReplyGuardNotifier struct {
	calls chan GuardRequest
	block chan struct{}
	err   error
}

func newChannelReplyGuardNotifier() *channelReplyGuardNotifier {
	return &channelReplyGuardNotifier{calls: make(chan GuardRequest, 1)}
}

func (f *channelReplyGuardNotifier) NotifyDraftCreated(_ context.Context, req GuardRequest) error {
	if f.block != nil {
		<-f.block
	}
	f.calls <- req
	return f.err
}

func newTestService() (*Service, *fakeRepository, *fakeTicketReader, *fakeTicketCommenter, *fakeAudit, *channelReplyGuardNotifier) {
	repo := newFakeRepository()
	reader := newFakeTicketReader()
	commenter := &fakeTicketCommenter{}
	audit := &fakeAudit{}
	guard := newChannelReplyGuardNotifier()
	svc := NewService(repo, reader, commenter, audit, guard)
	return svc, repo, reader, commenter, audit, guard
}

func awaitGuardCall(t *testing.T, calls chan GuardRequest) GuardRequest {
	t.Helper()
	select {
	case req := <-calls:
		return req
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for guard notification")
		return GuardRequest{}
	}
}

// --- CreateDraft: AC-1 through AC-6 ---

func TestCreateDraft_AC1_ValidInputReturnsPendingReviewDraft(t *testing.T) {
	svc, _, reader, _, _, guard := newTestService()
	reader.seed("tkt_1", TicketContext{Subject: "s"}, nil)

	d, err := svc.CreateDraft(context.Background(), "agent", "tkt_1", "Alice", "  Hello there  ")
	if err != nil {
		t.Fatalf("CreateDraft returned error: %v", err)
	}
	if d.Status != StatusPendingReview {
		t.Errorf("Status = %q, want %q", d.Status, StatusPendingReview)
	}
	if d.GuardResult != nil {
		t.Errorf("GuardResult = %+v, want nil", d.GuardResult)
	}
	if d.Author != "Alice" || d.Body != "Hello there" {
		t.Errorf("Author/Body = %q/%q, want trimmed Alice/Hello there", d.Author, d.Body)
	}
	awaitGuardCall(t, guard.calls)
}

func TestCreateDraft_AC2_EmptyAuthorReturnsValidationError(t *testing.T) {
	svc, _, reader, _, _, _ := newTestService()
	reader.seed("tkt_1", TicketContext{}, nil)

	_, err := svc.CreateDraft(context.Background(), "agent", "tkt_1", "  ", "body")
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *ValidationError, got %v", err)
	}
}

func TestCreateDraft_AC2_EmptyBodyReturnsValidationError(t *testing.T) {
	svc, _, reader, _, _, _ := newTestService()
	reader.seed("tkt_1", TicketContext{}, nil)

	_, err := svc.CreateDraft(context.Background(), "agent", "tkt_1", "Alice", "   ")
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *ValidationError, got %v", err)
	}
}

func TestCreateDraft_AC3_UnknownTicketReturnsNotFoundError(t *testing.T) {
	svc, _, _, _, _, _ := newTestService()

	_, err := svc.CreateDraft(context.Background(), "agent", "tkt_missing", "Alice", "body")
	var nf *NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("expected *NotFoundError, got %v", err)
	}
}

func TestCreateDraft_AC4_DoesNotBlockOnGuardNotification(t *testing.T) {
	svc, _, reader, _, _, guard := newTestService()
	guard.block = make(chan struct{})
	reader.seed("tkt_1", TicketContext{}, nil)

	done := make(chan struct{})
	go func() {
		if _, err := svc.CreateDraft(context.Background(), "agent", "tkt_1", "Alice", "body"); err != nil {
			t.Errorf("CreateDraft returned error: %v", err)
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("CreateDraft blocked on the guard notification")
	}
	close(guard.block)
	awaitGuardCall(t, guard.calls)
}

func TestCreateDraft_AC5_FindByIDReflectsPendingState(t *testing.T) {
	svc, _, reader, _, _, guard := newTestService()
	guard.block = make(chan struct{})
	reader.seed("tkt_1", TicketContext{}, nil)

	created, err := svc.CreateDraft(context.Background(), "agent", "tkt_1", "Alice", "body")
	if err != nil {
		t.Fatalf("CreateDraft returned error: %v", err)
	}

	found, err := svc.FindByID(context.Background(), "tkt_1", created.ID)
	if err != nil {
		t.Fatalf("FindByID returned error: %v", err)
	}
	if found.Status != StatusPendingReview || found.GuardResult != nil {
		t.Errorf("found = %+v, want pending_review with nil GuardResult", found)
	}
	close(guard.block)
}

func TestCreateDraft_AC6_GuardRequestCarriesExactlyTicketContextNotesAndDraftBody(t *testing.T) {
	svc, _, reader, _, _, guard := newTestService()
	ticketCtx := TicketContext{Subject: "Can't log in", Description: "desc", Status: "open", Priority: "high"}
	notes := []InternalNote{{Author: "bob", Body: "internal note", At: "t1"}}
	reader.seed("tkt_1", ticketCtx, notes)

	created, err := svc.CreateDraft(context.Background(), "agent", "tkt_1", "Alice", "the reply")
	if err != nil {
		t.Fatalf("CreateDraft returned error: %v", err)
	}

	req := awaitGuardCall(t, guard.calls)
	if req.TicketID != "tkt_1" || req.DraftID != created.ID {
		t.Errorf("req ids = %q/%q, want tkt_1/%q", req.TicketID, req.DraftID, created.ID)
	}
	if req.Ticket != ticketCtx {
		t.Errorf("req.Ticket = %+v, want %+v", req.Ticket, ticketCtx)
	}
	if len(req.InternalNotes) != 1 || req.InternalNotes[0] != notes[0] {
		t.Errorf("req.InternalNotes = %+v, want %+v", req.InternalNotes, notes)
	}
	if req.DraftBody != "the reply" {
		t.Errorf("req.DraftBody = %q, want %q", req.DraftBody, "the reply")
	}
}

// --- AC-17: guard notification failure ---

func TestCreateDraft_AC17_GuardNotifyFailureMarksDraftGuardFailed(t *testing.T) {
	svc, repo, reader, _, _, guard := newTestService()
	guard.err = errors.New("reply-guard unreachable")
	reader.seed("tkt_1", TicketContext{}, nil)

	created, err := svc.CreateDraft(context.Background(), "agent", "tkt_1", "Alice", "body")
	if err != nil {
		t.Fatalf("CreateDraft returned error: %v", err)
	}
	awaitGuardCall(t, guard.calls)

	// The failure is recorded from the same background goroutine that
	// called the (failing) notifier; give it a moment to finish.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		d, _ := repo.FindByID(context.Background(), created.ID)
		if d.Status == StatusGuardFailed {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("draft never transitioned to guard_failed after a failed guard notification")
}

// --- AC-18/AC-19: RecordGuardResult ---

func TestRecordGuardResult_AC18_StoresResultAndMovesToGuarded(t *testing.T) {
	svc, _, reader, _, audit, guard := newTestService()
	guard.block = make(chan struct{})
	reader.seed("tkt_1", TicketContext{}, nil)
	created, _ := svc.CreateDraft(context.Background(), "agent", "tkt_1", "Alice", "body")
	close(guard.block)
	awaitGuardCall(t, guard.calls)

	result := GuardResult{
		Verdict:    VerdictSend,
		Confidence: tickets.ConfidenceHigh,
		Reasoning:  "clean",
	}
	updated, err := svc.RecordGuardResult(context.Background(), "reply-guard-service", "tkt_1", created.ID, result)
	if err != nil {
		t.Fatalf("RecordGuardResult returned error: %v", err)
	}
	if updated.Status != StatusGuarded {
		t.Errorf("Status = %q, want %q", updated.Status, StatusGuarded)
	}
	if updated.GuardResult == nil || updated.GuardResult.Verdict != VerdictSend {
		t.Errorf("GuardResult = %+v, want Verdict send", updated.GuardResult)
	}

	entries := audit.forTicket("tkt_1", "draft.guarded")
	if len(entries) != 1 {
		t.Fatalf("AC-19: expected one draft.guarded audit entry, got %d", len(entries))
	}
	if entries[0].Details["verdict"] != "send" {
		t.Errorf("AC-19: audit details verdict = %v, want send", entries[0].Details["verdict"])
	}
}

// --- AC-20 through AC-26: Send/UpdateBody ---

func guardedDraft(t *testing.T, svc *Service, reader *fakeTicketReader, guard *channelReplyGuardNotifier, verdict Verdict) *Draft {
	t.Helper()
	guard.block = make(chan struct{})
	reader.seed("tkt_1", TicketContext{}, nil)
	created, err := svc.CreateDraft(context.Background(), "agent", "tkt_1", "Alice", "body")
	if err != nil {
		t.Fatalf("CreateDraft returned error: %v", err)
	}
	close(guard.block)
	awaitGuardCall(t, guard.calls)

	result := GuardResult{Verdict: verdict, Confidence: tickets.ConfidenceHigh}
	if verdict != VerdictSend {
		result.Findings = []Finding{{Policy: PolicyTone, Severity: SeverityMedium, Issue: "issue", Quote: "body"}}
	}
	updated, err := svc.RecordGuardResult(context.Background(), "reply-guard-service", "tkt_1", created.ID, result)
	if err != nil {
		t.Fatalf("RecordGuardResult returned error: %v", err)
	}
	return updated
}

func TestSend_AC20_SendVerdictSendsUnconditionally(t *testing.T) {
	svc, _, reader, commenter, audit, guard := newTestService()
	d := guardedDraft(t, svc, reader, guard, VerdictSend)

	sent, err := svc.Send(context.Background(), "agent", "tkt_1", d.ID, "")
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if sent.Status != StatusSent {
		t.Errorf("Status = %q, want %q", sent.Status, StatusSent)
	}
	if len(commenter.calls) != 1 {
		t.Fatalf("expected AddReply called once, got %d", len(commenter.calls))
	}
	if commenter.calls[0] != (replyCall{"agent", "tkt_1", "Alice", "body"}) {
		t.Errorf("AddReply call = %+v", commenter.calls[0])
	}
	if len(audit.forTicket("tkt_1", "draft.sent")) != 1 {
		t.Error("expected one draft.sent audit entry")
	}
}

func TestSend_AC21_ReviseVerdictWithoutOverrideReturnsConflict(t *testing.T) {
	svc, _, reader, commenter, _, guard := newTestService()
	d := guardedDraft(t, svc, reader, guard, VerdictRevise)

	_, err := svc.Send(context.Background(), "agent", "tkt_1", d.ID, "")
	var ce *ConflictError
	if !errors.As(err, &ce) {
		t.Fatalf("expected *ConflictError, got %v", err)
	}
	if len(commenter.calls) != 0 {
		t.Error("expected no Comment to be created")
	}
}

func TestSend_AC22_PendingReviewStatusReturnsConflict(t *testing.T) {
	svc, _, reader, commenter, _, guard := newTestService()
	guard.block = make(chan struct{})
	reader.seed("tkt_1", TicketContext{}, nil)
	created, _ := svc.CreateDraft(context.Background(), "agent", "tkt_1", "Alice", "body")

	_, err := svc.Send(context.Background(), "agent", "tkt_1", created.ID, "any reason")
	var ce *ConflictError
	if !errors.As(err, &ce) {
		t.Fatalf("expected *ConflictError, got %v", err)
	}
	if len(commenter.calls) != 0 {
		t.Error("expected no Comment to be created")
	}
	close(guard.block)
	awaitGuardCall(t, guard.calls)
}

func TestSend_AC22_GuardFailedStatusReturnsConflictEvenWithOverride(t *testing.T) {
	svc, _, reader, commenter, _, guard := newTestService()
	guard.err = errors.New("unreachable")
	reader.seed("tkt_1", TicketContext{}, nil)
	created, _ := svc.CreateDraft(context.Background(), "agent", "tkt_1", "Alice", "body")
	awaitGuardCall(t, guard.calls)

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		d, _ := svc.FindByID(context.Background(), "tkt_1", created.ID)
		if d.Status == StatusGuardFailed {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	_, err := svc.Send(context.Background(), "agent", "tkt_1", created.ID, "override anyway")
	var ce *ConflictError
	if !errors.As(err, &ce) {
		t.Fatalf("expected *ConflictError, got %v", err)
	}
	if len(commenter.calls) != 0 {
		t.Error("expected no Comment to be created")
	}
}

func TestSend_AC23_ReviseVerdictWithOverrideReasonSendsAndAudits(t *testing.T) {
	svc, _, reader, commenter, audit, guard := newTestService()
	d := guardedDraft(t, svc, reader, guard, VerdictRevise)

	sent, err := svc.Send(context.Background(), "agent", "tkt_1", d.ID, "manager approved this wording")
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if sent.Status != StatusSent {
		t.Errorf("Status = %q, want %q", sent.Status, StatusSent)
	}
	if len(commenter.calls) != 1 {
		t.Fatalf("expected AddReply called once, got %d", len(commenter.calls))
	}

	overrides := audit.forTicket("tkt_1", "draft.send_overridden")
	if len(overrides) != 1 {
		t.Fatalf("expected one draft.send_overridden audit entry, got %d", len(overrides))
	}
	if overrides[0].Details["overrideReason"] != "manager approved this wording" {
		t.Errorf("overrideReason = %v, want the given reason", overrides[0].Details["overrideReason"])
	}
}

func TestSend_AC23a_EscalateVerdictIsAHardBlockEvenWithOverride(t *testing.T) {
	svc, _, reader, commenter, _, guard := newTestService()
	d := guardedDraft(t, svc, reader, guard, VerdictEscalate)

	_, err := svc.Send(context.Background(), "agent", "tkt_1", d.ID, "please let me send this")
	var ce *ConflictError
	if !errors.As(err, &ce) {
		t.Fatalf("expected *ConflictError even with an override reason, got %v", err)
	}
	if len(commenter.calls) != 0 {
		t.Error("expected no Comment to be created")
	}
}

func TestSend_AC24_SendingAnAlreadySentDraftIsIdempotentlyRejected(t *testing.T) {
	svc, _, reader, commenter, _, guard := newTestService()
	d := guardedDraft(t, svc, reader, guard, VerdictSend)

	if _, err := svc.Send(context.Background(), "agent", "tkt_1", d.ID, ""); err != nil {
		t.Fatalf("first Send returned error: %v", err)
	}
	if len(commenter.calls) != 1 {
		t.Fatalf("expected one Comment after first send, got %d", len(commenter.calls))
	}

	_, err := svc.Send(context.Background(), "agent", "tkt_1", d.ID, "")
	var ce *ConflictError
	if !errors.As(err, &ce) {
		t.Fatalf("expected *ConflictError on replay, got %v", err)
	}
	if len(commenter.calls) != 1 {
		t.Errorf("expected still only one Comment after replayed send, got %d", len(commenter.calls))
	}
}

func TestSend_AC25_UnknownDraftIDReturnsNotFound(t *testing.T) {
	svc, _, reader, _, _, _ := newTestService()
	reader.seed("tkt_1", TicketContext{}, nil)

	_, err := svc.Send(context.Background(), "agent", "tkt_1", "dft_missing", "")
	var nf *NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("expected *NotFoundError, got %v", err)
	}
}

func TestSend_AC25_DraftBelongingToAnotherTicketReturnsNotFound(t *testing.T) {
	svc, _, reader, _, _, guard := newTestService()
	d := guardedDraft(t, svc, reader, guard, VerdictSend)

	_, err := svc.Send(context.Background(), "agent", "tkt_other", d.ID, "")
	var nf *NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("expected *NotFoundError, got %v", err)
	}
}

func TestUpdateBody_AC26_EditableAfterReviseResetsToPendingReviewAndReguards(t *testing.T) {
	svc, _, reader, _, _, guard := newTestService()
	d := guardedDraft(t, svc, reader, guard, VerdictRevise)

	guard.block = make(chan struct{})
	updated, err := svc.UpdateBody(context.Background(), "agent", "tkt_1", d.ID, "a revised reply")
	if err != nil {
		t.Fatalf("UpdateBody returned error: %v", err)
	}
	if updated.Status != StatusPendingReview || updated.GuardResult != nil {
		t.Errorf("updated = %+v, want pending_review with nil GuardResult", updated)
	}
	if updated.Body != "a revised reply" {
		t.Errorf("Body = %q, want %q", updated.Body, "a revised reply")
	}
	close(guard.block)
	req := awaitGuardCall(t, guard.calls)
	if req.DraftBody != "a revised reply" {
		t.Errorf("re-dispatched guard request body = %q", req.DraftBody)
	}
}

func TestUpdateBody_AC26_EditableAfterEscalate(t *testing.T) {
	svc, _, reader, _, _, guard := newTestService()
	d := guardedDraft(t, svc, reader, guard, VerdictEscalate)

	guard.block = make(chan struct{})
	updated, err := svc.UpdateBody(context.Background(), "agent", "tkt_1", d.ID, "a safer reply")
	if err != nil {
		t.Fatalf("UpdateBody returned error: %v", err)
	}
	if updated.Status != StatusPendingReview {
		t.Errorf("Status = %q, want %q", updated.Status, StatusPendingReview)
	}
	close(guard.block)
	awaitGuardCall(t, guard.calls)
}

func TestUpdateBody_AC26_SentDraftCannotBeEdited(t *testing.T) {
	svc, _, reader, _, _, guard := newTestService()
	d := guardedDraft(t, svc, reader, guard, VerdictSend)
	if _, err := svc.Send(context.Background(), "agent", "tkt_1", d.ID, ""); err != nil {
		t.Fatalf("Send returned error: %v", err)
	}

	_, err := svc.UpdateBody(context.Background(), "agent", "tkt_1", d.ID, "too late")
	var ce *ConflictError
	if !errors.As(err, &ce) {
		t.Fatalf("expected *ConflictError, got %v", err)
	}
}

func TestUpdateBody_AC26_SendVerdictDraftCannotBeEditedBeforeSending(t *testing.T) {
	svc, _, reader, _, _, guard := newTestService()
	d := guardedDraft(t, svc, reader, guard, VerdictSend)

	_, err := svc.UpdateBody(context.Background(), "agent", "tkt_1", d.ID, "why edit a clean draft")
	var ce *ConflictError
	if !errors.As(err, &ce) {
		t.Fatalf("expected *ConflictError, got %v", err)
	}
}
