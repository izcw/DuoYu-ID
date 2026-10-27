package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func (s *Server) sendCode(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email   string `json:"email"`
		Purpose string `json:"purpose"`
	}
	if decode(r, &in) != nil {
		fail(w, 400, "请求格式错误")
		return
	}
	in.Email = normalizeEmail(in.Email)
	if in.Email == "" || (in.Purpose != "login" && in.Purpose != "reset" && in.Purpose != "email") {
		fail(w, 400, "请填写有效邮箱和用途")
		return
	}
	if in.Purpose == "email" {
		if _, yes := s.require(w, r, false); !yes {
			return
		}
	}
	if s.limited(r, "send-ip:"+remoteIP(r), 20, time.Hour) || s.limited(r, "send-email:"+in.Email, 10, time.Hour) {
		fail(w, 429, "发送过于频繁，请稍后再试")
		return
	}
	n, e := rand.Int(rand.Reader, big.NewInt(1000000))
	if e != nil {
		fail(w, 500, "生成验证码失败")
		return
	}
	code := fmt.Sprintf("%06d", n.Int64())
	tag, e := s.db.Exec(r.Context(), `INSERT INTO challenges(email,purpose,digest,expires_at) VALUES($1,$2,$3,$4) ON CONFLICT(email,purpose) DO UPDATE SET digest=EXCLUDED.digest,attempts=0,expires_at=EXCLUDED.expires_at,sent_at=now() WHERE challenges.sent_at<now()-interval '60 seconds'`, in.Email, in.Purpose, s.mac(in.Email+"|"+in.Purpose+"|"+code), time.Now().Add(5*time.Minute))
	if e != nil {
		fail(w, 500, "暂时无法发送")
		return
	}
	if tag.RowsAffected() == 0 {
		fail(w, 429, "请等待 60 秒后重试")
		return
	}
	// Reset requests for unknown users deliberately return the same response.
	var exists bool
	s.db.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM users WHERE email=$1 AND enabled)", in.Email).Scan(&exists)
	if in.Purpose == "reset" && !exists {
		ok(w, s.mailReceipt())
		return
	}
	if e = s.sendMail(in.Email, code, in.Purpose); e != nil {
		s.db.Exec(r.Context(), "DELETE FROM challenges WHERE email=$1 AND purpose=$2 AND digest=$3", in.Email, in.Purpose, s.mac(in.Email+"|"+in.Purpose+"|"+code))
		s.audit(r.Context(), "", "mail.failed", in.Purpose, false)
		fail(w, 503, "邮件服务暂不可用，请稍后再试")
		return
	}
	s.audit(r.Context(), "", "mail.queued", in.Purpose, true)
	ok(w, s.mailReceipt())
}

// A row lock serializes concurrent attempts. Failed attempts are committed too.
func (s *Server) consumeCode(ctx context.Context, email, purpose, code string) bool {
	tx, e := s.db.Begin(ctx)
	if e != nil {
		return false
	}
	defer tx.Rollback(ctx)
	var want string
	var attempts int
	var expires time.Time
	e = tx.QueryRow(ctx, "SELECT digest,attempts,expires_at FROM challenges WHERE email=$1 AND purpose=$2 FOR UPDATE", email, purpose).Scan(&want, &attempts, &expires)
	if e != nil || attempts >= 5 || time.Now().After(expires) {
		return false
	}
	good := len(code) == 6 && subtle.ConstantTimeCompare([]byte(want), []byte(s.mac(email+"|"+purpose+"|"+code))) == 1
	if good {
		_, e = tx.Exec(ctx, "DELETE FROM challenges WHERE email=$1 AND purpose=$2", email, purpose)
	} else {
		_, e = tx.Exec(ctx, "UPDATE challenges SET attempts=attempts+1 WHERE email=$1 AND purpose=$2", email, purpose)
	}
	if e != nil {
		return false
	}
	return tx.Commit(ctx) == nil && good
}
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Code     string `json:"code"`
		Mode     string `json:"mode"`
		Register bool   `json:"register"`
	}
	if decode(r, &in) != nil {
		fail(w, 400, "请求格式错误")
		return
	}
	in.Email = normalizeEmail(in.Email)
	if in.Email == "" || len(in.Password) > 128 || (in.Mode != "code" && in.Mode != "password") {
		fail(w, 400, "请检查登录信息")
		return
	}
	if s.limited(r, "login-ip:"+remoteIP(r), 50, 15*time.Minute) || s.limited(r, "login-email:"+in.Email, 15, 15*time.Minute) {
		fail(w, 429, "尝试次数过多，请稍后再试")
		return
	}
	var id, h string
	var enabled bool
	e := s.db.QueryRow(r.Context(), "SELECT id,password_hash,enabled FROM users WHERE email=$1", in.Email).Scan(&id, &h, &enabled)
	missing := errors.Is(e, pgx.ErrNoRows)
	if e != nil && !missing {
		fail(w, 503, "登录服务暂不可用")
		return
	}
	valid := false
	amr := "pwd"
	if in.Mode == "code" {
		valid = s.consumeCode(r.Context(), in.Email, "login", in.Code)
		amr = "otp"
	} else {
		valid = checkPassword(in.Password, h)
	}
	if !valid || (!missing && !enabled) {
		s.audit(r.Context(), id, "login.failed", "", false)
		fail(w, 401, "邮箱、密码或验证码不正确")
		return
	}
	if missing {
		var registration string
		s.db.QueryRow(r.Context(), "SELECT value FROM settings WHERE key='registration'").Scan(&registration)
		if in.Mode != "code" || registration != "true" {
			fail(w, 403, "当前未开放新账号注册，请联系管理员")
			return
		}
		id = randomID()
		_, e = s.db.Exec(r.Context(), "INSERT INTO users(id,email,name) VALUES($1,$2,$3)", id, in.Email, strings.Split(in.Email, "@")[0])
		if e != nil {
			fail(w, 409, "账号创建失败，请重新登录")
			return
		}
		s.audit(r.Context(), id, "account.created", "", true)
	}
	if s.newSession(w, r, id, amr) != nil {
		fail(w, 500, "创建会话失败")
		return
	}
	s.audit(r.Context(), id, "login.success", amr, true)
	ok(w, map[string]bool{"logged_in": true})
}
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, e := r.Cookie("duoyu_session"); e == nil {
		if _, err := s.db.Exec(r.Context(), "DELETE FROM sessions WHERE hash=$1", digest(c.Value)); err != nil {
			fail(w, 503, "暂时无法退出，请重试")
			return
		}
	}
	s.cookie(w, "duoyu_session", "", -1)
	ok(w, nil)
}

// Public session probe for the login page. Protected resources still require auth.
func (s *Server) sessionStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	cookie, err := r.Cookie("duoyu_session")
	if err != nil {
		ok(w, map[string]any{"authenticated": false})
		return
	}
	var name, email string
	err = s.db.QueryRow(r.Context(), `SELECT u.name,u.email FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.hash=$1 AND s.expires_at>now() AND u.enabled`, digest(cookie.Value)).Scan(&name, &email)
	if errors.Is(err, pgx.ErrNoRows) {
		ok(w, map[string]any{"authenticated": false})
		return
	}
	if err != nil {
		fail(w, 503, "登录状态暂不可用")
		return
	}
	ok(w, map[string]any{"authenticated": true, "user": map[string]string{"name": name, "email": email}})
}
func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	u, yes := s.require(w, r, false)
	if yes {
		ok(w, u)
	}
}
func (s *Server) artUser(w http.ResponseWriter, r *http.Request) {
	u, yes := s.require(w, r, false)
	if !yes {
		return
	}
	roles := []string{"R_USER"}
	if u.Admin {
		roles = []string{"R_SUPER"}
	} else if u.ManagementRole == "auditor" {
		roles = []string{"R_AUDITOR"}
	} else if u.ManagementRole == "maintainer" {
		roles = []string{"R_MAINTAINER"}
	}
	ok(w, map[string]any{"userId": u.ID, "userName": u.Name, "email": u.Email, "avatar": u.Avatar, "roles": roles, "buttons": []string{}})
}
func (s *Server) profile(w http.ResponseWriter, r *http.Request) {
	u, yes := s.require(w, r, false)
	if !yes {
		return
	}
	var in struct {
		Name string `json:"name"`
	}
	if decode(r, &in) != nil || len([]rune(strings.TrimSpace(in.Name))) < 1 || len([]rune(in.Name)) > 40 {
		fail(w, 400, "昵称须为 1–40 个字符")
		return
	}
	if _, e := s.db.Exec(r.Context(), "UPDATE users SET name=$1 WHERE id=$2", strings.TrimSpace(in.Name), u.ID); e != nil {
		fail(w, 500, "更新失败")
		return
	}
	ok(w, nil)
}
func (s *Server) resetPassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email"`
		Code     string `json:"code"`
		Password string `json:"password"`
	}
	if decode(r, &in) != nil || validPassword(in.Password) != nil {
		fail(w, 400, "密码须为 12–128 字节")
		return
	}
	in.Email = normalizeEmail(in.Email)
	if s.limited(r, "reset:"+remoteIP(r), 20, time.Hour) || !s.consumeCode(r.Context(), in.Email, "reset", in.Code) {
		fail(w, 400, "验证码无效或已过期")
		return
	}
	tx, e := s.db.Begin(r.Context())
	if e != nil {
		fail(w, 500, "更新失败")
		return
	}
	defer tx.Rollback(r.Context())
	var id string
	e = tx.QueryRow(r.Context(), "UPDATE users SET password_hash=$1 WHERE email=$2 AND enabled RETURNING id", hashPassword(in.Password), in.Email).Scan(&id)
	if e != nil {
		fail(w, 400, "无法重置该账号")
		return
	}
	tx.Exec(r.Context(), "DELETE FROM sessions WHERE user_id=$1", id)
	tx.Exec(r.Context(), "DELETE FROM access_tokens WHERE user_id=$1", id)
	if tx.Commit(r.Context()) != nil {
		fail(w, 500, "更新失败")
		return
	}
	s.audit(r.Context(), id, "password.reset", "", true)
	ok(w, nil)
}
func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	u, yes := s.require(w, r, false)
	if !yes {
		return
	}
	var in struct {
		Code     string `json:"code"`
		Password string `json:"password"`
	}
	if decode(r, &in) != nil || validPassword(in.Password) != nil {
		fail(w, 400, "密码须为 12–128 字节")
		return
	}
	if !s.consumeCode(r.Context(), u.Email, "reset", in.Code) {
		fail(w, 400, "验证码无效或已过期")
		return
	}
	tx, e := s.db.Begin(r.Context())
	if e != nil {
		fail(w, 500, "更新失败")
		return
	}
	defer tx.Rollback(r.Context())
	tx.Exec(r.Context(), "UPDATE users SET password_hash=$1 WHERE id=$2", hashPassword(in.Password), u.ID)
	tx.Exec(r.Context(), "DELETE FROM sessions WHERE user_id=$1", u.ID)
	tx.Exec(r.Context(), "DELETE FROM access_tokens WHERE user_id=$1", u.ID)
	if tx.Commit(r.Context()) != nil {
		fail(w, 500, "更新失败")
		return
	}
	s.cookie(w, "duoyu_session", "", -1)
	s.audit(r.Context(), u.ID, "password.changed", "", true)
	ok(w, nil)
}
func (s *Server) changeEmail(w http.ResponseWriter, r *http.Request) {
	u, yes := s.require(w, r, false)
	if !yes {
		return
	}
	var in struct {
		Email   string `json:"email"`
		OldCode string `json:"old_code"`
		NewCode string `json:"new_code"`
	}
	if decode(r, &in) != nil {
		fail(w, 400, "请求格式错误")
		return
	}
	in.Email = normalizeEmail(in.Email)
	if in.Email == "" || in.Email == u.Email {
		fail(w, 400, "请填写新的邮箱")
		return
	}
	if !s.consumeCode(r.Context(), u.Email, "email", in.OldCode) || !s.consumeCode(r.Context(), in.Email, "email", in.NewCode) {
		fail(w, 400, "验证码无效或已过期")
		return
	}
	tx, e := s.db.Begin(r.Context())
	if e != nil {
		fail(w, 500, "更新失败")
		return
	}
	defer tx.Rollback(r.Context())
	if _, e = tx.Exec(r.Context(), "UPDATE users SET email=$1 WHERE id=$2", in.Email, u.ID); e != nil {
		fail(w, 409, "该邮箱不可用")
		return
	}
	tx.Exec(r.Context(), "DELETE FROM sessions WHERE user_id=$1", u.ID)
	tx.Exec(r.Context(), "DELETE FROM access_tokens WHERE user_id=$1", u.ID)
	if tx.Commit(r.Context()) != nil {
		fail(w, 500, "更新失败")
		return
	}
	s.cookie(w, "duoyu_session", "", -1)
	s.audit(r.Context(), u.ID, "email.changed", "", true)
	ok(w, nil)
}
func (s *Server) sessions(w http.ResponseWriter, r *http.Request) {
	u, yes := s.require(w, r, false)
	if !yes {
		return
	}
	rows, e := s.db.Query(r.Context(), "SELECT hash,created_at,expires_at,agent FROM sessions WHERE user_id=$1 AND expires_at>now() ORDER BY created_at DESC", u.ID)
	if e != nil {
		fail(w, 500, "查询失败")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	cookie, _ := r.Cookie("duoyu_session")
	for rows.Next() {
		var id, agent string
		var created, expires time.Time
		rows.Scan(&id, &created, &expires, &agent)
		out = append(out, map[string]any{"id": id, "agent": agent, "created_at": created, "expires_at": expires, "current": cookie != nil && digest(cookie.Value) == id})
	}
	ok(w, out)
}
func (s *Server) revokeSession(w http.ResponseWriter, r *http.Request) {
	u, yes := s.require(w, r, false)
	if !yes {
		return
	}
	if _, e := s.db.Exec(r.Context(), "DELETE FROM sessions WHERE hash=$1 AND user_id=$2", r.PathValue("id"), u.ID); e != nil {
		fail(w, 500, "撤销失败")
		return
	}
	s.audit(r.Context(), u.ID, "session.revoked", "", true)
	ok(w, nil)
}
