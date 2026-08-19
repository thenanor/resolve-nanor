package replyguard

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func newTestHandler(classifier Classifier) *Handler {
	return NewHandler(NewService(classifier))
}

func TestHandler_Create_ValidBodyReturnsGuardResult(t *testing.T) {
	h := newTestHandler(fakeClassifier{result: Result{Confidence: 0.9}})
	r := chi.NewRouter()
	h.Mount(r)

	body := `{"ticketSubject":"s","ticketDescription":"d","ticketStatus":"open","ticketPriority":"high","internalNotes":[{"author":"bob","body":"secret"}],"body":"hello"}`
	req := httptest.NewRequest(http.MethodPost, "/guard", strings.NewReader(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got GuardResult
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Verdict != VerdictSend {
		t.Errorf("Verdict = %q, want %q", got.Verdict, VerdictSend)
	}
}

func TestHandler_Create_InvalidJSONBodyReturns400(t *testing.T) {
	h := newTestHandler(fakeClassifier{})
	r := chi.NewRouter()
	h.Mount(r)

	req := httptest.NewRequest(http.MethodPost, "/guard", strings.NewReader(`{not valid json`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandler_Create_ClassifierErrorReturns500(t *testing.T) {
	h := newTestHandler(fakeClassifier{err: errClassifyFailed})
	r := chi.NewRouter()
	h.Mount(r)

	body := `{"body":"hello"}`
	req := httptest.NewRequest(http.MethodPost, "/guard", strings.NewReader(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

var errClassifyFailed = context.DeadlineExceeded
