package main

import "net/http"

func (s *Server) diagnostics(w http.ResponseWriter, r *http.Request) {
	if _, yes := s.require(w, r, true); !yes {
		return
	}
	var queued, failed, mailPending, mailUnknown int64
	err := s.db.QueryRow(r.Context(), `SELECT (SELECT count(*) FROM logout_jobs WHERE status='queued'),(SELECT count(*) FROM logout_jobs WHERE status='failed'),(SELECT count(*) FROM mail_jobs WHERE status IN ('queued','retry')),(SELECT count(*) FROM mail_jobs WHERE status='unknown')`).Scan(&queued, &failed, &mailPending, &mailUnknown)
	if err != nil {
		fail(w, 503, "诊断查询失败")
		return
	}
	ok(w, map[string]any{"issuer": s.origin, "development": s.dev, "signing_kid": (signingKey{s.signing}).ID(), "logout_queued": queued, "logout_failed": failed, "mail_pending": mailPending, "mail_unknown": mailUnknown, "access_token_seconds": 300, "session_seconds": 43200})
}
