# 4. Dispute integrity: idempotency, one open dispute, optimistic locking

**Status:** Accepted

**Context.** Customers double-click and networks retry. Two agents can open the same dispute. The timeline is a customer-facing record and an audit trail.

**Decision.**
- `POST /api/disputes` requires an `Idempotency-Key`. The key is claimed in the same transaction that creates the dispute; a replay returns the original, and reusing a key with a different body is refused.
- A partial unique index allows one open (`SUBMITTED`/`UNDER_REVIEW`) dispute per transaction. This holds even if application logic is wrong.
- Status changes carry the `version` the agent last saw. The row is locked, the version compared, and the state machine checked before writing.
- `dispute_events` is append-only, enforced by a trigger.
- A dispute stores a snapshot of the transaction (merchant, amount, card) and has no foreign key into core-banking data, as it would not in production.

**Consequences.** Correctness is guaranteed by the database under concurrency, and is covered by concurrent integration tests.
