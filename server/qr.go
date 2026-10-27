package main

import (
	"encoding/base64"
	"net/http"
	"time"

	"github.com/skip2/go-qrcode"
)

func (s *Server) createQR(w http.ResponseWriter, r *http.Request) {
	if s.limited(r, "qr-create:"+remoteIP(r), 30, 15*time.Minute) {
		fail(w, 429, "操作频繁，请稍后再试")
		return
	}
	id, owner, ticket := randomID(), randomID(), randomID()
	label := s.mac(id)[:6]
	expires := time.Now().Add(2 * time.Minute)
	// The approval capability stays in a URL fragment, absent from server access logs.
	png, e := qrcode.Encode(s.origin+"/scan#"+ticket, qrcode.Medium, 240)
	if e != nil {
		fail(w, 500, "生成二维码失败")
		return
	}
	tx, e := s.db.Begin(r.Context())
	if e != nil {
		fail(w, 500, "创建失败")
		return
	}
	defer tx.Rollback(r.Context())
	if old, e := r.Cookie("duoyu_qr"); e == nil {
		tx.Exec(r.Context(), "DELETE FROM qr_logins WHERE owner_hash=$1", digest(old.Value))
	}
	tx.Exec(r.Context(), "DELETE FROM qr_logins WHERE expires_at<now()")
	_, e = tx.Exec(r.Context(), "INSERT INTO qr_logins(id,owner_hash,ticket_hash,label,expires_at) VALUES($1,$2,$3,$4,$5)", id, digest(owner), digest(ticket), label, expires)
	if e != nil || tx.Commit(r.Context()) != nil {
		fail(w, 500, "创建失败")
		return
	}
	s.cookie(w, "duoyu_qr", owner, 120)
	ok(w, map[string]any{"id": id, "image": "data:image/png;base64," + base64.StdEncoding.EncodeToString(png), "label": label, "expires_at": expires, "local_only": s.dev})
}
func (s *Server) cancelQR(w http.ResponseWriter, r *http.Request) {
	c, e := r.Cookie("duoyu_qr")
	if e != nil {
		fail(w, 403, "请求不属于此浏览器")
		return
	}
	s.db.Exec(r.Context(), "DELETE FROM qr_logins WHERE id=$1 AND owner_hash=$2", r.PathValue("id"), digest(c.Value))
	ok(w, nil)
}
func (s *Server) pollQR(w http.ResponseWriter, r *http.Request) {
	c, e := r.Cookie("duoyu_qr")
	if e != nil {
		fail(w, 403, "请求不属于此浏览器")
		return
	}
	tx, e := s.db.Begin(r.Context())
	if e != nil {
		fail(w, 500, "暂时无法查询")
		return
	}
	defer tx.Rollback(r.Context())
	var status string
	var uid, approver *string
	var expires time.Time
	e = tx.QueryRow(r.Context(), "SELECT status,user_id,approver_session,expires_at FROM qr_logins WHERE id=$1 AND owner_hash=$2 FOR UPDATE", r.PathValue("id"), digest(c.Value)).Scan(&status, &uid, &approver, &expires)
	if e != nil || time.Now().After(expires) || status == "consumed" {
		ok(w, map[string]string{"status": "expired"})
		return
	}
	if status != "approved" {
		ok(w, map[string]string{"status": status})
		return
	}
	var enabled bool
	e = tx.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM users u JOIN sessions s ON s.user_id=u.id WHERE u.id=$1 AND u.enabled AND s.hash=$2 AND s.expires_at>now())", uid, approver).Scan(&enabled)
	if e != nil || !enabled {
		fail(w, 403, "确认已失效，请重新扫码")
		return
	}
	token := randomID()
	_, e = tx.Exec(r.Context(), "INSERT INTO sessions(hash,user_id,expires_at,agent,amr) VALUES($1,$2,$3,$4,'hwk user qr')", digest(token), uid, time.Now().Add(12*time.Hour), truncate(r.UserAgent(), 300))
	if e != nil {
		fail(w, 500, "创建会话失败")
		return
	}
	if old, e := r.Cookie("duoyu_session"); e == nil {
		tx.Exec(r.Context(), "DELETE FROM sessions WHERE hash=$1", digest(old.Value))
	}
	tx.Exec(r.Context(), "UPDATE qr_logins SET status='consumed' WHERE id=$1", r.PathValue("id"))
	if tx.Commit(r.Context()) != nil {
		fail(w, 500, "登录失败")
		return
	}
	s.cookie(w, "duoyu_session", token, 43200)
	s.audit(r.Context(), *uid, "login.success", "passkey.qr", true)
	ok(w, map[string]string{"status": "approved"})
}
func (s *Server) qrContext(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Ticket string `json:"ticket"`
	}
	if decode(r, &in) != nil || len(in.Ticket) != 43 {
		fail(w, 400, "二维码无效")
		return
	}
	var label string
	e := s.db.QueryRow(r.Context(), "UPDATE qr_logins SET status='scanned' WHERE ticket_hash=$1 AND status IN ('pending','scanned') AND expires_at>now() RETURNING label", digest(in.Ticket)).Scan(&label)
	if e != nil {
		fail(w, 410, "二维码已过期或已使用")
		return
	}
	ok(w, map[string]string{"label": label, "origin": s.origin})
}
func (s *Server) approveQR(w http.ResponseWriter, r *http.Request) {
	u, yes := s.require(w, r, false)
	if !yes {
		return
	}
	var in struct {
		Ticket string `json:"ticket"`
		Reject bool   `json:"reject"`
	}
	if decode(r, &in) != nil || len(in.Ticket) != 43 {
		fail(w, 400, "二维码无效")
		return
	}
	c, _ := r.Cookie("duoyu_session")
	status := "approved"
	if in.Reject {
		status = "rejected"
	}
	// A password login or an old browser session cannot approve a QR login.
	tag, e := s.db.Exec(r.Context(), `UPDATE qr_logins q SET status=$1,user_id=$2,approver_session=$3 WHERE ticket_hash=$4 AND status IN ('pending','scanned') AND expires_at>now() AND EXISTS(SELECT 1 FROM sessions s WHERE s.hash=$3 AND s.user_id=$2 AND s.amr='hwk user' AND s.created_at>=q.created_at AND s.expires_at>now())`, status, u.ID, digest(c.Value), digest(in.Ticket))
	if e != nil || tag.RowsAffected() != 1 {
		fail(w, 403, "请重新扫码并使用通行密钥验证，或二维码已失效")
		return
	}
	s.audit(r.Context(), u.ID, "qr."+status, "", true)
	ok(w, nil)
}
func (s *Server) scanPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	s.templates.ExecuteTemplate(w, "scan.html", nil)
}
