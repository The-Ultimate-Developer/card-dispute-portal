CREATE TABLE users (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email         citext NOT NULL UNIQUE,
    full_name     text NOT NULL,
    password_hash text NOT NULL,
    role          text NOT NULL CHECK (role IN ('CUSTOMER', 'AGENT', 'SUPPORT')),
    locked_at     timestamptz,
    lock_reason   text,
    created_at    timestamptz NOT NULL DEFAULT now()
);

-- Keyed by the submitted email, not the user, so unknown emails lock out
-- exactly like real ones and responses never reveal which accounts exist.
CREATE TABLE login_failures (
    email          citext PRIMARY KEY,
    failed_count   integer NOT NULL,
    last_failed_at timestamptz NOT NULL
);

CREATE TABLE sessions (
    token_hash   bytea PRIMARY KEY,
    user_id      uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    csrf_token   text NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    expires_at   timestamptz NOT NULL
);
CREATE INDEX sessions_user_id_idx ON sessions (user_id);
CREATE INDEX sessions_expires_at_idx ON sessions (expires_at);

CREATE TABLE auth_events (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    event      text NOT NULL,
    email      citext,
    user_id    uuid REFERENCES users (id),
    actor_id   uuid REFERENCES users (id),
    ip         text,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX auth_events_user_id_idx ON auth_events (user_id, created_at);

-- Stand-in for the core banking system. Disputes keep their own snapshot of
-- the transaction and deliberately have no foreign key into these tables.
CREATE TABLE cards (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id uuid NOT NULL REFERENCES users (id),
    brand       text NOT NULL,
    last4       char(4) NOT NULL CHECK (last4 ~ '^[0-9]{4}$'),
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX cards_customer_id_idx ON cards (customer_id);

CREATE TABLE transactions (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    card_id      uuid NOT NULL REFERENCES cards (id),
    merchant     text NOT NULL,
    category     text NOT NULL,
    amount_cents bigint NOT NULL CHECK (amount_cents > 0),
    currency     char(3) NOT NULL DEFAULT 'ZAR',
    status       text NOT NULL CHECK (status IN ('PENDING', 'POSTED')),
    occurred_at  timestamptz NOT NULL
);
CREATE INDEX transactions_card_occurred_idx ON transactions (card_id, occurred_at DESC, id DESC);

CREATE TABLE disputes (
    id                      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id             uuid NOT NULL REFERENCES users (id),
    transaction_id          uuid NOT NULL,
    merchant                text NOT NULL,
    amount_cents            bigint NOT NULL,
    currency                char(3) NOT NULL,
    card_last4              char(4) NOT NULL,
    transaction_occurred_at timestamptz NOT NULL,
    reason                  text NOT NULL CHECK (reason IN (
                                'UNRECOGNISED', 'DUPLICATE', 'INCORRECT_AMOUNT',
                                'NOT_RECEIVED', 'CANCELLED_SUBSCRIPTION', 'OTHER')),
    description             text NOT NULL,
    status                  text NOT NULL CHECK (status IN ('SUBMITTED', 'UNDER_REVIEW', 'RESOLVED', 'REJECTED')),
    version                 integer NOT NULL DEFAULT 1,
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now()
);

-- The last line of defence against duplicate disputes, even if application
-- logic has a bug or two requests race.
CREATE UNIQUE INDEX disputes_one_active_per_transaction
    ON disputes (transaction_id) WHERE status IN ('SUBMITTED', 'UNDER_REVIEW');
CREATE INDEX disputes_customer_created_idx ON disputes (customer_id, created_at DESC, id DESC);
CREATE INDEX disputes_status_created_idx ON disputes (status, created_at DESC, id DESC);

CREATE TABLE dispute_events (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    dispute_id  uuid NOT NULL REFERENCES disputes (id),
    from_status text,
    to_status   text NOT NULL,
    actor_id    uuid NOT NULL REFERENCES users (id),
    actor_role  text NOT NULL,
    note        text NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX dispute_events_dispute_id_idx ON dispute_events (dispute_id, id);

CREATE TABLE idempotency_keys (
    user_id      uuid NOT NULL REFERENCES users (id),
    key          text NOT NULL CHECK (length(key) BETWEEN 8 AND 128),
    request_hash bytea NOT NULL,
    dispute_id   uuid REFERENCES disputes (id),
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, key)
);

-- Audit tables are append-only, enforced by the database rather than by convention.
CREATE FUNCTION forbid_mutation() RETURNS trigger
    LANGUAGE plpgsql AS
$$
BEGIN
    RAISE EXCEPTION '% is append-only', TG_TABLE_NAME USING ERRCODE = 'insufficient_privilege';
END;
$$;

CREATE TRIGGER dispute_events_append_only
    BEFORE UPDATE OR DELETE ON dispute_events
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();
CREATE TRIGGER dispute_events_no_truncate
    BEFORE TRUNCATE ON dispute_events
    FOR EACH STATEMENT EXECUTE FUNCTION forbid_mutation();

CREATE TRIGGER auth_events_append_only
    BEFORE UPDATE OR DELETE ON auth_events
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();
CREATE TRIGGER auth_events_no_truncate
    BEFORE TRUNCATE ON auth_events
    FOR EACH STATEMENT EXECUTE FUNCTION forbid_mutation();
