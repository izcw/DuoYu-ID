CREATE TABLE IF NOT EXISTS users (
 id text PRIMARY KEY, email text UNIQUE NOT NULL, name text NOT NULL,
 password_hash text NOT NULL DEFAULT '', avatar text NOT NULL DEFAULT '',
 admin boolean NOT NULL DEFAULT false, enabled boolean NOT NULL DEFAULT true,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS sessions (
 hash text PRIMARY KEY, user_id text NOT NULL REFERENCES users(id),
 created_at timestamptz NOT NULL DEFAULT now(), expires_at timestamptz NOT NULL,
 agent text NOT NULL DEFAULT '', amr text NOT NULL DEFAULT 'pwd'
);
CREATE TABLE IF NOT EXISTS challenges (
 email text NOT NULL, purpose text NOT NULL, digest text NOT NULL,
 attempts integer NOT NULL DEFAULT 0, expires_at timestamptz NOT NULL,
 sent_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(email,purpose)
);
CREATE TABLE IF NOT EXISTS apps (
 id text PRIMARY KEY, name text NOT NULL, description text NOT NULL DEFAULT '',
 secret_hash text NOT NULL DEFAULT '', redirects text[] NOT NULL,
 logout_redirects text[] NOT NULL DEFAULT '{}', public boolean NOT NULL DEFAULT false,
 enabled boolean NOT NULL DEFAULT true, restricted boolean NOT NULL DEFAULT false,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS grants (
 app_id text REFERENCES apps(id) ON DELETE CASCADE,
 user_id text REFERENCES users(id) ON DELETE CASCADE, PRIMARY KEY(app_id,user_id)
);
CREATE TABLE IF NOT EXISTS auth_requests (
 id text PRIMARY KEY, body jsonb NOT NULL, code_hash text UNIQUE,
 expires_at timestamptz NOT NULL
);
CREATE TABLE IF NOT EXISTS access_tokens (
 id text PRIMARY KEY, user_id text NOT NULL REFERENCES users(id),
 app_id text NOT NULL REFERENCES apps(id), scopes text[] NOT NULL,
 expires_at timestamptz NOT NULL
);
CREATE TABLE IF NOT EXISTS audit (
 id bigserial PRIMARY KEY, user_id text NOT NULL DEFAULT '',
 action text NOT NULL, target text NOT NULL DEFAULT '', success boolean NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS api_keys (
 id text PRIMARY KEY, name text NOT NULL, digest text UNIQUE NOT NULL,
 app_id text NOT NULL REFERENCES apps(id), scopes text[] NOT NULL,
 expires_at timestamptz NOT NULL, revoked boolean NOT NULL DEFAULT false,
 created_at timestamptz NOT NULL DEFAULT now(), last_used_at timestamptz
);
CREATE TABLE IF NOT EXISTS settings (key text PRIMARY KEY, value text NOT NULL);
ALTER TABLE api_keys ALTER COLUMN expires_at DROP NOT NULL;
INSERT INTO settings(key,value) VALUES('registration','true') ON CONFLICT DO NOTHING;
CREATE TABLE IF NOT EXISTS passkeys (
 id text PRIMARY KEY, user_id text NOT NULL REFERENCES users(id),
 name text NOT NULL, credential jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS webauthn_challenges (
 id text PRIMARY KEY, user_id text NOT NULL, purpose text NOT NULL,
 data jsonb NOT NULL, expires_at timestamptz NOT NULL
);
CREATE TABLE IF NOT EXISTS rate_limits (
 key text PRIMARY KEY, count integer NOT NULL, until_at timestamptz NOT NULL
);
CREATE INDEX IF NOT EXISTS sessions_user ON sessions(user_id);
CREATE INDEX IF NOT EXISTS audit_created ON audit(created_at DESC);
CREATE TABLE IF NOT EXISTS mail_messages (
 id text PRIMARY KEY, app_id text NOT NULL REFERENCES apps(id), key_id text NOT NULL REFERENCES api_keys(id),
 idempotency_hash text NOT NULL, content_hash text NOT NULL,
 recipient text NOT NULL, category text NOT NULL, format text NOT NULL,
 status text NOT NULL DEFAULT 'sending', error_code text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(app_id,idempotency_hash)
);
CREATE INDEX IF NOT EXISTS mail_messages_created ON mail_messages(created_at DESC);
CREATE TABLE IF NOT EXISTS qr_logins (
 id text PRIMARY KEY, owner_hash text NOT NULL, ticket_hash text UNIQUE NOT NULL,
 label text NOT NULL, status text NOT NULL DEFAULT 'pending',
 user_id text, approver_session text,
 created_at timestamptz NOT NULL DEFAULT now(), expires_at timestamptz NOT NULL
);
CREATE INDEX IF NOT EXISTS qr_logins_expiry ON qr_logins(expires_at);
CREATE TABLE IF NOT EXISTS app_users (
 app_id text NOT NULL REFERENCES apps(id), user_id text NOT NULL REFERENCES users(id),
 first_login_at timestamptz NOT NULL DEFAULT now(), last_login_at timestamptz NOT NULL DEFAULT now(),
 login_count bigint NOT NULL DEFAULT 1, PRIMARY KEY(app_id,user_id)
);
