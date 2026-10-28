# Card Dispute Portal

A secure portal for customers to view card transactions, submit disputes and follow each dispute's status on a timeline.

> Status: backend foundation (config, logging, Postgres, migrations, health checks, CI/CD). Auth, transactions and disputes are in progress.

## Stack

| Layer | Choice |
|---|---|
| API | Go (standard library `net/http`), pgx, golang-migrate |
| Database | PostgreSQL 16 |
| Frontend | React + TypeScript (coming) |
| CI/CD | GitHub Actions → GitHub Container Registry → Render |

## Run everything locally

Requires Docker.

```bash
docker compose up --build
```

- API: http://localhost:8080
- Health: `curl localhost:8080/readyz`
- Build info: `curl localhost:8080/version`

## Develop the API without Docker

Requires Go 1.26+.

```bash
cd backend
cp .env.example .env
make db          # starts only Postgres in Docker
make run         # runs the API, applies migrations on start-up
```

## Test

```bash
cd backend
make test              # unit tests (race detector)
make test-integration  # also runs tests against Postgres (needs: make db)
```

## Endpoints so far

| Method | Path | Purpose |
|---|---|---|
| GET | `/livez` | Liveness: is the process alive (never checks dependencies) |
| GET | `/readyz` | Readiness: can it serve traffic (checks the database) |
| GET | `/version` | Commit SHA and build time of the running binary |

All errors use RFC 7807 `application/problem+json` and include the request ID.

## Pipeline

On every pull request: lint → tests (with a real Postgres) → dependency vulnerability scan → image build → image scan.
On merge to `main`: the same, then push the image to GHCR, deploy that exact image to Render by digest, and smoke-test until `/version` reports the new commit.
