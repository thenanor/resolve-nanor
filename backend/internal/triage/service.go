package triage

import (
	"context"
	"fmt"
	"log"

	"resolve/internal/tickets"
)

type Service struct {
	classifier Classifier
	updater    TicketUpdater
}

func NewService(classifier Classifier, updater TicketUpdater) *Service {
	return &Service{classifier: classifier, updater: updater}
}

// Triage classifies a ticket and writes the result back. It runs fully
// synchronously — the main app already decouples ticket-creation latency by
// calling this service from its own goroutine, so there is no benefit (and
// real cost, in the form of a second uncoordinated fire-and-forget layer) to
// also backgrounding work on this side. See tickets.Service.Create.
func (s *Service) Triage(ctx context.Context, ticketID, subject, description string) error {
	result, err := s.classifier.Classify(ctx, subject, description)
	if err != nil {
		return fmt.Errorf("classify ticket %s: %w", ticketID, err)
	}
	if result.InjectionSuspected {
		log.Printf("triage: possible prompt injection in ticket %s: %s", ticketID, result.Reasoning)
	}
	confidence := confidenceLabel(result.Confidence)
	if err := s.updater.UpdateTriage(ctx, ticketID, string(result.Category), string(result.Priority), string(confidence)); err != nil {
		return fmt.Errorf("update ticket %s with triage result: %w", ticketID, err)
	}
	return nil
}

// confidenceLabel buckets a 0-1 confidence score into the low/medium/high
// enum the main app's /triage endpoint accepts, since its pending-vs-direct-
// apply threshold logic (tickets.Service.ApplyTriage) is unaffected by this
// package moving to a numeric confidence score internally.
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
