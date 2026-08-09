package cannedresponses

import (
	"context"
	"errors"
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

func (r *PostgresRepository) Create(ctx context.Context, cr *CannedResponse) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO canned_responses (id, title, body, tags, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, cr.ID, cr.Title, cr.Body, cr.Tags, cr.CreatedAt, cr.UpdatedAt)
	return err
}

// FindAll filters by tag case-insensitively (AC-9) and orders by title
// ascending (AC-8); an empty/whitespace Tag applies no filter (AC-10).
func (r *PostgresRepository) FindAll(ctx context.Context, filter Filter) ([]CannedResponse, error) {
	query := `SELECT id, title, body, tags, created_at, updated_at FROM canned_responses`
	var args []any
	if tag := strings.ToLower(strings.TrimSpace(filter.Tag)); tag != "" {
		args = append(args, tag)
		query += " WHERE $1 = ANY(tags)"
	}
	query += " ORDER BY title ASC"

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []CannedResponse{}
	for rows.Next() {
		cr, err := scanCannedResponse(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *cr)
	}
	return result, rows.Err()
}

func (r *PostgresRepository) FindByID(ctx context.Context, id string) (*CannedResponse, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, title, body, tags, created_at, updated_at FROM canned_responses WHERE id = $1
	`, id)

	cr, err := scanCannedResponse(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return cr, nil
}

func (r *PostgresRepository) Update(ctx context.Context, cr *CannedResponse) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE canned_responses SET title = $2, body = $3, tags = $4, updated_at = $5
		WHERE id = $1
	`, cr.ID, cr.Title, cr.Body, cr.Tags, cr.UpdatedAt)
	return err
}

func (r *PostgresRepository) Delete(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM canned_responses WHERE id = $1`, id)
	return err
}

// rowScanner is satisfied by both pgx.Row and pgx.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanCannedResponse(row rowScanner) (*CannedResponse, error) {
	var cr CannedResponse
	if err := row.Scan(&cr.ID, &cr.Title, &cr.Body, &cr.Tags, &cr.CreatedAt, &cr.UpdatedAt); err != nil {
		return nil, err
	}
	return &cr, nil
}
