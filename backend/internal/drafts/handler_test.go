package drafts

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func newTestHandler() (*Handler, *fakeTicketReader, *channelReplyGuardNotifier) {
	svc, _, reader, _, _, guard := newTestService()
	return NewHandler(svc), reader, guard
}

func TestHandler_CreateDraft_ValidBodyReturns201(t *testing.T) {
	h, reader, guard := newTestHandler()
	guard.block = make(chan struct{})
	reader.seed("tkt_1", TicketContext{}, nil)
	r := chi.NewRouter()
	h.Mount(r)

	req := httptest.NewRequest(http.MethodPost, "/tickets/tkt_1/drafts", strings.NewReader(`{"author":"Alice","body":"Hello"}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var d Draft
	if err := json.NewDecoder(rec.Body).Decode(&d); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if d.Status != StatusPendingReview {
		t.Errorf("Status = %q, want %q", d.Status, StatusPendingReview)
	}
	close(guard.block)
}

func TestHandler_CreateDraft_InvalidJSONReturns400(t *testing.T) {
	h, _, _ := newTestHandler()
	r := chi.NewRouter()
	h.Mount(r)

	req := httptest.NewRequest(http.MethodPost, "/tickets/tkt_1/drafts", strings.NewReader(`{not valid json`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandler_CreateDraft_UnknownTicketReturns404(t *testing.T) {
	h, _, _ := newTestHandler()
	r := chi.NewRouter()
	h.Mount(r)

	req := httptest.NewRequest(http.MethodPost, "/tickets/tkt_missing/drafts", strings.NewReader(`{"author":"Alice","body":"Hello"}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandler_FindOne_UnknownDraftReturns404(t *testing.T) {
	h, _, _ := newTestHandler()
	r := chi.NewRouter()
	h.Mount(r)

	req := httptest.NewRequest(http.MethodGet, "/tickets/tkt_1/drafts/dft_missing", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandler_Send_ConflictOnPendingReviewReturns409(t *testing.T) {
	h, reader, guard := newTestHandler()
	guard.block = make(chan struct{})
	reader.seed("tkt_1", TicketContext{}, nil)
	r := chi.NewRouter()
	h.Mount(r)

	createReq := httptest.NewRequest(http.MethodPost, "/tickets/tkt_1/drafts", strings.NewReader(`{"author":"Alice","body":"Hello"}`))
	createRec := httptest.NewRecorder()
	r.ServeHTTP(createRec, createReq)
	var d Draft
	if err := json.NewDecoder(createRec.Body).Decode(&d); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	sendReq := httptest.NewRequest(http.MethodPost, "/tickets/tkt_1/drafts/"+d.ID+"/send", strings.NewReader(`{}`))
	sendRec := httptest.NewRecorder()
	r.ServeHTTP(sendRec, sendReq)

	if sendRec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body: %s", sendRec.Code, http.StatusConflict, sendRec.Body.String())
	}
	close(guard.block)
}

func TestHandler_RecordGuardResult_ValidBodyReturns200(t *testing.T) {
	h, reader, guard := newTestHandler()
	guard.block = make(chan struct{})
	reader.seed("tkt_1", TicketContext{}, nil)
	r := chi.NewRouter()
	h.Mount(r)

	createReq := httptest.NewRequest(http.MethodPost, "/tickets/tkt_1/drafts", strings.NewReader(`{"author":"Alice","body":"Hello"}`))
	createRec := httptest.NewRecorder()
	r.ServeHTTP(createRec, createReq)
	var d Draft
	if err := json.NewDecoder(createRec.Body).Decode(&d); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	close(guard.block)

	guardBody := `{"verdict":"send","findings":[],"confidence":"high","reasoning":"clean","injectionSuspected":false,"requireHuman":false}`
	guardReq := httptest.NewRequest(http.MethodPost, "/tickets/tkt_1/drafts/"+d.ID+"/guard-result", strings.NewReader(guardBody))
	guardRec := httptest.NewRecorder()
	r.ServeHTTP(guardRec, guardReq)

	if guardRec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", guardRec.Code, http.StatusOK, guardRec.Body.String())
	}
	var updated Draft
	if err := json.NewDecoder(guardRec.Body).Decode(&updated); err != nil {
		t.Fatalf("decode guard-result response: %v", err)
	}
	if updated.Status != StatusGuarded || updated.GuardResult == nil || updated.GuardResult.Verdict != VerdictSend {
		t.Errorf("updated = %+v, want guarded with a send verdict", updated)
	}
}
