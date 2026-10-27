package main

import (
	"net/http"
	"time"
)

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
	var logoutAge, mailAge, databaseBytes int64
	var latency *float64
	err = s.db.QueryRow(r.Context(), `SELECT
 COALESCE((SELECT EXTRACT(EPOCH FROM now()-min(created_at))::bigint FROM logout_jobs WHERE status='queued'),0),
 COALESCE((SELECT EXTRACT(EPOCH FROM now()-min(created_at))::bigint FROM mail_jobs WHERE status IN ('queued','retry')),0),
 pg_database_size(current_database()),
 (SELECT percentile_cont(0.95) WITHIN GROUP (ORDER BY duration_ms) FROM logout_attempts WHERE created_at>now()-interval '24 hours')`).Scan(&logoutAge, &mailAge, &databaseBytes, &latency)
	if err != nil {
		fail(w, 503, "诊断查询失败")
		return
	}
	alerts := []string{}
	if logoutAge > 300 {
		alerts = append(alerts, "退出通知已积压超过 5 分钟")
	}
	if mailAge > 120 {
		alerts = append(alerts, "邮件已积压超过 2 分钟")
	}
	if failed > 0 {
		alerts = append(alerts, "存在投递失败的退出通知")
	}
	if mailUnknown > 0 {
		alerts = append(alerts, "存在发送结果不确定的邮件")
	}
	var monitorChecked any
	rows, err := s.db.Query(r.Context(), "SELECT name FROM operational_alerts WHERE active AND name IN ('login_failure_burst','database_capacity') ORDER BY name")
	if err != nil {
		fail(w, 503, "告警查询失败")
		return
	}
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			rows.Close()
			fail(w, 503, "告警读取失败")
			return
		}
		if name == "database_capacity" {
			alerts = append(alerts, "数据库已达到容量告警阈值")
		} else {
			alerts = append(alerts, "最近 5 分钟登录失败达到 20 次")
		}
	}
	if rows.Err() != nil {
		rows.Close()
		fail(w, 503, "告警读取失败")
		return
	}
	rows.Close()

	if err = s.db.QueryRow(r.Context(), "SELECT min(checked_at) FROM operational_alerts").Scan(&monitorChecked); err != nil {
		fail(w, 503, "监控状态读取失败")
		return
	}
	if checked, yes := monitorChecked.(time.Time); yes && time.Since(checked) > 3*time.Minute {
		alerts = append(alerts, "后台监控超过 3 分钟未更新，请检查 worker")
	}
	if monitorChecked == nil {
		alerts = append(alerts, "后台监控尚未报告，请检查 worker 是否启动")
	}
	ok(w, map[string]any{"monitor_checked_at": monitorChecked, "alerts": alerts, "logout_oldest_seconds": logoutAge, "mail_oldest_seconds": mailAge, "database_bytes": databaseBytes, "logout_p95_ms": latency, "issuer": s.origin, "development": s.dev, "signing_kid": (signingKey{s.signing}).ID(), "logout_queued": queued, "logout_failed": failed, "mail_pending": mailPending, "mail_unknown": mailUnknown, "access_token_seconds": 300, "session_seconds": 43200})
}
