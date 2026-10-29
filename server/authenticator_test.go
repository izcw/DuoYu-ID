package main

import (
	"context"
	"encoding/base32"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTOTP(t *testing.T) {
	secret := []byte("12345678901234567890")
	// RFC 6238 Appendix B SHA-1 vectors reduced to six digits.
	for _, v := range []struct {
		ts   int64
		code string
	}{{59, "287082"}, {1111111109, "081804"}, {1111111111, "050471"}, {1234567890, "005924"}, {2000000000, "279037"}, {20000000000, "353130"}} {
		if got := totpCode(secret, v.ts/30); got != v.code {
			t.Fatalf("%d: %s", v.ts, got)
		}
		if totpStep(secret, v.code, time.Unix(v.ts, 0), v.ts/30) != -1 {
			t.Fatal("replay")
		}
	}
	now := time.Unix(1234567890, 0)
	if totpStep(secret, totpCode(secret, now.Unix()/30-2), now, -1) != -1 {
		t.Fatal("expired")
	}
	s := &Server{}
	cipher, err := s.sealAuthenticator("alice", secret)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.openAuthenticator("bob", cipher); err == nil {
		t.Fatal("AAD not enforced")
	}
	cipher[len(cipher)-1] ^= 1
	if _, err = s.openAuthenticator("alice", cipher); err == nil {
		t.Fatal("tamper accepted")
	}
}
func testAuthenticator(t *testing.T, s *Server, origin string) {
	ctx := context.Background()
	uid := randomID()
	email := "totp-test@example.com"
	if _, err := s.db.Exec(ctx, "INSERT INTO users(id,email,name) VALUES($1,$2,'TOTP test')", uid, email); err != nil {
		t.Fatal(err)
	}
	var account string
	s.db.QueryRow(ctx, "SELECT account_id FROM users WHERE id=$1", uid).Scan(&account)
	request := httptest.NewRequest("GET", origin+"/", nil)
	rec := httptest.NewRecorder()
	if err := s.newSession(rec, request, uid, "pwd"); err != nil {
		t.Fatal(err)
	}
	cookie := rec.Result().Cookies()[0]
	call := func(method, path string, body any, auth bool) (int, map[string]any) {
		t.Helper()
		b, _ := json.Marshal(body)
		r := httptest.NewRequest(method, origin+path, strings.NewReader(string(b)))
		r.Header.Set("Origin", origin)
		r.Header.Set("X-Duoyu-Request", "1")
		r.Header.Set("Content-Type", "application/json")
		if auth {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		s.routes().ServeHTTP(w, r)
		out := map[string]any{}
		json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}
	seed := func() {
		t.Helper()
		_, err := s.db.Exec(ctx, `INSERT INTO challenges(email,purpose,digest,expires_at) VALUES($1,'email',$2,now()+interval '5 minutes') ON CONFLICT(email,purpose) DO UPDATE SET digest=EXCLUDED.digest,attempts=0,expires_at=EXCLUDED.expires_at`, email, s.mac(email+"|email|123456"))
		if err != nil {
			t.Fatal(err)
		}
	}
	if status, _ := call("POST", "/api/me/authenticator/begin", map[string]string{"code": "123456"}, false); status != 401 {
		t.Fatal(status)
	}
	if status, _ := call("POST", "/api/me/authenticator/begin", map[string]string{"code": "123456"}, true); status != 400 {
		t.Fatal(status)
	}
	seed()
	status, out := call("POST", "/api/me/authenticator/begin", map[string]string{"code": "123456"}, true)
	if status != 200 {
		t.Fatal(status, out)
	}
	secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(out["data"].(map[string]any)["secret"].(string))
	if err != nil {
		t.Fatal(err)
	}
	code := totpCode(secret, time.Now().Unix()/30)
	var stored []byte
	if err := s.db.QueryRow(ctx, "SELECT secret FROM authenticators WHERE user_id=$1", uid).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if string(stored) == string(secret) {
		t.Fatal("plaintext secret")
	}
	s.db.Exec(ctx, "UPDATE authenticators SET expires_at=now()-interval '1 second' WHERE user_id=$1", uid)
	if _, valid, e := s.verifyAuthenticator(ctx, uid, code, digest(cookie.Value), false, "activate"); valid || e != nil {
		t.Fatal("expired setup", e)
	}
	s.db.Exec(ctx, "UPDATE authenticators SET expires_at=now()+interval '10 minutes' WHERE user_id=$1", uid)

	if status, _ = call("POST", "/api/auth/authenticator", map[string]string{"account": email, "code": code}, false); status != 401 {
		t.Fatal("unconfirmed login", status)
	}
	if _, valid, err := s.verifyAuthenticator(ctx, uid, code, "wrong-session", false, "activate"); valid || err != nil {
		t.Fatal("setup session", err)
	}
	status, out = call("POST", "/api/me/authenticator/finish", map[string]string{"code": code}, true)
	if status != 200 {
		t.Fatal(status, out)
	}
	codes := out["data"].(map[string]any)["recovery_codes"].([]any)
	if len(codes) != 8 {
		t.Fatal("recovery count")
	}
	if status, _ = call("POST", "/api/auth/authenticator", map[string]string{"account": email, "code": code}, false); status != 401 {
		t.Fatal("replayed setup code")
	}
	// Move the consumed step back only in the isolated fixture, avoiding wall-clock sleeps.
	s.db.Exec(ctx, "UPDATE authenticators SET last_step=-1 WHERE user_id=$1", uid)
	status, out = call("POST", "/api/auth/authenticator", map[string]string{"account": account, "code": totpCode(secret, time.Now().Unix()/30)}, false)
	if status != 200 {
		t.Fatal("numeric login", status, out)
	}
	status, out = call("POST", "/api/auth/authenticator", map[string]any{"account": email, "code": codes[0], "recovery": true}, false)
	if status != 200 {
		t.Fatal("recovery login", status, out)
	}
	if status, _ = call("POST", "/api/auth/authenticator", map[string]any{"account": email, "code": codes[0], "recovery": true}, false); status != 401 {
		t.Fatal("recovery replay")
	}
	s.db.Exec(ctx, "UPDATE authenticators SET failures=0 WHERE user_id=$1", uid)
	var won atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, valid, e := s.verifyAuthenticator(ctx, uid, codes[1].(string), "", true, "login")
			if e != nil {
				t.Error(e)
			}
			if valid {
				won.Add(1)
			}
		}()
	}
	wg.Wait()
	if won.Load() != 1 {
		t.Fatal("concurrent recovery", won.Load())
	}
	s.db.Exec(ctx, "UPDATE authenticators SET failures=0 WHERE user_id=$1", uid)
	for i := 0; i < 5; i++ {
		s.verifyAuthenticator(ctx, uid, "invalid", "", false, "login")
	}
	if _, valid, e := s.verifyAuthenticator(ctx, uid, codes[2].(string), "", true, "login"); valid || e != nil {
		t.Fatal("lock bypass", e)
	}
	s.db.Exec(ctx, "UPDATE authenticators SET locked_until=now()-interval '1 second' WHERE user_id=$1", uid)
	if _, valid, e := s.verifyAuthenticator(ctx, uid, codes[2].(string), "", true, "login"); !valid || e != nil {
		t.Fatal("lock expiry", e)
	}
	s.db.Exec(ctx, "UPDATE users SET enabled=false WHERE id=$1", uid)
	if status, _ = call("POST", "/api/auth/authenticator", map[string]any{"account": email, "code": codes[3], "recovery": true}, false); status != 401 {
		t.Fatal("disabled account")
	}
	s.db.Exec(ctx, "UPDATE users SET enabled=true WHERE id=$1", uid)
	seed()
	status, out = call("DELETE", "/api/me/authenticator", map[string]any{"email_code": "123456", "code": codes[3], "recovery": true}, true)
	if status != 200 {
		t.Fatal(status, out)
	}
	if status, _ = call("GET", "/api/me/authenticator", nil, true); status != 401 {
		t.Fatal("session not revoked")
	}
	if _, valid, e := s.verifyAuthenticator(ctx, uid, codes[4].(string), "", true, "login"); valid || e != nil {
		t.Fatal("removed credential", e)
	}
}
