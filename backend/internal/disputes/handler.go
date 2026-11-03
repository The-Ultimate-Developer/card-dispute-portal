package disputes

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/auth"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/platform/apperr"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/platform/httpx"
)

type Handler struct {
	svc *Service
	log *slog.Logger
}

func NewHandler(svc *Service, log *slog.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

type createRequest struct {
	TransactionID string `json:"transactionId"`
	Reason        Reason `json:"reason"`
	Description   string `json:"description"`
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, r, h.log, err)
		return
	}

	p, _ := auth.PrincipalFrom(r.Context())
	d, created, err := h.svc.Create(r.Context(), p, CreateInput{
		TransactionID:  req.TransactionID,
		Reason:         req.Reason,
		Description:    req.Description,
		IdempotencyKey: r.Header.Get("Idempotency-Key"),
	})
	if err != nil {
		httpx.WriteError(w, r, h.log, err)
		return
	}

	w.Header().Set("Location", "/api/disputes/"+d.ID)
	if !created {
		w.Header().Set("Idempotent-Replayed", "true")
		httpx.WriteJSON(w, http.StatusOK, d)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, d)
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, err := optionalInt(q.Get("limit"))
	if err != nil {
		httpx.WriteError(w, r, h.log, apperr.Validation(map[string]string{"limit": "must be a number"}))
		return
	}

	p, _ := auth.PrincipalFrom(r.Context())
	page, err := h.svc.List(r.Context(), p, ListInput{Status: q.Get("status"), Cursor: q.Get("cursor"), Limit: limit})
	if err != nil {
		httpx.WriteError(w, r, h.log, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, page)
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFrom(r.Context())
	d, err := h.svc.Get(r.Context(), p, r.PathValue("id"))
	if err != nil {
		httpx.WriteError(w, r, h.log, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, d)
}

type changeStatusRequest struct {
	Status  Status `json:"status"`
	Version int    `json:"version"`
	Note    string `json:"note"`
}

func (h *Handler) ChangeStatus(w http.ResponseWriter, r *http.Request) {
	var req changeStatusRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, r, h.log, err)
		return
	}

	p, _ := auth.PrincipalFrom(r.Context())
	d, err := h.svc.ChangeStatus(r.Context(), p, r.PathValue("id"), ChangeStatusInput(req))
	if err != nil {
		httpx.WriteError(w, r, h.log, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, d)
}

func optionalInt(s string) (int, error) {
	if s == "" {
		return 0, nil
	}
	return strconv.Atoi(s)
}
