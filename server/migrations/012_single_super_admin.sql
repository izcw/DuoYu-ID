UPDATE users SET management_role='none' WHERE management_role<>'none';
DELETE FROM app_maintainers;
ALTER TABLE users ADD CONSTRAINT users_no_delegated_management CHECK (management_role='none');
CREATE UNIQUE INDEX users_single_super_admin ON users ((true)) WHERE admin;
