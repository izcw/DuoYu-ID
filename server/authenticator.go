package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/skip2/go-qrcode"
)

// RFC 6238, SHA-1, six digits, 30-second periods (Authenticator compatible).
func totpCode(secret []byte, step int64) string {
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(step))
	h := hmac.New(sha1.New, secret)
	h.Write(counter[:])
	sum := h.Sum(nil)
	offset := sum[len(sum)-1] & 15
	value := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", value%1000000)
}
func totpStep(secret []byte, code string, now time.Time, last int64) int64 {
	if len(code) != 6 {
		return -1
	}
	for _, c := range code {
		if c < '0' || c > '9' {
			return -1
		}
	}
	step := now.Unix() / 30
	for _, candidate := range []int64{step, step - 1, step + 1} {
		if candidate > last && subtle.ConstantTimeCompare([]byte(totpCode(secret, candidate)), []byte(code)) == 1 {
			return candidate
		}
	}
	return -1
}
func (s *Server) authenticatorCipher() (cipher.AEAD, error) {
	key := sha256.Sum256(append([]byte("duoyu-authenticator-v1:"), s.key[:]...))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
func (s *Server) sealAuthenticator(uid string, secret []byte) ([]byte, error) {
	a, err := s.authenticatorCipher()
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, a.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, err
	}
	return a.Seal(nonce, nonce, secret, []byte(uid)), nil
}
func (s *Server) openAuthenticator(uid string, data []byte) ([]byte, error) {
	a, err := s.authenticatorCipher()
	if err != nil {
		return nil, err
	}
	if len(data) < a.NonceSize() {
		return nil, errors.New("invalid authenticator secret")
	}
	return a.Open(nil, data[:a.NonceSize()], data[a.NonceSize():], []byte(uid))
}
func (s *Server) authenticatorStatus(w http.ResponseWriter, r *http.Request) {
	u, yes := s.require(w, r, false)
	if !yes {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	var active bool
	var remaining int
	err := s.db.QueryRow(r.Context(), "SELECT active,cardinality(recovery_hashes) FROM authenticators WHERE user_id=$1", u.ID).Scan(&active, &remaining)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		fail(w, 503, "读取身份验证器失败")
		return
	}
	ok(w, map[string]any{"enabled": active, "recovery_remaining": remaining})
}
func (s *Server) beginAuthenticator(w http.ResponseWriter, r *http.Request) {
	u, yes := s.require(w, r, false)
	if !yes {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	var in struct {
		Code string `json:"code"`
	}
	if decode(r, &in) != nil {
		fail(w, 400, "请求格式错误")
		return
	}
	if s.limited(r, "totp-setup:"+u.ID, 10, time.Hour) || !s.consumeCode(r.Context(), u.Email, "email", in.Code) {
		fail(w, 400, "邮箱验证码无效或已过期")
		return
	}
	c, err := r.Cookie("duoyu_session")
	if err != nil {
		fail(w, 401, "请重新登录")
		return
	}
	secret := make([]byte, 20)
	if _, err = rand.Read(secret); err != nil {
		fail(w, 503, "生成密钥失败")
		return
	}
	encrypted, err := s.sealAuthenticator(u.ID, secret)
	if err != nil {
		fail(w, 503, "生成密钥失败")
		return
	}
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret)
	uri := "otpauth://totp/" + url.PathEscape("DuoYu ID:"+u.Email) + "?" + url.Values{"secret": {encoded}, "issuer": {"DuoYu ID"}, "algorithm": {"SHA1"}, "digits": {"6"}, "period": {"30"}}.Encode()
	png, err := qrcode.Encode(uri, qrcode.Medium, 240)
	if err != nil {
		fail(w, 503, "生成二维码失败")
		return
	}
	tag, err := s.db.Exec(r.Context(), `INSERT INTO authenticators(user_id,secret,setup_session,expires_at) VALUES($1,$2,$3,now()+interval '10 minutes') ON CONFLICT(user_id) DO UPDATE SET secret=EXCLUDED.secret,setup_session=EXCLUDED.setup_session,expires_at=EXCLUDED.expires_at,failures=0,locked_until='-infinity',last_step=-1 WHERE NOT authenticators.active`, u.ID, encrypted, digest(c.Value))
	if err != nil {
		fail(w, 503, "绑定暂不可用")
		return
	}
	if tag.RowsAffected() == 0 {
		fail(w, 409, "已绑定身份验证器，请先解绑")
		return
	}
	s.audit(r.Context(), u.ID, "authenticator.setup", "", true)
	ok(w, map[string]string{"secret": encoded, "image": "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)})
}

// The row lock makes time-step consumption and recovery codes single-use, including concurrent requests.
func (s *Server) verifyAuthenticator(ctx context.Context, uid, code, setupSession string, recovery bool, operation string) ([]string, bool, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback(ctx)
	var encrypted []byte
	var active bool
	var session string
	var expires time.Time
	var last int64
	var hashes []string
	var locked bool
	err = tx.QueryRow(ctx, `SELECT secret,active,setup_session,expires_at,last_step,recovery_hashes,locked_until>now() FROM authenticators WHERE user_id=$1 FOR UPDATE`, uid).Scan(&encrypted, &active, &session, &expires, &last, &hashes, &locked)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	setup := operation == "activate"
	if locked || (setup && (active || session != setupSession || time.Now().After(expires))) || (!setup && !active) {
		return nil, false, nil
	}
	secret, err := s.openAuthenticator(uid, encrypted)
	if err != nil {
		return nil, false, err
	}
	step := int64(-1)
	valid := false
	if recovery && !setup {
		hash := s.mac("totp-recovery:" + uid + ":" + strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(code), "-", "")))
		if i := slices.Index(hashes, hash); i >= 0 {
			hashes = append(hashes[:i], hashes[i+1:]...)
			valid = true
		}
	} else {
		step = totpStep(secret, strings.TrimSpace(code), time.Now(), last)
		valid = step >= 0
	}
	if !valid {
		_, err = tx.Exec(ctx, `UPDATE authenticators SET locked_until=CASE WHEN failures+1>=5 THEN now()+interval '10 minutes' ELSE locked_until END, failures=CASE WHEN failures+1>=5 THEN 0 ELSE failures+1 END WHERE user_id=$1`, uid)
		if err != nil {
			return nil, false, err
		}
		return nil, false, tx.Commit(ctx)
	}
	codes := []string{}
	if setup {
		hashes = []string{}
		for i := 0; i < 8; i++ {
			b := make([]byte, 10)
			if _, err = rand.Read(b); err != nil {
				return nil, false, err
			}
			raw := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)
			codes = append(codes, raw[:4]+"-"+raw[4:8]+"-"+raw[8:12]+"-"+raw[12:])
			hashes = append(hashes, s.mac("totp-recovery:"+uid+":"+raw))
		}
	}
	if operation == "delete" {
		if _, err = tx.Exec(ctx, "DELETE FROM authenticators WHERE user_id=$1", uid); err != nil {
			return nil, false, err
		}
		if _, err = tx.Exec(ctx, "DELETE FROM sessions WHERE user_id=$1", uid); err != nil {
			return nil, false, err
		}
		if _, err = tx.Exec(ctx, "DELETE FROM access_tokens WHERE user_id=$1", uid); err != nil {
			return nil, false, err
		}
	} else {
		if step < 0 {
			step = last
		}
		_, err = tx.Exec(ctx, `UPDATE authenticators SET active=true,last_step=$2,recovery_hashes=$3,failures=0,locked_until='-infinity',setup_session='' WHERE user_id=$1`, uid, step, hashes)
		if err != nil {
			return nil, false, err
		}
	}
	err = tx.Commit(ctx)
	return codes, err == nil, err
}
func (s *Server) finishAuthenticator(w http.ResponseWriter, r *http.Request) {
	u, yes := s.require(w, r, false)
	if !yes {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	var in struct {
		Code string `json:"code"`
	}
	if decode(r, &in) != nil {
		fail(w, 400, "请求格式错误")
		return
	}
	c, err := r.Cookie("duoyu_session")
	if err != nil {
		fail(w, 401, "请重新登录")
		return
	}
	codes, valid, err := s.verifyAuthenticator(r.Context(), u.ID, in.Code, digest(c.Value), false, "activate")
	if err != nil {
		fail(w, 503, "绑定暂不可用")
		return
	}
	if !valid {
		fail(w, 400, "动态码错误、已使用或绑定已过期；连续错误 5 次需等待 10 分钟")
		return
	}
	s.audit(r.Context(), u.ID, "authenticator.bound", "", true)
	ok(w, map[string]any{"recovery_codes": codes})
}
func (s *Server) deleteAuthenticator(w http.ResponseWriter, r *http.Request) {
	u, yes := s.require(w, r, false)
	if !yes {
		return
	}
	var in struct {
		EmailCode string `json:"email_code"`
		Code      string `json:"code"`
		Recovery  bool   `json:"recovery"`
	}
	if decode(r, &in) != nil {
		fail(w, 400, "请求格式错误")
		return
	}
	if !s.consumeCode(r.Context(), u.Email, "email", in.EmailCode) {
		fail(w, 400, "邮箱验证码无效或已过期")
		return
	}
	_, valid, err := s.verifyAuthenticator(r.Context(), u.ID, in.Code, "", in.Recovery, "delete")
	if err != nil {
		fail(w, 503, "解绑暂不可用")
		return
	}
	if !valid {
		fail(w, 400, "动态码或恢复码无效，请使用新的邮箱验证码重试")
		return
	}
	s.cookie(w, "duoyu_session", "", -1)
	s.audit(r.Context(), u.ID, "authenticator.removed", "", true)
	ok(w, nil)
}
func (s *Server) loginAuthenticator(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Account       string `json:"account"`
		Code          string `json:"code"`
		Recovery      bool   `json:"recovery"`
		AuthRequestID string `json:"auth_request_id"`
	}
	if decode(r, &in) != nil || len(in.Account) > 254 || len(in.Code) > 64 {
		fail(w, 400, "请检查登录信息")
		return
	}
	if !s.checkProjectMethod(w, r, in.AuthRequestID, "totp") {
		return
	}
	account := strings.ToLower(strings.TrimSpace(in.Account))
	if s.limited(r, "totp-ip:"+remoteIP(r), 50, 15*time.Minute) || s.limited(r, "totp-account:"+account, 15, 15*time.Minute) {
		fail(w, 429, "尝试次数过多，请稍后再试")
		return
	}
	var uid, email string
	err := s.db.QueryRow(r.Context(), "SELECT id,email FROM users WHERE (email=$1 OR account_id=$1) AND enabled", account).Scan(&uid, &email)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 401, "账号或动态码无效，或暂时锁定；连续错误 5 次请等待 10 分钟")
		return
	}
	if err != nil {
		fail(w, 503, "登录暂不可用")
		return
	}
	if !s.checkLoginProject(w, r, email, in.AuthRequestID) {
		return
	}
	_, valid, err := s.verifyAuthenticator(r.Context(), uid, in.Code, "", in.Recovery, "login")
	if err != nil {
		fail(w, 503, "登录暂不可用")
		return
	}
	if !valid {
		s.audit(r.Context(), uid, "login.failed", "totp", false)
		fail(w, 401, "账号或动态码无效、已使用，或暂时锁定；连续错误 5 次请等待 10 分钟")
		return
	}
	if err = s.newSession(w, r, uid, "otp totp"); err != nil {
		fail(w, 503, "创建会话失败")
		return
	}
	s.audit(r.Context(), uid, "login.success", "totp", true)
	ok(w, map[string]bool{"logged_in": true})
}
