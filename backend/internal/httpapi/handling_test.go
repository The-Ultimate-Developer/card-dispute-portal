package httpapi

import (
	"net/http"
	"testing"
)

func TestOnlyStaffSeeWhoHandledADispute(t *testing.T) {
	e := start(t)
	customer := e.as(customerA)
	id := e.createDispute(customer, "actor-key-0001")
	expect(t, e.as(agentEmail).do(http.MethodPatch, "/api/disputes/"+id+"/status",
		map[string]any{"status": "UNDER_REVIEW", "version": 1}), http.StatusOK)

	staffView := e.as(adminEmail).do(http.MethodGet, "/api/disputes/"+id, nil)
	expect(t, staffView, http.StatusOK)
	timeline := staffView.body["timeline"].([]any)
	review := timeline[1].(map[string]any)
	actor, ok := review["actor"].(map[string]any)
	if !ok || actor["email"] != agentEmail || actor["fullName"] != "Test AGENT" {
		t.Fatalf("staff should see the agent behind the review: %v", review)
	}

	customerView := customer.do(http.MethodGet, "/api/disputes/"+id, nil)
	expect(t, customerView, http.StatusOK)
	for _, event := range customerView.body["timeline"].([]any) {
		if _, leaked := event.(map[string]any)["actor"]; leaked {
			t.Fatalf("customers must not see staff identities: %v", event)
		}
	}
}

func TestOnlyDeclinedDisputesCanBeRaisedAgain(t *testing.T) {
	e := start(t)
	customer := e.as(customerA)
	agent := e.as(agentEmail)

	disputable := func() bool {
		res := customer.do(http.MethodGet, "/api/transactions", nil)
		expect(t, res, http.StatusOK)
		return res.body["items"].([]any)[0].(map[string]any)["disputable"] == true
	}

	declined := e.createDispute(customer, "again-key-0001")
	expect(t, agent.do(http.MethodPatch, "/api/disputes/"+declined+"/status",
		map[string]any{"status": "REJECTED", "version": 1, "note": "Merchant proved delivery"}), http.StatusOK)
	if !disputable() {
		t.Fatal("a declined dispute can be raised again with new evidence")
	}

	resolved := e.createDispute(customer, "again-key-0002")
	expect(t, agent.do(http.MethodPatch, "/api/disputes/"+resolved+"/status",
		map[string]any{"status": "UNDER_REVIEW", "version": 1}), http.StatusOK)
	expect(t, agent.do(http.MethodPatch, "/api/disputes/"+resolved+"/status",
		map[string]any{"status": "RESOLVED", "version": 2, "note": "Refunded in full"}), http.StatusOK)
	if disputable() {
		t.Fatal("a transaction refunded through a dispute must not be disputable")
	}

	res := customer.do(http.MethodPost, "/api/disputes",
		map[string]string{"transactionId": e.txnID, "reason": "DUPLICATE", "description": "Trying to dispute it again."},
		"Idempotency-Key", "again-key-0003")
	expect(t, res, http.StatusConflict)
	if res.str("code") != "DISPUTE_ALREADY_RESOLVED" {
		t.Fatalf("expected DISPUTE_ALREADY_RESOLVED: %v", res.body)
	}
}
