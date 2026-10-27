CREATE TABLE mail_jobs (
 id text PRIMARY KEY,
 message_id text UNIQUE REFERENCES mail_messages(id),
 lane text NOT NULL CHECK(lane IN ('auth','project')),
 payload bytea,
 status text NOT NULL DEFAULT 'queued',
 attempts integer NOT NULL DEFAULT 0,
 available_at timestamptz NOT NULL DEFAULT now(),
 expires_at timestamptz NOT NULL,
 locked_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(),
 error_code text NOT NULL DEFAULT ''
);
CREATE INDEX mail_jobs_ready ON mail_jobs(lane,available_at) WHERE status IN ('queued','retry');
