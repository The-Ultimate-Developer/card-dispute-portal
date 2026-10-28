-- Baseline: extensions the schema depends on.
-- citext = case-insensitive text, used for email so "Jason@x.com" and "jason@x.com" are the same user.
CREATE EXTENSION IF NOT EXISTS citext;
