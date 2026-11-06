ALTER TABLE apps ADD COLUMN persistent_login boolean NOT NULL DEFAULT false;
UPDATE apps SET persistent_login=true WHERE platform='extension' AND refresh_enabled;
