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
func (s *Server) storeWA(w http.ResponseWriter, r *http.Request, uid, purpose string, data *webauthn.SessionData, qrEnrollment ...bool) error {
	qrEnabled := len(qrEnrollment) > 0 && qrEnrollment[0]
	id := randomID()
	b, e := json.Marshal(data)
	if e != nil {
		return e
	}
	_, e = s.db.Exec(r.Context(), "INSERT INTO webauthn_challenges(id,user_id,purpose,data,expires_at,qr_enabled) VALUES($1,$2,$3,$4,$5,$6)", digest(id), uid, purpose, b, time.Now().Add(5*time.Minute), qrEnabled)
	if e == nil {
		s.cookie(w, "duoyu_webauthn", id, 300)
	}
	return e
}
func (s *Server) takeWA(w http.ResponseWriter, r *http.Request, uid, purpose string) (webauthn.SessionData, error) {
	var data webauthn.SessionData
	c, e := r.Cookie("duoyu_webauthn")
	if e != nil {
		return data, e
	}
	var b []byte
	e = s.db.QueryRow(r.Context(), "DELETE FROM webauthn_challenges WHERE id=$1 AND user_id=$2 AND purpose=$3 AND expires_at>now() RETURNING data", digest(c.Value), uid, purpose).Scan(&b)
	s.cookie(w, "duoyu_webauthn", "", -1)
	if e != nil {
		return data, e
	}
	e = json.Unmarshal(b, &data)
	return data, e
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
	if decode(r, &in) != nil || !s.consumeCode(r.Context(), u.Email, "login", in.Code) {
		fail(w, 400, "请先验证邮箱验证码")
		return
	}
	p, e := s.passkeyUser(r.Context(), u.ID)
	if e != nil || len(p.Credentials) >= 10 {
		fail(w, 400, "无法创建，最多绑定 10 个通行密钥")
		return
	}
	options, data, e := s.wa.BeginRegistration(p, webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementRequired), webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{ResidentKey: protocol.ResidentKeyRequirementRequired, UserVerification: protocol.VerificationRequired}))
	if e != nil || s.storeWA(w, r, u.ID, "register", data, in.QR) != nil {
		fail(w, 500, "无法创建注册请求")
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
	var qrEnabled bool
	if cookie, err := r.Cookie("duoyu_webauthn"); err == nil {
		if err = s.db.QueryRow(r.Context(), "SELECT qr_enabled FROM webauthn_challenges WHERE id=$1 AND user_id=$2 AND purpose='register'", digest(cookie.Value), u.ID).Scan(&qrEnabled); err != nil {
			fail(w, 400, "请求已过期")
			return
		}
	}
	data, e := s.takeWA(w, r, u.ID, "register")
	if e != nil {
		fail(w, 400, "请求已过期")
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
	b, _ := json.Marshal(c)
	_, e = s.db.Exec(r.Context(), "INSERT INTO passkeys(id,user_id,name,credential,registration_device,qr_enabled) VALUES($1,$2,$3,$4,$5,$6)", base64.RawURLEncoding.EncodeToString(c.ID), u.ID, "通行密钥 · "+time.Now().Format("2006-01-02"), b, registrationDevice(r.UserAgent()), qrEnabled)
	if e != nil {
		fail(w, 409, "密钥已存在或保存失败")
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
	if decode(r, &in) != nil || !s.consumeCode(r.Context(), u.Email, "login", in.Code) {
		fail(w, 400, "请先验证邮箱验证码")
		return
	}
	if _, e := s.db.Exec(r.Context(), "DELETE FROM passkeys WHERE id=$1 AND user_id=$2", r.PathValue("id"), u.ID); e != nil {
		fail(w, 500, "删除失败")
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
	if e != nil || s.storeWA(w, r, "", purpose, data) != nil {
		fail(w, 500, "无法创建登录请求")
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
	data, e := s.takeWA(w, r, "", purpose)
	if e != nil {
		fail(w, 400, "登录请求已过期")
		return
	}
	var logged passkeyUser
	c, e := s.wa.FinishDiscoverableLogin(func(rawID, userHandle []byte) (webauthn.User, error) {
		p, e := s.passkeyUser(r.Context(), string(userHandle))
		if e != nil {
			return nil, errors.New("invalid user")
		}
		logged = p
		return p, nil
	}, data, r)
	if e != nil || c.Authenticator.CloneWarning {
		fail(w, 401, "通行密钥验证失败")
		return
	}
	b, _ := json.Marshal(c)
	tag, e := s.db.Exec(r.Context(), "UPDATE passkeys SET credential=$1 WHERE id=$2 AND user_id=$3 AND ($4=false OR qr_enabled)", b, base64.RawURLEncoding.EncodeToString(c.ID), logged.ID, method == "qr")
	if e != nil || tag.RowsAffected() != 1 {
		fail(w, 401, "密钥已撤销")
		return
	}
	amr := "hwk user"
	if method == "qr" {
		amr = "hwk user qr"
	}
	if s.newSession(w, r, logged.ID, amr, base64.RawURLEncoding.EncodeToString(c.ID)) != nil {
		fail(w, 500, "创建会话失败")
		return
	}
	s.audit(r.Context(), logged.ID, "login.success", "passkey", true)
	ok(w, nil)
}

func (s *Server) requireWebAuthn(w http.ResponseWriter) bool {
	if s.wa != nil {
		return true
	}
	fail(w, 503, "当前认证地址为 IP，通行密钥与扫码验证需要配置 HTTPS 域名；请使用邮箱、密码或身份验证器登录")
	return false
}
