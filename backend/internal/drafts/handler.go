package drafts

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"resolve/internal/httpx"
	"resolve/internal/tickets"
)

// Handler exposes the /tickets/{id}/drafts* surface. It mounts alongside
// tickets.Handler under the same "/tickets" prefix rather than being folded
// into it, keeping drafts' own package boundary (see package doc).
type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Mount(r chi.Router) {
	r.Route("/tickets/{id}/drafts", func(r chi.Router) {
		r.Post("/", h.create)
		r.Get("/{draftId}", h.findOne)
		r.Put("/{draftId}", h.update)
		r.Post("/{draftId}/send", h.send)
		r.Post("/{draftId}/guard-result", h.recordGuardResult)
	})
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Author string `json:"author"`
		Body   string `json:"body"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	d, err := h.service.CreateDraft(r.Context(), httpx.Actor(r), chi.URLParam(r, "id"), body.Author, body.Body)
	if respondIfError(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, d)
}

func (h *Handler) findOne(w http.ResponseWriter, r *http.Request) {
	d, err := h.service.FindByID(r.Context(), chi.URLParam(r, "id"), chi.URLParam(r, "draftId"))
	if respondIfError(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, d)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Body string `json:"body"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	d, err := h.service.UpdateBody(r.Context(), httpx.Actor(r), chi.URLParam(r, "id"), chi.URLParam(r, "draftId"), body.Body)
	if respondIfError(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, d)
}

func (h *Handler) send(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OverrideReason string `json:"overrideReason"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	d, err := h.service.Send(r.Context(), httpx.Actor(r), chi.URLParam(r, "id"), chi.URLParam(r, "draftId"), body.OverrideReason)
	if respondIfError(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, d)
}

// recordGuardResult is called by reply-guard's HTTPDraftUpdater, not end
// users — not part of the public OpenAPI surface's intended caller, though
// it is documented (like /tickets/{id}/triage) since it's reached through
// the same public port.
func (h *Handler) recordGuardResult(w http.ResponseWriter, r *http.Request) {
	var body struct {
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
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	findings := make([]Finding, len(body.Findings))
	for i, f := range body.Findings {
		findings[i] = Finding{
			Policy:   Policy(f.Policy),
			Severity: Severity(f.Severity),
			Issue:    f.Issue,
			Quote:    f.Quote,
		}
	}

	d, err := h.service.RecordGuardResult(r.Context(), httpx.Actor(r), chi.URLParam(r, "id"), chi.URLParam(r, "draftId"), GuardResult{
		Verdict:            Verdict(body.Verdict),
		Findings:           findings,
		Confidence:         tickets.Confidence(body.Confidence),
		Reasoning:          body.Reasoning,
		InjectionSuspected: body.InjectionSuspected,
		RequireHuman:       body.RequireHuman,
	})
	if respondIfError(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, d)
}

// respondIfError writes the appropriate error response and returns true if
// err was non-nil, mirroring tickets/handler.go's respondIfError with an
// added ConflictError branch for drafts' lifecycle-state errors.
func respondIfError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	var ve *ValidationError
	var nf *NotFoundError
	var ce *ConflictError
	switch {
	case errors.As(err, &ve):
		httpx.WriteError(w, http.StatusBadRequest, ve.Message)
	case errors.As(err, &nf):
		httpx.WriteError(w, http.StatusNotFound, nf.Error())
	case errors.As(err, &ce):
		httpx.WriteError(w, http.StatusConflict, ce.Message)
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "internal server error")
	}
	return true
}
