package tickets

import (
	"context"
)

type Filter struct {
	Status   Status
	Priority Priority
}

// Pagination bounds a result page. It is deliberately kept separate from
// Filter (which describes *which* rows match) and from Page (which
// describes what came back). Today it's offset-based; cursor-based
// pagination can land later by adding a Cursor field here and a
// corresponding branch in the repository implementations, without touching
// Filter or any call site that only filters.
type Pagination struct {
	Limit  int
	Offset int
}

// Page is one page of tickets. HasMore is derived by the repository (e.g.
// by fetching one row past Limit) rather than from a total count, so the
// cost stays flat regardless of how deep the caller pages — the same
// property a cursor-based implementation would have.
type Page struct {
	Tickets []Ticket `json:"tickets"`
	Limit   int      `json:"limit"`
	Offset  int      `json:"offset"`
	HasMore bool     `json:"hasMore"`
}

// Repository is the persistence boundary for tickets. It splits what
// TypeORM's cascade-save did implicitly into explicit operations: creating a
// ticket, updating its mutable fields, and appending a single comment.
type Repository interface {
	CreateTicket(ctx context.Context, t *Ticket) error
	FindAll(ctx context.Context, filter Filter) ([]Ticket, error)
	FindPage(ctx context.Context, filter Filter, page Pagination) (Page, error)
	FindByID(ctx context.Context, id string) (*Ticket, error)
	UpdateTicket(ctx context.Context, t *Ticket) error
	AddComment(ctx context.Context, ticketID string, c Comment) error

	// SetTriage applies a triage result directly (medium/high confidence),
	// clearing any stale pending suggestion.
	SetTriage(ctx context.Context, id string, category Category, priority Priority, updatedAt string) error
	// SetPendingTriage stores a low-confidence triage result as a pending
	// suggestion without touching the ticket's actual category/priority.
	SetPendingTriage(ctx context.Context, id string, category Category, priority Priority, updatedAt string) error
	// AcceptPendingTriage promotes a pending suggestion to the ticket's
	// actual category/priority and clears the pending fields.
	AcceptPendingTriage(ctx context.Context, id string, updatedAt string) error
	// RejectPendingTriage clears a pending suggestion without applying it.
	RejectPendingTriage(ctx context.Context, id string, updatedAt string) error
}
