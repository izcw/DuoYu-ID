package main

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

func testProtocolContract(t *testing.T, s *Server, origin string) {
	ctx := context.Background()
	resp, err := http.Get(origin + "/.well-known/openid-configuration")
	if err != nil {
		t.Fatal(err)
	}
	var d map[string]any
	json.NewDecoder(resp.Body).Decode(&d)
	resp.Body.Close()
	for key, want := range map[string][]any{"response_types_supported": {"code"}, "grant_types_supported": {"authorization_code", "refresh_token"}, "scopes_supported": {"openid", "profile", "email", "offline_access"}, "code_challenge_methods_supported": {"S256"}} {
		if !reflect.DeepEqual(d[key], want) {
			t.Fatalf("%s: %v", key, d[key])
		}
	}
	for _, key := range []string{"device_authorization_endpoint", "check_session_iframe"} {
		if v, ok := d[key]; ok && v != "" {
			t.Fatalf("unsupported %s: %v", key, v)
		}
	}
	for _, raw := range []string{"https://app.example.com/", "https://*.example.com", "null", "https://a.test?", "https://a.test/#x", "chrome-extension://bad"} {
		if s.validOrigin(raw) {
			t.Fatalf("accepted origin %q", raw)
		}
	}
	chrome := "chrome-extension://" + strings.Repeat("a", 32)
	firefox := "moz-extension://12345678-abcd-1234-abcd-123456789012"
	_, err = s.db.Exec(ctx, `INSERT INTO apps(id,name,redirects,public,allowed_origins,platform) VALUES('protocol-app','Protocol',ARRAY['https://callback.example.com/cb'],true,$1,'extension')`, []string{chrome, firefox, "https://spa.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.db.Exec(ctx, `INSERT INTO users(id,email,name) VALUES('protocol-user','protocol@example.com','Protocol'); INSERT INTO access_tokens(id,user_id,app_id,scopes,expires_at) VALUES('protocol-token','protocol-user','protocol-app',ARRAY['openid','profile'],now()+interval '5 minutes')`)
	if err != nil {
		t.Fatal(err)
	}
	token, err := s.provider.Crypto().Encrypt("protocol-token:protocol-user")
	if err != nil {
		t.Fatal(err)
	}
	check := func(path, method, body, originHeader, bearer string, want int, code string) {
		t.Helper()
		r, _ := http.NewRequest(method, origin+path, strings.NewReader(body))
		if method == "POST" {
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		if originHeader != "" {
			r.Header.Set("Origin", originHeader)
		}
		if bearer != "" {
			r.Header.Set("Authorization", "Bearer "+bearer)
		}
		if method == "OPTIONS" {
			r.Header.Set("Access-Control-Request-Method", "POST")
		}
		resp, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var out map[string]any
		json.NewDecoder(resp.Body).Decode(&out)
		if resp.StatusCode != want || code != "" && out["error"] != code {
			t.Fatalf("%s %s status=%d body=%v", method, path, resp.StatusCode, out)
		}
		if code == "invalid_token" && resp.Header.Get("WWW-Authenticate") != `Bearer error="invalid_token"` {
			t.Fatal("missing Bearer challenge")
		}
		if originHeader != "" && want < 300 && resp.Header.Get("Access-Control-Allow-Origin") != originHeader {
			t.Fatal("incorrect CORS origin")
		}
	}
	for _, o := range []string{chrome, firefox, "https://spa.example.com"} {
		check("/userinfo", "GET", "", o, token, 200, "")
		check("/oauth/token", "OPTIONS", "", o, "", 204, "")
	}
	check("/userinfo", "GET", "", "https://callback.example.com", token, 403, "origin_not_allowed")
	s.db.Exec(ctx, "UPDATE access_tokens SET scopes=ARRAY['profile'] WHERE id='protocol-token'")
	check("/userinfo", "GET", "", "", token, 403, "insufficient_scope")
	s.db.Exec(ctx, "UPDATE access_tokens SET scopes=ARRAY['openid','profile'] WHERE id='protocol-token'")
	check("/userinfo", "GET", "", "", "bad-token", 401, "invalid_token")
	check("/userinfo", "GET", "", "", "", 401, "invalid_token")
	for _, g := range []string{"implicit", "client_credentials", "urn:ietf:params:oauth:grant-type:jwt-bearer", "urn:ietf:params:oauth:grant-type:device_code"} {
		check("/oauth/token", "POST", url.Values{"grant_type": {g}, "client_id": {"protocol-app"}}.Encode(), "", "", 400, "unsupported_grant_type")
	}
	// Fault injection changes only the isolated test server pool, never the running database.
	original := s.db
	cfg := original.Config()
	cfg.ConnConfig.RuntimeParams["search_path"] = "duoyu_missing_schema"
	faulty, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	func() {
		s.db = faulty
		defer func() { s.db = original; faulty.Close() }()
		check("/userinfo", "GET", "", "", token, 503, "temporarily_unavailable")
		check("/oauth/token", "POST", "grant_type=refresh_token&client_id=protocol-app&refresh_token=missing", "", "", 503, "temporarily_unavailable")
		check("/oauth/token", "POST", "grant_type=authorization_code&client_id=protocol-app&code=missing", "", "", 503, "temporarily_unavailable")
	}()
	check("/userinfo", "GET", "", "", token, 200, "")
	s.db.Exec(ctx, "DELETE FROM access_tokens WHERE id='protocol-token'")
	check("/userinfo", "GET", "", "", token, 401, "invalid_token")
}
