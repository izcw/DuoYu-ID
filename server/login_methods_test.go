package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"
)

func testLoginMethods(t *testing.T, s *Server, origin string) {
	ctx := context.Background()
	_, err := s.db.Exec(ctx, "INSERT INTO apps(id,name,redirects,public) VALUES('methods-app','Methods',ARRAY['http://localhost:8199/callback'],true)")
	if err != nil {
		t.Fatal(err)
	}
	for method, amr := range map[string]string{"password": "pwd", "code": "otp", "passkey": "hwk user", "qr": "hwk user qr", "totp": "otp totp"} {
		if !s.loginAMRAllowed(ctx, "methods-app", amr) {
			t.Fatal("default method disabled")
		}
		s.db.Exec(ctx, "UPDATE apps SET login_methods=$1 WHERE id='methods-app'", []string{method})
		for other, otherAMR := range map[string]string{"password": "pwd", "code": "otp", "passkey": "hwk user", "qr": "hwk user qr", "totp": "otp totp"} {
			if s.loginAMRAllowed(ctx, "methods-app", otherAMR) != (other == method) {
				t.Fatal("AMR mismatch", method, other)
			}
		}
		s.db.Exec(ctx, "UPDATE apps SET login_methods=$1 WHERE id='methods-app'", allLoginMethods())
	}
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	q := url.Values{"client_id": {"methods-app"}, "redirect_uri": {"http://localhost:8199/callback"}, "response_type": {"code"}, "scope": {"openid"}, "code_challenge": {strings.Repeat("a", 43)}, "code_challenge_method": {"S256"}}
	resp, err := client.Get(origin + "/authorize?" + q.Encode())
	if err != nil {
		t.Fatal(err)
	}
	loc := resp.Header.Get("Location")
	resp.Body.Close()
	u, _ := url.Parse(loc)
	id := u.Query().Get("authRequestID")
	if id == "" {
		t.Fatal("no flow", loc)
	}
	if !strings.HasPrefix(loc, "http") {
		loc = origin + loc
	}
	resp, err = client.Get(loc)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	s.db.Exec(ctx, "UPDATE apps SET login_methods='{}' WHERE id='methods-app'")
	post := func(path string, data map[string]string, flow bool) (int, map[string]any) {
		t.Helper()
		b, _ := json.Marshal(data)
		r, _ := http.NewRequest("POST", origin+path, strings.NewReader(string(b)))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", origin)
		r.Header.Set("X-Duoyu-Request", "1")
		if flow {
			r.Header.Set("X-Duoyu-Auth-Request", id)
		}
		res, e := client.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		var out map[string]any
		json.NewDecoder(res.Body).Decode(&out)
		return res.StatusCode, out
	}
	for path, data := range map[string]map[string]string{
		"/api/auth/code":            {"email": "method@example.com", "purpose": "login", "auth_request_id": id},
		"/api/auth/login":           {"email": "method@example.com", "mode": "code", "code": "123456", "auth_request_id": id},
		"/api/auth/authenticator":   {"account": "method@example.com", "code": "123456"},
		"/api/auth/qr/device/begin": {}, "/api/auth/qr/device/finish": {},
		"/api/auth/passkey/begin": {}, "/api/auth/passkey/finish": {}, "/api/auth/qr": {},
	} {
		if status, _ := post(path, data, true); status != 403 {
			t.Fatal(path, status)
		}
	}
	if status, _ := post("/api/auth/login", map[string]string{"email": "admin@example.com", "mode": "password", "password": "test-admin-password-123"}, false); status != 200 {
		t.Fatal("center login affected", status)
	}
	if status, _ := post("/api/auth/continue", map[string]string{"id": id}, false); status != 403 {
		t.Fatal("SSO bypass", status)
	}
	resp, err = client.Get(origin + "/api/auth/context?id=" + id)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	json.NewDecoder(resp.Body).Decode(&out)
	resp.Body.Close()
	if out["data"].(map[string]any)["reauthenticate"] != true {
		t.Fatal("missing fresh authentication")
	}
	s.db.Exec(ctx, "UPDATE apps SET login_methods=ARRAY['password'] WHERE id='methods-app'")
	if status, _ := post("/api/auth/continue", map[string]string{"id": id}, false); status != 200 {
		t.Fatal("enabled SSO rejected", status)
	}
}
