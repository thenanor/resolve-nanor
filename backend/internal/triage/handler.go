package triage

import (
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"

	"resolve/internal/httpx"
)

// Handler exposes POST /triage. It is an internal, service-to-service
// endpoint (called only by the main app's fire-and-forget notification) and
// is not part of the public API surface, so it has no OpenAPI entry and no
// typed-error response mapping.
type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Mount(r chi.Router) {
	r.Post("/triage", h.create)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TicketID    string `json:"ticketId"`
		Subject     string `json:"subject"`
		Description string `json:"description"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if err := h.service.Triage(r.Context(), body.TicketID, body.Subject, body.Description); err != nil {
		log.Printf("triage failed for ticket %s: %v", body.TicketID, err)
		httpx.WriteError(w, http.StatusInternalServerError, "triage failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
