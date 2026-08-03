package health

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"resolve/internal/httpx"
)

const serviceName = "resolve"

type Handler struct {
	version   string
	startedAt time.Time
}

func NewHandler(version string) *Handler {
	return &Handler{version: version, startedAt: time.Now()}
}

func (h *Handler) Mount(r chi.Router) {
	r.Get("/health", h.health)
}

type response struct {
	Service       string  `json:"service"`
	Version       string  `json:"version"`
	UptimeSeconds float64 `json:"uptimeSeconds"`
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, response{
		Service:       serviceName,
		Version:       h.version,
		UptimeSeconds: time.Since(h.startedAt).Seconds(),
	})
}
