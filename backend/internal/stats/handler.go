package stats

import (
	"context"
	"math"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"resolve/internal/httpx"
	"resolve/internal/tickets"
)

type ticketLister interface {
	FindAll(ctx context.Context, filter tickets.Filter) ([]tickets.Ticket, error)
}

type Handler struct {
	tickets ticketLister
}

func NewHandler(t ticketLister) *Handler {
	return &Handler{tickets: t}
}

func (h *Handler) Mount(r chi.Router) {
	r.Get("/stats", h.stats)
}

type response struct {
	Total                int            `json:"total"`
	ByStatus             map[string]int `json:"byStatus"`
	ByPriority           map[string]int `json:"byPriority"`
	AvgResolutionMinutes *int           `json:"avgResolutionMinutes"`
}

func (h *Handler) stats(w http.ResponseWriter, r *http.Request) {
	all, err := h.tickets.FindAll(r.Context(), tickets.Filter{})
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	byStatus := map[string]int{}
	byPriority := map[string]int{}
	var resolvedMinutes []float64

	for _, t := range all {
		byStatus[string(t.Status)]++
		byPriority[string(t.Priority)]++
		if t.ResolvedAt != nil {
			resolvedAt, errR := time.Parse(time.RFC3339Nano, *t.ResolvedAt)
			createdAt, errC := time.Parse(time.RFC3339Nano, t.CreatedAt)
			if errR == nil && errC == nil {
				resolvedMinutes = append(resolvedMinutes, resolvedAt.Sub(createdAt).Minutes())
			}
		}
	}

	var avg *int
	if len(resolvedMinutes) > 0 {
		sum := 0.0
		for _, m := range resolvedMinutes {
			sum += m
		}
		rounded := int(math.Round(sum / float64(len(resolvedMinutes))))
		avg = &rounded
	}

	httpx.WriteJSON(w, http.StatusOK, response{
		Total:                len(all),
		ByStatus:             byStatus,
		ByPriority:           byPriority,
		AvgResolutionMinutes: avg,
	})
}
