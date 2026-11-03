# 6. Customer notifications through a transactional outbox

**Status:** Accepted

**Context.** Customers must be told when a dispute is received, when its status changes, and when their account is locked. Sending an email inside the request creates two failure modes: the database commits but the email fails (the customer is never told), or the email sends but the transaction rolls back (the customer is told about something that never happened).

**Decision.** Write an `outbox` row in the **same transaction** as the change. A background worker claims due rows with `FOR UPDATE SKIP LOCKED` and a lease, sends them, and marks them `SENT`. Failures retry with exponential backoff (2s, 4s, 8s … capped at 10 minutes); after `NOTIFY_MAX_ATTEMPTS` a message becomes `DEAD` and shows up in the admin overview. Messages that cannot ever succeed (for example, a deleted record) go straight to `DEAD`.

**Consequences.**
- Delivery is at-least-once. Each email carries a stable ID (`outbox-<id>`) that a real provider uses to drop duplicates.
- If an instance dies mid-send, its lease expires and another instance delivers the message.
- Several instances can poll safely; `SKIP LOCKED` means they never take the same message.
- The email content captures what changed at the time (status, note) from the payload, so a later change cannot rewrite an earlier email.
- Sending is simulated (`LogSender`, recipient masked in logs). A real provider (SES, SendGrid) is a new `Sender`, with no other change. Moving to a message broker later replaces only the worker.
