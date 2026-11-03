# 2. Server-side sessions instead of JWTs

**Status:** Accepted

**Context.** A banking portal must be able to end a session immediately: on logout, on account lockout, or on suspected compromise.

**Decision.** Opaque random tokens in an httpOnly, Secure, SameSite=Strict `__Host-` cookie. Only the token's SHA-256 is stored. Idle timeout 15 minutes, absolute lifetime 8 hours. CSRF is handled with a per-session synchronizer token sent in `X-CSRF-Token`.

**Consequences.** Revocation is a single `DELETE`, and a database leak does not expose live sessions. Every request costs one indexed query; at higher scale, sessions would move to a cache such as Redis. JWT revocation would need a denylist, which is server-side state anyway.
