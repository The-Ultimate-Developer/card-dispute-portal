package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/admin"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/auth"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/corebanking"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/disputes"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/notify"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/platform/httpx"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/platform/metrics"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/platform/ratelimit"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/platform/testdb"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/server"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/transactions"
)

const (
	pw          = "Demo@2026!"
	metricsKey  = "test-metrics-token-0123456789abcdef"
	customerA   = "customer@example.com"
	customerB   = "other@example.com"
	agentEmail  = "agent@example.com"
	supportMail = "support@example.com"
	adminEmail  = "admin@example.com"
)

type options struct {
	loginPerMinute int
	bank           corebanking.Provider
}

type env struct {
	t       *testing.T
	url     string
	db      *pgxpool.Pool
	txnID   string
	worker  *notify.Worker
	sent    *[]notify.Email
	userIDs map[string]string
}

type captureSender struct{ sent *[]notify.Email }

func (c captureSender) Send(_ context.Context, e notify.Email) error {
	*c.sent = append(*c.sent, e)
	return nil
}

func start(t *testing.T, opts ...func(*options)) *env {
	t.Helper()
	o := options{loginPerMinute: 50}
	for _, f := range opts {
		f(&o)
	}

	db := testdb.New(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	m := metrics.New()

	hash, _ := auth.HashPassword(pw)
	ids := map[string]string{}
	for email, role := range map[string]auth.Role{
		customerA: auth.RoleCustomer, customerB: auth.RoleCustomer,
		agentEmail: auth.RoleAgent, supportMail: auth.RoleSupport, adminEmail: auth.RoleAdmin,
	} {
		ids[email] = testdb.CreateUser(t, db, email, string(role), hash)
	}
	card := testdb.CreateCard(t, db, ids[customerA], "4821")
	txn := testdb.CreateTransaction(t, db, card, "POSTED", 25_000, time.Now().Add(-time.Hour))

	bank := o.bank
	if bank == nil {
		bank = corebanking.NewMock(db, corebanking.Faults{})
	}

	authSvc := auth.NewService(auth.NewRepository(db), auth.Config{MaxFailedAttempts: 3, IdleTimeout: time.Hour, AbsoluteTimeout: 8 * time.Hour}, log, m)
	disputeSvc := disputes.NewService(disputes.NewRepository(db), bank, m)
	clientIP := func(r *http.Request) string { return httpx.ClientIP(r, 0) }

	routes := Routes(Deps{
		Log:          log,
		Auth:         auth.NewHandler(authSvc, auth.HandlerConfig{ClientIP: clientIP}, log),
		Transactions: transactions.NewHandler(transactions.NewService(bank, disputeSvc), log),
		Disputes:     disputes.NewHandler(disputeSvc, log),
		Admin:        admin.NewHandler(admin.NewService(db), log),
		LoginLimiter: ratelimit.New(o.loginPerMinute, time.Minute),
		WriteLimiter: ratelimit.New(1000, time.Hour),
		ClientIP:     clientIP,
		Metrics:      m.Handler(metricsKey),
		EnableDocs:   true,
	})
	srv := httptest.NewServer(server.New(server.Options{Log: log, DB: db, Routes: routes, Observer: m}).Handler())
	t.Cleanup(srv.Close)

	sent := &[]notify.Email{}
	worker := notify.NewWorker(db, captureSender{sent}, m, log,
		notify.Config{PollInterval: time.Second, BatchSize: 50, MaxAttempts: 3, Lease: time.Minute})

	return &env{t: t, url: srv.URL, db: db, txnID: txn, worker: worker, sent: sent, userIDs: ids}
}

func withLoginLimit(n int) func(*options) { return func(o *options) { o.loginPerMinute = n } }
func withBank(p corebanking.Provider) func(*options) {
	return func(o *options) { o.bank = p }
}

type client struct {
	t    *testing.T
	base string
	http *http.Client
	csrf string
}

func (e *env) client() *client {
	jar, _ := cookiejar.New(nil)
	return &client{t: e.t, base: e.url, http: &http.Client{Jar: jar}}
}

func (e *env) as(email string) *client {
	c := e.client()
	c.login(email)
	return c
}

type response struct {
	status int
	header http.Header
	body   map[string]any
}

func (r response) str(key string) string {
	s, _ := r.body[key].(string)
	return s
}

func (c *client) do(method, path string, body any, headers ...string) response {
	c.t.Helper()
	var reader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	}
	req, _ := http.NewRequestWithContext(context.Background(), method, c.base+path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.csrf != "" {
		req.Header.Set(auth.CSRFHeader, c.csrf)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	res, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	out := response{status: res.StatusCode, header: res.Header}
	_ = json.NewDecoder(res.Body).Decode(&out.body)
	return out
}

func (c *client) login(email string) {
	c.t.Helper()
	res := c.do(http.MethodPost, "/api/auth/login", map[string]string{"email": email, "password": pw})
	if res.status != http.StatusOK {
		c.t.Fatalf("login %s: %d %v", email, res.status, res.body)
	}
	c.csrf = res.str("csrfToken")
}

func expect(t *testing.T, res response, status int) {
	t.Helper()
	if res.status != status {
		t.Fatalf("got %d, want %d: %v", res.status, status, res.body)
	}
}

func (e *env) createDispute(c *client, key string) string {
	e.t.Helper()
	res := c.do(http.MethodPost, "/api/disputes",
		map[string]string{"transactionId": e.txnID, "reason": "UNRECOGNISED", "description": "I don't recognise this payment."},
		"Idempotency-Key", key)
	expect(e.t, res, http.StatusCreated)
	return res.str("id")
}
