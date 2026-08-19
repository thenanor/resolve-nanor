// Package replyguard reviews a candidate customer-facing reply against a
// fixed policy (disclosure, commitment, answer, tone) by calling the Claude
// API, and returns the full assessment synchronously — see Service for the
// single guard-and-return call. It mirrors resolve/internal/triage's shape
// closely; see that package for the precedent this one follows.
package replyguard

import (
	"context"

	"resolve/internal/tickets"
)

// Note is one internal (agent-only) comment on the ticket, as sent to
// reply-guard for context.
type Note struct {
	Author string
	Body   string
}

// GuardInput is everything reply-guard needs to assess one candidate reply.
type GuardInput struct {
	TicketSubject     string
	TicketDescription string
	TicketStatus      string
	TicketPriority    string
	InternalNotes     []Note
	Body              string
}

// Verdict is reply-guard's recommendation for a candidate reply.
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

// Finding is one reply-policy issue reply-guard found in a candidate reply.
type Finding struct {
	Policy   Policy   `json:"policy"`
	Severity Severity `json:"severity"`
	Issue    string   `json:"issue"`
	// Quote is a literal, contiguous substring of the candidate reply's
	// body that triggered this finding — never a paraphrase.
	Quote string `json:"quote"`
}

// GuardResult is reply-guard's full assessment of a candidate reply,
// returned synchronously from Service.Guard. Verdict and RequireHuman are
// deliberately derived here rather than reported by the model: the model
// reports findings and injectionSuspected only, and Service derives
// Verdict/RequireHuman from those deterministically (see
// deriveVerdict/deriveRequireHuman in service.go) so there is no way for a
// model-reported verdict to drift from what the findings actually say.
type GuardResult struct {
	Verdict            Verdict            `json:"verdict"`
	Findings           []Finding          `json:"findings"`
	Confidence         tickets.Confidence `json:"confidence"`
	Reasoning          string             `json:"reasoning"`
	InjectionSuspected bool               `json:"injectionSuspected"`
	RequireHuman       bool               `json:"requireHuman"`
}

// Result is what a Classifier returns: the model's own findings and
// injectionSuspected/confidence/reasoning, before Service derives
// Verdict/RequireHuman from them (see GuardResult's doc comment).
type Result struct {
	Findings           []Finding
	Confidence         float64
	Reasoning          string
	InjectionSuspected bool
}

// Classifier hides the concrete Claude client behind a local interface so
// unit tests substitute a fake instead of hitting the real Anthropic API,
// mirroring triage.Classifier.
type Classifier interface {
	Guard(ctx context.Context, input GuardInput) (Result, error)
}
