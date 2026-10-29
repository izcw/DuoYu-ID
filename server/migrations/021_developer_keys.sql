ALTER TABLE api_keys ALTER COLUMN app_id DROP NOT NULL;
ALTER TABLE api_keys ADD COLUMN owner_id text REFERENCES users(id);
ALTER TABLE api_keys ADD COLUMN all_projects boolean NOT NULL DEFAULT false;
UPDATE api_keys SET owner_id=(SELECT id FROM users WHERE admin LIMIT 1);
INSERT INTO api_keys(id,name,digest,scopes,owner_id,all_projects,expires_at,revoked,created_at,last_used_at)
 SELECT id,name,digest,scopes,owner_id,all_projects,expires_at,revoked,created_at,last_used_at FROM management_api_keys;
ALTER TABLE management_api_grants DROP CONSTRAINT management_api_grants_key_id_fkey;
ALTER TABLE management_api_grants ADD FOREIGN KEY(key_id) REFERENCES api_keys(id) ON DELETE CASCADE;
INSERT INTO management_api_grants(key_id,app_id) SELECT id,app_id FROM api_keys WHERE app_id IS NOT NULL ON CONFLICT DO NOTHING;
ALTER TABLE management_create_requests DROP CONSTRAINT management_create_requests_key_id_fkey;
ALTER TABLE management_create_requests ADD FOREIGN KEY(key_id) REFERENCES api_keys(id) ON DELETE CASCADE;
DROP TABLE management_api_keys;
