package main

import (
	"context"
	"sync"
	"time"
)

// Small batches avoid long locks; token mappings are retained for the lifetime
// of their center session so a later logout can still find all relying parties.
func (s *Server) cleanupExpired(ctx context.Context) error {
	for _, q := range []string{
		"UPDATE management_create_requests SET result_cipher=NULL WHERE result_cipher IS NOT NULL AND created_at<now()-interval '24 hours'",
		"DELETE FROM authenticators WHERE user_id IN(SELECT user_id FROM authenticators WHERE NOT active AND expires_at<now() LIMIT 500)",
		"DELETE FROM sessions WHERE hash IN(SELECT hash FROM sessions WHERE expires_at<now() LIMIT 500)",
		"DELETE FROM auth_requests WHERE id IN(SELECT id FROM auth_requests WHERE expires_at<now() LIMIT 500)",
		"DELETE FROM webauthn_challenges WHERE id IN(SELECT id FROM webauthn_challenges WHERE expires_at<now() LIMIT 500)",
		"DELETE FROM challenges WHERE (email,purpose) IN(SELECT email,purpose FROM challenges WHERE expires_at<now()-interval '1 day' LIMIT 500)",
		"DELETE FROM rate_limits WHERE key IN(SELECT key FROM rate_limits WHERE until_at<now()-interval '1 day' LIMIT 500)",
		"DELETE FROM access_tokens WHERE id IN(SELECT id FROM access_tokens WHERE session_hash IS NULL AND expires_at<now()-interval '1 day' LIMIT 500)",
		"DELETE FROM logout_jobs WHERE id IN(SELECT id FROM logout_jobs WHERE status='delivered' AND created_at<now()-interval '30 days' LIMIT 500)",
		"DELETE FROM audit WHERE id IN(SELECT id FROM audit WHERE created_at<now()-interval '90 days' LIMIT 500)",
	} {
		if _, e := s.db.Exec(ctx, q); e != nil {
			return e
		}
	}
	return nil
}
func (s *Server) startMaintenance(ctx context.Context, wg *sync.WaitGroup) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(time.Minute)
		s.monitorSafely(ctx)
		ticks := 0
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.monitorSafely(ctx)
				ticks++
				if ticks%10 != 0 {
					continue
				}
				if e := s.cleanupExpired(ctx); e != nil {
					s.audit(ctx, "", "maintenance.failed", "expired_records", false)
				}
			}
		}
	}()
}
