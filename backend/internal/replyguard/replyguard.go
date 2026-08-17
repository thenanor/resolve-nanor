// Package replyguard reviews a draft reply against a fixed policy
// (disclosure, commitment, answer, tone) by calling the Claude API, then
// writes the result back onto the draft via the main app's HTTP API. It has
// no database of its own — see Service for the guard-then-write-back flow.
// It mirrors resolve/internal/triage's shape closely; see that package for
// the precedent this one follows.
package replyguard

import (
	"context"

	"resolve/internal/drafts"
)

// Note is one internal (agent-only) comment on the ticket, as sent to
// reply-guard for context.
type Note struct {
	Author string
	Body   string
}

// GuardInput is everything reply-guard needs to assess one draft.
type GuardInput struct {
	TicketSubject     string
	TicketDescription string
	TicketStatus      string
	TicketPriority    string
	InternalNotes     []Note
	DraftBody         string
}

// Result is a single guard assessment produced by a Classifier. Findings
// reuses drafts.Finding directly since it is assigned straight onto a
// drafts.GuardResult. Verdict and RequireHuman are deliberately not part of
// this type: the model reports findings and injectionSuspected only, and
// Service derives Verdict/RequireHuman from those deterministically (see
// deriveVerdict/deriveRequireHuman in service.go) so there is no way for a
// model-reported verdict to drift from what the findings actually say.
type Result struct {
	Findings []drafts.Finding
	// Confidence is between 0 and 1, where 0 means the model could not
	// confidently assess the draft at all.
	Confidence float64
	// Reasoning is a one/two-sentence explanation of the overall
	// assessment.
	Reasoning string
	// InjectionSuspected is true when the internal notes or the draft
	// itself appear to contain instructions aimed at the classifier,
	// rather than genuine ticket content.
	InjectionSuspected bool
}

// Classifier hides the concrete Claude client behind a local interface so
// unit tests substitute a fake instead of hitting the real Anthropic API,
// mirroring triage.Classifier.
type Classifier interface {
	Guard(ctx context.Context, input GuardInput) (Result, error)
}

// DraftUpdater is the slice of the main app's HTTP API that reply-guard
// depends on: writing a completed guard assessment back onto the draft it
// was computed for. Confidence is passed as the bucketed low/medium/high
// wire value (mirrors triage.TicketUpdater's string-enum convention).
type DraftUpdater interface {
	UpdateGuardResult(ctx context.Context, ticketID, draftID string, verdict string, findings []drafts.Finding, confidence, reasoning string, injectionSuspected, requireHuman bool) error
}
