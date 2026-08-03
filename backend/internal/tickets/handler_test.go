package tickets

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
