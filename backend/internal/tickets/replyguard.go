package tickets

import "context"

// ReplyGuardNote is one internal (agent-only) comment on the ticket, as
// sent to reply-guard for context (REPLYGUARD-1 AC-6).
type ReplyGuardNote struct {
	Author string
	Body   string
}

// ReplyGuardInput is everything reply-guard needs to review one candidate
// reply: the ticket's subject/description/status/priority, its internal
// notes, and the reply body itself — nothing else (AC-6).
type ReplyGuardInput struct {
	TicketSubject     string
	TicketDescription string
	TicketStatus      string
	TicketPriority    string
	InternalNotes     []ReplyGuardNote
	Body              string
}

// ReplyGuardFinding mirrors one entry of reply-guard's findings array.
// Policy/Severity are left as plain strings rather than new tickets-owned
// enum types — tickets does not own or validate reply-guard's
// policy/severity vocabulary, replyguard.Service does. This is a
// pass-through wire shape, not a domain type.
type ReplyGuardFinding struct {
	Policy   string `json:"policy"`
	Severity string `json:"severity"`
	Issue    string `json:"issue"`
	Quote    string `json:"quote"`
}

// ReplyGuardOutcome is reply-guard's full synchronous assessment of one
// candidate reply. It is never persisted — it exists only for the
// duration of the AddComment call that produced it (returned inline in
// the API response, folded into the audit entry's details), never as a
// field on Comment itself, since Comment is also embedded read-only in
// Ticket.comments.
//
// Confidence reuses tickets.Confidence directly, same bucketed shape as
// triage's public confidence contract. Verdict stays a plain string for
// the same reason ReplyGuardFinding's Policy/Severity do.
type ReplyGuardOutcome struct {
	Verdict            string              `json:"verdict"`
	Findings           []ReplyGuardFinding `json:"findings"`
	Confidence         Confidence          `json:"confidence"`
	Reasoning          string              `json:"reasoning"`
	InjectionSuspected bool                `json:"injectionSuspected"`
	RequireHuman       bool                `json:"requireHuman"`
}

// Reply-guard verdict wire values. Untyped constants (not a tickets.Verdict
// enum) for the same reason ReplyGuardOutcome.Verdict is a plain string.
const (
	guardVerdictSend     = "send"
	guardVerdictRevise   = "revise"
	guardVerdictEscalate = "escalate"
)

// ReplyGuardClient is the slice of the reply-guard service's HTTP API that
// tickets depends on to synchronously review a candidate reply before
// creating an internal:false, non-canned Comment. Declared here (consumer
// side), like AuditRecorder/TriageNotifier, so tickets does not need to
// import an HTTP-client package for reply-guard.
type ReplyGuardClient interface {
	// Guard reviews input synchronously and returns reply-guard's full
	// assessment. A non-nil error means the guard call itself could not be
	// completed (network failure, non-2xx response, malformed/invalid
	// model output) — AddComment maps that to *GuardUnavailableError and
	// never falls back to creating the comment unguarded (AC-22).
	Guard(ctx context.Context, input ReplyGuardInput) (ReplyGuardOutcome, error)
}
