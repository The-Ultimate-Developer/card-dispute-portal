package httpapi

import (
	"context"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/auth"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/corebanking"
)

func TestDisputeJourneyEndToEnd(t *testing.T) {
	e := start(t)

	anon := e.client()
	expect(t, anon.do(http.MethodGet, "/api/transactions", nil), http.StatusUnauthorized)

	customer := e.as(customerA)
	res := customer.do(http.MethodGet, "/api/transactions", nil)
	expect(t, res, http.StatusOK)
	items := res.body["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["disputable"] != true {
		t.Fatalf("unexpected transactions: %v", res.body)
	}

	create := map[string]string{"transactionId": e.txnID, "reason": "UNRECOGNISED", "description": "I don't recognise this payment."}
	res = customer.do(http.MethodPost, "/api/disputes", create, "Idempotency-Key", "e2e-key-0001")
	expect(t, res, http.StatusCreated)
	id := res.str("id")
	if res.header.Get("Location") != "/api/disputes/"+id {
		t.Fatalf("Location header: %q", res.header.Get("Location"))
	}

	res = customer.do(http.MethodPost, "/api/disputes", create, "Idempotency-Key", "e2e-key-0001")
	expect(t, res, http.StatusOK)
	if res.header.Get("Idempotent-Replayed") != "true" || res.str("id") != id {
		t.Fatalf("replay should return the original dispute: %v", res.body)
	}

	res = customer.do(http.MethodPost, "/api/disputes", create, "Idempotency-Key", "e2e-key-0002")
	expect(t, res, http.StatusConflict)

	agent := e.as(agentEmail)
	res = agent.do(http.MethodPatch, "/api/disputes/"+id+"/status", map[string]any{"status": "UNDER_REVIEW", "version": 1, "note": "Contacting merchant"})
	expect(t, res, http.StatusOK)

	res = agent.do(http.MethodPatch, "/api/disputes/"+id+"/status", map[string]any{"status": "RESOLVED", "version": 1, "note": "Refunded"})
	expect(t, res, http.StatusConflict)
	if res.str("code") != "STALE_VERSION" {
		t.Fatalf("expected STALE_VERSION: %v", res.body)
	}

	res = customer.do(http.MethodGet, "/api/disputes/"+id, nil)
	expect(t, res, http.StatusOK)
	if res.str("status") != "UNDER_REVIEW" || len(res.body["timeline"].([]any)) != 2 {
		t.Fatalf("customer should see the agent's update: %v", res.body)
	}

	expect(t, e.as(customerB).do(http.MethodGet, "/api/disputes/"+id, nil), http.StatusNotFound)

	expect(t, customer.do(http.MethodPost, "/api/auth/logout", nil), http.StatusNoContent)
	expect(t, customer.do(http.MethodGet, "/api/auth/me", nil), http.StatusUnauthorized)
}

func TestCustomerIsNotifiedAtEachStep(t *testing.T) {
	e := start(t)
	id := e.createDispute(e.as(customerA), "notify-key-0001")
	expect(t, e.as(agentEmail).do(http.MethodPatch, "/api/disputes/"+id+"/status",
		map[string]any{"status": "REJECTED", "version": 1, "note": "Payment matches a verified order."}), http.StatusOK)

	if _, err := e.worker.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(*e.sent) != 2 {
		t.Fatalf("sent %d emails, want 2 (received, declined)", len(*e.sent))
	}
	if !strings.Contains((*e.sent)[0].Subject, "received") || !strings.Contains((*e.sent)[1].Subject, "declined") {
		t.Fatalf("unexpected subjects: %q, %q", (*e.sent)[0].Subject, (*e.sent)[1].Subject)
	}

	res := e.as(adminEmail).do(http.MethodGet, "/api/admin/notifications?status=SENT", nil)
	expect(t, res, http.StatusOK)
	if len(res.body["items"].([]any)) != 2 {
		t.Fatalf("admin should see both deliveries: %v", res.body)
	}
}

func TestCSRFIsRequiredForWrites(t *testing.T) {
	e := start(t)
	c := e.as(customerA)
	c.csrf = ""
	res := c.do(http.MethodPost, "/api/disputes",
		map[string]string{"transactionId": e.txnID, "reason": "OTHER", "description": "Long enough description."},
		"Idempotency-Key", "csrf-key-0001")
	expect(t, res, http.StatusForbidden)
	if res.str("code") != "CSRF_TOKEN_INVALID" {
		t.Fatalf("got %v", res.body)
	}
}

func TestLockoutAndSupportUnlockOverHTTP(t *testing.T) {
	e := start(t)
	c := e.client()
	wrong := map[string]string{"email": customerA, "password": "nope"}

	expect(t, c.do(http.MethodPost, "/api/auth/login", wrong), http.StatusUnauthorized)
	expect(t, c.do(http.MethodPost, "/api/auth/login", wrong), http.StatusUnauthorized)
	res := c.do(http.MethodPost, "/api/auth/login", wrong)
	expect(t, res, http.StatusLocked)
	if !strings.Contains(res.str("detail"), "contact support") {
		t.Fatalf("locked message should direct the user to support: %v", res.body)
	}

	support := e.as(supportMail)
	res = support.do(http.MethodGet, "/api/support/locked-accounts", nil)
	expect(t, res, http.StatusOK)
	locked := res.body["items"].([]any)
	if len(locked) != 1 {
		t.Fatalf("expected one locked account: %v", res.body)
	}
	userID := locked[0].(map[string]any)["id"].(string)

	expect(t, support.do(http.MethodPost, "/api/support/accounts/"+userID+"/unlock", nil), http.StatusNoContent)
	e.as(customerA)

	if _, err := e.worker.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(*e.sent) != 1 || !strings.Contains((*e.sent)[0].Subject, "blocked") {
		t.Fatalf("the customer should have been warned about the lock: %+v", *e.sent)
	}
}

func TestLoginIsRateLimitedPerIP(t *testing.T) {
	e := start(t, withLoginLimit(2))
	c := e.client()
	body := map[string]string{"email": "nobody@example.com", "password": "x"}

	c.do(http.MethodPost, "/api/auth/login", body)
	c.do(http.MethodPost, "/api/auth/login", body)
	res := c.do(http.MethodPost, "/api/auth/login", body)
	expect(t, res, http.StatusTooManyRequests)
	if res.header.Get("Retry-After") == "" {
		t.Fatal("429 must say when to retry")
	}
}

type downBank struct{}

func (downBank) ListTransactions(context.Context, string, string, int) (corebanking.Page, error) {
	return corebanking.Page{}, corebanking.ErrUnavailable
}

func (downBank) GetTransaction(context.Context, string, string) (corebanking.Transaction, error) {
	return corebanking.Transaction{}, corebanking.ErrUnavailable
}

func TestCoreBankingOutageDegradesGracefully(t *testing.T) {
	e := start(t, withBank(downBank{}))
	c := e.as(customerA)

	res := c.do(http.MethodGet, "/api/transactions", nil)
	expect(t, res, http.StatusServiceUnavailable)
	if res.str("code") != "CORE_BANKING_UNAVAILABLE" {
		t.Fatalf("got %v", res.body)
	}
	expect(t, c.do(http.MethodGet, "/api/disputes", nil), http.StatusOK)
}

func TestAdminBackOffice(t *testing.T) {
	e := start(t)
	customer := e.as(customerA)
	res := customer.do(http.MethodPost, "/api/disputes",
		map[string]string{"transactionId": e.txnID, "reason": "DUPLICATE", "description": "Charged twice for one order."},
		"Idempotency-Key", "admin-e2e-0001")
	expect(t, res, http.StatusCreated)
	id := res.str("id")
	if _, leaked := res.body["customer"]; leaked {
		t.Fatal("customers must not receive the staff-only customer block")
	}

	adm := e.as(adminEmail)
	res = adm.do(http.MethodGet, "/api/disputes", nil)
	expect(t, res, http.StatusOK)
	first := res.body["items"].([]any)[0].(map[string]any)
	if first["customer"].(map[string]any)["email"] != customerA {
		t.Fatalf("staff should see whose dispute it is: %v", first)
	}

	res = adm.do(http.MethodPatch, "/api/disputes/"+id+"/status",
		map[string]any{"status": "REJECTED", "version": 1, "note": "Duplicate was reversed by the merchant."})
	expect(t, res, http.StatusOK)
	timeline := res.body["timeline"].([]any)
	if timeline[len(timeline)-1].(map[string]any)["actorRole"] != "ADMIN" {
		t.Fatalf("timeline should attribute the change to the admin: %v", timeline)
	}

	res = adm.do(http.MethodGet, "/api/admin/overview", nil)
	expect(t, res, http.StatusOK)
	byStatus := res.body["disputesByStatus"].(map[string]any)
	if byStatus["REJECTED"].(float64) != 1 || res.body["customers"].(float64) != 2 || res.body["notificationsPending"].(float64) != 2 {
		t.Fatalf("unexpected overview: %v", res.body)
	}

	res = adm.do(http.MethodGet, "/api/admin/audit-events?event=LOGIN_SUCCEEDED&limit=1", nil)
	expect(t, res, http.StatusOK)
	if len(res.body["items"].([]any)) != 1 || res.body["nextBefore"] == nil {
		t.Fatalf("expected a one-item page with a cursor: %v", res.body)
	}
	expect(t, adm.do(http.MethodGet, "/api/admin/audit-events?event=NOPE", nil), http.StatusBadRequest)
	expect(t, adm.do(http.MethodGet, "/api/admin/notifications?status=NOPE", nil), http.StatusBadRequest)
}

// TestAccessPolicy proves the endpoint table: anonymous callers are refused by
// every protected route, and every role is refused wherever it is not listed.
func TestAccessPolicy(t *testing.T) {
	e := start(t)
	sessions := map[auth.Role]*client{
		auth.RoleCustomer: e.as(customerA),
		auth.RoleAgent:    e.as(agentEmail),
		auth.RoleSupport:  e.as(supportMail),
		auth.RoleAdmin:    e.as(adminEmail),
	}
	anon := e.client()

	for _, ep := range endpoints {
		if ep.roles == nil {
			continue
		}
		path := strings.NewReplacer("{id}", "3f1c2b8e-9a4d-4c51-8f0e-2b7d6a1c9e05").Replace(ep.path)
		name := ep.method + " " + ep.path

		if res := anon.do(ep.method, path, nil); res.status != http.StatusUnauthorized {
			t.Errorf("%s: anonymous got %d, want 401", name, res.status)
		}
		if ep.path == "/api/auth/logout" {
			continue
		}
		for role, c := range sessions {
			res := c.do(ep.method, path, nil)
			denied := res.status == http.StatusForbidden && res.str("code") == "FORBIDDEN"
			if allowed := slices.Contains(ep.roles, role); allowed == denied {
				t.Errorf("%s as %s: got %d %v (allowed=%v)", name, role, res.status, res.body, allowed)
			}
		}
	}
}

func TestOperationalEndpoints(t *testing.T) {
	e := start(t)
	c := e.client()

	expect(t, c.do(http.MethodPost, "/api/auth/login", nil, "Content-Type", "text/plain"), http.StatusUnsupportedMediaType)
	expect(t, c.do(http.MethodDelete, "/api/disputes", nil), http.StatusMethodNotAllowed)

	expect(t, c.do(http.MethodGet, "/metrics", nil), http.StatusNotFound)
	expect(t, c.do(http.MethodGet, "/metrics", nil, "Authorization", "Bearer wrong"), http.StatusNotFound)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, e.url+"/metrics", nil)
	req.Header.Set("Authorization", "Bearer "+metricsKey)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	for _, want := range []string{`http_requests_total{method="DELETE",route="unmatched",status="405"}`, "go_goroutines"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("metrics missing %q", want)
		}
	}

	req, _ = http.NewRequestWithContext(context.Background(), http.MethodGet, e.url+"/docs", nil)
	docs, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = docs.Body.Close()
	if docs.StatusCode != http.StatusOK || !strings.Contains(docs.Header.Get("Content-Security-Policy"), "unpkg.com") {
		t.Fatalf("docs: %d %q", docs.StatusCode, docs.Header.Get("Content-Security-Policy"))
	}
}
