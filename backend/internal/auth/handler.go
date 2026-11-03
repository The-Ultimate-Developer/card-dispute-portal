package auth

import (
	"crypto/subtle"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/platform/apperr"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/platform/httpx"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/platform/validate"
)

const CSRFHeader = "X-CSRF-Token"

type HandlerConfig struct {
	CookieSecure bool
	ClientIP     func(*http.Request) string
}

type Handler struct {
	svc *Service
	cfg HandlerConfig
	log *slog.Logger
}

func NewHandler(svc *Service, cfg HandlerConfig, log *slog.Logger) *Handler {
	return &Handler{svc: svc, cfg: cfg, log: log}
}

// The __Host- prefix makes the browser refuse the cookie unless it is Secure,
// host-only and path=/, which blocks cookie injection from sibling subdomains.
func (h *Handler) cookieName() string {
	if h.cfg.CookieSecure {
		return "__Host-session"
	}
	return "session"
}

func (h *Handler) setSessionCookie(w http.ResponseWriter, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // Secure comes from config, which refuses false in production
		Name:     h.cookieName(),
		Value:    token,
		Path:     "/",
		Expires:  expires,
		MaxAge:   int(time.Until(expires).Seconds()),
		HttpOnly: true,
		Secure:   h.cfg.CookieSecure,
		SameSite: http.SameSiteStrictMode,
	})
}

func (h *Handler) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // Secure comes from config, which refuses false in production
		Name: h.cookieName(), Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: h.cfg.CookieSecure, SameSite: http.SameSiteStrictMode,
	})
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type sessionResponse struct {
	User      User   `json:"user"`
	CSRFToken string `json:"csrfToken"`
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, r, h.log, err)
		return
	}

	errs := validate.Errors{}
	errs.Length("email", req.Email, 3, 254)
	errs.Length("password", req.Password, 1, 128)
	if errs.Any() {
		httpx.WriteError(w, r, h.log, apperr.Validation(errs))
		return
	}

	res, err := h.svc.Login(r.Context(), req.Email, req.Password, h.cfg.ClientIP(r))
	if err != nil {
		httpx.WriteError(w, r, h.log, err)
		return
	}

	h.setSessionCookie(w, res.SessionToken, res.ExpiresAt)
	httpx.WriteJSON(w, http.StatusOK, sessionResponse{User: res.User, CSRFToken: res.CSRFToken})
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	p, _ := PrincipalFrom(r.Context())
	if err := h.svc.Logout(r.Context(), p, h.cfg.ClientIP(r)); err != nil {
		httpx.WriteError(w, r, h.log, err)
		return
	}
	h.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	p, _ := PrincipalFrom(r.Context())
	u, err := h.svc.CurrentUser(r.Context(), p)
	if err != nil {
		httpx.WriteError(w, r, h.log, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, sessionResponse{User: u, CSRFToken: p.CSRFToken})
}

func (h *Handler) LockedAccounts(w http.ResponseWriter, r *http.Request) {
	accounts, err := h.svc.LockedAccounts(r.Context())
	if err != nil {
		httpx.WriteError(w, r, h.log, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": accounts})
}

func (h *Handler) Unlock(w http.ResponseWriter, r *http.Request) {
	p, _ := PrincipalFrom(r.Context())
	if err := h.svc.Unlock(r.Context(), p, r.PathValue("id"), h.cfg.ClientIP(r)); err != nil {
		httpx.WriteError(w, r, h.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Authenticate resolves the session cookie into a Principal if one is present.
// It never rejects; RequireAuth decides whether a route needs a user.
func (h *Handler) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(h.cookieName())
		if err != nil || c.Value == "" {
			next.ServeHTTP(w, r)
			return
		}
		p, err := h.svc.Authenticate(r.Context(), c.Value)
		switch {
		case errors.Is(err, ErrUnauthenticated):
			h.clearSessionCookie(w)
			next.ServeHTTP(w, r)
		case err != nil:
			httpx.WriteError(w, r, h.log, err)
		default:
			next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), p)))
		}
	})
}

func (h *Handler) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := PrincipalFrom(r.Context()); !ok {
			httpx.WriteError(w, r, h.log, ErrUnauthenticated)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireRole is the coarse gate. Ownership ("is this dispute yours?") is
// enforced in the services, because only they know which record is involved.
func (h *Handler) RequireRole(roles ...Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, ok := PrincipalFrom(r.Context())
			if !ok {
				httpx.WriteError(w, r, h.log, ErrUnauthenticated)
				return
			}
			if !slices.Contains(roles, p.Role) {
				httpx.WriteError(w, r, h.log, apperr.Forbidden("FORBIDDEN", "You do not have access to this resource."))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// CSRF uses the synchronizer-token pattern: the token is bound to the session
// server-side and must be echoed in a header that a cross-site form cannot set.
func (h *Handler) CSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		p, ok := PrincipalFrom(r.Context())
		got := r.Header.Get(CSRFHeader)
		if !ok || got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(p.CSRFToken)) != 1 {
			httpx.WriteError(w, r, h.log, apperr.Forbidden("CSRF_TOKEN_INVALID", "Missing or invalid CSRF token."))
			return
		}
		next.ServeHTTP(w, r)
	})
}
