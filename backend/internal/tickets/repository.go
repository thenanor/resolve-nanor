package tickets

import (
	"context"
)

type Filter struct {
	Status   Status
	Priority Priority
}

// Repository is the persistence boundary for tickets. It splits what
// TypeORM's cascade-save did implicitly into explicit operations: creating a
// ticket, updating its mutable fields, and appending a single comment.
type Repository interface {
	CreateTicket(ctx context.Context, t *Ticket) error
	FindAll(ctx context.Context, filter Filter) ([]Ticket, error)
	FindByID(ctx context.Context, id string) (*Ticket, error)
	UpdateTicket(ctx context.Context, t *Ticket) error
	AddComment(ctx context.Context, ticketID string, c Comment) error
}
