// Package triage classifies a newly-created ticket into a category and
// priority by calling the Claude API, then writes the result back onto the
// ticket via the main app's HTTP API. It has no database of its own — see
// Service for the classify-then-write-back flow.
package triage

import (
	"context"

	"resolve/internal/tickets"
)

// Result is a single classification produced by a Classifier. Category and
// Priority reuse the tickets package's enum types rather than duplicating
// them, since they're assigned straight onto a tickets.Ticket.
type Result struct {
	Category tickets.Category
	Priority tickets.Priority
	// Confidence is between 0 and 1, where 0 means the classifier could not
	// confidently classify the ticket at all.
	Confidence float64
	// InjectionSuspected is true when the ticket text appears to contain
	// instructions aimed at the classifier itself, rather than genuine
	// support content. Defaults to false.
	InjectionSuspected bool
	// Reasoning is a one-sentence (max 25 words) explanation of why the
	// category was chosen.
	Reasoning string
}

// Classifier hides the concrete Claude client behind a local interface so
// unit tests substitute a fake instead of hitting the real Anthropic API,
// mirroring this repo's fake-repository testing convention.
type Classifier interface {
	Classify(ctx context.Context, subject, description string) (Result, error)
}

// TicketUpdater is the slice of the main app's HTTP API that triage depends
// on: writing a classification result back onto the ticket it was computed
// for. It mirrors the main app's wire contract (string enums), so callers
// convert a Result's typed fields at the boundary.
type TicketUpdater interface {
	UpdateTriage(ctx context.Context, ticketID, category, priority, confidence string) error
}
