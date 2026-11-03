# 1. One repository for API and frontend

**Status:** Accepted

**Context.** One team ships one product, and the API contract (`openapi.yaml`) is shared by both sides.

**Decision.** Keep `backend/` and `frontend/` in one repository, with path-aware CI and CODEOWNERS per area.

**Consequences.** Contract changes land atomically with the code on both sides, and reviewers run everything with one `docker compose up`. If separate teams with separate release cadences owned each side, we would split the repositories and publish the OpenAPI contract as a versioned artifact.
