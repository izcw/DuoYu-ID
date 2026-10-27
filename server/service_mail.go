package main

import (
	"bytes"

	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func validKeyScopes(scopes []string) bool {
	if len(scopes) < 1 || len(scopes) > 3 {
		return false
	}
	seen := map[string]bool{}
	for _, scope := range scopes {
		if seen[scope] || !slices.Contains([]string{"app:read", "mail:send", "mail:read"}, scope) {
			return false
		}
		seen[scope] = true
	}
	return true
}

func (s *Server) requireService(w http.ResponseWriter, r *http.Request, scope string) (key string, app *App, yes bool) {
	if s.limited(r, "service:"+remoteIP(r), 120, time.Minute) {
		fail(w, 429, "请求过于频繁")
		return
	}
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") || len(header) > 512 {
		fail(w, 401, "需要 API 凭证")
		return
	}
	var appID string
	var scopes []string
	err := s.db.QueryRow(r.Context(), "SELECT id,app_id,scopes FROM api_keys WHERE digest=$1 AND NOT revoked AND (expires_at IS NULL OR expires_at>now())", s.mac(strings.TrimPrefix(header, "Bearer "))).Scan(&key, &appID, &scopes)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 401, "无效的 API 凭证")
		return
	}
	if err != nil {
		fail(w, 503, "认证服务暂不可用")
		return
	}
	if !slices.Contains(scopes, scope) {
		fail(w, 403, "凭证缺少 "+scope+" 权限")
		return
	}
	app, err = s.app(r.Context(), appID)
	if err != nil {
		fail(w, 403, "应用不可用")
		return
	}
	s.db.Exec(r.Context(), "UPDATE api_keys SET last_used_at=now() WHERE id=$1", key)
	return key, app, true
}

type ProjectMail struct {
	To       string `json:"to"`
	Subject  string `json:"subject"`
	Text     string `json:"text"`
	HTML     string `json:"html"`
	Category string `json:"category"`
}

func (s *Server) projectMessage(in ProjectMail, appName string) (MailMessage, error) {
	m := MailMessage{To: in.To, Subject: in.Subject, Text: in.Text, HTML: in.HTML}
	if in.HTML == "" {
		var b bytes.Buffer
		err := s.templates.ExecuteTemplate(&b, "notification.html", map[string]string{"Title": in.Subject, "Text": in.Text, "Project": appName, "Logo": s.origin + "/logo.png"})
		if err != nil {
			return m, err
		}
		m.HTML = b.String()
	} else if m.Text == "" {
		m.Text = "这封邮件由 " + appName + " 发出。请使用支持 HTML 的邮件客户端查看。"
	}
	return m, nil
}

func mailLimit(name string, fallback int) int {
	n, err := strconv.Atoi(env(name, strconv.Itoa(fallback)))
	if err != nil || n < 1 {
		return fallback
	}
	return n
}

func (s *Server) serviceMail(w http.ResponseWriter, r *http.Request) {
	key, app, yes := s.requireService(w, r, "mail:send")
	if !yes {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 128<<10)
	var in ProjectMail
	idem := r.Header.Get("Idempotency-Key")
	if decode(r, &in) != nil || len(idem) < 8 || len(idem) > 128 || strings.ContainsAny(idem, "\r\n") {
		fail(w, 400, "需要 JSON 正文及 8–128 字符的 Idempotency-Key")
		return
	}
	in.To = normalizeEmail(in.To)
	in.Subject = strings.TrimSpace(in.Subject)
	if in.Category == "" {
		in.Category = "notification"
	}
	if in.To == "" || len(in.To) > 254 || in.Subject == "" || len(in.Subject) > 200 || strings.ContainsAny(in.Subject, "\r\n\x00") || len(in.Text) > 16384 || len(in.HTML) > 65536 || (strings.TrimSpace(in.Text) == "" && strings.TrimSpace(in.HTML) == "") || !slices.Contains([]string{"notification", "verification", "alert"}, in.Category) {
		fail(w, 400, "请检查收件邮箱、单行主题、正文长度与邮件类别")
		return
	}
	message, err := s.projectMessage(in, app.Name)
	if err != nil {
		fail(w, 500, "邮件模板不可用")
		return
	}
	content, _ := json.Marshal(in)
	hash := s.mac(string(content))
	id := "mail_" + randomID()[:22]
	format := "default"
	if in.HTML != "" {
		format = "html"
	}
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		fail(w, 503, "无法保存发送记录")
		return
	}
	defer tx.Rollback(r.Context())
	tag, err := tx.Exec(r.Context(), `INSERT INTO mail_messages(id,app_id,key_id,idempotency_hash,content_hash,recipient,category,format) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(app_id,idempotency_hash) DO NOTHING`, id, app.ID, key, s.mac(idem), hash, in.To, in.Category, format)
	if err != nil {
		fail(w, 503, "无法保存发送记录")
		return
	}
	if tag.RowsAffected() == 0 {
		tx.Rollback(r.Context())
		var storedHash string
		err = s.db.QueryRow(r.Context(), "SELECT id,content_hash FROM mail_messages WHERE app_id=$1 AND idempotency_hash=$2", app.ID, s.mac(idem)).Scan(&id, &storedHash)
		if err != nil {
			fail(w, 503, "无法读取发送记录")
			return
		}
		if storedHash != hash {
			fail(w, 409, "此 Idempotency-Key 已用于不同的邮件内容")
			return
		}
		s.mailStatus(w, r, id, app.ID)
		return
	}
	// Independent project/recipient budgets; keys in the same project share quotas.
	// Use this transaction's connection: waiting idempotent requests must not
	// exhaust the pool while the winning request waits for another connection.
	limited := func(key string, max int, period time.Duration) bool {
		var n int
		e := tx.QueryRow(r.Context(), `INSERT INTO rate_limits(key,count,until_at) VALUES($1,1,$2) ON CONFLICT(key) DO UPDATE SET count=CASE WHEN rate_limits.until_at<now() THEN 1 ELSE rate_limits.count+1 END,until_at=CASE WHEN rate_limits.until_at<now() THEN EXCLUDED.until_at ELSE rate_limits.until_at END RETURNING count`, key, time.Now().Add(period)).Scan(&n)
		return e != nil || n > max
	}
	if limited("mail:app:min:"+app.ID, mailLimit("MAIL_APP_PER_MINUTE", 10), time.Minute) || limited("mail:app:day:"+app.ID, mailLimit("MAIL_APP_PER_DAY", 100), 24*time.Hour) || limited("mail:recipient:"+s.mac(in.To), 3, time.Minute) || limited("mail:global", mailLimit("MAIL_GLOBAL_PER_DAY", 500), 24*time.Hour) {
		tx.Exec(r.Context(), "UPDATE mail_messages SET status='limited',error_code='quota_exceeded',updated_at=now() WHERE id=$1", id)
		if err = tx.Commit(r.Context()); err != nil {
			fail(w, 503, "无法保存发送记录")
			return
		}
		w.Header().Set("Retry-After", "60")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(429)
		json.NewEncoder(w).Encode(map[string]any{"code": 429, "msg": "邮件发送超过限额", "data": map[string]string{"id": id, "status": "limited", "error_code": "quota_exceeded"}})
		return
	}
	payload, err := s.encryptMail(id, queuedMail{Message: message})
	if err != nil {
		fail(w, 500, "邮件加密失败")
		return
	}
	ttl := 24 * time.Hour
	if in.Category == "verification" {
		ttl = 5 * time.Minute
	}
	if _, err = tx.Exec(r.Context(), "INSERT INTO mail_jobs(id,message_id,lane,payload,expires_at) VALUES($1,$1,'project',$2,$3)", id, payload, time.Now().Add(ttl)); err != nil {
		fail(w, 503, "邮件入队失败")
		return
	}
	if _, err = tx.Exec(r.Context(), "UPDATE mail_messages SET status='queued' WHERE id=$1", id); err != nil {
		fail(w, 503, "邮件入队失败")
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		fail(w, 503, "邮件入队失败")
		return
	}
	ok(w, map[string]string{"id": id, "status": "queued", "error_code": ""})
}

// Stale sending records are ambiguous after process termination; do not claim delivery.
const mailStatusExpr = "CASE WHEN m.status='sending' AND m.updated_at<now()-interval '2 minutes' THEN 'unknown' ELSE m.status END"

func (s *Server) mailStatus(w http.ResponseWriter, r *http.Request, id, appID string) {
	var status, category, format, errorCode string
	var created, updated time.Time
	err := s.db.QueryRow(r.Context(), "SELECT "+mailStatusExpr+",m.category,m.format,m.error_code,m.created_at,m.updated_at FROM mail_messages m WHERE m.id=$1 AND m.app_id=$2", id, appID).Scan(&status, &category, &format, &errorCode, &created, &updated)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "邮件记录不存在")
		return
	}
	if err != nil {
		fail(w, 503, "查询暂不可用")
		return
	}
	ok(w, map[string]any{"id": id, "status": status, "category": category, "format": format, "error_code": errorCode, "created_at": created, "updated_at": updated})
}
func (s *Server) serviceMailStatus(w http.ResponseWriter, r *http.Request) {
	_, app, yes := s.requireService(w, r, "mail:read")
	if yes {
		s.mailStatus(w, r, r.PathValue("id"), app.ID)
	}
}
func (s *Server) adminMail(w http.ResponseWriter, r *http.Request) {
	if _, yes := s.require(w, r, true); !yes {
		return
	}
	s.queryList(w, r, "SELECT m.id,m.app_id,a.name AS app_name,m.recipient,m.category,m.format,"+mailStatusExpr+" AS status,m.error_code,m.created_at,m.updated_at FROM mail_messages m JOIN apps a ON a.id=m.app_id ORDER BY m.created_at DESC LIMIT 200")
}
