ALTER TABLE users ADD COLUMN account_id text;

-- Serialize allocation across concurrent registrations and retry collisions.
CREATE FUNCTION allocate_account_id() RETURNS text LANGUAGE plpgsql VOLATILE AS $$
DECLARE candidate text;
BEGIN
  PERFORM pg_advisory_xact_lock(630102609);
  LOOP
    candidate := (1000000000 + floor(random() * 9000000000)::bigint)::text;
    IF NOT EXISTS (SELECT 1 FROM users WHERE account_id = candidate) THEN
      RETURN candidate;
    END IF;
  END LOOP;
END;
$$;

-- Allocate individually so each existing account is visible to the next lookup.
DO $$
DECLARE uid text;
BEGIN
  FOR uid IN SELECT id FROM users ORDER BY id LOOP
    UPDATE users SET account_id = allocate_account_id() WHERE id = uid;
  END LOOP;
END;
$$;
ALTER TABLE users ALTER COLUMN account_id SET DEFAULT allocate_account_id();
ALTER TABLE users ALTER COLUMN account_id SET NOT NULL;
ALTER TABLE users ADD CONSTRAINT users_account_id_unique UNIQUE (account_id);
ALTER TABLE users ADD CONSTRAINT users_account_id_format CHECK (account_id ~ '^[1-9][0-9]{9}$');
ALTER TABLE users ADD COLUMN password_failures integer NOT NULL DEFAULT 0 CHECK (password_failures BETWEEN 0 AND 4);
ALTER TABLE users ADD COLUMN password_lock_level integer NOT NULL DEFAULT 0 CHECK (password_lock_level >= 0);
ALTER TABLE users ADD COLUMN password_locked_until timestamptz;
