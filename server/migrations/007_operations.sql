ALTER TABLE logout_jobs ADD COLUMN last_error text NOT NULL DEFAULT '';
ALTER TABLE logout_jobs ADD COLUMN http_status integer NOT NULL DEFAULT 0;
ALTER TABLE logout_jobs ADD COLUMN completed_at timestamptz;
ALTER TABLE logout_jobs ADD COLUMN retry_count integer NOT NULL DEFAULT 0;
CREATE TABLE logout_attempts (
 id bigserial PRIMARY KEY, job_id bigint NOT NULL REFERENCES logout_jobs(id) ON DELETE CASCADE,
 created_at timestamptz NOT NULL DEFAULT now(), http_status integer NOT NULL,
 error_class text NOT NULL, duration_ms bigint NOT NULL
);
CREATE INDEX logout_attempts_job ON logout_attempts(job_id,id DESC);
ALTER TABLE audit ADD COLUMN request_id text NOT NULL DEFAULT '';
CREATE INDEX audit_request_id ON audit(request_id) WHERE request_id<>'';
ALTER TABLE users ADD COLUMN management_role text NOT NULL DEFAULT 'none' CHECK(management_role IN ('none','auditor','maintainer'));
CREATE TABLE app_maintainers(user_id text REFERENCES users(id) ON DELETE CASCADE, app_id text REFERENCES apps(id) ON DELETE CASCADE, PRIMARY KEY(user_id,app_id));
ALTER TABLE api_keys ADD COLUMN previous_digest text NOT NULL DEFAULT '';
ALTER TABLE api_keys ADD COLUMN previous_until timestamptz;
ALTER TABLE api_keys ADD COLUMN last_ip text NOT NULL DEFAULT '';
