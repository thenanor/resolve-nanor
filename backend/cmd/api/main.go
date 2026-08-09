package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"resolve/internal/audit"
	"resolve/internal/cannedresponses"
	"resolve/internal/config"
	"resolve/internal/docs"
	"resolve/internal/health"
	"resolve/internal/migrations"
	"resolve/internal/stats"
	"resolve/internal/tickets"
)

func main() {
	cfg := config.Load()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connect to database: %v", err)
	}
	defer pool.Close()

	if err := applySchema(ctx, pool); err != nil {
		log.Fatalf("apply schema: %v", err)
	}

	auditRepo := audit.NewPostgresRepository(pool)
	auditService := audit.NewService(auditRepo)

	ticketsRepo := tickets.NewPostgresRepository(pool)
	ticketsService := tickets.NewService(ticketsRepo, ticketsAuditAdapter{auditService})

	cannedResponsesRepo := cannedresponses.NewPostgresRepository(pool)
	cannedResponsesService := cannedresponses.NewService(cannedResponsesRepo)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	tickets.NewHandler(ticketsService).Mount(r)
	audit.NewHandler(auditService).Mount(r)
	stats.NewHandler(ticketsService).Mount(r)
	cannedresponses.NewHandler(cannedResponsesService).Mount(r)
	health.NewHandler(cfg.Version).Mount(r)
	docs.NewHandler().Mount(r)

	log.Printf("resolve listening on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, r); err != nil {
		log.Fatal(err)
	}
}

func applySchema(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, migrations.Schema)
	return err
}

// ticketsAuditAdapter satisfies tickets.AuditRecorder by delegating to
// audit.Service, translating audit.Entry into tickets.AuditEntry so neither
// package needs to import the other.
type ticketsAuditAdapter struct {
	svc *audit.Service
}

func (a ticketsAuditAdapter) Record(ctx context.Context, actor, action, ticketID string, details map[string]any) error {
	return a.svc.Record(ctx, actor, action, ticketID, details)
}

func (a ticketsAuditAdapter) List(ctx context.Context, ticketID string) ([]tickets.AuditEntry, error) {
	entries, err := a.svc.List(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	out := make([]tickets.AuditEntry, len(entries))
	for i, e := range entries {
		out[i] = tickets.AuditEntry{
			Actor:    e.Actor,
			Action:   e.Action,
			TicketID: e.TicketID,
			Details:  e.Details,
			At:       e.At,
		}
	}
	return out, nil
}
