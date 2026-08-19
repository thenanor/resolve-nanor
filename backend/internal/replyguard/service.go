package replyguard

import (
	"context"
	"fmt"
	"log"

	"resolve/internal/tickets"
)

type Service struct {
	classifier Classifier
}

func NewService(classifier Classifier) *Service {
	return &Service{classifier: classifier}
}

// Guard assesses a candidate reply and returns the full result. It runs
// fully synchronously — the caller (tickets.Service.AddComment) is itself
// the request path that gates comment creation on this call, so there is
// no benefit to backgrounding work on this side.
func (s *Service) Guard(ctx context.Context, input GuardInput) (GuardResult, error) {
	result, err := s.classifier.Guard(ctx, input)
	if err != nil {
		return GuardResult{}, fmt.Errorf("guard candidate reply: %w", err)
	}
	if result.InjectionSuspected {
		log.Printf("reply-guard: possible prompt injection suspected: %s", result.Reasoning)
	}

	confidence := confidenceLabel(result.Confidence)
	verdict := deriveVerdict(result.Findings, result.InjectionSuspected)
	requireHuman := deriveRequireHuman(verdict, result.Findings, confidence)

	return GuardResult{
		Verdict:            verdict,
		Findings:           result.Findings,
		Confidence:         confidence,
		Reasoning:          result.Reasoning,
		InjectionSuspected: result.InjectionSuspected,
		RequireHuman:       requireHuman,
	}, nil
}

// confidenceLabel buckets a 0-1 confidence score into the low/medium/high
// enum the main app's write-back endpoint accepts, mirroring
// triage's confidenceLabel exactly.
func confidenceLabel(confidence float64) tickets.Confidence {
	switch {
	case confidence < 1.0/3:
		return tickets.ConfidenceLow
	case confidence < 2.0/3:
		return tickets.ConfidenceMedium
	default:
		return tickets.ConfidenceHigh
	}
}

// deriveVerdict implements the reply-guard spec's AC-14/15/16 exactly:
// escalate on any high-severity finding or suspected injection; send only
// when there's nothing above low severity; revise otherwise. Verdict is
// always computed here, never taken from the model directly (see
// GuardResult's doc comment for why).
func deriveVerdict(findings []Finding, injectionSuspected bool) Verdict {
	hasHigh := false
	hasMedium := false
	for _, f := range findings {
		switch f.Severity {
		case SeverityHigh:
			hasHigh = true
		case SeverityMedium:
			hasMedium = true
		}
	}
	if hasHigh || injectionSuspected {
		return VerdictEscalate
	}
	if hasMedium {
		return VerdictRevise
	}
	return VerdictSend
}

// deriveRequireHuman implements AC-16: true whenever the verdict is
// escalate, whenever any finding is high severity, or whenever confidence
// is low; false only for a clean send at non-low confidence. The
// high-severity check is redundant with escalate given deriveVerdict's
// logic (a high finding always produces escalate) but is kept explicit,
// matching the spec's wording, as defense against the two functions ever
// drifting apart.
func deriveRequireHuman(verdict Verdict, findings []Finding, confidence tickets.Confidence) bool {
	if verdict == VerdictEscalate {
		return true
	}
	if confidence == tickets.ConfidenceLow {
		return true
	}
	for _, f := range findings {
		if f.Severity == SeverityHigh {
			return true
		}
	}
	return false
}
