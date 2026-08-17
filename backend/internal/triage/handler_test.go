package triage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func newTestHandler(classifier Classifier, updater TicketUpdater) *Handler {
	return NewHandler(NewService(classifier, updater))
}

func TestHandler_Create_ValidBodyTriggersTriage(t *testing.T) {
	updater := &fakeTicketUpdater{}
	h := newTestHandler(fakeClassifier{result: Result{Category: "billing", Priority: "high", Confidence: 0.9}}, updater)
	r := chi.NewRouter()
	h.Mount(r)

	body := `{"ticketId":"tkt_1","subject":"Charged twice","description":"My card was charged twice this month"}`
	req := httptest.NewRequest(http.MethodPost, "/triage", strings.NewReader(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if len(updater.calls) != 1 || updater.calls[0].ticketID != "tkt_1" {
		t.Errorf("updater calls = %+v, want one call for tkt_1", updater.calls)
	}
}

func TestHandler_Create_InvalidJSONBodyReturns400(t *testing.T) {
	h := newTestHandler(fakeClassifier{}, &fakeTicketUpdater{})
	r := chi.NewRouter()
	h.Mount(r)

	req := httptest.NewRequest(http.MethodPost, "/triage", strings.NewReader(`{not valid json`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandler_Create_ClassifierErrorReturns500(t *testing.T) {
	h := newTestHandler(fakeClassifier{err: errClassifyFailed}, &fakeTicketUpdater{})
	r := chi.NewRouter()
	h.Mount(r)

	body := `{"ticketId":"tkt_1","subject":"s","description":"d"}`
	req := httptest.NewRequest(http.MethodPost, "/triage", strings.NewReader(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

var errClassifyFailed = context.DeadlineExceeded
