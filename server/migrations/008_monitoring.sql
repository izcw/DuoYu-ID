CREATE TABLE operational_alerts(name text PRIMARY KEY, active boolean NOT NULL, checked_at timestamptz NOT NULL DEFAULT now(), changed_at timestamptz NOT NULL DEFAULT now());
CREATE INDEX logout_attempts_created ON logout_attempts(created_at);
CREATE INDEX audit_login_failed_time ON audit(created_at) WHERE action='login.failed';
