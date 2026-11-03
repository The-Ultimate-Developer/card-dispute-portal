# 7. Circuit breaker and metrics

**Status:** Accepted

**Context.** Core banking is an external dependency. If it slows down, every request waits for its timeout, holding goroutines and database connections, and the retries add load to a system that is already struggling. Separately, "is it healthy?" needs numbers, not log searches.

**Decision.**
- Calls go through `timeout → circuit breaker`. After `CORE_BANKING_BREAKER_THRESHOLD` consecutive availability failures, the circuit opens and calls fail immediately with a friendly 503 for `CORE_BANKING_BREAKER_OPEN_FOR`. Then one probe is let through: success closes the circuit, failure reopens it. "Not found" is an answer, not a fault, and never trips it.
- Prometheus metrics: requests and latency per **route pattern** (never raw paths, so cardinality stays bounded), login outcomes, dispute status changes, notification delivery, core banking outcomes, circuit state, and database pool usage.
- `/metrics` requires a bearer token and returns 404 otherwise, because the service is on the public internet.

**Consequences.** Outages fail fast and recover on their own. Disputes still load during a core banking outage because they live in our database. Breaker state is per instance, which is the normal trade-off; the metric shows each instance's view.
