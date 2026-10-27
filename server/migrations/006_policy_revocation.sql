-- Policy revocation is durable: re-enabling a project must not revive old tokens.
CREATE OR REPLACE FUNCTION logout_on_app_change() RETURNS trigger LANGUAGE plpgsql AS $$
 BEGIN
 IF OLD.enabled AND NOT NEW.enabled THEN
  PERFORM enqueue_logout_events(NULL,NEW.id,NULL);
  UPDATE refresh_families SET revoked=true WHERE app_id=NEW.id;
  DELETE FROM access_tokens WHERE app_id=NEW.id;
  DELETE FROM auth_requests WHERE body->'Request'->>'client_id'=NEW.id;
 ELSIF NOT OLD.restricted AND NEW.restricted THEN
  INSERT INTO logout_jobs(app_id,user_id,session_hash,endpoint)
  SELECT DISTINCT NEW.id,t.user_id,t.session_hash,NEW.backchannel_logout_uri FROM access_tokens t
  WHERE t.app_id=NEW.id AND t.session_hash IS NOT NULL AND NEW.backchannel_logout_uri<>''
  AND NOT EXISTS(SELECT 1 FROM grants g WHERE g.app_id=NEW.id AND g.user_id=t.user_id);
  UPDATE refresh_families f SET revoked=true WHERE f.app_id=NEW.id AND NOT EXISTS(SELECT 1 FROM grants g WHERE g.app_id=NEW.id AND g.user_id=f.user_id);
  DELETE FROM access_tokens t WHERE t.app_id=NEW.id AND NOT EXISTS(SELECT 1 FROM grants g WHERE g.app_id=NEW.id AND g.user_id=t.user_id);
 END IF;
 RETURN NEW; END;
$$;
CREATE OR REPLACE FUNCTION logout_on_grant_delete() RETURNS trigger LANGUAGE plpgsql AS $$
 BEGIN
 IF EXISTS(SELECT 1 FROM apps WHERE id=OLD.app_id AND restricted) THEN
  PERFORM enqueue_logout_events(OLD.user_id,OLD.app_id,NULL);
  UPDATE refresh_families SET revoked=true WHERE app_id=OLD.app_id AND user_id=OLD.user_id;
  DELETE FROM access_tokens WHERE app_id=OLD.app_id AND user_id=OLD.user_id;
  DELETE FROM auth_requests WHERE body->'Request'->>'client_id'=OLD.app_id AND body->>'UserID'=OLD.user_id;
 END IF;
 RETURN OLD; END;
$$;
