ALTER TABLE access_tokens ADD COLUMN session_hash text REFERENCES sessions(hash) ON DELETE CASCADE;
CREATE INDEX access_tokens_session ON access_tokens(session_hash);
