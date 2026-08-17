package drafts

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) CreateDraft(ctx context.Context, d *Draft) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO drafts (id, ticket_id, author, body, status, guard_result, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, d.ID, d.TicketID, d.Author, d.Body, string(d.Status), nil, d.CreatedAt, d.UpdatedAt)
	return err
}

func (r *PostgresRepository) FindByID(ctx context.Context, draftID string) (*Draft, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, ticket_id, author, body, status, guard_result, created_at, updated_at
		FROM drafts WHERE id = $1
	`, draftID)
	return scanDraft(row)
}

func (r *PostgresRepository) UpdateBody(ctx context.Context, draftID, body, updatedAt string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE drafts SET body = $2, status = $3, guard_result = NULL, updated_at = $4
		WHERE id = $1
	`, draftID, body, string(StatusPendingReview), updatedAt)
	return err
}

func (r *PostgresRepository) SetGuardResult(ctx context.Context, draftID string, result GuardResult, updatedAt string) error {
	raw, err := json.Marshal(result)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, `
		UPDATE drafts SET status = $2, guard_result = $3, updated_at = $4
		WHERE id = $1
	`, draftID, string(StatusGuarded), raw, updatedAt)
	return err
}

func (r *PostgresRepository) SetGuardFailed(ctx context.Context, draftID, updatedAt string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE drafts SET status = $2, updated_at = $3
		WHERE id = $1
	`, draftID, string(StatusGuardFailed), updatedAt)
	return err
}

func (r *PostgresRepository) MarkSent(ctx context.Context, draftID, updatedAt string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE drafts SET status = $2, updated_at = $3
		WHERE id = $1
	`, draftID, string(StatusSent), updatedAt)
	return err
}

// rowScanner is satisfied by both pgx.Row and pgx.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanDraft(row rowScanner) (*Draft, error) {
	var d Draft
	var status string
	var guardResult []byte
	err := row.Scan(&d.ID, &d.TicketID, &d.Author, &d.Body, &status, &guardResult, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	d.Status = Status(status)
	if guardResult != nil {
		var gr GuardResult
		if err := json.Unmarshal(guardResult, &gr); err != nil {
			return nil, err
		}
		d.GuardResult = &gr
	}
	return &d, nil
}
