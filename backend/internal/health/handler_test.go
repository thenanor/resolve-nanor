package health

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestHealth_ReturnsServiceVersionAndUptime(t *testing.T) {
	r := chi.NewRouter()
	NewHandler("1.2.3").Mount(r)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body response
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Service != "resolve" {
		t.Errorf("service = %q, want %q", body.Service, "resolve")
	}
	if body.Version != "1.2.3" {
		t.Errorf("version = %q, want %q", body.Version, "1.2.3")
	}
	if body.UptimeSeconds < 0 {
		t.Errorf("uptimeSeconds = %v, want >= 0", body.UptimeSeconds)
	}
}
