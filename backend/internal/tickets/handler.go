package tickets

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

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
	r.Route("/tickets", func(r chi.Router) {
		r.Post("/", h.create)
		r.Get("/", h.findAll)
		r.Get("/{id}", h.findOne)
		r.Post("/{id}/status", h.changeStatus)
		r.Post("/{id}/comments", h.addComment)
		r.Get("/{id}/audit", h.listAudit)
	})
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Subject       string `json:"subject"`
		Description   string `json:"description"`
		CustomerEmail string `json:"customerEmail"`
		Priority      string `json:"priority"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	t, err := h.service.Create(r.Context(), httpx.Actor(r), CreateInput{
		Subject:       body.Subject,
		Description:   body.Description,
		CustomerEmail: body.CustomerEmail,
		Priority:      body.Priority,
	})
	if respondIfError(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, t)
}

func (h *Handler) findAll(w http.ResponseWriter, r *http.Request) {
	filter := Filter{
		Status:   Status(r.URL.Query().Get("status")),
		Priority: Priority(r.URL.Query().Get("priority")),
	}

	page, err := parsePagination(r.URL.Query())
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	result, err := h.service.FindPage(r.Context(), filter, page)
	if respondIfError(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

// parsePagination reads limit/offset off the query string. It only rejects
// malformed values (not-an-integer); range validation (negative, over the
// max) is a business rule owned by Service.FindPage.
func parsePagination(q url.Values) (Pagination, error) {
	var page Pagination
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return Pagination{}, fmt.Errorf("limit must be an integer")
		}
		page.Limit = n
	}
	if v := q.Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return Pagination{}, fmt.Errorf("offset must be an integer")
		}
		page.Offset = n
	}
	return page, nil
}

func (h *Handler) findOne(w http.ResponseWriter, r *http.Request) {
	t, err := h.service.FindByID(r.Context(), chi.URLParam(r, "id"))
	if respondIfError(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, t)
}

func (h *Handler) changeStatus(w http.ResponseWriter, r *http.Request) {
	var body struct {
		To string `json:"to"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	t, err := h.service.ChangeStatus(r.Context(), httpx.Actor(r), chi.URLParam(r, "id"), body.To)
	if respondIfError(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, t)
}

func (h *Handler) addComment(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Author   string `json:"author"`
		Body     string `json:"body"`
		Internal bool   `json:"internal"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	c, err := h.service.AddComment(r.Context(), httpx.Actor(r), chi.URLParam(r, "id"), CommentInput{
		Author:   body.Author,
		Body:     body.Body,
		Internal: body.Internal,
	})
	if respondIfError(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, c)
}

func (h *Handler) listAudit(w http.ResponseWriter, r *http.Request) {
	page, err := parsePagination(r.URL.Query())
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	result, err := h.service.ListAudit(r.Context(), chi.URLParam(r, "id"), page)
	if respondIfError(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
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
