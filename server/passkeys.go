package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/jackc/pgx/v5"
)

type passkeyUser struct {
	User
	Credentials []webauthn.Credential
}

func (u passkeyUser) WebAuthnID() []byte                         { return []byte(u.ID) }
func (u passkeyUser) WebAuthnName() string                       { return u.Email }
func (u passkeyUser) WebAuthnDisplayName() string                { return u.Name }
func (u passkeyUser) WebAuthnCredentials() []webauthn.Credential { return u.Credentials }
func (s *Server) passkeyUser(ctx context.Context, id string) (passkeyUser, error) {
	u, e := s.user(ctx, id)
	p := passkeyUser{User: u}
	if e != nil {
		return p, e
	}
	rows, e := s.db.Query(ctx, "SELECT credential FROM passkeys WHERE user_id=$1", id)
	if e != nil {
		return p, e
	}
	defer rows.Close()
	for rows.Next() {
		var b []byte
		if e = rows.Scan(&b); e != nil {
			return p, e
		}
		var c webauthn.Credential
		if e = json.Unmarshal(b, &c); e != nil {
			return p, e
		}
		p.Credentials = append(p.Credentials, c)
	}
	return p, rows.Err()
}
func (s *Server) listPasskeys(w http.ResponseWriter, r *http.Request) {
	u, yes := s.require(w, r, false)
	if !yes {
		return
	}
	rows, err := s.db.Query(r.Context(), "SELECT id,name,created_at,registration_device,credential,qr_enabled FROM passkeys WHERE user_id=$1 ORDER BY created_at DESC", u.ID)
	if err != nil {
		fail(w, 500, "读取通行密钥失败")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, name, device string
		var created time.Time
		var raw []byte
		var credential webauthn.Credential
		var qrEnabled bool
		if err = rows.Scan(&id, &name, &created, &device, &raw, &qrEnabled); err != nil {
			fail(w, 500, "读取通行密钥失败")
			return
		}
		if err = json.Unmarshal(raw, &credential); err != nil {
			fail(w, 500, "读取通行密钥失败")
			return
		}
		kind, methods := passkeyDetails(credential)
		if device == "" {
			device = "历史记录未保存"
		}
		out = append(out, map[string]any{"id": id, "name": name, "created_at": created, "registration_device": device, "device_type": kind, "methods": methods, "backup_eligible": credential.Flags.BackupEligible, "qr_enabled": qrEnabled})
	}
	if rows.Err() != nil {
		fail(w, 500, "读取通行密钥失败")
		return
	}
	ok(w, out)
}
func (s *Server) storeWA(w http.ResponseWriter, r *http.Request, u *User, code, purpose string, data *webauthn.SessionData, qrEnabled bool) bool {
	id := randomID()
	b, e := json.Marshal(data)
	if e != nil {
		fail(w, 500, "无法创建验证请求")
		return false
	}
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		fail(w, 503, "无法创建验证请求")
		return false
	}
	defer tx.Rollback(r.Context())
	uid := ""
	if u != nil {
		uid = u.ID
		if !s.authorizeChange(w, r.Context(), tx, "login", emailCode{u.Email, code}) || !lockCredentialSession(w, r, tx, *u) {
			return false
		}
		cookie, _ := r.Cookie("duoyu_session")
		purpose += ":" + digest(cookie.Value)
	}
	_, e = tx.Exec(r.Context(), "INSERT INTO webauthn_challenges(id,user_id,purpose,data,expires_at,qr_enabled) VALUES($1,$2,$3,$4,$5,$6)", digest(id), uid, purpose, b, time.Now().Add(5*time.Minute), qrEnabled)
	if e != nil || tx.Commit(r.Context()) != nil {
		fail(w, 503, "无法保存验证请求")
		return false
	}
	s.cookie(w, "duoyu_webauthn", id, 300)
	return true
}
func (s *Server) takeWA(w http.ResponseWriter, r *http.Request, uid, purpose string) (webauthn.SessionData, bool, error) {
	var data webauthn.SessionData
	c, e := r.Cookie("duoyu_webauthn")
	if e != nil {
		return data, false, e
	}
	var b []byte
	bound := purpose
	if purpose == "register" {
		session, e := r.Cookie("duoyu_session")
		if e != nil {
			return data, false, e
		}
		bound += ":" + digest(session.Value)
	}
	var qrEnabled bool
	// The plain registration purpose covers unexpired requests from the prior release.
	e = s.db.QueryRow(r.Context(), "DELETE FROM webauthn_challenges WHERE id=$1 AND user_id=$2 AND purpose IN ($3,$4) AND expires_at>now() RETURNING data,qr_enabled", digest(c.Value), uid, purpose, bound).Scan(&b, &qrEnabled)
	if e == nil || errors.Is(e, pgx.ErrNoRows) {
		s.cookie(w, "duoyu_webauthn", "", -1)
	}
	if e != nil {
		return data, false, e
	}
	e = json.Unmarshal(b, &data)
	return data, qrEnabled, e
}
func (s *Server) beginPasskey(w http.ResponseWriter, r *http.Request) {
	if !s.requireWebAuthn(w) {
		return
	}
	u, yes := s.require(w, r, false)
	if !yes {
		return
	}
	var in struct {
		Code string `json:"code"`
		QR   bool   `json:"qr"`
	}
	if decode(r, &in) != nil {
		fail(w, 400, "请先验证邮箱验证码")
		return
	}
	p, e := s.passkeyUser(r.Context(), u.ID)
	if e != nil {
		fail(w, 503, "通行密钥暂不可读取")
		return
	}
	if len(p.Credentials) >= 10 {
		fail(w, 400, "无法创建，最多绑定 10 个通行密钥")
		return
	}
	options, data, e := s.wa.BeginRegistration(p, webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementRequired), webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{ResidentKey: protocol.ResidentKeyRequirementRequired, UserVerification: protocol.VerificationRequired}))
	if e != nil {
		fail(w, 500, "无法创建注册请求")
		return
	}
	if !s.storeWA(w, r, &u, in.Code, "register", data, in.QR) {
		return
	}
	ok(w, options)
}
func (s *Server) finishPasskey(w http.ResponseWriter, r *http.Request) {
	if !s.requireWebAuthn(w) {
		return
	}
	u, yes := s.require(w, r, false)
	if !yes {
		return
	}
	data, qrEnabled, e := s.takeWA(w, r, u.ID, "register")
	if e != nil {
		waChallengeFailure(w, e)
		return
	}
	p, e := s.passkeyUser(r.Context(), u.ID)
	if e != nil {
		fail(w, 400, "账号不可用")
		return
	}
	c, e := s.wa.FinishRegistration(p, data, r)
	if e != nil {
		fail(w, 400, "通行密钥验证失败")
		return
	}
	b, e := json.Marshal(c)
	if e != nil {
		fail(w, 500, "密钥保存失败")
		return
	}
	tx, e := s.db.Begin(r.Context())
	if e != nil {
		fail(w, 503, "密钥保存失败")
		return
	}
	defer tx.Rollback(r.Context())
	if !lockCredentialSession(w, r, tx, u) {
		return
	}
	tag, e := tx.Exec(r.Context(), `INSERT INTO passkeys(id,user_id,name,credential,registration_device,qr_enabled)
		SELECT $1,$2,$3,$4,$5,$6 WHERE (SELECT count(*) FROM passkeys WHERE user_id=$2)<10
		ON CONFLICT(id) DO NOTHING`, base64.RawURLEncoding.EncodeToString(c.ID), u.ID, "通行密钥 · "+time.Now().Format("2006-01-02"), b, registrationDevice(r.UserAgent()), qrEnabled)
	if e != nil {
		fail(w, 503, "密钥保存失败")
		return
	}
	if tag.RowsAffected() != 1 {
		fail(w, 409, "密钥已存在或已达到 10 个上限")
		return
	}
	if tx.Commit(r.Context()) != nil {
		fail(w, 503, "密钥保存失败")
		return
	}
	s.audit(r.Context(), u.ID, "passkey.added", "", true)
	ok(w, nil)
}
func (s *Server) deletePasskey(w http.ResponseWriter, r *http.Request) {
	u, yes := s.require(w, r, false)
	if !yes {
		return
	}
	var in struct {
		Code string `json:"code"`
	}
	if decode(r, &in) != nil {
		fail(w, 400, "请先验证邮箱验证码")
		return
	}
	tx, e := s.db.Begin(r.Context())
	if e != nil {
		fail(w, 503, "删除暂不可用")
		return
	}
	defer tx.Rollback(r.Context())
	if !s.authorizeChange(w, r.Context(), tx, "login", emailCode{u.Email, in.Code}) || !lockCredentialSession(w, r, tx, u) {
		return
	}
	tag, e := tx.Exec(r.Context(), "DELETE FROM passkeys WHERE id=$1 AND user_id=$2", r.PathValue("id"), u.ID)
	if e != nil {
		fail(w, 500, "删除失败")
		return
	}
	if tag.RowsAffected() == 0 {
		ok(w, nil)
		return
	} // Idempotent; preserve the unused proof.
	if tx.Commit(r.Context()) != nil {
		fail(w, 503, "删除失败")
		return
	}
	s.audit(r.Context(), u.ID, "passkey.deleted", "", true)
	ok(w, nil)
}
func (s *Server) beginPasskeyLogin(w http.ResponseWriter, r *http.Request) {
	if !s.requireWebAuthn(w) {
		return
	}
	method, purpose := "passkey", "login"
	if strings.HasPrefix(r.URL.Path, "/api/auth/qr/device/") {
		method, purpose = "qr", "qr-device-login"
	}
	if !s.checkProjectMethod(w, r, "", method) {
		return
	}
	if s.limited(r, "passkey:"+remoteIP(r), 30, 15*time.Minute) {
		fail(w, 429, "尝试过于频繁")
		return
	}
	options, data, e := s.wa.BeginDiscoverableLogin(webauthn.WithUserVerification(protocol.VerificationRequired))
	if e != nil {
		fail(w, 500, "无法创建登录请求")
		return
	}
	if !s.storeWA(w, r, nil, "", purpose, data, false) {
		return
	}
	ok(w, options)
}
func (s *Server) finishPasskeyLogin(w http.ResponseWriter, r *http.Request) {
	if !s.requireWebAuthn(w) {
		return
	}
	method, purpose := "passkey", "login"
	if strings.HasPrefix(r.URL.Path, "/api/auth/qr/device/") {
		method, purpose = "qr", "qr-device-login"
	}
	if !s.checkProjectMethod(w, r, "", method) {
		return
	}
	data, _, e := s.takeWA(w, r, "", purpose)
	if e != nil {
		waChallengeFailure(w, e)
		return
	}
	parsed, e := protocol.ParseCredentialRequestResponse(r)
	if e != nil {
		fail(w, 401, "通行密钥验证失败")
		return
	}
	tx, e := s.db.Begin(r.Context())
	if e != nil {
		fail(w, 503, "通行密钥登录暂不可用")
		return
	}
	defer tx.Rollback(r.Context())
	var logged passkeyUser
	var lookupErr error
	c, e := s.wa.ValidateDiscoverableLogin(func(rawID, userHandle []byte) (webauthn.User, error) {
		var body []byte
		lookupErr = tx.QueryRow(r.Context(), `SELECT u.id,u.email,u.name,p.credential FROM users u JOIN passkeys p ON p.user_id=u.id
			WHERE u.id=$1 AND u.enabled AND p.id=$2 AND (NOT $3 OR p.qr_enabled)
			FOR SHARE OF u FOR UPDATE OF p`, string(userHandle), base64.RawURLEncoding.EncodeToString(rawID), method == "qr").Scan(&logged.ID, &logged.Email, &logged.Name, &body)
		if lookupErr != nil {
			return nil, lookupErr
		}
		var credential webauthn.Credential
		if lookupErr = json.Unmarshal(body, &credential); lookupErr != nil {
			return nil, lookupErr
		}
		logged.Credentials = []webauthn.Credential{credential}
		return logged, nil
	}, data, parsed)
	if lookupErr != nil && !errors.Is(lookupErr, pgx.ErrNoRows) {
		fail(w, 503, "通行密钥暂不可读取")
		return
	}
	if e != nil || c.Authenticator.CloneWarning {
		fail(w, 401, "通行密钥验证失败")
		return
	}
	b, e := json.Marshal(c)
	if e == nil {
		_, e = tx.Exec(r.Context(), "UPDATE passkeys SET credential=$1 WHERE id=$2 AND user_id=$3", b, base64.RawURLEncoding.EncodeToString(c.ID), logged.ID)
	}
	if e != nil {
		fail(w, 503, "密钥状态保存失败")
		return
	}
	amr := "hwk user"
	if method == "qr" {
		amr = "hwk user qr"
	}
	if s.commitLoginSession(w, r, tx, logged.ID, amr, base64.RawURLEncoding.EncodeToString(c.ID)) != nil {
		fail(w, 500, "创建会话失败")
		return
	}
	s.audit(r.Context(), logged.ID, "login.success", "passkey", true)
	ok(w, nil)
}

func waChallengeFailure(w http.ResponseWriter, err error) {
	if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, http.ErrNoCookie) {
		fail(w, 400, "验证请求已过期")
		return
	}
	fail(w, 503, "暂时无法读取验证请求")
}

func (s *Server) requireWebAuthn(w http.ResponseWriter) bool {
	if s.wa != nil {
		return true
	}
	fail(w, 503, "当前认证地址为 IP，通行密钥与扫码验证需要配置 HTTPS 域名；请使用邮箱、密码或身份验证器登录")
	return false
}
