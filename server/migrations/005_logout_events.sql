ALTER TABLE apps ADD COLUMN backchannel_logout_uri text NOT NULL DEFAULT '';
CREATE TABLE logout_jobs (
 id bigserial PRIMARY KEY, app_id text NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
 user_id text NOT NULL, session_hash text NOT NULL, endpoint text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(), next_attempt timestamptz NOT NULL DEFAULT now(),
 attempts integer NOT NULL DEFAULT 0, status text NOT NULL DEFAULT 'queued'
);
CREATE INDEX logout_jobs_pending ON logout_jobs(next_attempt) WHERE status='queued';
CREATE FUNCTION enqueue_logout_events(uid text, aid text, sid text) RETURNS void LANGUAGE sql AS $$
 INSERT INTO logout_jobs(app_id,user_id,session_hash,endpoint)
 SELECT DISTINCT t.app_id,t.user_id,t.session_hash,a.backchannel_logout_uri
 FROM access_tokens t JOIN apps a ON a.id=t.app_id
 WHERE t.session_hash IS NOT NULL AND a.backchannel_logout_uri<>''
 AND (uid IS NULL OR t.user_id=uid) AND (aid IS NULL OR t.app_id=aid) AND (sid IS NULL OR t.session_hash=sid);
$$;
CREATE FUNCTION logout_on_session_delete() RETURNS trigger LANGUAGE plpgsql AS $$
 BEGIN PERFORM enqueue_logout_events(OLD.user_id,NULL,OLD.hash); RETURN OLD; END;
$$;
CREATE TRIGGER logout_session BEFORE DELETE ON sessions FOR EACH ROW EXECUTE FUNCTION logout_on_session_delete();
CREATE FUNCTION logout_on_user_disable() RETURNS trigger LANGUAGE plpgsql AS $$
 BEGIN IF OLD.enabled AND NOT NEW.enabled THEN PERFORM enqueue_logout_events(NEW.id,NULL,NULL); END IF; RETURN NEW; END;
$$;
CREATE TRIGGER logout_user BEFORE UPDATE ON users FOR EACH ROW EXECUTE FUNCTION logout_on_user_disable();
CREATE FUNCTION logout_on_app_change() RETURNS trigger LANGUAGE plpgsql AS $$
 BEGIN IF OLD.enabled AND NOT NEW.enabled THEN PERFORM enqueue_logout_events(NULL,NEW.id,NULL);
 ELSIF NOT OLD.restricted AND NEW.restricted THEN
 INSERT INTO logout_jobs(app_id,user_id,session_hash,endpoint)
 SELECT DISTINCT NEW.id,t.user_id,t.session_hash,NEW.backchannel_logout_uri FROM access_tokens t
 WHERE t.app_id=NEW.id AND t.session_hash IS NOT NULL AND NEW.backchannel_logout_uri<>''
 AND NOT EXISTS(SELECT 1 FROM grants g WHERE g.app_id=NEW.id AND g.user_id=t.user_id);
 END IF; RETURN NEW; END;
$$;
CREATE TRIGGER logout_app BEFORE UPDATE ON apps FOR EACH ROW EXECUTE FUNCTION logout_on_app_change();
CREATE FUNCTION logout_on_grant_delete() RETURNS trigger LANGUAGE plpgsql AS $$
 BEGIN IF EXISTS(SELECT 1 FROM apps WHERE id=OLD.app_id AND restricted) THEN PERFORM enqueue_logout_events(OLD.user_id,OLD.app_id,NULL); END IF; RETURN OLD; END;
$$;
CREATE TRIGGER logout_grant BEFORE DELETE ON grants FOR EACH ROW EXECUTE FUNCTION logout_on_grant_delete();
