CREATE TABLE app_blacklist (
 app_id text NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
 identifier text NOT NULL,
 email text NOT NULL DEFAULT '',
 user_id text REFERENCES users(id) ON DELETE SET NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(app_id,identifier)
);
CREATE INDEX app_blacklist_user ON app_blacklist(user_id);
CREATE INDEX app_blacklist_email ON app_blacklist(email);
CREATE FUNCTION revoke_blacklisted_project() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE uid text;
BEGIN
 FOR uid IN SELECT id FROM users WHERE id=NEW.user_id OR email=NEW.email LOOP
  PERFORM enqueue_logout_events(uid,NEW.app_id,NULL);
  UPDATE refresh_families SET revoked=true WHERE app_id=NEW.app_id AND user_id=uid;
  DELETE FROM access_tokens WHERE app_id=NEW.app_id AND user_id=uid;
  DELETE FROM auth_requests WHERE body->'Request'->>'client_id'=NEW.app_id AND body->>'UserID'=uid;
 END LOOP;
 RETURN NEW;
END; $$;
CREATE TRIGGER blacklist_revoke AFTER INSERT OR UPDATE ON app_blacklist FOR EACH ROW EXECUTE FUNCTION revoke_blacklisted_project();
-- Retain account identity when an email-blocked user registers elsewhere or changes email.
CREATE FUNCTION bind_blacklisted_email() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 UPDATE app_blacklist SET user_id=NEW.id WHERE email=NEW.email AND user_id IS NULL;
 RETURN NEW;
END; $$;
CREATE TRIGGER blacklist_bind AFTER INSERT OR UPDATE OF email ON users FOR EACH ROW EXECUTE FUNCTION bind_blacklisted_email();
