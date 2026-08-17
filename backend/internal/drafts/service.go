package drafts

import (
	"context"
	"log"
	"strings"
	"time"

	"resolve/internal/ids"
)

// TicketContext is the slice of a ticket's fields drafts needs to build a
// reply-guard request. Deliberately not tickets.Ticket itself, to avoid
// coupling drafts to that type's full shape.
type TicketContext struct {
	Subject     string
	Description string
	Status      string
	Priority    string
}

// InternalNote is one internal (agent-only) comment on a ticket, as read by
// drafts for reply-guard context.
type InternalNote struct {
	Author string
	Body   string
	At     string
}

// TicketReader is the slice of tickets.Service that drafts depends on for
// context-gathering. Declared here (consumer side), like
// tickets.AuditRecorder, to avoid an import cycle and to keep drafts'
// dependency on tickets narrow and testable.
type TicketReader interface {
	// ReadContext returns ticketID's context and its internal (Internal ==
	// true) comments. Implementations must return a *NotFoundError (this
	// package's, not tickets') when the ticket doesn't exist, so
	// respondIfError maps it to 404 without drafts needing to know about
	// tickets.NotFoundError.
	ReadContext(ctx context.Context, ticketID string) (TicketContext, []InternalNote, error)
}

// TicketCommenter is the slice of tickets.Service that drafts depends on to
// actually send a draft: creating the real, customer-visible Comment.
type TicketCommenter interface {
	AddReply(ctx context.Context, actor, ticketID, author, body string) error
}

// AuditRecorder mirrors tickets.AuditRecorder's Record method — drafts
// records its own audit trail entries against the ticket's existing audit
// log rather than depending on tickets.AuditRecorder directly.
type AuditRecorder interface {
	Record(ctx context.Context, actor, action, ticketID string, details map[string]any) error
}

// GuardRequest is everything reply-guard needs to assess one draft.
type GuardRequest struct {
	TicketID      string
	DraftID       string
	Ticket        TicketContext
	InternalNotes []InternalNote
	DraftBody     string
}

// ReplyGuardNotifier is the slice of the reply-guard service's HTTP API that
// drafts depends on. Declared here (consumer side), mirroring
// tickets.TriageNotifier.
type ReplyGuardNotifier interface {
	NotifyDraftCreated(ctx context.Context, req GuardRequest) error
}

// guardNotifyTimeout bounds how long a fire-and-forget guard notification
// may run. Generous rather than tuned, mirroring
// tickets.triageNotifyTimeout's rationale — it never blocks a caller.
const guardNotifyTimeout = 20 * time.Second

type Service struct {
	repo      Repository
	tickets   TicketReader
	commenter TicketCommenter
	audit     AuditRecorder
	guard     ReplyGuardNotifier
}

func NewService(repo Repository, tickets TicketReader, commenter TicketCommenter, audit AuditRecorder, guard ReplyGuardNotifier) *Service {
	return &Service{repo: repo, tickets: tickets, commenter: commenter, audit: audit, guard: guard}
}

func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }

// CreateDraft stages a reply for review. It dispatches a fire-and-forget
// call to reply-guard and returns immediately — draft creation latency is
// unaffected by LLM round-trip time (AC-4).
func (s *Service) CreateDraft(ctx context.Context, actor, ticketID, author, body string) (*Draft, error) {
	if strings.TrimSpace(author) == "" {
		return nil, invalid("author must be a non-empty string")
	}
	if strings.TrimSpace(body) == "" {
		return nil, invalid("body must be a non-empty string")
	}

	ticketCtx, notes, err := s.tickets.ReadContext(ctx, ticketID)
	if err != nil {
		return nil, err
	}

	ts := now()
	d := &Draft{
		ID:        ids.New("dft"),
		TicketID:  ticketID,
		Author:    strings.TrimSpace(author),
		Body:      strings.TrimSpace(body),
		Status:    StatusPendingReview,
		CreatedAt: ts,
		UpdatedAt: ts,
	}
	if err := s.repo.CreateDraft(ctx, d); err != nil {
		return nil, err
	}

	s.dispatchGuard(GuardRequest{
		TicketID:      d.TicketID,
		DraftID:       d.ID,
		Ticket:        ticketCtx,
		InternalNotes: notes,
		DraftBody:     d.Body,
	})

	return d, nil
}

// dispatchGuard fires the guard notification in its own goroutine, with its
// own context and timeout rather than the caller's — chi cancels the
// request context the moment the handler returns, which happens right after
// CreateDraft/UpdateBody does, so a goroutine inheriting it would fail with
// context.Canceled on effectively every call (see
// tickets.Service.Create for the identical rationale). A notify failure —
// network error, or reply-guard's own guard call failing — moves the draft
// to StatusGuardFailed rather than leaving it silently stuck in
// pending_review (AC-17).
func (s *Service) dispatchGuard(req GuardRequest) {
	go func(req GuardRequest) {
		ctx, cancel := context.WithTimeout(context.Background(), guardNotifyTimeout)
		defer cancel()
		if err := s.guard.NotifyDraftCreated(ctx, req); err != nil {
			log.Printf("reply-guard notify failed for draft %s (ticket %s): %v", req.DraftID, req.TicketID, err)
			if _, ferr := s.RecordGuardFailure(ctx, req.TicketID, req.DraftID); ferr != nil {
				log.Printf("failed to record guard failure for draft %s: %v", req.DraftID, ferr)
			}
		}
	}(req)
}

// FindByID returns ticketID's draft draftID. A draft that doesn't exist, or
// that belongs to a different ticket, is reported identically as not found
// (AC-25) — a mismatched ticket id gives no information about drafts on
// other tickets.
func (s *Service) FindByID(ctx context.Context, ticketID, draftID string) (*Draft, error) {
	d, err := s.repo.FindByID(ctx, draftID)
	if err != nil {
		return nil, err
	}
	if d == nil || d.TicketID != ticketID {
		return nil, &NotFoundError{ID: draftID}
	}
	return d, nil
}

// RecordGuardResult stores a completed guard assessment (AC-18) and audits
// it (AC-19). Called by the internal guard-result write-back endpoint,
// which reply-guard's HTTPDraftUpdater posts to.
func (s *Service) RecordGuardResult(ctx context.Context, actor, ticketID, draftID string, result GuardResult) (*Draft, error) {
	d, err := s.FindByID(ctx, ticketID, draftID)
	if err != nil {
		return nil, err
	}

	d.UpdatedAt = now()
	if err := s.repo.SetGuardResult(ctx, d.ID, result, d.UpdatedAt); err != nil {
		return nil, err
	}
	d.Status = StatusGuarded
	d.GuardResult = &result

	if err := s.audit.Record(ctx, actor, "draft.guarded", d.TicketID, map[string]any{
		"draftId":            d.ID,
		"verdict":            string(result.Verdict),
		"findingCount":       len(result.Findings),
		"confidence":         string(result.Confidence),
		"injectionSuspected": result.InjectionSuspected,
	}); err != nil {
		return nil, err
	}
	return d, nil
}

// RecordGuardFailure moves a draft to StatusGuardFailed after the guard
// call itself could not be completed (AC-17). It looks the draft up
// directly by id (bypassing FindByID's ticket-scoping check) since it is
// only ever called from the trusted fire-and-forget dispatch path in this
// same process, which already knows the correct ticketID/draftID pairing.
func (s *Service) RecordGuardFailure(ctx context.Context, ticketID, draftID string) (*Draft, error) {
	d, err := s.repo.FindByID(ctx, draftID)
	if err != nil {
		return nil, err
	}
	if d == nil {
		return nil, &NotFoundError{ID: draftID}
	}

	d.UpdatedAt = now()
	if err := s.repo.SetGuardFailed(ctx, d.ID, d.UpdatedAt); err != nil {
		return nil, err
	}
	d.Status = StatusGuardFailed
	d.GuardResult = nil
	return d, nil
}

// Send authorizes and performs sending a draft as a real, customer-visible
// comment. See the reply-guard spec's Invariants: a draft never reaches
// StatusSent without either a "send" verdict or a logged override on a
// "revise" verdict, and an "escalate" verdict can never be sent, override
// or not (AC-23a) — the only way out of escalate is editing the draft
// (UpdateBody), which clears the guard result and re-triggers review.
func (s *Service) Send(ctx context.Context, actor, ticketID, draftID, overrideReason string) (*Draft, error) {
	d, err := s.FindByID(ctx, ticketID, draftID)
	if err != nil {
		return nil, err
	}

	if d.Status != StatusGuarded {
		return nil, conflict("draft %s cannot be sent from status %q; it must be guarded first", d.ID, d.Status)
	}

	overridden := false
	switch d.GuardResult.Verdict {
	case VerdictSend:
		// no gate.
	case VerdictEscalate:
		return nil, conflict("draft %s has an escalate verdict and cannot be sent, even with an override; edit and resubmit it for another review", d.ID)
	case VerdictRevise:
		if strings.TrimSpace(overrideReason) == "" {
			return nil, conflict("draft %s has a revise verdict; sending requires overrideReason", d.ID)
		}
		overridden = true
	}

	if err := s.commenter.AddReply(ctx, actor, d.TicketID, d.Author, d.Body); err != nil {
		return nil, err
	}

	d.UpdatedAt = now()
	if err := s.repo.MarkSent(ctx, d.ID, d.UpdatedAt); err != nil {
		return nil, err
	}
	d.Status = StatusSent

	if err := s.audit.Record(ctx, actor, "draft.sent", d.TicketID, map[string]any{
		"draftId":    d.ID,
		"verdict":    string(d.GuardResult.Verdict),
		"overridden": overridden,
	}); err != nil {
		return nil, err
	}
	if overridden {
		if err := s.audit.Record(ctx, actor, "draft.send_overridden", d.TicketID, map[string]any{
			"draftId":        d.ID,
			"overrideReason": strings.TrimSpace(overrideReason),
		}); err != nil {
			return nil, err
		}
	}
	return d, nil
}

// UpdateBody edits a draft's text and re-submits it for review (AC-26).
// Only allowed once a prior review is in and it wasn't a clean pass: a
// "revise" verdict (something to fix), or a failed guard run (nothing
// useful came back at all). A "send" verdict should be sent, not edited; an
// "escalate" verdict's only way forward is this same edit-and-resubmit
// path.
func (s *Service) UpdateBody(ctx context.Context, actor, ticketID, draftID, body string) (*Draft, error) {
	d, err := s.FindByID(ctx, ticketID, draftID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(body) == "" {
		return nil, invalid("body must be a non-empty string")
	}

	editable := d.Status == StatusGuardFailed ||
		(d.Status == StatusGuarded && (d.GuardResult.Verdict == VerdictRevise || d.GuardResult.Verdict == VerdictEscalate))
	if !editable {
		return nil, conflict("draft %s cannot be edited from status %q", d.ID, d.Status)
	}

	ticketCtx, notes, err := s.tickets.ReadContext(ctx, ticketID)
	if err != nil {
		return nil, err
	}

	d.Body = strings.TrimSpace(body)
	d.UpdatedAt = now()
	if err := s.repo.UpdateBody(ctx, d.ID, d.Body, d.UpdatedAt); err != nil {
		return nil, err
	}
	d.Status = StatusPendingReview
	d.GuardResult = nil

	s.dispatchGuard(GuardRequest{
		TicketID:      d.TicketID,
		DraftID:       d.ID,
		Ticket:        ticketCtx,
		InternalNotes: notes,
		DraftBody:     d.Body,
	})

	return d, nil
}
