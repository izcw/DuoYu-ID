package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func (s *Server) sendCode(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email         string `json:"email"`
		Purpose       string `json:"purpose"`
		AuthRequestID string `json:"auth_request_id"`
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
	if in.Purpose == "login" && !s.checkLoginProject(w, r, in.Email, in.AuthRequestID) {
		return
	}
	if in.Purpose == "login" && !s.checkProjectMethod(w, r, in.AuthRequestID, "code") {
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
	var exists bool
	if in.Purpose == "reset" {
		if err := s.db.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM users WHERE email=$1 AND enabled)", in.Email).Scan(&exists); err != nil {
			fail(w, 503, "账号服务暂不可用，请稍后再试")
			return
		}
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

type emailCode struct{ email, value string }

// The authenticated browser session and its owner must still be current at
// the time a credential is changed. Serialize enrollment limits per user.
func lockCredentialSession(w http.ResponseWriter, r *http.Request, tx pgx.Tx, u User) bool {
	c, err := r.Cookie("duoyu_session")
	var id string
	if err == nil {
		err = tx.QueryRow(r.Context(), `SELECT u.id FROM users u JOIN sessions s ON s.user_id=u.id
			WHERE u.id=$1 AND u.email=$2 AND u.enabled AND s.hash=$3 AND s.expires_at>clock_timestamp()
			FOR UPDATE OF u FOR SHARE OF s`, u.ID, u.Email, digest(c.Value)).Scan(&id)
	}
	if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, http.ErrNoCookie) {
		fail(w, 401, "请重新登录")
		return false
	}
	if err != nil {
		fail(w, 503, "暂时无法核验登录状态")
		return false
	}
	return true
}

// Lock multiple addresses in a consistent order. Failed attempts count, but a
// valid code is only deleted once all the required proofs have matched.
func (s *Server) matchEmailCodes(ctx context.Context, tx pgx.Tx, purpose string, codes ...emailCode) (bool, error) {
	slices.SortFunc(codes, func(a, b emailCode) int { return strings.Compare(a.email, b.email) })
	for _, c := range codes {
		var want string
		var attempts int
		var expired bool
		err := tx.QueryRow(ctx, "SELECT digest,attempts,expires_at<=clock_timestamp() FROM challenges WHERE email=$1 AND purpose=$2 FOR UPDATE", c.email, purpose).Scan(&want, &attempts, &expired)
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if expired || attempts >= 5 {
			return false, nil
		}
		if len(c.value) != 6 || subtle.ConstantTimeCompare([]byte(want), []byte(s.mac(c.email+"|"+purpose+"|"+c.value))) != 1 {
			_, err = tx.Exec(ctx, "UPDATE challenges SET attempts=attempts+1 WHERE email=$1 AND purpose=$2", c.email, purpose)
			return false, err
		}
	}
	return true, nil
}

func (s *Server) checkEmailCodes(ctx context.Context, tx pgx.Tx, purpose string, codes ...emailCode) (bool, error) {
	if ok, err := s.matchEmailCodes(ctx, tx, purpose, codes...); err != nil || !ok {
		return false, err
	}
	for _, c := range codes {
		if _, err := tx.Exec(ctx, "DELETE FROM challenges WHERE email=$1 AND purpose=$2", c.email, purpose); err != nil {
			return false, err
		}
	}
	return true, nil
}

func (s *Server) consumeCode(ctx context.Context, email, purpose, code string) bool {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return false
	}
	defer tx.Rollback(ctx)
	good, err := s.checkEmailCodes(ctx, tx, purpose, emailCode{email, code})
	return err == nil && tx.Commit(ctx) == nil && good
}

// A successful proof stays in this transaction until the account change and
// revocation commit. Commit only the attempt counter on an invalid proof.
func (s *Server) authorizeChange(w http.ResponseWriter, ctx context.Context, tx pgx.Tx, purpose string, codes ...emailCode) bool {
	valid, err := s.checkEmailCodes(ctx, tx, purpose, codes...)
	if err == nil && !valid {
		err = tx.Commit(ctx)
	}
	if err != nil {
		fail(w, 503, "验证码验证暂不可用")
		return false
	}
	if !valid {
		fail(w, 400, "验证码无效或已过期")
		return false
	}
	return true
}

func revokeAccountCredentials(ctx context.Context, tx pgx.Tx, id string) error {
	if _, err := tx.Exec(ctx, "DELETE FROM sessions WHERE user_id=$1", id); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, "DELETE FROM access_tokens WHERE user_id=$1", id)
	return err
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email         string `json:"email"`
		Password      string `json:"password"`
		Code          string `json:"code"`
		Mode          string `json:"mode"`
		Register      bool   `json:"register"`
		AuthRequestID string `json:"auth_request_id"`
	}
	if decode(r, &in) != nil {
		fail(w, 400, "请求格式错误")
		return
	}
	in.Email = normalizeEmail(in.Email)
	if in.Email == "" || len(in.Password) > 256 || (in.Mode != "code" && in.Mode != "password") {
		fail(w, 400, "请检查登录信息")
		return
	}
	if !s.checkLoginProject(w, r, in.Email, in.AuthRequestID) {
		return
	}
	if !s.checkProjectMethod(w, r, in.AuthRequestID, in.Mode) {
		return
	}
	if s.limited(r, "login-ip:"+remoteIP(r), 50, 15*time.Minute) || s.limited(r, "login-email:"+in.Email, 15, 15*time.Minute) {
		// Keep the account's remaining lock time visible even when request limiting also applies.
		if in.Mode == "password" {
			var until time.Time
			if s.db.QueryRow(r.Context(), "SELECT password_locked_until FROM users WHERE email=$1 AND password_locked_until>now()", in.Email).Scan(&until) == nil {
				passwordLockedResponse(w, until)
				return
			}
		}
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
		var lockedUntil time.Time
		valid, lockedUntil, e = s.verifyAccountPassword(r.Context(), in.Email, in.Password)
		if e != nil {
			fail(w, 503, "登录暂不可用")
			return
		}
		if !lockedUntil.IsZero() {
			s.audit(r.Context(), id, "login.failed", "", false)
			passwordLockedResponse(w, lockedUntil)
			return
		}
	}
	if !valid || (!missing && !enabled) {
		s.audit(r.Context(), id, "login.failed", "", false)
		fail(w, 401, "邮箱、密码或验证码不正确")
		return
	}
	if missing {
		var registration string
		if err := s.db.QueryRow(r.Context(), "SELECT value FROM settings WHERE key='registration'").Scan(&registration); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			fail(w, 503, "注册设置暂不可用")
			return
		}
		if in.Mode != "code" || registration != "true" {
			fail(w, 403, "当前未开放新账号注册，请联系管理员")
			return
		}
		id = randomID()
		initialPassword, err := randomRegistrationPassword()
		if err != nil {
			fail(w, 503, "账号创建暂不可用，请重试")
			return
		}
		tag, e := s.db.Exec(r.Context(), `INSERT INTO users(id,email,name,password_hash) SELECT $1,$2,$3,$4
			FROM settings WHERE key='registration' AND value='true' FOR SHARE`, id, in.Email, strings.Split(in.Email, "@")[0], hashPassword(initialPassword))
		if e != nil {
			fail(w, 409, "账号创建失败，请重新登录")
			return
		}
		if tag.RowsAffected() == 0 {
			fail(w, 403, "当前未开放新账号注册，请联系管理员")
			return
		}
		s.audit(r.Context(), id, "account.created", "", true)
	}
	if err := s.issueEmailSession(w, r, id, amr, in.Email, h); errors.Is(err, pgx.ErrNoRows) {
		fail(w, 401, "账号已变更，请重新登录")
		return
	} else if err != nil {
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
		fail(w, 400, "密码须为 6–64 个字符")
		return
	}
	in.Email = normalizeEmail(in.Email)
	if s.limited(r, "reset:"+remoteIP(r), 20, time.Hour) {
		fail(w, 400, "验证码无效或已过期")
		return
	}
	tx, e := s.db.Begin(r.Context())
	if e != nil {
		fail(w, 500, "更新失败")
		return
	}
	defer tx.Rollback(r.Context())
	if !s.authorizeChange(w, r.Context(), tx, "reset", emailCode{in.Email, in.Code}) {
		return
	}
	var id string
	e = tx.QueryRow(r.Context(), "UPDATE users SET password_hash=$1,password_failures=0,password_lock_level=0,password_locked_until=NULL WHERE email=$2 AND enabled RETURNING id", hashPassword(in.Password), in.Email).Scan(&id)
	if errors.Is(e, pgx.ErrNoRows) {
		fail(w, 400, "无法重置该账号")
		return
	}
	if e != nil || revokeAccountCredentials(r.Context(), tx, id) != nil {
		fail(w, 503, "更新失败")
		return
	}
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
		fail(w, 400, "密码须为 6–64 个字符")
		return
	}
	tx, e := s.db.Begin(r.Context())
	if e != nil {
		fail(w, 500, "更新失败")
		return
	}
	defer tx.Rollback(r.Context())
	if !s.authorizeChange(w, r.Context(), tx, "reset", emailCode{u.Email, in.Code}) {
		return
	}
	tag, err := tx.Exec(r.Context(), "UPDATE users SET password_hash=$1,password_failures=0,password_lock_level=0,password_locked_until=NULL WHERE id=$2 AND email=$3 AND enabled", hashPassword(in.Password), u.ID, u.Email)
	if err != nil {
		fail(w, 503, "更新失败")
		return
	}
	if tag.RowsAffected() != 1 {
		fail(w, 409, "账号已变更，请重新登录")
		return
	}
	if revokeAccountCredentials(r.Context(), tx, u.ID) != nil {
		fail(w, 503, "更新失败")
		return
	}
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
	tx, e := s.db.Begin(r.Context())
	if e != nil {
		fail(w, 500, "更新失败")
		return
	}
	defer tx.Rollback(r.Context())
	if !s.authorizeChange(w, r.Context(), tx, "email", emailCode{u.Email, in.OldCode}, emailCode{in.Email, in.NewCode}) {
		return
	}
	tag, e := tx.Exec(r.Context(), "UPDATE users SET email=$1 WHERE id=$2 AND email=$3 AND enabled", in.Email, u.ID, u.Email)
	if e != nil {
		fail(w, 409, "该邮箱不可用")
		return
	}
	if tag.RowsAffected() != 1 {
		fail(w, 409, "账号已变更，请重新登录")
		return
	}
	if revokeAccountCredentials(r.Context(), tx, u.ID) != nil {
		fail(w, 503, "更新失败")
		return
	}
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
		if err := rows.Scan(&id, &created, &expires, &agent); err != nil {
			fail(w, 503, "会话查询暂不可用")
			return
		}
		out = append(out, map[string]any{"id": id, "agent": agent, "created_at": created, "expires_at": expires, "current": cookie != nil && digest(cookie.Value) == id})
	}
	if rows.Err() != nil {
		fail(w, 503, "会话查询暂不可用")
		return
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
