package cannedresponses

import (
	"errors"
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
	r.Route("/canned-responses", func(r chi.Router) {
		r.Post("/", h.create)
		r.Get("/", h.findAll)
		r.Get("/{id}", h.findOne)
		r.Put("/{id}", h.update)
		r.Delete("/{id}", h.delete)
	})
}

type saveRequest struct {
	Title string   `json:"title"`
	Body  string   `json:"body"`
	Tags  []string `json:"tags"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var body saveRequest
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	cr, err := h.service.Create(r.Context(), CreateInput(body))
	if respondIfError(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, cr)
}

func (h *Handler) findAll(w http.ResponseWriter, r *http.Request) {
	filter := Filter{Tag: r.URL.Query().Get("tag")}

	list, err := h.service.FindAll(r.Context(), filter)
	if respondIfError(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, list)
}

func (h *Handler) findOne(w http.ResponseWriter, r *http.Request) {
	cr, err := h.service.FindByID(r.Context(), chi.URLParam(r, "id"))
	if respondIfError(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, cr)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	var body saveRequest
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	cr, err := h.service.Update(r.Context(), chi.URLParam(r, "id"), CreateInput(body))
	if respondIfError(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, cr)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	err := h.service.Delete(r.Context(), chi.URLParam(r, "id"))
	if respondIfError(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusNoContent, nil)
}

// respondIfError writes the appropriate error response and returns true if
// err was non-nil.
func respondIfError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	var ve *ValidationError
	var nf *NotFoundError
	switch {
	case errors.As(err, &ve):
		httpx.WriteError(w, http.StatusBadRequest, ve.Message)
	case errors.As(err, &nf):
		httpx.WriteError(w, http.StatusNotFound, nf.Error())
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "internal server error")
	}
	return true
}
