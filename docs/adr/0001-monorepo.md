# 1. One repository for API and frontend

**Status:** Accepted

**Context.** The brief asks for one GitHub link. One team ships one product, and the API contract (`openapi.yaml`) is shared by both sides.

**Decision.** Keep `backend/` and `frontend/` in one repository (a monorepo, not a monolith: they remain two separately built and deployed applications). CODEOWNERS assigns reviewers per area. CI runs every job on every change and deploys both on every merge to `main`, which keeps `main` always fully released.

**Consequences.** Contract changes land atomically with the code on both sides, CI fails if the frontend's generated types drift from the contract, and reviewers run everything with one `docker compose up`. The cost is that a frontend-only change also rebuilds and redeploys the API. The next step would be path-filtered CI. If separate teams with separate release cadences owned each side, we would split the repositories and publish the OpenAPI contract as a versioned artifact.
