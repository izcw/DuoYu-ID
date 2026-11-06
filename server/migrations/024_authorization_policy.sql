-- Revoke affected in-flight authorizations when a project tightens policy.
-- Once removed they cannot be revived by restoring the old configuration.
CREATE FUNCTION revoke_ineligible_authorizations() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 DELETE FROM auth_requests r
 WHERE r.body->'Request'->>'client_id'=NEW.id AND (
   (NOT OLD.restricted AND NEW.restricted AND r.body->>'Completed'='true'
    AND NOT EXISTS (SELECT 1 FROM grants g WHERE g.app_id=NEW.id AND g.user_id=r.body->>'UserID'))
   OR (OLD.refresh_enabled AND NOT NEW.refresh_enabled
       AND 'offline_access'=ANY(string_to_array(r.body->'Request'->>'scope',' ')))
   OR (NOT (OLD.login_methods <@ NEW.login_methods) AND r.body->>'Completed'='true'
       -- Requests created before this migration only carry AMR.
       AND NOT (coalesce(r.body->>'LoginMethod',(
         SELECT method FROM (VALUES (1,'totp','totp'),(2,'qr','qr'),(3,'hwk','passkey'),
                                    (4,'pwd','password'),(5,'otp','code')) m(priority,amr,method)
         WHERE m.amr=ANY(regexp_split_to_array(r.body->>'AMR','[[:space:]]+'))
         ORDER BY priority LIMIT 1
       ),'')=ANY(NEW.login_methods)))
   OR OLD.platform<>NEW.platform
   OR (NOT (OLD.redirects <@ NEW.redirects)
       AND NOT (r.body->'Request'->>'redirect_uri'=ANY(NEW.redirects))
       AND NOT coalesce((r.body->'RegisteredRedirects') ?| NEW.redirects,false))
 );
 RETURN NEW;
END;
$$;
CREATE TRIGGER revoke_authorizations_after_policy_change AFTER UPDATE ON apps
FOR EACH ROW WHEN ((NOT OLD.restricted AND NEW.restricted)
 OR (OLD.refresh_enabled AND NOT NEW.refresh_enabled)
 OR NOT (OLD.login_methods <@ NEW.login_methods)
 OR NOT (OLD.redirects <@ NEW.redirects) OR OLD.platform<>NEW.platform)
EXECUTE FUNCTION revoke_ineligible_authorizations();
