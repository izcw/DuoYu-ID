CREATE TABLE user_legal_acceptances (
 user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 version text NOT NULL, adult_asserted boolean NOT NULL CHECK(adult_asserted),
 accepted_at timestamptz NOT NULL DEFAULT now(), last_accepted_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(user_id,version)
);
