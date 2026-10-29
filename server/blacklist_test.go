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

func testProjectBlacklist(t *testing.T, s *Server, origin string) {
	ctx := context.Background()
	_, err := s.db.Exec(ctx, `INSERT INTO apps(id,name,redirects,public) VALUES('blacklist-app','Blacklist',ARRAY['http://localhost:8199/callback'],true),('blacklist-other','Other',ARRAY['http://localhost:8199/callback'],true); INSERT INTO users(id,email,name) VALUES('blacklist-user','blacklist-user@example.com','Blocked')`)
	if err != nil {
		t.Fatal(err)
	}
	jar, _ := cookiejar.New(nil)
	admin := &http.Client{Jar: jar}
	call := func(client *http.Client, path string, data any) (int, map[string]any) {
		t.Helper()
		b, _ := json.Marshal(data)
		method := "POST"
		if strings.HasSuffix(path, "/blacklist") {
			method = "PUT"
		}
		req, _ := http.NewRequest(method, origin+path, strings.NewReader(string(b)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", origin)
		req.Header.Set("X-Duoyu-Request", "1")
		req.Header.Set("X-Duoyu-Legal-Version", legalVersion)
		req.Header.Set("X-Duoyu-Adult", "true")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var out map[string]any
		json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out
	}
	if status, _ := call(admin, "/api/auth/login", map[string]string{"email": "admin@example.com", "mode": "password", "password": "test-admin-password-123"}); status != 200 {
		t.Fatal("admin login", status)
	}
	var account string
	s.db.QueryRow(ctx, "SELECT account_id FROM users WHERE id='blacklist-user'").Scan(&account)
	if !s.allowed(ctx, "blacklist-user", "blacklist-app") {
		t.Fatal("empty blacklist denied access")
	}
	if status, _ := call(admin, "/api/admin/apps/blacklist-app/blacklist", map[string]any{"entries": []string{account, "new-blocked@example.com"}}); status != 200 {
		t.Fatal("save blacklist", status)
	}
	if s.allowed(ctx, "blacklist-user", "blacklist-app") || !s.allowed(ctx, "blacklist-user", "blacklist-other") {
		t.Fatal("blacklist project isolation failed")
	}
	s.db.Exec(ctx, "INSERT INTO grants(app_id,user_id) VALUES('blacklist-app','blacklist-user')")
	if s.allowed(ctx, "blacklist-user", "blacklist-app") {
		t.Fatal("allowlist bypassed blacklist")
	}
	s.db.Exec(ctx, "UPDATE users SET email='changed-blocked@example.com' WHERE id='blacklist-user'")
	if s.allowed(ctx, "blacklist-user", "blacklist-app") {
		t.Fatal("email change bypassed account blacklist")
	}
	visitorJar, _ := cookiejar.New(nil)
	visitor := &http.Client{Jar: visitorJar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	query := url.Values{"client_id": {"blacklist-app"}, "redirect_uri": {"http://localhost:8199/callback"}, "response_type": {"code"}, "scope": {"openid"}, "code_challenge": {strings.Repeat("a", 43)}, "code_challenge_method": {"S256"}}
	response, err := visitor.Get(origin + "/authorize?" + query.Encode())
	if err != nil {
		t.Fatal(err)
	}
	location := response.Header.Get("Location")
	response.Body.Close()
	parsed, _ := url.Parse(location)
	id := parsed.Query().Get("authRequestID")
	if id == "" {
		t.Fatal("missing auth request", location)
	}
	response, err = visitor.Get(origin + location)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	for _, path := range []string{"/api/auth/code", "/api/auth/login"} {
		data := map[string]string{"email": "new-blocked@example.com", "auth_request_id": id}
		if path == "/api/auth/code" {
			data["purpose"] = "login"
		} else {
			data["mode"] = "code"
			data["code"] = "123456"
		}
		if status, _ := call(visitor, path, data); status != 403 {
			t.Fatal("blocked project registration accepted", path, status)
		}
	}
	var count int
	s.db.QueryRow(ctx, "SELECT count(*) FROM users WHERE email='new-blocked@example.com'").Scan(&count)
	if count != 0 {
		t.Fatal("blocked email registered")
	}
	// Registration elsewhere binds the email entry to an immutable account identity.
	if _, err = s.db.Exec(ctx, "INSERT INTO users(id,email,name) VALUES('blacklist-new','new-blocked@example.com','New')"); err != nil {
		t.Fatal(err)
	}
	s.db.Exec(ctx, "UPDATE users SET email='new-address@example.com' WHERE id='blacklist-new'")
	if s.allowed(ctx, "blacklist-new", "blacklist-app") || !s.allowed(ctx, "blacklist-new", "blacklist-other") {
		t.Fatal("email blacklist identity binding failed")
	}
	// Invalid edits are atomic and cannot accidentally clear the previous policy.
	if status, _ := call(admin, "/api/admin/apps/blacklist-app/blacklist", map[string]any{"entries": []string{"not-an-email"}}); status != 400 {
		t.Fatal("invalid entry accepted")
	}
	if s.allowed(ctx, "blacklist-user", "blacklist-app") {
		t.Fatal("invalid edit cleared blacklist")
	}
	if status, _ := call(admin, "/api/admin/apps/blacklist-app/blacklist", map[string]any{"entries": []string{}}); status != 200 {
		t.Fatal("clear failed")
	}
	if !s.allowed(ctx, "blacklist-user", "blacklist-app") {
		t.Fatal("clearing failed to restore new login access")
	}
}
