ALTER TABLE users DROP CONSTRAINT users_role_check;
ALTER TABLE users ADD CONSTRAINT users_role_check CHECK (role IN ('CUSTOMER', 'AGENT', 'SUPPORT', 'ADMIN'));

CREATE INDEX auth_events_event_created_idx ON auth_events (event, created_at);
