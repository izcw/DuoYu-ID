package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPassword(t *testing.T) {
	h := hashPassword("a strong password")
	if !checkPassword("a strong password", h) || checkPassword("wrong", h) || checkPassword("test", "a2$bad$bad") {
		t.Fatal("password verification failed")
	}
}
func TestRedirectValidation(t *testing.T) {
	s := &Server{}
	for _, v := range []string{"https://app.example/callback", "https://app.example/callback?mode=1"} {
		if !s.validRedirect(v) {
			t.Fatal(v)
		}
	}
	for _, v := range []string{"javascript:alert(1)", "https://user:pass@app.example/", "https://app.example/*", "https://app.example/callback#x", "http://localhost/callback"} {
		if s.validRedirect(v) {
			t.Fatal(v)
		}
	}
}
func TestIntegration(t *testing.T) {
	dsn := os.Getenv("DUOYU_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set DUOYU_TEST_DATABASE_URL for isolated PostgreSQL integration tests")
	}
	ctx := context.Background()
	db, e := pgxpool.New(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	schema := "test_" + strings.ToLower(randomID()[:12])
	schema = strings.ReplaceAll(strings.ReplaceAll(schema, "-", "a"), "_", "b")
	if _, e = db.Exec(ctx, "CREATE SCHEMA "+schema); e != nil {
		t.Fatal(e)
	}
	defer db.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	d, e := url.Parse(dsn)
	if e != nil {
		t.Fatal(e)
	}
	q := d.Query()
	q.Set("search_path", schema)
	d.RawQuery = q.Encode()
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	origin := "http://localhost:" + fmt.Sprint(listener.Addr().(*net.TCPAddr).Port)
	t.Setenv("DATABASE_URL", d.String())
	t.Setenv("DUOYU_DEV", "true")
	t.Setenv("DUOYU_ORIGIN", origin)
	t.Setenv("DUOYU_DATA_DIR", t.TempDir())
	t.Setenv("DUOYU_SECRET", strings.Repeat("a1", 32))
	t.Setenv("DUOYU_ADMIN_EMAIL", "admin@example.com")
	t.Setenv("DUOYU_ADMIN_PASSWORD", "test-admin-password-123")
	s, e := newServer()
	if e != nil {
		t.Fatal(e)
	}
	defer s.db.Close()
	srv := httptest.NewUnstartedServer(s.routes())
	srv.Listener = listener
	srv.Start()
	defer srv.Close()
	t.Run("Project blacklist", func(t *testing.T) { testProjectBlacklist(t, s, origin) })
	t.Run("Account password policy and numeric IDs", func(t *testing.T) { testAccountPolicy(t, s) })
	t.Run("QR login ownership and single use", func(t *testing.T) { testQRLogin(t, s, origin) })
	t.Run("Project mail isolation and idempotency", func(t *testing.T) { testProjectMail(t, s, origin) })
	t.Run("Discovery, origins and protocol faults", func(t *testing.T) { testProtocolContract(t, s, origin) })
	t.Run("Authenticator enrollment and login", func(t *testing.T) { testAuthenticator(t, s, origin) })
	t.Run("Account projects and avatars", func(t *testing.T) { testAccountProjects(t, s, origin) })
	t.Run("Management API", func(t *testing.T) { testManagementAPI(t, s, origin) })
	t.Run("Project roles", func(t *testing.T) { testProjectRoles(t, s, origin) })
	t.Run("Project login methods", func(t *testing.T) { testLoginMethods(t, s, origin) })
	t.Run("Queue recovery and migrations", func(t *testing.T) { testMailQueue(t, s) })
	t.Run("Multi-client protocol matrix", func(t *testing.T) { testClientMatrix(t, s, origin) })
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	request := func(method, path string, data any) (int, map[string]any) {
		t.Helper()
		var b io.Reader
		if data != nil {
			x, _ := json.Marshal(data)
			b = bytes.NewReader(x)
		}
		r, _ := http.NewRequest(method, origin+path, b)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", origin)
		r.Header.Set("X-Duoyu-Request", "1")
		res, e := client.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		var out map[string]any
		if e = json.NewDecoder(res.Body).Decode(&out); e != nil {
			t.Fatalf("%s: %v", path, e)
		}
		return res.StatusCode, out
	}
	code := func(email, purpose string) {
		_, e := s.db.Exec(ctx, `INSERT INTO challenges(email,purpose,digest,expires_at) VALUES($1,$2,$3,$4) ON CONFLICT(email,purpose) DO UPDATE SET digest=EXCLUDED.digest,attempts=0,expires_at=EXCLUDED.expires_at`, email, purpose, s.mac(email+"|"+purpose+"|123456"), time.Now().Add(time.Minute))
		if e != nil {
			t.Fatal(e)
		}
	}
	assertStatus := func(want, got int, out any) {
		t.Helper()
		if got != want {
			t.Fatalf("status %d want %d: %#v", got, want, out)
		}
	}
	status, out := request("GET", "/api/admin/users", nil)
	assertStatus(401, status, out)
	status, out = request("GET", "/api/auth/session", nil)
	assertStatus(200, status, out)
	if out["data"].(map[string]any)["authenticated"] != false {
		t.Fatal("anonymous session must not be authenticated")
	}
	status, out = request("GET", "/api/me", nil)
	assertStatus(401, status, out)
	req, _ := http.NewRequest("POST", origin+"/api/auth/login", strings.NewReader(`{}`))
	res, e := client.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	assertStatus(403, res.StatusCode, nil)
	res.Body.Close()
	status, out = request("POST", "/api/auth/login", map[string]any{"email": "admin@example.com", "mode": "password", "password": "incorrect-password"})
	assertStatus(401, status, out)
	status, out = request("POST", "/api/auth/login", map[string]any{"email": "admin@example.com", "mode": "password", "password": "test-admin-password-123"})
	assertStatus(200, status, out)
	status, out = request("GET", "/api/auth/session", nil)
	assertStatus(200, status, out)
	if out["data"].(map[string]any)["authenticated"] != true {
		t.Fatal("valid session must be authenticated")
	}
	status, out = request("GET", "/api/admin/overview", nil)
	assertStatus(200, status, out)

	for _, logo := range []string{"", "/project-logos/" + strings.Repeat("a", 43) + ".png", "https://images.example.com/logo.png", "javascript:alert(1)", "data:image/png;base64,abc", "https://user:password@example.com/logo.png", "http://example.com/logo.png"} {
		status, out = request("POST", "/api/admin/apps", map[string]any{"name": "Invalid logo", "logo": logo, "public": true, "enabled": true, "redirects": []string{"http://localhost:8091/callback"}})
		assertStatus(400, status, out)
	}
	logo := testUploadProjectLogo(t, client, origin)
	var apps []string
	for _, name := range []string{"笔记项目", "记账项目"} {
		status, out = request("POST", "/api/admin/apps", map[string]any{"name": name, "logo": logo, "public": true, "enabled": true, "redirects": []string{"http://localhost:8091/callback"}, "logout_redirects": []string{}})
		assertStatus(200, status, out)
		apps = append(apps, out["data"].(map[string]any)["id"].(string))
	}

	status, out = request("GET", "/api/admin/apps/"+apps[0], nil)
	assertStatus(200, status, out)
	if out["data"].([]any)[0].(map[string]any)["logo"] != logo {
		t.Fatal("project logo not saved")
	}
	status, out = request("POST", "/api/admin/keys", map[string]any{"name": "test read key", "app_id": apps[0], "days": 1})
	assertStatus(200, status, out)
	keyData := out["data"].(map[string]any)
	key := keyData["secret"].(string)
	keyID := keyData["id"].(string)
	service := func() int {
		r, _ := http.NewRequest("GET", origin+"/api/service/app", nil)
		r.Header.Set("Authorization", "Bearer "+key)
		res, e := client.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		return res.StatusCode
	}
	assertStatus(200, service(), nil)
	status, out = request("PUT", "/api/admin/keys/"+keyID, map[string]any{"name": "permanent key", "permanent": true, "change_expiry": true})
	assertStatus(200, status, out)
	var perpetual bool
	s.db.QueryRow(ctx, "SELECT expires_at IS NULL FROM api_keys WHERE id=$1", keyID).Scan(&perpetual)
	if !perpetual {
		t.Fatal("permanent key still expires")
	}
	assertStatus(200, service(), nil)
	status, out = request("GET", "/api/admin/analytics?days=7", nil)
	assertStatus(200, status, out)
	if len(out["data"].(map[string]any)["series"].([]any)) != 7 {
		t.Fatal("analytics missing dates")
	}
	status, out = request("POST", "/api/admin/keys", map[string]any{"name": "new permanent", "app_id": apps[0], "permanent": true})
	assertStatus(200, status, out)
	status, out = request("DELETE", "/api/admin/keys/"+keyID, nil)
	assertStatus(200, status, out)
	assertStatus(401, service(), nil)
	status, out = request("PUT", "/api/admin/keys/"+keyID, map[string]any{"name": "revoked edit", "permanent": true, "change_expiry": true})
	assertStatus(400, status, out)
	status, out = request("POST", "/api/auth/logout", map[string]any{})
	assertStatus(200, status, out)
	code("user@example.com", "login")
	status, out = request("POST", "/api/auth/login", map[string]any{"email": "user@example.com", "mode": "code", "code": "123456"})
	assertStatus(200, status, out)
	status, out = request("GET", "/api/me", nil)
	assertStatus(200, status, out)
	uid := out["data"].(map[string]any)["id"].(string)
	status, out = request("GET", "/api/admin/users", nil)
	assertStatus(403, status, out)
	// A code can only be consumed once, including simultaneous requests.
	code("race@example.com", "login")
	var count atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if s.consumeCode(ctx, "race@example.com", "login", "123456") {
				count.Add(1)
			}
		}()
	}
	wg.Wait()
	if count.Load() != 1 {
		t.Fatalf("code consumed %d times", count.Load())
	}
	code("attempt@example.com", "login")
	for range 5 {
		s.consumeCode(ctx, "attempt@example.com", "login", "000000")
	}
	if s.consumeCode(ctx, "attempt@example.com", "login", "123456") {
		t.Fatal("attempt limit bypass")
	}
	verifier := randomID()
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	authorize := func(app string, modify func(url.Values)) string {
		t.Helper()
		v := url.Values{"client_id": {app}, "redirect_uri": {"http://localhost:8091/callback"}, "response_type": {"code"}, "scope": {"openid profile email"}, "state": {"test-state"}, "nonce": {"test-nonce"}, "code_challenge": {challenge}, "code_challenge_method": {"S256"}}
		if modify != nil {
			modify(v)
		}
		res, e := client.Get(origin + "/authorize?" + v.Encode())
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		return res.Header.Get("Location")
	}
	bad := authorize(apps[0], func(v url.Values) { v.Set("redirect_uri", "http://evil.example/cb") })
	if strings.HasPrefix(bad, "http://evil.example") {
		t.Fatal("open redirect")
	}
	bad = authorize(apps[0], func(v url.Values) { v.Del("code_challenge") })
	if strings.HasPrefix(bad, "/login") {
		t.Fatal("missing PKCE accepted")
	}
	exchange := func(app, code, v string) (int, map[string]any) {
		form := url.Values{"grant_type": {"authorization_code"}, "client_id": {app}, "code": {code}, "redirect_uri": {"http://localhost:8091/callback"}, "code_verifier": {v}}
		res, e := client.PostForm(origin+"/oauth/token", form)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		var o map[string]any
		json.NewDecoder(res.Body).Decode(&o)
		return res.StatusCode, o
	}
	var access string
	for _, app := range apps {
		location := authorize(app, nil)
		if !strings.HasPrefix(location, "/login?") {
			t.Fatalf("unexpected authorization redirect %q", location)
		}
		res, e = client.Get(origin + location)
		if e != nil {
			t.Fatal(e)
		}
		assertStatus(200, res.StatusCode, nil)
		res.Body.Close()
		u, _ := url.Parse(location)
		id := u.Query().Get("authRequestID")
		status, out = request("POST", "/api/auth/continue", map[string]string{"id": id})
		assertStatus(200, status, out)
		callback := out["data"].(map[string]any)["redirect"].(string)
		res, e = client.Get(callback)
		if e != nil {
			t.Fatal(e)
		}
		loc := res.Header.Get("Location")
		res.Body.Close()
		u, _ = url.Parse(loc)
		authCode := u.Query().Get("code")
		if authCode == "" {
			t.Fatalf("callback missing code: %s", loc)
		}
		if u.Query().Get("state") != "test-state" {
			t.Fatal("state lost")
		}
		status, tokens := exchange(app, authCode, verifier)
		assertStatus(200, status, tokens)
		access = tokens["access_token"].(string)
		signed, e := jose.ParseSigned(tokens["id_token"].(string), []jose.SignatureAlgorithm{jose.RS256})
		if e != nil {
			t.Fatal(e)
		}
		payload, e := signed.Verify(&s.signing.PublicKey)
		if e != nil {
			t.Fatal(e)
		}
		var claims map[string]any
		json.Unmarshal(payload, &claims)
		if claims["sub"] != uid || claims["iss"] != origin || claims["nonce"] != "test-nonce" {
			t.Fatalf("unexpected claims %#v", claims)
		}
		status, reused := exchange(app, authCode, verifier)
		if status == 200 {
			t.Fatalf("authorization code reused: %#v", reused)
		}
		r, _ := http.NewRequest("GET", origin+"/userinfo", nil)
		r.Header.Set("Authorization", "Bearer "+access)
		res, e = client.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		var info map[string]any
		json.NewDecoder(res.Body).Decode(&info)
		res.Body.Close()
		if info["sub"] != uid || info["email"] != "user@example.com" {
			t.Fatalf("userinfo: %#v", info)
		}
	}
	var appCount int
	if e = s.db.QueryRow(ctx, "SELECT count(*) FROM app_users WHERE user_id=$1", uid).Scan(&appCount); e != nil || appCount != 2 {
		t.Fatalf("project association count: %d, %v", appCount, e)
	}
	// Registration switch and email verification remain enforced without a checkbox.
	request("POST", "/api/auth/logout", map[string]any{})
	s.db.Exec(ctx, "UPDATE settings SET value='false' WHERE key='registration'")
	code("closed@example.com", "login")
	status, out = request("POST", "/api/auth/login", map[string]any{"email": "closed@example.com", "mode": "code", "code": "123456"})
	assertStatus(403, status, out)
	s.db.Exec(ctx, "UPDATE settings SET value='true' WHERE key='registration'")
	status, out = request("POST", "/api/auth/login", map[string]any{"email": "unverified@example.com", "mode": "code", "code": "000000"})
	assertStatus(401, status, out)
	var unwanted int
	s.db.QueryRow(ctx, "SELECT count(*) FROM users WHERE email IN ('closed@example.com','unverified@example.com')").Scan(&unwanted)
	if unwanted != 0 {
		t.Fatal("unverified or closed registration created an account")
	}

	code("random-password@example.com", "login")
	status, out = request("POST", "/api/auth/login", map[string]any{"email": "random-password@example.com", "mode": "code", "code": "123456"})
	assertStatus(200, status, out)
	var initialHash, nextHash string
	if err := s.db.QueryRow(ctx, "SELECT password_hash FROM users WHERE email='random-password@example.com'").Scan(&initialHash); err != nil || !strings.HasPrefix(initialHash, "a2$") {
		t.Fatal("registration password hash missing")
	}
	code("random-password@example.com", "login")
	status, out = request("POST", "/api/auth/login", map[string]any{"email": "random-password@example.com", "mode": "code", "code": "123456"})
	assertStatus(200, status, out)
	if err := s.db.QueryRow(ctx, "SELECT password_hash FROM users WHERE email='random-password@example.com'").Scan(&nextHash); err != nil || initialHash != nextHash {
		t.Fatal("OTP login replaced existing password")
	}
	// Disabling a user invalidates UserInfo even if its opaque token has time left.
	s.db.Exec(ctx, "UPDATE users SET enabled=false WHERE id=$1", uid)
	r, _ := http.NewRequest("GET", origin+"/userinfo", nil)
	r.Header.Set("Authorization", "Bearer "+access)
	res, e = client.Do(r)
	if e != nil {
		t.Fatal(e)
	}
	if res.StatusCode == 200 {
		t.Fatal("disabled user accepted")
	}
	res.Body.Close()
	t.Log("Verified two-app SSO, signed ID tokens, UserInfo, code single-use, OTP concurrency/lockout, CSRF, API key revoke, admin authorization, disabled account")
}
