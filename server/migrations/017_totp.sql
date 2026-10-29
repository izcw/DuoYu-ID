ALTER TABLE apps DROP CONSTRAINT apps_login_methods_check;
ALTER TABLE apps ADD CONSTRAINT apps_login_methods_check CHECK(login_methods <@ ARRAY['password','code','passkey','qr','totp']::text[]);
ALTER TABLE apps ALTER COLUMN login_methods SET DEFAULT ARRAY['password','code','passkey','qr','totp'];
-- Extend only the previous unrestricted policy; preserve restricted and paused projects.
UPDATE apps SET login_methods=array_append(login_methods,'totp') WHERE login_methods @> ARRAY['password','code','passkey','qr']::text[];
CREATE TABLE authenticators (
 user_id text PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
 secret bytea NOT NULL,
 active boolean NOT NULL DEFAULT false,
 setup_session text NOT NULL,
 expires_at timestamptz NOT NULL,
 last_step bigint NOT NULL DEFAULT -1,
 failures integer NOT NULL DEFAULT 0,
 locked_until timestamptz NOT NULL DEFAULT '-infinity',
 recovery_hashes text[] NOT NULL DEFAULT '{}',
 created_at timestamptz NOT NULL DEFAULT now()
);
