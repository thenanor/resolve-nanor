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
	"resolve/internal/config"
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
	ticketsService := tickets.NewService(ticketsRepo, auditService)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	tickets.NewHandler(ticketsService).Mount(r)
	audit.NewHandler(auditService).Mount(r)
	stats.NewHandler(ticketsService).Mount(r)
	health.NewHandler(cfg.Version).Mount(r)

	log.Printf("resolve listening on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, r); err != nil {
		log.Fatal(err)
	}
}

func applySchema(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, migrations.Schema)
	return err
}
