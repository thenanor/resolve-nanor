package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	ticketsService := tickets.NewService(
		ticketsRepo,
		ticketsAuditAdapter{auditService},
		httpTriageNotifier{baseURL: cfg.TriageServiceURL, client: http.DefaultClient},
	).WithReplyGuard(httpReplyGuardClient{baseURL: cfg.ReplyGuardServiceURL, client: http.DefaultClient})

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

// httpTriageNotifier satisfies tickets.TriageNotifier by POSTing to the
// separate triage service, mirroring ticketsAuditAdapter's role of bridging
// a concrete cross-package client into a tickets-local interface.
type httpTriageNotifier struct {
	baseURL string
	client  *http.Client
}

func (n httpTriageNotifier) NotifyTicketCreated(ctx context.Context, ticketID, subject, description string) error {
	body, err := json.Marshal(map[string]string{
		"ticketId":    ticketID,
		"subject":     subject,
		"description": description,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.baseURL+"/triage", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("triage service returned status %d for ticket %s", resp.StatusCode, ticketID)
	}
	return nil
}

// httpReplyGuardClient satisfies tickets.ReplyGuardClient by calling the
// separate reply-guard service synchronously and decoding its full
// assessment out of the response body — unlike httpTriageNotifier (which
// only fires-and-forgets a notification), tickets.Service.AddComment blocks
// on this call and uses its result to gate comment creation.
type httpReplyGuardClient struct {
	baseURL string
	client  *http.Client
}

func (c httpReplyGuardClient) Guard(ctx context.Context, input tickets.ReplyGuardInput) (tickets.ReplyGuardOutcome, error) {
	notes := make([]map[string]string, len(input.InternalNotes))
	for i, n := range input.InternalNotes {
		notes[i] = map[string]string{"author": n.Author, "body": n.Body}
	}

	reqBody, err := json.Marshal(map[string]any{
		"ticketSubject":     input.TicketSubject,
		"ticketDescription": input.TicketDescription,
		"ticketStatus":      input.TicketStatus,
		"ticketPriority":    input.TicketPriority,
		"internalNotes":     notes,
		"body":              input.Body,
	})
	if err != nil {
		return tickets.ReplyGuardOutcome{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/guard", bytes.NewReader(reqBody))
	if err != nil {
		return tickets.ReplyGuardOutcome{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return tickets.ReplyGuardOutcome{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return tickets.ReplyGuardOutcome{}, fmt.Errorf("reply-guard service returned status %d", resp.StatusCode)
	}

	var wire struct {
		Verdict  string `json:"verdict"`
		Findings []struct {
			Policy   string `json:"policy"`
			Severity string `json:"severity"`
			Issue    string `json:"issue"`
			Quote    string `json:"quote"`
		} `json:"findings"`
		Confidence         string `json:"confidence"`
		Reasoning          string `json:"reasoning"`
		InjectionSuspected bool   `json:"injectionSuspected"`
		RequireHuman       bool   `json:"requireHuman"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wire); err != nil {
		return tickets.ReplyGuardOutcome{}, fmt.Errorf("decode reply-guard response: %w", err)
	}

	confidence := tickets.Confidence(wire.Confidence)
	if !confidence.Valid() {
		return tickets.ReplyGuardOutcome{}, fmt.Errorf("reply-guard returned invalid confidence %q", wire.Confidence)
	}

	findings := make([]tickets.ReplyGuardFinding, len(wire.Findings))
	for i, f := range wire.Findings {
		findings[i] = tickets.ReplyGuardFinding{Policy: f.Policy, Severity: f.Severity, Issue: f.Issue, Quote: f.Quote}
	}

	return tickets.ReplyGuardOutcome{
		Verdict:            wire.Verdict,
		Findings:           findings,
		Confidence:         confidence,
		Reasoning:          wire.Reasoning,
		InjectionSuspected: wire.InjectionSuspected,
		RequireHuman:       wire.RequireHuman,
	}, nil
}
