// Package admin is the back-office read side: system health at a glance and the
// security audit trail. It only reads; every change still goes through the
// owning module's service and rules.
package admin

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/auth"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/platform/apperr"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/platform/httpx"
)

const (
	defaultAuditLimit = 50
	maxAuditLimit     = 200
)

type Overview struct {
	DisputesByStatus        map[string]int `json:"disputesByStatus"`
	OpenDisputes            int            `json:"openDisputes"`
	OpenDisputedAmountCents int64          `json:"openDisputedAmountCents"`
	OldestOpenDisputeAt     *time.Time     `json:"oldestOpenDisputeAt"`
	DisputesLast7Days       int            `json:"disputesLast7Days"`
	Customers               int            `json:"customers"`
	LockedAccounts          int            `json:"lockedAccounts"`
	FailedLoginsLast24h     int            `json:"failedLoginsLast24h"`
	NotificationsPending    int            `json:"notificationsPending"`
	NotificationsFailed     int            `json:"notificationsFailed"`
}

type Notification struct {
	ID          int64      `json:"id"`
	Topic       string     `json:"topic"`
	Status      string     `json:"status"`
	Attempts    int        `json:"attempts"`
	LastError   *string    `json:"lastError"`
	CreatedAt   time.Time  `json:"createdAt"`
	NextAttempt *time.Time `json:"nextAttemptAt"`
	SentAt      *time.Time `json:"sentAt"`
}

type NotificationPage struct {
	Items      []Notification `json:"items"`
	NextBefore *int64         `json:"nextBefore,omitempty"`
}

type AuditEvent struct {
	ID         int64     `json:"id"`
	Event      string    `json:"event"`
	Email      *string   `json:"email"`
	UserID     *string   `json:"userId"`
	ActorEmail *string   `json:"actorEmail"`
	IP         *string   `json:"ip"`
	CreatedAt  time.Time `json:"createdAt"`
}

type AuditPage struct {
	Items      []AuditEvent `json:"items"`
	NextBefore *int64       `json:"nextBefore,omitempty"`
}

var auditEvents = map[string]bool{
	string(auth.EventLoginSucceeded): true, string(auth.EventLoginFailed): true,
	string(auth.EventLoginBlocked): true, string(auth.EventAccountLocked): true,
	string(auth.EventAccountUnlocked): true, string(auth.EventLogout): true,
}

type Service struct {
	db *pgxpool.Pool
}

func NewService(db *pgxpool.Pool) *Service {
	return &Service{db: db}
}

func (s *Service) Overview(ctx context.Context) (Overview, error) {
	o := Overview{DisputesByStatus: map[string]int{"SUBMITTED": 0, "UNDER_REVIEW": 0, "RESOLVED": 0, "REJECTED": 0}}

	rows, err := s.db.Query(ctx, `SELECT status, count(*) FROM disputes GROUP BY status`)
	if err != nil {
		return Overview{}, fmt.Errorf("disputes by status: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var n int
		if err := rows.Scan(&status, &n); err != nil {
			return Overview{}, fmt.Errorf("scan status counts: %w", err)
		}
		o.DisputesByStatus[status] = n
	}
	if err := rows.Err(); err != nil {
		return Overview{}, err
	}

	err = s.db.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM disputes WHERE status IN ('SUBMITTED', 'UNDER_REVIEW')),
			(SELECT coalesce(sum(amount_cents), 0) FROM disputes WHERE status IN ('SUBMITTED', 'UNDER_REVIEW')),
			(SELECT min(created_at) FROM disputes WHERE status IN ('SUBMITTED', 'UNDER_REVIEW')),
			(SELECT count(*) FROM disputes WHERE created_at > now() - interval '7 days'),
			(SELECT count(*) FROM users WHERE role = 'CUSTOMER'),
			(SELECT count(*) FROM users WHERE locked_at IS NOT NULL),
			(SELECT count(*) FROM auth_events WHERE event = 'LOGIN_FAILED' AND created_at > now() - interval '24 hours'),
			(SELECT count(*) FROM outbox WHERE status = 'PENDING'),
			(SELECT count(*) FROM outbox WHERE status = 'DEAD')`).
		Scan(&o.OpenDisputes, &o.OpenDisputedAmountCents, &o.OldestOpenDisputeAt, &o.DisputesLast7Days,
			&o.Customers, &o.LockedAccounts, &o.FailedLoginsLast24h, &o.NotificationsPending, &o.NotificationsFailed)
	if err != nil {
		return Overview{}, fmt.Errorf("overview: %w", err)
	}
	return o, nil
}

// AuditEvents pages backwards by ID: `before` is the smallest ID already seen.
func (s *Service) AuditEvents(ctx context.Context, event string, before int64, limit int) (AuditPage, error) {
	if event != "" && !auditEvents[event] {
		return AuditPage{}, apperr.Validation(map[string]string{"event": "is not a known audit event"})
	}
	limit = clamp(limit)

	rows, err := s.db.Query(ctx, `
		SELECT e.id, e.event, e.email, e.user_id, a.email, e.ip, e.created_at
		FROM auth_events e
		LEFT JOIN users a ON a.id = e.actor_id
		WHERE ($1 = '' OR e.event = $1)
		  AND ($2 = 0 OR e.id < $2)
		ORDER BY e.id DESC
		LIMIT $3`, event, before, limit+1)
	if err != nil {
		return AuditPage{}, fmt.Errorf("audit events: %w", err)
	}
	items, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (AuditEvent, error) {
		var e AuditEvent
		err := row.Scan(&e.ID, &e.Event, &e.Email, &e.UserID, &e.ActorEmail, &e.IP, &e.CreatedAt)
		return e, err
	})
	if err != nil {
		return AuditPage{}, fmt.Errorf("scan audit events: %w", err)
	}

	page := AuditPage{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		next := page.Items[limit-1].ID
		page.NextBefore = &next
	}
	return page, nil
}

var notificationStatuses = map[string]bool{"PENDING": true, "SENT": true, "DEAD": true}

// Notifications shows delivery state only. Payloads stay out of the response;
// operators need to know what failed and why, not read customer emails.
func (s *Service) Notifications(ctx context.Context, status string, before int64, limit int) (NotificationPage, error) {
	if status != "" && !notificationStatuses[status] {
		return NotificationPage{}, apperr.Validation(map[string]string{"status": "must be PENDING, SENT or DEAD"})
	}
	limit = clamp(limit)

	rows, err := s.db.Query(ctx, `
		SELECT id, topic, status, attempts, last_error, created_at,
		       CASE WHEN status = 'PENDING' THEN available_at END, sent_at
		FROM outbox
		WHERE ($1 = '' OR status = $1) AND ($2 = 0 OR id < $2)
		ORDER BY id DESC
		LIMIT $3`, status, before, limit+1)
	if err != nil {
		return NotificationPage{}, fmt.Errorf("notifications: %w", err)
	}
	items, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Notification, error) {
		var n Notification
		err := row.Scan(&n.ID, &n.Topic, &n.Status, &n.Attempts, &n.LastError, &n.CreatedAt, &n.NextAttempt, &n.SentAt)
		return n, err
	})
	if err != nil {
		return NotificationPage{}, fmt.Errorf("scan notifications: %w", err)
	}

	page := NotificationPage{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		next := page.Items[limit-1].ID
		page.NextBefore = &next
	}
	return page, nil
}

func clamp(limit int) int {
	switch {
	case limit <= 0:
		return defaultAuditLimit
	case limit > maxAuditLimit:
		return maxAuditLimit
	}
	return limit
}

type Handler struct {
	svc *Service
	log *slog.Logger
}

func NewHandler(svc *Service, log *slog.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

func (h *Handler) Overview(w http.ResponseWriter, r *http.Request) {
	o, err := h.svc.Overview(r.Context())
	if err != nil {
		httpx.WriteError(w, r, h.log, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, o)
}

func (h *Handler) AuditEvents(w http.ResponseWriter, r *http.Request) {
	before, limit, ok := h.paging(w, r)
	if !ok {
		return
	}
	page, err := h.svc.AuditEvents(r.Context(), r.URL.Query().Get("event"), before, limit)
	if err != nil {
		httpx.WriteError(w, r, h.log, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, page)
}

func (h *Handler) Notifications(w http.ResponseWriter, r *http.Request) {
	before, limit, ok := h.paging(w, r)
	if !ok {
		return
	}
	page, err := h.svc.Notifications(r.Context(), r.URL.Query().Get("status"), before, limit)
	if err != nil {
		httpx.WriteError(w, r, h.log, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, page)
}

func (h *Handler) paging(w http.ResponseWriter, r *http.Request) (int64, int, bool) {
	q := r.URL.Query()
	var before int64
	var limit int
	var err1, err2 error
	if v := q.Get("before"); v != "" {
		before, err1 = strconv.ParseInt(v, 10, 64)
	}
	if v := q.Get("limit"); v != "" {
		limit, err2 = strconv.Atoi(v)
	}
	if err1 != nil || err2 != nil || before < 0 {
		httpx.WriteError(w, r, h.log, apperr.Validation(map[string]string{"query": "before and limit must be positive numbers"}))
		return 0, 0, false
	}
	return before, limit, true
}
