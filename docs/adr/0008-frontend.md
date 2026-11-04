# 8. Frontend: a same-origin React SPA

**Status:** Accepted

**Context.** Customers and staff need a browser UI over the existing API. The API already uses a first-party session cookie with `SameSite=Strict` and a CSRF token, and the brief asks for a production-grade, testable build.

**Decision.**
- **React + TypeScript (strict), built with Vite.** A single-page app with React Router; one codebase for customers and staff, with navigation and route guards driven by one role table (`src/auth/access.ts`) that mirrors the API's policy.
- **Same origin everywhere.** The browser only ever calls `/api` on the site it loaded from: the Vite dev proxy locally, nginx in Docker, and a Netlify proxy rule in production. The cookie stays first-party, no CORS is configured, and the CSP can be `connect-src 'self'`.
- **Server state in TanStack Query**, not a global store: caching, deduplication, retries only for network errors and 5xx, and targeted invalidation after each write. The only client state is the CSRF token, held in memory.
- **Types generated from `openapi.yaml`** (`openapi-typescript`). CI regenerates them and fails if they differ from the committed file, so the UI cannot drift from the contract.
- **Forms with React Hook Form + zod**, using the same limits as the API; the API's field errors are mapped back onto the form because the server stays the source of truth.
- **Strict CSP with no inline script or style**: fonts are self-hosted, and dynamic sizes are set through the CSSOM, which CSP allows.
- **Tests with Vitest, Testing Library and MSW**, rendering the real route tree; any request without a handler fails the test.

**Consequences.** One deployable static bundle with no server runtime; Netlify adds one proxy hop, so the API's `TRUSTED_PROXY_HOPS` must count it. Route guards are UX only: the API enforces every permission. Staff and customers share a bundle; in production the back office would be a separate app on an internal network (see the roadmap).
