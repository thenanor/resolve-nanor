package audit

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) Insert(ctx context.Context, e Entry) error {
	details, err := json.Marshal(e.Details)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO audit_entries (actor, action, ticket_id, details, at)
		VALUES ($1, $2, $3, $4, $5)
	`, e.Actor, e.Action, e.TicketID, details, e.At)
	return err
}

func (r *PostgresRepository) List(ctx context.Context, ticketID string) ([]Entry, error) {
	var rows pgx.Rows
	var err error
	if ticketID != "" {
		rows, err = r.pool.Query(ctx, `
			SELECT seq, actor, action, ticket_id, details, at FROM audit_entries
			WHERE ticket_id = $1 ORDER BY seq ASC
		`, ticketID)
	} else {
		rows, err = r.pool.Query(ctx, `
			SELECT seq, actor, action, ticket_id, details, at FROM audit_entries
			ORDER BY seq ASC
		`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := []Entry{}
	for rows.Next() {
		var e Entry
		var details []byte
		if err := rows.Scan(&e.Seq, &e.Actor, &e.Action, &e.TicketID, &details, &e.At); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(details, &e.Details); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
