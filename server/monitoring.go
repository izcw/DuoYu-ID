package main

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"
)

// Runs in worker/combined role; transitions are durable and deduplicated across workers.
func (s *Server) monitorOperations(ctx context.Context) error {
	tx, e := s.db.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(630102602)"); e != nil {
		return e
	}
	var logoutOld, mailOld, logoutFailed, mailUnknown, authFailed bool
	var databaseBytes int64
	limit, e := strconv.ParseInt(env("DUOYU_DATABASE_ALERT_BYTES", "10737418240"), 10, 64)
	if e != nil || limit <= 0 {
		return fmt.Errorf("invalid database alert threshold")
	}
	if e = tx.QueryRow(ctx, "SELECT pg_database_size(current_database())").Scan(&databaseBytes); e != nil {
		return e
	}
	e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM logout_jobs WHERE status='queued' AND created_at<now()-interval '5 minutes'), EXISTS(SELECT 1 FROM mail_jobs WHERE status IN ('queued','retry') AND created_at<now()-interval '2 minutes'),EXISTS(SELECT 1 FROM logout_jobs WHERE status='failed'),EXISTS(SELECT 1 FROM mail_jobs WHERE status='unknown'),(SELECT count(*)>=20 FROM audit WHERE action='login.failed' AND created_at>now()-interval '5 minutes')`).Scan(&logoutOld, &mailOld, &logoutFailed, &mailUnknown, &authFailed)
	if e != nil {
		return e
	}
	for name, active := range map[string]bool{"logout_queue_age": logoutOld, "mail_queue_age": mailOld, "logout_delivery_failed": logoutFailed, "mail_delivery_unknown": mailUnknown, "login_failure_burst": authFailed, "database_capacity": databaseBytes >= limit} {
		var previous bool
		if _, e = tx.Exec(ctx, "INSERT INTO operational_alerts(name,active) VALUES($1,false) ON CONFLICT DO NOTHING", name); e != nil {
			return e
		}
		if e = tx.QueryRow(ctx, "SELECT active FROM operational_alerts WHERE name=$1", name).Scan(&previous); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, "UPDATE operational_alerts SET active=$2,checked_at=now(),changed_at=CASE WHEN active<>$2 THEN now() ELSE changed_at END WHERE name=$1", name, active); e != nil {
			return e
		}
		if previous != active {
			action := "monitor.recovered"
			if active {
				action = "monitor.alert"
			}
			if _, e = tx.Exec(ctx, "INSERT INTO audit(user_id,action,target,success) VALUES('',$1,$2,$3)", action, name, !active); e != nil {
				return e
			}
		}
	}
	return tx.Commit(ctx)
}
func (s *Server) monitorSafely(ctx context.Context) {
	bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if e := s.monitorOperations(bounded); e != nil {
		slog.Error("operations monitor unavailable", "component", "monitor", "error_type", fmt.Sprintf("%T", e))
	}
}
