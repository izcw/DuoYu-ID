ALTER TABLE apps ADD COLUMN allowed_origins text[] NOT NULL DEFAULT '{}';
ALTER TABLE apps DROP CONSTRAINT apps_platform_check;
ALTER TABLE apps ADD CONSTRAINT apps_platform_check CHECK(platform IN ('web','spa','native','extension'));
-- One-time compatibility migration; subsequent edits manage origins independently.
UPDATE apps a SET allowed_origins=ARRAY(SELECT DISTINCT substring(r FROM '^(https?://[^/?#]+)') FROM unnest(a.redirects) r WHERE r ~ '^https?://');
