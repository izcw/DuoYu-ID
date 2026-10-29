-- Managed avatars stay on the current service regardless of hostname or port.
UPDATE users SET avatar=substring(avatar from '(/avatars/[A-Za-z0-9_-]{43}[.]png)$')
WHERE avatar ~ '^https?://[^/]+/avatars/[A-Za-z0-9_-]{43}[.]png$';
