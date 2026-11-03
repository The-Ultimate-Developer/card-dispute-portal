# 3. Account lockout after three failed sign-ins

**Status:** Accepted

**Context.** The business wants accounts locked after three failed attempts, unlocked only by support. Naive lockout has two flaws: parallel requests can all read "2 failures" and each get a free guess, and different responses for unknown emails reveal which accounts exist.

**Decision.**
- Count the attempt atomically (`INSERT ... ON CONFLICT DO UPDATE ... RETURNING`) **before** checking the password, so concurrent requests serialise on the row and at most three passwords are ever evaluated.
- Key the counter on the submitted email, not the user, so unknown emails lock out identically. Unknown emails also run a dummy argon2 verification so timing matches.
- A per-IP rate limit sits in front, so an attacker cannot cheaply lock out many customers.
- Locking revokes all existing sessions. Every lock, unlock and attempt is written to an append-only audit table.

**Consequences.** The threshold (3) is configuration, not code. Support handles unlocks; self-service unlock via OTP is on the roadmap.
