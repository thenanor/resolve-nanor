package replyguard

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func newTestHandler(classifier Classifier, updater DraftUpdater) *Handler {
	return NewHandler(NewService(classifier, updater))
}

func TestHandler_Create_ValidBodyTriggersGuardAndWritesBack(t *testing.T) {
	updater := &fakeDraftUpdater{}
	h := newTestHandler(fakeClassifier{result: Result{Confidence: 0.9}}, updater)
	r := chi.NewRouter()
	h.Mount(r)

	body := `{"ticketId":"tkt_1","draftId":"dft_1","ticketSubject":"s","ticketDescription":"d","ticketStatus":"open","ticketPriority":"high","internalNotes":[{"author":"bob","body":"secret"}],"draftBody":"hello"}`
	req := httptest.NewRequest(http.MethodPost, "/guard", strings.NewReader(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if len(updater.calls) != 1 || updater.calls[0].ticketID != "tkt_1" || updater.calls[0].draftID != "dft_1" {
		t.Errorf("updater calls = %+v, want one call for tkt_1/dft_1", updater.calls)
	}
}

func TestHandler_Create_InvalidJSONBodyReturns400(t *testing.T) {
	h := newTestHandler(fakeClassifier{}, &fakeDraftUpdater{})
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
	h := newTestHandler(fakeClassifier{err: errClassifyFailed}, &fakeDraftUpdater{})
	r := chi.NewRouter()
	h.Mount(r)

	body := `{"ticketId":"tkt_1","draftId":"dft_1"}`
	req := httptest.NewRequest(http.MethodPost, "/guard", strings.NewReader(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

var errClassifyFailed = context.DeadlineExceeded
