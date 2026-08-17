package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	"resolve/internal/drafts"
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
	)

	cannedResponsesRepo := cannedresponses.NewPostgresRepository(pool)
	cannedResponsesService := cannedresponses.NewService(cannedResponsesRepo)

	draftsRepo := drafts.NewPostgresRepository(pool)
	draftsService := drafts.NewService(
		draftsRepo,
		ticketReaderAdapter{ticketsService},
		ticketCommenterAdapter{ticketsService},
		draftsAuditAdapter{auditService},
		httpReplyGuardNotifier{baseURL: cfg.ReplyGuardServiceURL, client: http.DefaultClient},
	)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	tickets.NewHandler(ticketsService).Mount(r)
	audit.NewHandler(auditService).Mount(r)
	stats.NewHandler(ticketsService).Mount(r)
	cannedresponses.NewHandler(cannedResponsesService).Mount(r)
	drafts.NewHandler(draftsService).Mount(r)
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

// ticketReaderAdapter satisfies drafts.TicketReader by delegating to
// tickets.Service, translating its Ticket/Comment shape into drafts' own
// narrower TicketContext/InternalNote types so neither package needs to
// import the other's full domain, mirroring ticketsAuditAdapter's role.
type ticketReaderAdapter struct {
	svc *tickets.Service
}

func (a ticketReaderAdapter) ReadContext(ctx context.Context, ticketID string) (drafts.TicketContext, []drafts.InternalNote, error) {
	t, err := a.svc.FindByID(ctx, ticketID)
	if err != nil {
		var nf *tickets.NotFoundError
		if errors.As(err, &nf) {
			return drafts.TicketContext{}, nil, &drafts.NotFoundError{ID: ticketID}
		}
		return drafts.TicketContext{}, nil, err
	}

	notes := []drafts.InternalNote{}
	for _, c := range t.Comments {
		if c.Internal {
			notes = append(notes, drafts.InternalNote{Author: c.Author, Body: c.Body, At: c.At})
		}
	}

	return drafts.TicketContext{
		Subject:     t.Subject,
		Description: t.Description,
		Status:      string(t.Status),
		Priority:    string(t.Priority),
	}, notes, nil
}

// ticketCommenterAdapter satisfies drafts.TicketCommenter by delegating to
// tickets.Service.AddComment — a sent draft becomes a real, customer-visible
// (Internal: false) Comment through the exact same path a direct
// POST /tickets/{id}/comments call uses.
type ticketCommenterAdapter struct {
	svc *tickets.Service
}

func (a ticketCommenterAdapter) AddReply(ctx context.Context, actor, ticketID, author, body string) error {
	_, err := a.svc.AddComment(ctx, actor, ticketID, tickets.CommentInput{Author: author, Body: body, Internal: false})
	return err
}

// draftsAuditAdapter satisfies drafts.AuditRecorder the same way
// ticketsAuditAdapter satisfies tickets.AuditRecorder: delegating to
// audit.Service so drafts doesn't need to import it directly.
type draftsAuditAdapter struct {
	svc *audit.Service
}

func (a draftsAuditAdapter) Record(ctx context.Context, actor, action, ticketID string, details map[string]any) error {
	return a.svc.Record(ctx, actor, action, ticketID, details)
}

// httpReplyGuardNotifier satisfies drafts.ReplyGuardNotifier by POSTing to
// the separate reply-guard service, mirroring httpTriageNotifier.
type httpReplyGuardNotifier struct {
	baseURL string
	client  *http.Client
}

func (n httpReplyGuardNotifier) NotifyDraftCreated(ctx context.Context, req drafts.GuardRequest) error {
	notes := make([]map[string]string, len(req.InternalNotes))
	for i, note := range req.InternalNotes {
		notes[i] = map[string]string{"author": note.Author, "body": note.Body}
	}

	body, err := json.Marshal(map[string]any{
		"ticketId":          req.TicketID,
		"draftId":           req.DraftID,
		"ticketSubject":     req.Ticket.Subject,
		"ticketDescription": req.Ticket.Description,
		"ticketStatus":      req.Ticket.Status,
		"ticketPriority":    req.Ticket.Priority,
		"internalNotes":     notes,
		"draftBody":         req.DraftBody,
	})
	if err != nil {
		return err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, n.baseURL+"/guard", bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(httpReq)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("reply-guard service returned status %d for draft %s (ticket %s)", resp.StatusCode, req.DraftID, req.TicketID)
	}
	return nil
}
