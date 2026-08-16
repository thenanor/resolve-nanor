package tickets

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func newTestHandler(t *testing.T, n int) *Handler {
	t.Helper()
	repo := newFakeRepository()
	svc := NewService(repo, &fakeAudit{})
	seedTickets(t, svc, repo, n, nil)
	return NewHandler(svc)
}

func getTickets(t *testing.T, h *Handler, query string) (*httptest.ResponseRecorder, Page) {
	t.Helper()
	r := chi.NewRouter()
	h.Mount(r)

	req := httptest.NewRequest(http.MethodGet, "/tickets"+query, nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	var page Page
	if rec.Code == http.StatusOK {
		if err := json.NewDecoder(rec.Body).Decode(&page); err != nil {
			t.Fatalf("decode response: %v", err)
		}
	}
	return rec, page
}

func TestFindAll_HTTP_DefaultsPaginationWhenOmitted(t *testing.T) {
	h := newTestHandler(t, 3)

	rec, page := getTickets(t, h, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if page.Limit != DefaultPageLimit {
		t.Errorf("limit = %d, want %d", page.Limit, DefaultPageLimit)
	}
	if page.Offset != 0 {
		t.Errorf("offset = %d, want 0", page.Offset)
	}
	if len(page.Tickets) != 3 {
		t.Errorf("len(tickets) = %d, want 3", len(page.Tickets))
	}
}

func TestFindAll_HTTP_RespectsLimitAndOffset(t *testing.T) {
	h := newTestHandler(t, 5)

	rec, page := getTickets(t, h, "?limit=2&offset=1")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if page.Limit != 2 || page.Offset != 1 {
		t.Errorf("limit=%d offset=%d, want limit=2 offset=1", page.Limit, page.Offset)
	}
	if len(page.Tickets) != 2 {
		t.Errorf("len(tickets) = %d, want 2", len(page.Tickets))
	}
	if !page.HasMore {
		t.Error("hasMore = false, want true")
	}
}

func TestFindAll_HTTP_ClampsLimitAboveMax(t *testing.T) {
	h := newTestHandler(t, 2)

	rec, page := getTickets(t, h, "?limit=99999")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if page.Limit != MaxPageLimit {
		t.Errorf("limit = %d, want %d", page.Limit, MaxPageLimit)
	}
}

func TestFindAll_HTTP_RejectsNonIntegerLimit(t *testing.T) {
	h := newTestHandler(t, 1)

	rec, _ := getTickets(t, h, "?limit=not-a-number")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestFindAll_HTTP_RejectsNegativeOffset(t *testing.T) {
	h := newTestHandler(t, 1)

	rec, _ := getTickets(t, h, "?offset=-5")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestListAudit_HTTP_ReturnsEntriesNewestFirst(t *testing.T) {
	repo := newFakeRepository()
	audit := &fakeAudit{}
	svc := NewService(repo, audit)
	h := NewHandler(svc)
	ctx := context.Background()

	ticket, err := svc.Create(ctx, "nanor", validInput)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ChangeStatus(ctx, "nanor", ticket.ID, "open"); err != nil {
		t.Fatal(err)
	}

	r := chi.NewRouter()
	h.Mount(r)
	req := httptest.NewRequest(http.MethodGet, "/tickets/"+ticket.ID+"/audit", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var page AuditPage
	if err := json.NewDecoder(rec.Body).Decode(&page); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(page.Entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(page.Entries))
	}
	if page.Entries[0].Action != "ticket.status_changed" {
		t.Errorf("entries[0].Action = %q, want ticket.status_changed", page.Entries[0].Action)
	}
	if page.Entries[1].Action != "ticket.created" {
		t.Errorf("entries[1].Action = %q, want ticket.created", page.Entries[1].Action)
	}
	if page.Limit != DefaultPageLimit {
		t.Errorf("limit = %d, want %d", page.Limit, DefaultPageLimit)
	}
}

func TestListAudit_HTTP_RespectsLimitAndOffset(t *testing.T) {
	repo := newFakeRepository()
	audit := &fakeAudit{}
	svc := NewService(repo, audit)
	h := NewHandler(svc)
	ctx := context.Background()

	ticket, err := svc.Create(ctx, "nanor", validInput)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ChangeStatus(ctx, "nanor", ticket.ID, "open"); err != nil {
		t.Fatal(err)
	}

	r := chi.NewRouter()
	h.Mount(r)
	req := httptest.NewRequest(http.MethodGet, "/tickets/"+ticket.ID+"/audit?limit=1", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var page AuditPage
	if err := json.NewDecoder(rec.Body).Decode(&page); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(page.Entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(page.Entries))
	}
	if page.Entries[0].Action != "ticket.status_changed" {
		t.Errorf("entries[0].Action = %q, want ticket.status_changed", page.Entries[0].Action)
	}
	if !page.HasMore {
		t.Error("hasMore = false, want true")
	}
}

func TestListAudit_HTTP_RejectsNonIntegerLimit(t *testing.T) {
	h := newTestHandler(t, 0)

	r := chi.NewRouter()
	h.Mount(r)
	req := httptest.NewRequest(http.MethodGet, "/tickets/tkt_missing/audit?limit=not-a-number", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestListAudit_HTTP_404sOnUnknownTicket(t *testing.T) {
	h := newTestHandler(t, 0)

	r := chi.NewRouter()
	h.Mount(r)
	req := httptest.NewRequest(http.MethodGet, "/tickets/tkt_missing/audit", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestApplyTriage_HTTP_HighConfidenceReturns200WithAppliedFields(t *testing.T) {
	h := newTestHandler(t, 1)
	_ = h // reassigned below once the seeded ticket's repo/service are in scope
	repo := newFakeRepository()
	svc := NewService(repo, &fakeAudit{})
	tks := seedTickets(t, svc, repo, 1, nil)
	h = NewHandler(svc)

	r := chi.NewRouter()
	h.Mount(r)
	body := `{"category":"billing","priority":"urgent","confidence":"high"}`
	req := httptest.NewRequest(http.MethodPost, "/tickets/"+tks[0].ID+"/triage", strings.NewReader(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got Ticket
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Category == nil || *got.Category != CategoryBilling {
		t.Errorf("Category = %v, want %q", got.Category, CategoryBilling)
	}
	if got.Priority != PriorityUrgent {
		t.Errorf("Priority = %q, want %q", got.Priority, PriorityUrgent)
	}
}

func TestApplyTriage_HTTP_InvalidCategoryReturns400(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo, &fakeAudit{})
	tks := seedTickets(t, svc, repo, 1, nil)
	h := NewHandler(svc)

	r := chi.NewRouter()
	h.Mount(r)
	body := `{"category":"not_a_category","priority":"urgent","confidence":"high"}`
	req := httptest.NewRequest(http.MethodPost, "/tickets/"+tks[0].ID+"/triage", strings.NewReader(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestApplyTriage_HTTP_404sOnUnknownTicket(t *testing.T) {
	h := newTestHandler(t, 0)

	r := chi.NewRouter()
	h.Mount(r)
	body := `{"category":"billing","priority":"urgent","confidence":"high"}`
	req := httptest.NewRequest(http.MethodPost, "/tickets/tkt_missing/triage", strings.NewReader(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestReviewTriage_HTTP_AcceptReturns200WithPromotedFields(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo, &fakeAudit{})
	tks := seedTickets(t, svc, repo, 1, nil)
	if _, err := svc.ApplyTriage(context.Background(), "triage-service", tks[0].ID, "how_to", "urgent", "low"); err != nil {
		t.Fatalf("ApplyTriage returned error: %v", err)
	}
	h := NewHandler(svc)

	r := chi.NewRouter()
	h.Mount(r)
	req := httptest.NewRequest(http.MethodPost, "/tickets/"+tks[0].ID+"/triage/review", strings.NewReader(`{"decision":"accept"}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got Ticket
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Category == nil || *got.Category != CategoryHowTo {
		t.Errorf("Category = %v, want %q", got.Category, CategoryHowTo)
	}
	if got.PendingCategory != nil {
		t.Errorf("PendingCategory = %v, want nil", got.PendingCategory)
	}
}

func TestReviewTriage_HTTP_InvalidDecisionReturns400(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo, &fakeAudit{})
	tks := seedTickets(t, svc, repo, 1, nil)
	h := NewHandler(svc)

	r := chi.NewRouter()
	h.Mount(r)
	req := httptest.NewRequest(http.MethodPost, "/tickets/"+tks[0].ID+"/triage/review", strings.NewReader(`{"decision":"maybe"}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestReviewTriage_HTTP_NoPendingSuggestionReturns400(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo, &fakeAudit{})
	tks := seedTickets(t, svc, repo, 1, nil)
	h := NewHandler(svc)

	r := chi.NewRouter()
	h.Mount(r)
	req := httptest.NewRequest(http.MethodPost, "/tickets/"+tks[0].ID+"/triage/review", strings.NewReader(`{"decision":"accept"}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
