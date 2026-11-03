# Card Dispute Portal

A secure portal for customers to view their card transactions, dispute them, and follow each dispute on a status timeline, with email updates at every step. Back-office staff use the same API with their own logins: agents work the dispute queue, support unlocks accounts, and admins oversee everything.

| | |
|---|---|
| API | Go (standard library `net/http`), pgx, golang-migrate, Prometheus client |
| Database | PostgreSQL 16 |
| Frontend | React + TypeScript (in `frontend/`) |
| CI/CD | GitHub Actions → GitHub Container Registry → Render |
| API docs | Swagger UI at `/docs`; contract in [`backend/api/openapi.yaml`](backend/api/openapi.yaml) |
| Operations | [`docs/runbook.md`](docs/runbook.md): environment, deploy, verify, roll back |

## Run it

Requires Docker.

```bash
docker compose up --build
```

- API: http://localhost:8080
- Swagger UI: http://localhost:8080/docs

Demo data is seeded on first start. Every account uses the password **`Demo@2026!`**:

| Email | Role | What to try |
|---|---|---|
| `thandi@demo.capitec.local` | Customer | Browse transactions, dispute one, follow its timeline |
| `sipho@demo.capitec.local` | Customer | Confirm you can't see Thandi's disputes |
| `agent@demo.capitec.local` | Agent | Move disputes through review to resolved or rejected |
| `support@demo.capitec.local` | Support | Unlock a locked account (fail Sipho's login 3 times first) |
| `admin@demo.capitec.local` | Admin | Overview, audit log, notification delivery, plus everything above |

Thandi starts with three disputes (submitted, under review, resolved), so every view has data immediately.

## Develop and test

Requires Go 1.26+.

```bash
cd backend
cp .env.example .env
make db                 # Postgres only, in Docker
make run                # API on :8080; migrates and seeds on start
make test               # unit tests, race detector on
make test-integration   # everything, against real Postgres
```

Each integration test creates and drops its own database, so tests are isolated and run in parallel.

## API

| Method | Path | Who | Purpose |
|---|---|---|---|
| POST | `/api/auth/login` | Anyone | Sign in; sets the session cookie, returns a CSRF token |
| POST | `/api/auth/logout` | Signed in | Revoke the session |
| GET | `/api/auth/me` | Signed in | Current user and CSRF token |
| GET | `/api/transactions` | Customer | Own transactions, newest first, with dispute status and disputability |
| POST | `/api/disputes` | Customer | Dispute a transaction (requires `Idempotency-Key`) |
| GET | `/api/disputes` | Customer, Agent, Admin | Own disputes, or the full queue with customer details for staff |
| GET | `/api/disputes/{id}` | Customer, Agent, Admin | Dispute with its status timeline |
| PATCH | `/api/disputes/{id}/status` | Agent, Admin | Move a dispute through its lifecycle |
| GET | `/api/support/locked-accounts` | Support, Admin | Locked accounts |
| POST | `/api/support/accounts/{id}/unlock` | Support, Admin | Unlock an account |
| GET | `/api/admin/overview` | Admin | Disputes, accounts and notification health at a glance |
| GET | `/api/admin/audit-events` | Admin | Security audit log |
| GET | `/api/admin/notifications` | Admin | Email delivery state (never content) |
| GET | `/livez`, `/readyz`, `/version` | Anyone | Liveness, readiness, build info |
| GET | `/metrics` | Bearer token | Prometheus metrics |

Every error is `application/problem+json` with a stable `code`, such as `ACTIVE_DISPUTE_EXISTS`. The full policy (route, roles, rate limit) is one table in `internal/httpapi/routes.go`.

## Design highlights

**Security**
- Server-side sessions in an httpOnly, `Secure`, `SameSite=Strict`, `__Host-` cookie; only a SHA-256 of the token is stored, so sessions can be revoked instantly.
- CSRF token bound to the session on every write, and JSON-only request bodies.
- argon2id password hashing.
- Lockout after 3 failed sign-ins, counted atomically *before* the password is checked, so parallel guesses can't exceed the limit. Unknown emails behave identically, so responses never reveal which accounts exist. Locking revokes sessions and emails the customer.
- Per-IP login rate limit and per-user write limit, using the real client IP behind Render's proxies.
- Customers only see their own data; another customer's dispute is 404, not 403.
- No card numbers or personal data in logs (email addresses are masked); responses are `no-store`.

**Correctness**
- Dispute lifecycle: `SUBMITTED → UNDER_REVIEW or REJECTED`, `UNDER_REVIEW → RESOLVED or REJECTED`; final states have no exits.
- Idempotent dispute creation; one open dispute per transaction enforced by a partial unique index; optimistic locking on status changes.
- Dispute timeline and auth audit log are append-only, enforced by database triggers.

**Reliability**
- Notifications go through a **transactional outbox**: written in the same transaction as the change, delivered by a worker with retries, backoff, a dead-letter state and ordering. Several instances can run safely.
- Core banking sits behind a **timeout and circuit breaker**. The mock can inject latency and errors (`MOCK_CORE_BANKING_*`) to demonstrate both live.
- Fail-fast config validation; separate liveness and readiness probes; graceful shutdown that drains requests, then stops workers.

**Observability**
- Structured JSON logs with a request ID, route and commit SHA on every line.
- Prometheus metrics for requests and latency per route, logins, dispute flow, notification delivery, circuit state and database pool.

**Quality gates (CI)**
- golangci-lint with security linters, a `go mod tidy` check, tests with the race detector against real Postgres, a coverage floor, govulncheck, and a Trivy image scan.
- Tests prove the access policy for every endpoint and every role, that every endpoint is in the OpenAPI contract (and vice versa), and that every migration rolls back cleanly.

## Failure modes

| What fails | What the user sees | Why it is safe |
|---|---|---|
| Core banking slow or down | "Can't load transactions right now" (503); disputes still load | Timeout frees the request; circuit breaker fails fast and probes for recovery |
| Postgres down | 503; readiness fails | Fails closed: no dispute is accepted that can't be stored |
| Email provider down | Nothing; the dispute is saved | Outbox retries with backoff; exhausted messages are visible to admins |
| Instance crashes mid-send | Nothing | The message's lease expires and another worker delivers it |
| Double-click or network retry | The same dispute | Idempotency key plus unique index |
| Two agents update one dispute | The second gets "changed by someone else" | Optimistic locking |
| Password guessing | Lockout after 3 tries, then 429s | Atomic attempt counter plus rate limit |

## Production roadmap

Deliberately out of scope, with the seams in place:

- **Email provider:** SES or SendGrid as a new `notify.Sender`.
- **Scale-out:** rate limits in Redis. Sessions, lockout and the outbox already live in Postgres, so they're correct across instances.
- **Tracing:** OpenTelemetry spans across HTTP, database and core banking calls.
- **Database:** HA standby with automatic failover, point-in-time recovery, read replicas.
- **Identity:** a real IdP with MFA; self-service unlock via OTP.
- **Back office:** a separate staff application on an internal network behind SSO, with four-eyes approval on sensitive actions.
- **Edge:** WAF and bot protection.

Architecture decisions are recorded in [`docs/adr`](docs/adr).
