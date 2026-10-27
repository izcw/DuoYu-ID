ALTER TABLE apps ADD COLUMN refresh_enabled boolean NOT NULL DEFAULT false;
ALTER TABLE apps ADD COLUMN platform text NOT NULL DEFAULT 'web' CHECK(platform IN ('web','spa','native'));
UPDATE apps SET platform='spa' WHERE public;
CREATE TABLE refresh_families (
 id text PRIMARY KEY, user_id text NOT NULL REFERENCES users(id),
 app_id text NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
 session_hash text NOT NULL REFERENCES sessions(hash) ON DELETE CASCADE,
 expires_at timestamptz NOT NULL, revoked boolean NOT NULL DEFAULT false
);
CREATE TABLE refresh_tokens (
 hash text PRIMARY KEY, family_id text NOT NULL REFERENCES refresh_families(id) ON DELETE CASCADE,
 body jsonb NOT NULL, used boolean NOT NULL DEFAULT false
);
CREATE INDEX refresh_tokens_family ON refresh_tokens(family_id);
ALTER TABLE access_tokens ADD COLUMN family_id text REFERENCES refresh_families(id) ON DELETE CASCADE;
