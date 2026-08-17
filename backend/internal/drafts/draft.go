// Package drafts holds a customer reply an agent has written but not yet
// sent. A draft is reviewed by the reply-guard service (see
// resolve/internal/replyguard) before it may become a real, customer-visible
// tickets.Comment — see Service.Send for the gate.
package drafts

import "resolve/internal/tickets"

// Status is a draft's review lifecycle state.
type Status string

const (
	StatusPendingReview Status = "pending_review"
	StatusGuarded       Status = "guarded"
	StatusGuardFailed   Status = "guard_failed"
	StatusSent          Status = "sent"
)

// Verdict is reply-guard's recommendation for a draft.
type Verdict string

const (
	VerdictSend     Verdict = "send"
	VerdictRevise   Verdict = "revise"
	VerdictEscalate Verdict = "escalate"
)

var AllVerdicts = []Verdict{VerdictSend, VerdictRevise, VerdictEscalate}

func (v Verdict) Valid() bool {
	for _, x := range AllVerdicts {
		if x == v {
			return true
		}
	}
	return false
}

// Policy is which reply-policy line a Finding is about.
type Policy string

const (
	PolicyDisclosure Policy = "disclosure"
	PolicyCommitment Policy = "commitment"
	PolicyAnswer     Policy = "answer"
	PolicyTone       Policy = "tone"
)

var AllPolicies = []Policy{PolicyDisclosure, PolicyCommitment, PolicyAnswer, PolicyTone}

func (p Policy) Valid() bool {
	for _, x := range AllPolicies {
		if x == p {
			return true
		}
	}
	return false
}

// Severity is how bad one Finding is, independent of which Policy line it's
// under.
type Severity string

const (
	SeverityLow    Severity = "low"
	SeverityMedium Severity = "medium"
	SeverityHigh   Severity = "high"
)

var AllSeverities = []Severity{SeverityLow, SeverityMedium, SeverityHigh}

func (s Severity) Valid() bool {
	for _, x := range AllSeverities {
		if x == s {
			return true
		}
	}
	return false
}

// Finding is one reply-policy issue reply-guard found in a draft.
type Finding struct {
	Policy   Policy   `json:"policy"`
	Severity Severity `json:"severity"`
	Issue    string   `json:"issue"`
	// Quote is a literal, contiguous substring of the draft body that
	// triggered this finding — never a paraphrase.
	Quote string `json:"quote"`
}

// GuardResult is reply-guard's full assessment of a draft, written back onto
// it once a Guard call completes (see Service.RecordGuardResult).
type GuardResult struct {
	Verdict  Verdict   `json:"verdict"`
	Findings []Finding `json:"findings"`
	// Confidence reuses tickets.Confidence — the triage service's public
	// confidence contract — for consistency across the app's two AI
	// services' wire shapes, even though this confidence describes a
	// verdict rather than a classification.
	Confidence         tickets.Confidence `json:"confidence"`
	Reasoning          string             `json:"reasoning"`
	InjectionSuspected bool               `json:"injectionSuspected"`
	RequireHuman       bool               `json:"requireHuman"`
}

// Draft is a reply an agent has written but not yet sent to the customer.
type Draft struct {
	ID          string       `json:"id"`
	TicketID    string       `json:"ticketId"`
	Author      string       `json:"author"`
	Body        string       `json:"body"`
	Status      Status       `json:"status"`
	GuardResult *GuardResult `json:"guardResult"`
	CreatedAt   string       `json:"createdAt"`
	UpdatedAt   string       `json:"updatedAt"`
}
