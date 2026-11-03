// Package httpapi wires every module's handlers to routes. The endpoint table
// below is the whole access policy as data: who can call what, and with which
// rate limit. Tests iterate it to prove the policy and that every endpoint is
// documented in the OpenAPI contract.
package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/admin"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/auth"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/disputes"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/platform/ratelimit"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/server/middleware"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/transactions"
)

type Deps struct {
	Log          *slog.Logger
	Auth         *auth.Handler
	Transactions *transactions.Handler
	Disputes     *disputes.Handler
	Admin        *admin.Handler
	LoginLimiter *ratelimit.Limiter
	WriteLimiter *ratelimit.Limiter
	ClientIP     func(*http.Request) string
	Metrics      http.Handler
	EnableDocs   bool
}

type limit int

const (
	noLimit limit = iota
	perIP
	perUser
)

type endpoint struct {
	method, path string
	roles        []auth.Role // nil means public
	limit        limit
	handler      func(Deps) http.HandlerFunc
}

var (
	customer       = []auth.Role{auth.RoleCustomer}
	adminOnly      = []auth.Role{auth.RoleAdmin}
	caseworkers    = []auth.Role{auth.RoleAgent, auth.RoleAdmin}
	accountSupport = []auth.Role{auth.RoleSupport, auth.RoleAdmin}
	disputeReaders = []auth.Role{auth.RoleCustomer, auth.RoleAgent, auth.RoleAdmin}
	anyone         = []auth.Role{auth.RoleCustomer, auth.RoleAgent, auth.RoleSupport, auth.RoleAdmin}
)

var endpoints = []endpoint{
	{"POST", "/api/auth/login", nil, perIP, func(d Deps) http.HandlerFunc { return d.Auth.Login }},
	{"POST", "/api/auth/logout", anyone, noLimit, func(d Deps) http.HandlerFunc { return d.Auth.Logout }},
	{"GET", "/api/auth/me", anyone, noLimit, func(d Deps) http.HandlerFunc { return d.Auth.Me }},

	{"GET", "/api/transactions", customer, noLimit, func(d Deps) http.HandlerFunc { return d.Transactions.List }},

	{"POST", "/api/disputes", customer, perUser, func(d Deps) http.HandlerFunc { return d.Disputes.Create }},
	{"GET", "/api/disputes", disputeReaders, noLimit, func(d Deps) http.HandlerFunc { return d.Disputes.List }},
	{"GET", "/api/disputes/{id}", disputeReaders, noLimit, func(d Deps) http.HandlerFunc { return d.Disputes.Get }},
	{"PATCH", "/api/disputes/{id}/status", caseworkers, perUser, func(d Deps) http.HandlerFunc { return d.Disputes.ChangeStatus }},

	{"GET", "/api/support/locked-accounts", accountSupport, noLimit, func(d Deps) http.HandlerFunc { return d.Auth.LockedAccounts }},
	{"POST", "/api/support/accounts/{id}/unlock", accountSupport, perUser, func(d Deps) http.HandlerFunc { return d.Auth.Unlock }},

	{"GET", "/api/admin/overview", adminOnly, noLimit, func(d Deps) http.HandlerFunc { return d.Admin.Overview }},
	{"GET", "/api/admin/audit-events", adminOnly, noLimit, func(d Deps) http.HandlerFunc { return d.Admin.AuditEvents }},
	{"GET", "/api/admin/notifications", adminOnly, noLimit, func(d Deps) http.HandlerFunc { return d.Admin.Notifications }},
}

func Routes(d Deps) func(*http.ServeMux) {
	return func(mux *http.ServeMux) {
		a := d.Auth
		limiters := map[limit]func(http.Handler) http.Handler{
			perIP: d.LoginLimiter.Middleware(d.Log, d.ClientIP),
			perUser: d.WriteLimiter.Middleware(d.Log, func(r *http.Request) string {
				p, _ := auth.PrincipalFrom(r.Context())
				return p.UserID
			}),
		}

		for _, e := range endpoints {
			var chain []func(http.Handler) http.Handler
			if e.roles != nil {
				chain = append(chain, a.Authenticate, a.RequireAuth, a.RequireRole(e.roles...), a.CSRF)
			}
			if mw, ok := limiters[e.limit]; ok {
				chain = append(chain, mw)
			}
			mux.Handle(e.method+" "+e.path, middleware.Chain(e.handler(d), chain...))
		}

		if d.Metrics != nil {
			mux.Handle("GET /metrics", d.Metrics)
		}
		if d.EnableDocs {
			registerDocs(mux)
		}
	}
}
