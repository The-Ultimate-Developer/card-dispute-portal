-- Messages written in the same transaction as the change that caused them,
-- then delivered by a background worker. A crash between "save" and "send"
-- can never lose or invent a notification.
CREATE TABLE outbox (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    topic        text NOT NULL,
    payload      jsonb NOT NULL,
    status       text NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING', 'SENT', 'DEAD')),
    attempts     integer NOT NULL DEFAULT 0,
    available_at timestamptz NOT NULL DEFAULT now(),
    last_error   text,
    created_at   timestamptz NOT NULL DEFAULT now(),
    sent_at      timestamptz
);

CREATE INDEX outbox_due_idx ON outbox (available_at, id) WHERE status = 'PENDING';
CREATE INDEX outbox_status_id_idx ON outbox (status, id DESC);
