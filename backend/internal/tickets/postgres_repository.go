package tickets

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) CreateTicket(ctx context.Context, t *Ticket) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO tickets (id, subject, description, customer_email, priority, category, status, created_at, updated_at, resolved_at, pending_category, pending_priority)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, t.ID, t.Subject, t.Description, t.CustomerEmail, string(t.Priority), categoryArg(t.Category), string(t.Status), t.CreatedAt, t.UpdatedAt, t.ResolvedAt, categoryArg(t.PendingCategory), priorityArg(t.PendingPriority))
	return err
}

func (r *PostgresRepository) UpdateTicket(ctx context.Context, t *Ticket) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE tickets SET status = $2, updated_at = $3, resolved_at = $4
		WHERE id = $1
	`, t.ID, string(t.Status), t.UpdatedAt, t.ResolvedAt)
	return err
}

// SetTriage applies a triage result directly, clearing any stale pending
// suggestion (see the tickets.Repository doc comment for the medium/high
// vs. low confidence split).
func (r *PostgresRepository) SetTriage(ctx context.Context, id string, category Category, priority Priority, updatedAt string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE tickets SET category = $2, priority = $3, pending_category = NULL, pending_priority = NULL, updated_at = $4
		WHERE id = $1
	`, id, string(category), string(priority), updatedAt)
	return err
}

// SetPendingTriage stores a low-confidence triage result as a pending
// suggestion without touching the ticket's actual category/priority.
func (r *PostgresRepository) SetPendingTriage(ctx context.Context, id string, category Category, priority Priority, updatedAt string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE tickets SET pending_category = $2, pending_priority = $3, updated_at = $4
		WHERE id = $1
	`, id, string(category), string(priority), updatedAt)
	return err
}

// AcceptPendingTriage promotes a pending suggestion to the ticket's actual
// category/priority and clears the pending fields, all in one statement so
// there's no read-then-write race with a concurrent triage write.
func (r *PostgresRepository) AcceptPendingTriage(ctx context.Context, id string, updatedAt string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE tickets
		SET category = pending_category,
		    priority = COALESCE(pending_priority, priority),
		    pending_category = NULL,
		    pending_priority = NULL,
		    updated_at = $2
		WHERE id = $1
	`, id, updatedAt)
	return err
}

// RejectPendingTriage clears a pending suggestion without applying it.
func (r *PostgresRepository) RejectPendingTriage(ctx context.Context, id string, updatedAt string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE tickets SET pending_category = NULL, pending_priority = NULL, updated_at = $2
		WHERE id = $1
	`, id, updatedAt)
	return err
}

func categoryArg(c *Category) *string {
	if c == nil {
		return nil
	}
	s := string(*c)
	return &s
}

func priorityArg(p *Priority) *string {
	if p == nil {
		return nil
	}
	s := string(*p)
	return &s
}

func (r *PostgresRepository) AddComment(ctx context.Context, ticketID string, c Comment) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO ticket_comments (id, ticket_id, author, body, internal, at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, c.ID, ticketID, c.Author, c.Body, c.Internal, c.At)
	return err
}

func (r *PostgresRepository) FindByID(ctx context.Context, id string) (*Ticket, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, subject, description, customer_email, priority, category, status, created_at, updated_at, resolved_at, pending_category, pending_priority
		FROM tickets WHERE id = $1
	`, id)

	t, err := scanTicket(row)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, nil
	}
	comments, err := r.commentsFor(ctx, t.ID)
	if err != nil {
		return nil, err
	}
	t.Comments = comments
	return t, nil
}

func (r *PostgresRepository) FindAll(ctx context.Context, filter Filter) ([]Ticket, error) {
	query, args := findTicketsQuery(filter)
	return r.queryTickets(ctx, query, args)
}

// FindPage applies filter, then LIMIT/OFFSET. It asks for one row past
// page.Limit so HasMore can be derived from what came back instead of a
// separate COUNT(*) query — the cost is O(page size), not O(table size),
// which is also what a cursor-based implementation would give us.
func (r *PostgresRepository) FindPage(ctx context.Context, filter Filter, page Pagination) (Page, error) {
	query, args := findTicketsQuery(filter)
	args = append(args, page.Limit+1, page.Offset)
	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)-1, len(args))

	tickets, err := r.queryTickets(ctx, query, args)
	if err != nil {
		return Page{}, err
	}

	hasMore := len(tickets) > page.Limit
	if hasMore {
		tickets = tickets[:page.Limit]
	}
	return Page{
		Tickets: tickets,
		Limit:   page.Limit,
		Offset:  page.Offset,
		HasMore: hasMore,
	}, nil
}

// findTicketsQuery builds the shared SELECT + filter WHERE clause used by
// both FindAll and FindPage. The ORDER BY includes id as a tiebreaker so
// rows with an identical created_at still sort consistently across pages.
func findTicketsQuery(filter Filter) (string, []any) {
	query := `
		SELECT id, subject, description, customer_email, priority, category, status, created_at, updated_at, resolved_at, pending_category, pending_priority
		FROM tickets
	`
	var conds []string
	var args []any
	if filter.Status != "" {
		args = append(args, string(filter.Status))
		conds = append(conds, fmt.Sprintf("status = $%d", len(args)))
	}
	if filter.Priority != "" {
		args = append(args, string(filter.Priority))
		conds = append(conds, fmt.Sprintf("priority = $%d", len(args)))
	}
	if len(conds) > 0 {
		query += " WHERE " + strings.Join(conds, " AND ")
	}
	query += " ORDER BY created_at ASC, id ASC"
	return query, args
}

func (r *PostgresRepository) queryTickets(ctx context.Context, query string, args []any) ([]Ticket, error) {
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []Ticket
	for rows.Next() {
		t, err := scanTicketRow(rows)
		if err != nil {
			return nil, err
		}
		comments, err := r.commentsFor(ctx, t.ID)
		if err != nil {
			return nil, err
		}
		t.Comments = comments
		result = append(result, *t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if result == nil {
		result = []Ticket{}
	}
	return result, nil
}

func (r *PostgresRepository) commentsFor(ctx context.Context, ticketID string) ([]Comment, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT seq, id, author, body, internal, at
		FROM ticket_comments WHERE ticket_id = $1 ORDER BY seq ASC
	`, ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	comments := []Comment{}
	for rows.Next() {
		var c Comment
		if err := rows.Scan(&c.Seq, &c.ID, &c.Author, &c.Body, &c.Internal, &c.At); err != nil {
			return nil, err
		}
		comments = append(comments, c)
	}
	return comments, rows.Err()
}

// rowScanner is satisfied by both pgx.Row and pgx.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanTicket(row rowScanner) (*Ticket, error) {
	return scanTicketRow(row)
}

// scanTicketRow's Scan targets must stay in the exact order of the SELECT
// column lists in FindByID and findTicketsQuery above — this file has no
// named-column scanning, so a reordered SELECT silently scans the wrong
// value into the wrong field.
func scanTicketRow(row rowScanner) (*Ticket, error) {
	var t Ticket
	var priority, status string
	var category, pendingCategory, pendingPriority *string
	err := row.Scan(&t.ID, &t.Subject, &t.Description, &t.CustomerEmail, &priority, &category, &status, &t.CreatedAt, &t.UpdatedAt, &t.ResolvedAt, &pendingCategory, &pendingPriority)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	t.Priority = Priority(priority)
	t.Status = Status(status)
	if category != nil {
		c := Category(*category)
		t.Category = &c
	}
	if pendingCategory != nil {
		c := Category(*pendingCategory)
		t.PendingCategory = &c
	}
	if pendingPriority != nil {
		p := Priority(*pendingPriority)
		t.PendingPriority = &p
	}
	return &t, nil
}
