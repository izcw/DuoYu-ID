ALTER TABLE apps ADD COLUMN login_methods text[] NOT NULL DEFAULT ARRAY['password','code','passkey','qr'];
ALTER TABLE apps ADD CONSTRAINT apps_login_methods_check CHECK(login_methods <@ ARRAY['password','code','passkey','qr']::text[]);
