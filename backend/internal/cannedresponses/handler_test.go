package cannedresponses

// These tests specify (but do not implement) the canned-responses HTTP API
// described in .claude/specs/canned-responses.md, AC-1 through AC-18. They
// reference the not-yet-written CannedResponse, Filter, Repository,
// Service, and Handler types/constructors so the suite doubles as the
// contract an implementation must satisfy. Running this package is
// expected to fail to build until that implementation exists.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// fakeRepository is an in-memory Repository used to exercise the HTTP
// handler without a database, mirroring tickets/handler_test.go's approach.
// It is test infrastructure only, not the production implementation.
type fakeRepository struct {
	byID map[string]*CannedResponse
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{byID: map[string]*CannedResponse{}}
}

func (f *fakeRepository) Create(_ context.Context, cr *CannedResponse) error {
	cp := *cr
	cp.Tags = append([]string{}, cr.Tags...)
	f.byID[cr.ID] = &cp
	return nil
}

func (f *fakeRepository) FindAll(_ context.Context, filter Filter) ([]CannedResponse, error) {
	var result []CannedResponse
	tag := strings.ToLower(strings.TrimSpace(filter.Tag))
	for _, cr := range f.byID {
		if tag != "" && !hasTag(cr.Tags, tag) {
			continue
		}
		cp := *cr
		cp.Tags = append([]string{}, cr.Tags...)
		result = append(result, cp)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Title < result[j].Title })
	return result, nil
}

func hasTag(tags []string, tag string) bool {
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}

func (f *fakeRepository) FindByID(_ context.Context, id string) (*CannedResponse, error) {
	cr, ok := f.byID[id]
	if !ok {
		return nil, nil
	}
	cp := *cr
	cp.Tags = append([]string{}, cr.Tags...)
	return &cp, nil
}

func (f *fakeRepository) Update(_ context.Context, cr *CannedResponse) error {
	if _, ok := f.byID[cr.ID]; !ok {
		return errors.New("not found")
	}
	cp := *cr
	cp.Tags = append([]string{}, cr.Tags...)
	f.byID[cr.ID] = &cp
	return nil
}

func (f *fakeRepository) Delete(_ context.Context, id string) error {
	if _, ok := f.byID[id]; !ok {
		return errors.New("not found")
	}
	delete(f.byID, id)
	return nil
}

func newTestHandler() *Handler {
	return NewHandler(NewService(newFakeRepository()))
}

func doRequest(t *testing.T, h *Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := chi.NewRouter()
	h.Mount(r)

	var reqBody io.Reader
	if body != "" {
		reqBody = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reqBody)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func createCannedResponse(t *testing.T, h *Handler, body string) CannedResponse {
	t.Helper()
	rec := doRequest(t, h, http.MethodPost, "/canned-responses", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("seed create status = %d, want %d, body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var cr CannedResponse
	if err := json.NewDecoder(rec.Body).Decode(&cr); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return cr
}

var idPattern = regexp.MustCompile(`^cr_[0-9a-f]{8}$`)

// --- Create (AC-1..AC-7) ---

func TestAC1_Create_Returns201WithCreatedResource(t *testing.T) {
	h := newTestHandler()
	rec := doRequest(t, h, http.MethodPost, "/canned-responses",
		`{"title":"Password reset","body":"Reset your password via the link.","tags":["billing","account"]}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var cr CannedResponse
	if err := json.NewDecoder(rec.Body).Decode(&cr); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if cr.ID == "" {
		t.Error("id is empty, want set")
	}
	if cr.Title != "Password reset" {
		t.Errorf("title = %q, want %q", cr.Title, "Password reset")
	}
	if cr.Body != "Reset your password via the link." {
		t.Errorf("body = %q, want %q", cr.Body, "Reset your password via the link.")
	}
	if len(cr.Tags) != 2 {
		t.Errorf("len(tags) = %d, want 2", len(cr.Tags))
	}
	if cr.CreatedAt == "" {
		t.Error("createdAt is empty, want set")
	}
	if cr.UpdatedAt == "" {
		t.Error("updatedAt is empty, want set")
	}
}

func TestAC2_Create_RejectsEmptyTitleNamingField(t *testing.T) {
	h := newTestHandler()
	rec := doRequest(t, h, http.MethodPost, "/canned-responses", `{"title":"   ","body":"some body"}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "title") {
		t.Errorf("body %q does not mention field %q", rec.Body.String(), "title")
	}
}

func TestAC3_Create_RejectsEmptyBodyNamingField(t *testing.T) {
	h := newTestHandler()
	rec := doRequest(t, h, http.MethodPost, "/canned-responses", `{"title":"Password reset","body":"   "}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "body") {
		t.Errorf("body %q does not mention field %q", rec.Body.String(), "body")
	}
}

func TestAC4_Create_DefaultsMissingTagsToEmptyArray(t *testing.T) {
	h := newTestHandler()
	rec := doRequest(t, h, http.MethodPost, "/canned-responses", `{"title":"Welcome","body":"Welcome to Resolve support!"}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var cr CannedResponse
	if err := json.NewDecoder(rec.Body).Decode(&cr); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(cr.Tags) != 0 {
		t.Errorf("len(tags) = %d, want 0", len(cr.Tags))
	}
}

func TestAC5_Create_NormalizesTagsTrimmedAndLowercased(t *testing.T) {
	h := newTestHandler()
	first := createCannedResponse(t, h, `{"title":"A","body":"body a","tags":["Billing"]}`)
	second := createCannedResponse(t, h, `{"title":"B","body":"body b","tags":[" billing "]}`)

	if len(first.Tags) != 1 || first.Tags[0] != "billing" {
		t.Errorf("first.Tags = %v, want [billing]", first.Tags)
	}
	if len(second.Tags) != 1 || second.Tags[0] != "billing" {
		t.Errorf("second.Tags = %v, want [billing]", second.Tags)
	}
}

func TestAC6_Create_RejectsEmptyTagEntry(t *testing.T) {
	h := newTestHandler()
	rec := doRequest(t, h, http.MethodPost, "/canned-responses", `{"title":"Welcome","body":"Welcome!","tags":["billing",""]}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestAC7_Create_GeneratesIDMatchingConvention(t *testing.T) {
	h := newTestHandler()
	cr := createCannedResponse(t, h, `{"title":"Welcome","body":"Welcome!"}`)

	if !idPattern.MatchString(cr.ID) {
		t.Errorf("id = %q, want to match %s", cr.ID, idPattern.String())
	}
}

// --- Read (AC-8..AC-12) ---

func TestAC8_List_OrdersByTitleAscending(t *testing.T) {
	h := newTestHandler()
	createCannedResponse(t, h, `{"title":"Welcome","body":"b"}`)
	createCannedResponse(t, h, `{"title":"Account closed","body":"b"}`)
	createCannedResponse(t, h, `{"title":"Refund processed","body":"b"}`)

	rec := doRequest(t, h, http.MethodGet, "/canned-responses", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var list []CannedResponse
	if err := json.NewDecoder(rec.Body).Decode(&list); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("len(list) = %d, want 3", len(list))
	}
	wantOrder := []string{"Account closed", "Refund processed", "Welcome"}
	for i, want := range wantOrder {
		if list[i].Title != want {
			t.Errorf("list[%d].Title = %q, want %q", i, list[i].Title, want)
		}
	}
}

func TestAC9_List_FiltersByTagCaseInsensitively(t *testing.T) {
	h := newTestHandler()
	createCannedResponse(t, h, `{"title":"Password reset","body":"b","tags":["billing","account"]}`)
	createCannedResponse(t, h, `{"title":"Welcome","body":"b","tags":["onboarding"]}`)

	rec := doRequest(t, h, http.MethodGet, "/canned-responses?tag=BILLING", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var list []CannedResponse
	if err := json.NewDecoder(rec.Body).Decode(&list); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("len(list) = %d, want 1", len(list))
	}
	if list[0].Title != "Password reset" {
		t.Errorf("list[0].Title = %q, want %q", list[0].Title, "Password reset")
	}
}

func TestAC10_List_EmptyTagParamReturnsFullList(t *testing.T) {
	h := newTestHandler()
	createCannedResponse(t, h, `{"title":"Password reset","body":"b","tags":["billing"]}`)
	createCannedResponse(t, h, `{"title":"Welcome","body":"b"}`)

	rec := doRequest(t, h, http.MethodGet, "/canned-responses?tag=", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var list []CannedResponse
	if err := json.NewDecoder(rec.Body).Decode(&list); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("len(list) = %d, want 2", len(list))
	}
}

func TestAC11_FindByID_Returns200WithResource(t *testing.T) {
	h := newTestHandler()
	created := createCannedResponse(t, h, `{"title":"Welcome","body":"Welcome!"}`)

	rec := doRequest(t, h, http.MethodGet, "/canned-responses/"+created.ID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var cr CannedResponse
	if err := json.NewDecoder(rec.Body).Decode(&cr); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if cr.ID != created.ID {
		t.Errorf("id = %q, want %q", cr.ID, created.ID)
	}
}

func TestAC12_FindByID_404sOnUnknownID(t *testing.T) {
	h := newTestHandler()
	rec := doRequest(t, h, http.MethodGet, "/canned-responses/cr_missing", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// --- Update (AC-13..AC-16) ---

func TestAC13_Update_Returns200WithUpdatedResourceAndChangedUpdatedAt(t *testing.T) {
	h := newTestHandler()
	created := createCannedResponse(t, h, `{"title":"Welcome","body":"Welcome!"}`)

	rec := doRequest(t, h, http.MethodPut, "/canned-responses/"+created.ID,
		`{"title":"Welcome (v2)","body":"Welcome to Resolve!","tags":["onboarding"]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var updated CannedResponse
	if err := json.NewDecoder(rec.Body).Decode(&updated); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if updated.Title != "Welcome (v2)" {
		t.Errorf("title = %q, want %q", updated.Title, "Welcome (v2)")
	}
	if updated.UpdatedAt == created.UpdatedAt {
		t.Error("updatedAt did not change")
	}
}

func TestAC14_Update_404sOnUnknownID(t *testing.T) {
	h := newTestHandler()
	rec := doRequest(t, h, http.MethodPut, "/canned-responses/cr_missing", `{"title":"x","body":"y"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestAC15_Update_RejectsEmptyTitleOrBody(t *testing.T) {
	h := newTestHandler()
	created := createCannedResponse(t, h, `{"title":"Welcome","body":"Welcome!"}`)

	rec := doRequest(t, h, http.MethodPut, "/canned-responses/"+created.ID, `{"title":"  ","body":"Welcome!"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestAC16_Update_RenormalizesTags(t *testing.T) {
	h := newTestHandler()
	created := createCannedResponse(t, h, `{"title":"Welcome","body":"Welcome!"}`)

	rec := doRequest(t, h, http.MethodPut, "/canned-responses/"+created.ID,
		`{"title":"Welcome","body":"Welcome!","tags":[" Onboarding "]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var updated CannedResponse
	if err := json.NewDecoder(rec.Body).Decode(&updated); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(updated.Tags) != 1 || updated.Tags[0] != "onboarding" {
		t.Errorf("tags = %v, want [onboarding]", updated.Tags)
	}
}

// --- Delete (AC-17..AC-18; AC-19 lives in ac19_integration_test.go) ---

func TestAC17_Delete_Returns204AndResourceNoLongerListed(t *testing.T) {
	h := newTestHandler()
	created := createCannedResponse(t, h, `{"title":"Welcome","body":"Welcome!"}`)

	rec := doRequest(t, h, http.MethodDelete, "/canned-responses/"+created.ID, "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	getRec := doRequest(t, h, http.MethodGet, "/canned-responses/"+created.ID, "")
	if getRec.Code != http.StatusNotFound {
		t.Errorf("GET after delete status = %d, want %d", getRec.Code, http.StatusNotFound)
	}

	listRec := doRequest(t, h, http.MethodGet, "/canned-responses", "")
	var list []CannedResponse
	if err := json.NewDecoder(listRec.Body).Decode(&list); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("len(list) = %d, want 0", len(list))
	}
}

func TestAC18_Delete_404sOnUnknownID(t *testing.T) {
	h := newTestHandler()
	rec := doRequest(t, h, http.MethodDelete, "/canned-responses/cr_missing", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
