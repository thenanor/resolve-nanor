package audit

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"resolve/internal/httpx"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Mount(r chi.Router) {
	r.Get("/audit", h.list)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	entries, err := h.service.List(r.Context(), r.URL.Query().Get("ticketId"))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, entries)
}
