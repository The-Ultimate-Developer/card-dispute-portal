DROP INDEX IF EXISTS auth_events_event_created_idx;
ALTER TABLE users DROP CONSTRAINT users_role_check;
ALTER TABLE users ADD CONSTRAINT users_role_check CHECK (role IN ('CUSTOMER', 'AGENT', 'SUPPORT'));
