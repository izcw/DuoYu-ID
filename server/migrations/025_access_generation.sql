-- Project-issued credentials retain this generation across ordinary device logouts.
ALTER TABLE app_users ADD COLUMN access_generation bigint NOT NULL DEFAULT 0;

CREATE FUNCTION advance_account_generation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 UPDATE app_users SET access_generation=access_generation+1 WHERE user_id=NEW.id;
 RETURN NEW;
END $$;
CREATE TRIGGER access_generation_user AFTER UPDATE OF enabled ON users FOR EACH ROW
WHEN (OLD.enabled AND NOT NEW.enabled) EXECUTE FUNCTION advance_account_generation();

CREATE FUNCTION advance_project_generation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 UPDATE app_users au SET access_generation=access_generation+1 WHERE au.app_id=NEW.id
 AND (NOT NEW.enabled OR NOT EXISTS(SELECT 1 FROM grants g WHERE g.app_id=au.app_id AND g.user_id=au.user_id));
 RETURN NEW;
END $$;
CREATE TRIGGER access_generation_app AFTER UPDATE ON apps FOR EACH ROW
WHEN ((OLD.enabled AND NOT NEW.enabled) OR (NOT OLD.restricted AND NEW.restricted))
EXECUTE FUNCTION advance_project_generation();

CREATE FUNCTION advance_grant_generation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF EXISTS(SELECT 1 FROM apps WHERE id=OLD.app_id AND restricted) THEN
  UPDATE app_users SET access_generation=access_generation+1 WHERE app_id=OLD.app_id AND user_id=OLD.user_id;
 END IF;
 RETURN OLD;
END $$;
CREATE TRIGGER access_generation_grant AFTER DELETE ON grants FOR EACH ROW EXECUTE FUNCTION advance_grant_generation();

CREATE FUNCTION advance_blacklist_generation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 UPDATE app_users au SET access_generation=access_generation+1 FROM users u
 WHERE au.user_id=u.id AND au.app_id=NEW.app_id AND (u.id=NEW.user_id OR u.email=NEW.email);
 RETURN NEW;
END $$;
CREATE TRIGGER access_generation_blacklist AFTER INSERT OR UPDATE ON app_blacklist
FOR EACH ROW EXECUTE FUNCTION advance_blacklist_generation();

-- Credentials issued before rollout must also stay revoked after a later re-enable.
UPDATE app_users au SET access_generation=1 FROM users u, apps a
WHERE au.user_id=u.id AND au.app_id=a.id AND (NOT u.enabled OR NOT a.enabled
 OR (a.restricted AND NOT EXISTS(SELECT 1 FROM grants g WHERE g.app_id=a.id AND g.user_id=u.id))
 OR EXISTS(SELECT 1 FROM app_blacklist b WHERE b.app_id=a.id AND (b.user_id=u.id OR b.email=u.email)));
