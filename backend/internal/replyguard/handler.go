package replyguard

import (
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"

	"resolve/internal/httpx"
)

// Handler exposes POST /guard. It is an internal, service-to-service
// endpoint (called only by the main app's fire-and-forget draft
// notification) and is not part of the public API surface, mirroring
// triage.Handler.
type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Mount(r chi.Router) {
	r.Post("/guard", h.create)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TicketID          string `json:"ticketId"`
		DraftID           string `json:"draftId"`
		TicketSubject     string `json:"ticketSubject"`
		TicketDescription string `json:"ticketDescription"`
		TicketStatus      string `json:"ticketStatus"`
		TicketPriority    string `json:"ticketPriority"`
		InternalNotes     []struct {
			Author string `json:"author"`
			Body   string `json:"body"`
		} `json:"internalNotes"`
		DraftBody string `json:"draftBody"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	notes := make([]Note, len(body.InternalNotes))
	for i, n := range body.InternalNotes {
		notes[i] = Note{Author: n.Author, Body: n.Body}
	}

	input := GuardInput{
		TicketSubject:     body.TicketSubject,
		TicketDescription: body.TicketDescription,
		TicketStatus:      body.TicketStatus,
		TicketPriority:    body.TicketPriority,
		InternalNotes:     notes,
		DraftBody:         body.DraftBody,
	}

	if err := h.service.Guard(r.Context(), body.TicketID, body.DraftID, input); err != nil {
		log.Printf("reply-guard failed for draft %s (ticket %s): %v", body.DraftID, body.TicketID, err)
		httpx.WriteError(w, http.StatusInternalServerError, "reply-guard failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
