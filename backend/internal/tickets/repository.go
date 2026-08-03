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
}
