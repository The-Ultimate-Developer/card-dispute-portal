# 5. Back-office access through roles on the same API

**Status:** Accepted

**Context.** Staff need to see and action every dispute. In production, a back office would usually be a separate application on an internal network, behind SSO with MFA.

**Decision.** For this assessment, staff use the same API with their own logins and four roles:
- `AGENT`: work the dispute queue.
- `SUPPORT`: unlock accounts.
- `ADMIN`: both of the above, plus a read-only system overview and the security audit log.

Every route's allowed roles are declared in one file (`internal/httpapi/routes.go`). Staff responses include the customer's name and email; customer responses never do. Every staff action is attributed in the dispute timeline or the auth audit log.

**Consequences.** Simple to run and review. Agents and support stay separate, so the person resolving a dispute isn't the person controlling account access (separation of duties); the admin role is a deliberate superuser for oversight. Splitting into a separate staff app later means moving routes, not rewriting rules, because authorisation already lives in the services.
