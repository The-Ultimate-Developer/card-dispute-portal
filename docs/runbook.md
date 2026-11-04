# Runbook: deploy and operate the portal

## How a change reaches production

```
pull request ─► lint · tidy check · tests on real Postgres · coverage floor · govulncheck ─► image build ─► image scan
                └► frontend: contract check · type check · lint · tests · npm audit · build
merge to main ─► all of the above ─► push image to GHCR ─► Render deploys that exact digest ─► smoke test until /version = commit
                                                         └► Netlify deploys the tested build ─► smoke test through the /api proxy
```

The API deploys before the web app. API changes only ever add fields, so a new frontend never meets an older API.

Nothing is built or deployed by hand. The image that was tested and scanned is byte-for-byte the image that runs.

## Render environment variables

Set these on the **web service** (Render → service → Environment).

| Variable | Value | Why |
|---|---|---|
| `APP_ENV` | `production` | Enables secure cookies and production checks |
| `DATABASE_URL` | Render Postgres **Internal Database URL** | Private network only; the database is never exposed |
| `TRUSTED_PROXY_HOPS` | `3` once the web app is on Netlify (`2` for the API alone) | Render places Cloudflare in front of every service, so direct calls arrive as `client, cloudflare`. Browser calls also pass through Netlify's proxy, which adds one more entry. Confirm with the client IP check below. |
| `METRICS_TOKEN` | output of `openssl rand -hex 32` | Protects `/metrics`; without it the endpoint is disabled |
| `SEED_DEMO_DATA` | `true` | Demo accounts and data for reviewers; safe to leave on (idempotent) |
| `LOG_LEVEL` | `info` | |
| `DB_MAX_CONNS` | `10` | Stay well within the Postgres plan's connection limit |

Leave `PORT` unset (Render injects it). `COOKIE_SECURE` defaults to `true` in production. Every other variable has a safe default; see `backend/.env.example`.

**Service settings:** Health Check Path `/readyz`. Region must match the database (Frankfurt), or the internal URL will not resolve.

## GitHub `production` environment

Repo → Settings → Environments → `production`:

- Secret `RENDER_DEPLOY_HOOK_URL`: Render service → Settings → Deploy Hook. Treat it as a password.
- Variable `API_BASE_URL`: `https://<service>.onrender.com`, no trailing slash.
- Secret `NETLIFY_AUTH_TOKEN`: Netlify → User settings → Applications → Personal access tokens.
- Secret `NETLIFY_SITE_ID`: Netlify → Site configuration → Site details → Site ID.
- Variable `FRONTEND_URL`: `https://<site>.netlify.app`, no trailing slash.
- Deployment branches: `main` only.

Each deploy job **fails** if anything it needs is missing, so a deploy that does nothing can never show green.

## Netlify site (one-time)

1. Netlify → Add new project → **Deploy manually**, and drop any folder (the pipeline replaces it on the first run).
2. Site configuration → Build & deploy → **Stop builds**. GitHub Actions builds and tests; Netlify only hosts.
3. Optional: rename the site, then set `FRONTEND_URL` to match.
4. Add the token, site ID and URL above to the `production` environment.

The pipeline writes `_redirects` at deploy time from `API_BASE_URL`:

```
/api/*  https://<service>.onrender.com/api/:splat  200
/*      /index.html                               200
```

The first line makes the API same-origin for the browser; the second lets deep links such as `/disputes/<id>` load the app. Security headers come from `frontend/public/_headers`.

## Deploy a change

1. Branch from an up-to-date `main`, make the change, run the local checks:
   ```bash
   cd backend
   go mod tidy
   make test && golangci-lint run && go run golang.org/x/vuln/cmd/govulncheck@latest ./...
   make db && make test-integration
   ```
2. Push and open a pull request. Wait for `lint`, `test` and `image` to pass.
3. If the change needs new environment variables, add them in Render **before** merging.
4. Merge (squash). Watch the run on `main`; the `deploy` job ends with `Commit <sha> is live and ready`.

## Verify a deploy

```bash
API=https://<service>.onrender.com
curl $API/version        # commit SHA of what is running
curl $API/readyz         # {"status":"ready"}
open $API/docs           # Swagger UI
curl -H "Authorization: Bearer $METRICS_TOKEN" $API/metrics | head
```

```bash
WEB=https://<site>.netlify.app
curl -sI $WEB/ | grep -i content-security-policy
curl -s $WEB/api/auth/me    # 401 problem+json: the proxy reaches the API
```

**Client IP check** (confirms `TRUSTED_PROXY_HOPS` is right): sign in once **through the web app** with a wrong password, then sign in as `admin@demo.capitec.local` and open **Sign-in activity**. The IP column must show your own public IP. If it shows a Cloudflare (104.x, 172.64-71.x) or Netlify address, the hop count is too low; if it is a value you sent in an `X-Forwarded-For` header yourself, it is too high.

## Roll back

API: Render → service → Events → pick the previous deploy → **Rollback**. Web: Netlify → Deploys → pick the previous deploy → **Publish deploy**. Migrations are additive and every down migration is exercised in CI, so the previous image runs against the newer schema.

## Operating signals

| Signal | Where | Action |
|---|---|---|
| `/readyz` failing | Render health checks | Database unreachable; Render stops routing to the instance |
| `corebanking_circuit_open = 1` | `/metrics` | Core banking is failing; transaction views return 503, disputes still load |
| `notificationsFailed > 0` | `GET /api/admin/overview` | Messages exhausted retries; inspect `GET /api/admin/notifications?status=DEAD` |
| `failedLoginsLast24h` spike | Admin overview, audit log | Possible credential stuffing; check IPs in the audit log |
| `db_pool_empty_acquire_total` rising | `/metrics` | Pool too small for load; raise `DB_MAX_CONNS` within the plan's limit |
