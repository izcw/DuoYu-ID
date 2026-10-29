package main

import (
	"context"
	"fmt"
	"strings"
)

// Each migration is immutable after release; one transaction and advisory lock
// serialize concurrent starters. Existing installations adopt the idempotent baseline.
func (s *Server) migrate(ctx context.Context) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(630102601)"); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "CREATE TABLE IF NOT EXISTS schema_migrations(version text PRIMARY KEY, checksum text NOT NULL, applied_at timestamptz NOT NULL DEFAULT now())"); err != nil {
		return err
	}
	for _, name := range []string{"schema.sql", "migrations/002_mail_queue.sql", "migrations/003_session_binding.sql", "migrations/004_refresh_tokens.sql", "migrations/005_logout_events.sql", "migrations/006_policy_revocation.sql", "migrations/007_operations.sql", "migrations/008_monitoring.sql", "migrations/009_account_policy.sql", "migrations/010_passkey_device.sql", "migrations/011_qr_enrollment.sql", "migrations/012_single_super_admin.sql", "migrations/013_project_logo.sql", "migrations/014_project_blacklist.sql", "migrations/015_extension_origins.sql", "migrations/016_login_methods.sql", "migrations/017_totp.sql", "migrations/018_project_roles.sql", "migrations/019_avatar_paths.sql", "migrations/020_management_api.sql"} {
		body, err := assets.ReadFile(name)
		if err != nil {
			return err
		}
		sum := migrationChecksum(body)
		var stored string
		rows, err := tx.Query(ctx, "SELECT checksum FROM schema_migrations WHERE version=$1", name)
		if err != nil {
			return err
		}
		found := rows.Next()
		if found {
			err = rows.Scan(&stored)
		}
		rows.Close()
		if err != nil {
			return err
		}
		if found {
			if stored != sum {
				return fmt.Errorf("migration checksum mismatch: %s", name)
			}
			continue
		}
		if _, err = tx.Exec(ctx, string(body)); err != nil {
			return fmt.Errorf("migration %s failed: %w", name, err)
		}
		if _, err = tx.Exec(ctx, "INSERT INTO schema_migrations(version,checksum) VALUES($1,$2)", name, sum); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func migrationChecksum(body []byte) string {
	return digest(strings.ReplaceAll(string(body), "\r\n", "\n"))
}
