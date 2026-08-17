package replyguard

import (
	"context"
	"fmt"
	"log"

	"resolve/internal/drafts"
	"resolve/internal/tickets"
)

type Service struct {
	classifier Classifier
	updater    DraftUpdater
}

func NewService(classifier Classifier, updater DraftUpdater) *Service {
	return &Service{classifier: classifier, updater: updater}
}

// Guard assesses a draft and writes the result back. It runs fully
// synchronously — the main app already decouples draft-creation latency by
// calling this service from its own goroutine (drafts.Service.dispatchGuard),
// so there is no benefit to also backgrounding work on this side. See
// triage.Service.Triage for the identical rationale.
func (s *Service) Guard(ctx context.Context, ticketID, draftID string, input GuardInput) error {
	result, err := s.classifier.Guard(ctx, input)
	if err != nil {
		return fmt.Errorf("guard draft %s (ticket %s): %w", draftID, ticketID, err)
	}
	if result.InjectionSuspected {
		log.Printf("reply-guard: possible prompt injection in draft %s (ticket %s): %s", draftID, ticketID, result.Reasoning)
	}

	confidence := confidenceLabel(result.Confidence)
	verdict := deriveVerdict(result.Findings, result.InjectionSuspected)
	requireHuman := deriveRequireHuman(verdict, result.Findings, confidence)

	if err := s.updater.UpdateGuardResult(ctx, ticketID, draftID, string(verdict), result.Findings, string(confidence), result.Reasoning, result.InjectionSuspected, requireHuman); err != nil {
		return fmt.Errorf("update draft %s with guard result: %w", draftID, err)
	}
	return nil
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
// always computed here, never taken from the model directly (see the
// Result doc comment for why).
func deriveVerdict(findings []drafts.Finding, injectionSuspected bool) drafts.Verdict {
	hasHigh := false
	hasMedium := false
	for _, f := range findings {
		switch f.Severity {
		case drafts.SeverityHigh:
			hasHigh = true
		case drafts.SeverityMedium:
			hasMedium = true
		}
	}
	if hasHigh || injectionSuspected {
		return drafts.VerdictEscalate
	}
	if hasMedium {
		return drafts.VerdictRevise
	}
	return drafts.VerdictSend
}

// deriveRequireHuman implements AC-13: true whenever the verdict is
// escalate, whenever any finding is high severity, or whenever confidence
// is low; false only for a clean send at non-low confidence. The
// high-severity check is redundant with escalate given deriveVerdict's
// logic (a high finding always produces escalate) but is kept explicit,
// matching the spec's wording, as defense against the two functions ever
// drifting apart.
func deriveRequireHuman(verdict drafts.Verdict, findings []drafts.Finding, confidence tickets.Confidence) bool {
	if verdict == drafts.VerdictEscalate {
		return true
	}
	if confidence == tickets.ConfidenceLow {
		return true
	}
	for _, f := range findings {
		if f.Severity == drafts.SeverityHigh {
			return true
		}
	}
	return false
}
