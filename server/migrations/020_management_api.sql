ALTER TABLE apps ADD COLUMN management_revision bigint NOT NULL DEFAULT 0;
CREATE TABLE management_api_keys (
 id text PRIMARY KEY, name text NOT NULL, digest text NOT NULL UNIQUE,
 owner_id text NOT NULL REFERENCES users(id), scopes text[] NOT NULL,
 all_projects boolean NOT NULL DEFAULT false, expires_at timestamptz,
 revoked boolean NOT NULL DEFAULT false, created_at timestamptz NOT NULL DEFAULT now(), last_used_at timestamptz,
 CHECK(scopes <@ ARRAY['projects:read','projects:create','projects:update','projects:rotate']::text[])
);
CREATE TABLE management_api_grants (
 key_id text NOT NULL REFERENCES management_api_keys(id) ON DELETE CASCADE,
 app_id text NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
 PRIMARY KEY(key_id,app_id)
);
CREATE TABLE management_create_requests (
 key_id text NOT NULL REFERENCES management_api_keys(id) ON DELETE CASCADE,
 request_key text NOT NULL, body_digest text NOT NULL,
 app_id text NOT NULL REFERENCES apps(id), result_cipher bytea,
 created_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(key_id,request_key)
);
