ALTER TABLE passkeys ADD COLUMN qr_enabled boolean NOT NULL DEFAULT false;
ALTER TABLE webauthn_challenges ADD COLUMN qr_enabled boolean NOT NULL DEFAULT false;
-- Keep the credential association so deletion revokes pending QR approvals too.
ALTER TABLE sessions ADD COLUMN passkey_id text NOT NULL DEFAULT '';
