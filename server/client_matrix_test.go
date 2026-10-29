package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	verifierlib "github.com/coreos/go-oidc/v3/oidc"
	"github.com/jackc/pgx/v5/pgxpool"
)

// This matrix runs only against TestIntegration's disposable PostgreSQL schema.
func testClientMatrix(t *testing.T, s *Server, origin string) {
	ctx := context.Background()
	uid := "matrix-user"
	password := "matrix-test-password-123"
	_, err := s.db.Exec(ctx, "INSERT INTO users(id,email,name,password_hash) VALUES($1,'matrix@example.invalid','Matrix',$2)", uid, hashPassword(password))
	if err != nil {
		t.Fatal(err)
	}
	pub, conf, other := "matrix-public", "matrix-confidential", "matrix-other"
	secret := "matrix-client-secret"
	redirect := "http://localhost:8199/callback"
	for _, id := range []string{pub, conf, other} {
		_, err = s.db.Exec(ctx, "INSERT INTO apps(id,name,secret_hash,redirects,logout_redirects,public,allowed_origins) VALUES($1,$1,$2,$3,$4,$5,ARRAY['http://localhost:8199'])", id, s.mac(secret), []string{redirect}, []string{"http://localhost:8199/"}, id != conf)
		if err != nil {
			t.Fatal(err)
		}
	}
	jar, _ := cookiejar.New(nil)
	browser := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	request := func(client *http.Client, method, path string, data any) (int, map[string]any) {
		t.Helper()
		b, _ := json.Marshal(data)
		r, _ := http.NewRequest(method, origin+path, strings.NewReader(string(b)))
		r.Header.Set("Origin", origin)
		r.Header.Set("X-Duoyu-Request", "1")
		r.Header.Set("Content-Type", "application/json")
		res, e := client.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		var out map[string]any
		json.NewDecoder(res.Body).Decode(&out)
		return res.StatusCode, out
	}
	login := func(client *http.Client) {
		status, _ := request(client, "POST", "/api/auth/login", map[string]string{"email": "matrix@example.invalid", "mode": "password", "password": password})
		if status != 200 {
			t.Fatal("fixture login", status)
		}
	}
	login(browser)
	res, e := http.Get(origin + "/.well-known/openid-configuration")
	if e != nil {
		t.Fatal(e)
	}
	var discovery map[string]any
	json.NewDecoder(res.Body).Decode(&discovery)
	res.Body.Close()
	provider, e := verifierlib.NewProvider(ctx, origin)
	if e != nil {
		t.Fatal(e)
	}
	type flow struct{ id, client, verifier, nonce, state string }
	begin := func(app, scope string, change func(url.Values)) (flow, string) {
		t.Helper()
		f := flow{client: app, verifier: randomID(), nonce: randomID(), state: randomID()}
		h := sha256.Sum256([]byte(f.verifier))
		q := url.Values{"client_id": {app}, "redirect_uri": {redirect}, "response_type": {"code"}, "scope": {scope}, "nonce": {f.nonce}, "state": {f.state}, "code_challenge": {base64.RawURLEncoding.EncodeToString(h[:])}, "code_challenge_method": {"S256"}}
		if change != nil {
			change(q)
		}
		r, e := browser.Get(origin + "/authorize?" + q.Encode())
		if e != nil {
			t.Fatal(e)
		}
		loc := r.Header.Get("Location")
		r.Body.Close()
		u, _ := url.Parse(loc)
		f.id = u.Query().Get("authRequestID")
		if f.id != "" {
			r, e = browser.Get(origin + loc)
			if e != nil {
				t.Fatal(e)
			}
			r.Body.Close()
		}
		return f, loc
	}
	complete := func(f flow) (int, string) {
		t.Helper()
		status, out := request(browser, "POST", "/api/auth/continue", map[string]string{"id": f.id})
		if status != 200 {
			return status, ""
		}
		callback := out["data"].(map[string]any)["redirect"].(string)
		r, e := browser.Get(callback)
		if e != nil {
			t.Fatal(e)
		}
		loc := r.Header.Get("Location")
		r.Body.Close()
		u, _ := url.Parse(loc)
		if u.Query().Get("state") != f.state {
			t.Fatal("state mismatch")
		}
		return status, u.Query().Get("code")
	}
	code := func(app, scope string) (flow, string) {
		f, _ := begin(app, scope, nil)
		status, c := complete(f)
		if status != 200 || c == "" {
			t.Fatal("authorization failed", status)
		}
		return f, c
	}
	exchange := func(f flow, c, clientSecret string, change func(url.Values)) (int, map[string]any) {
		t.Helper()
		q := url.Values{"grant_type": {"authorization_code"}, "client_id": {f.client}, "code": {c}, "redirect_uri": {redirect}, "code_verifier": {f.verifier}}
		if change != nil {
			change(q)
		}
		r, _ := http.NewRequest("POST", discovery["token_endpoint"].(string), strings.NewReader(q.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if clientSecret != "" {
			r.SetBasicAuth(f.client, clientSecret)
		}
		resp, e := http.DefaultClient.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		defer resp.Body.Close()
		var out map[string]any
		json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out
	}
	info := func(token string) (int, map[string]any) {
		r, _ := http.NewRequest("GET", discovery["userinfo_endpoint"].(string), nil)
		r.Header.Set("Authorization", "Bearer "+token)
		resp, e := http.DefaultClient.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		defer resp.Body.Close()
		var out map[string]any
		json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out
	}

	t.Run("project blacklist permanently revokes issued tokens", func(t *testing.T) {
		s.db.Exec(ctx, "UPDATE apps SET refresh_enabled=true WHERE id=$1", pub)
		defer s.db.Exec(ctx, "UPDATE apps SET refresh_enabled=false WHERE id=$1", pub)
		f, c := code(pub, "openid offline_access")
		status, out := exchange(f, c, "", nil)
		if status != 200 {
			t.Fatal("token fixture", status)
		}
		access := out["access_token"].(string)
		refresh := out["refresh_token"].(string)
		pending, pendingCode := code(pub, "openid")
		if _, err := s.db.Exec(ctx, "INSERT INTO app_blacklist(app_id,identifier,user_id) VALUES($1,'matrix-block',$2)", pub, uid); err != nil {
			t.Fatal(err)
		}
		if status, _ := info(access); status == 200 {
			t.Fatal("blacklisted access token still valid")
		}
		if status, _ := exchange(pending, pendingCode, "", nil); status == 200 {
			t.Fatal("blacklisted pending code accepted")
		}
		if !s.allowed(ctx, uid, other) {
			t.Fatal("blacklist affected other project")
		}
		s.db.Exec(ctx, "DELETE FROM app_blacklist WHERE app_id=$1", pub)
		if status, _ := info(access); status == 200 {
			t.Fatal("removed blacklist revived old access token")
		}
		res, err := http.PostForm(discovery["token_endpoint"].(string), url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}, "client_id": {pub}})
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		if res.StatusCode == 200 {
			t.Fatal("removed blacklist revived refresh token")
		}
	})

	t.Run("disabled method rejects access refresh and pending code", func(t *testing.T) {
		s.db.Exec(ctx, "UPDATE apps SET refresh_enabled=true WHERE id=$1", pub)
		defer s.db.Exec(ctx, "UPDATE apps SET refresh_enabled=false,login_methods=$2 WHERE id=$1", pub, allLoginMethods())
		f, c := code(pub, "openid offline_access")
		status, out := exchange(f, c, "", nil)
		if status != 200 {
			t.Fatal(status)
		}
		pending, pendingCode := code(pub, "openid")
		s.db.Exec(ctx, "UPDATE apps SET login_methods='{}' WHERE id=$1", pub)
		if status, _ := info(out["access_token"].(string)); status != 401 {
			t.Fatal("disabled access", status)
		}
		if status, _ := exchange(pending, pendingCode, "", nil); status == 200 {
			t.Fatal("disabled code exchanged")
		}
		resp, err := http.PostForm(discovery["token_endpoint"].(string), url.Values{"grant_type": {"refresh_token"}, "client_id": {pub}, "refresh_token": {out["refresh_token"].(string)}})
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			t.Fatal("disabled refresh succeeded")
		}
		if !s.loginAMRAllowed(ctx, other, "pwd") {
			t.Fatal("other project affected")
		}
	})
	t.Run("confidential and public signed-token SSO", func(t *testing.T) {
		for _, app := range []string{conf, pub} {
			f, c := code(app, "openid profile email")
			sec := ""
			if app == conf {
				sec = secret
			}
			status, out := exchange(f, c, sec, nil)
			if status != 200 {
				t.Fatal("token exchange", app, status)
			}
			id, e := provider.Verifier(&verifierlib.Config{ClientID: app}).Verify(ctx, out["id_token"].(string))
			if e != nil || id.Subject != uid || id.Nonce != f.nonce {
				t.Fatal("ID token validation", e)
			}
			if _, e = provider.Verifier(&verifierlib.Config{ClientID: other}).Verify(ctx, out["id_token"].(string)); e == nil {
				t.Fatal("audience mismatch accepted")
			}
			status, user := info(out["access_token"].(string))
			if status != 200 || user["sub"] != uid {
				t.Fatal("userinfo", status)
			}
			if status, _ = exchange(f, c, sec, nil); status == 200 {
				t.Fatal("code replay")
			}
		}
	})
	for _, tc := range []struct {
		name, app, sec string
		change         func(url.Values)
	}{
		{"confidential missing secret", conf, "", nil}, {"confidential wrong secret", conf, "wrong", nil},
		{"wrong PKCE", pub, "", func(q url.Values) { q.Set("code_verifier", randomID()) }},
		{"redirect mismatch", pub, "", func(q url.Values) { q.Set("redirect_uri", "http://localhost:8199/other") }},
		{"code bound to client", pub, "", func(q url.Values) { q.Set("client_id", other) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, c := code(tc.app, "openid")
			status, _ := exchange(f, c, tc.sec, tc.change)
			if status == 200 {
				t.Fatal("invalid exchange accepted")
			}
		})
	}
	for _, tc := range []struct {
		name   string
		change func(url.Values)
	}{
		{"unregistered redirect", func(q url.Values) { q.Set("redirect_uri", "https://invalid.example/callback") }},
		{"missing PKCE", func(q url.Values) { q.Del("code_challenge") }},
		{"plain PKCE", func(q url.Values) { q.Set("code_challenge_method", "plain") }},
		{"implicit flow rejected", func(q url.Values) { q.Set("response_type", "token") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, loc := begin(pub, "openid", tc.change)
			if f.id != "" || strings.HasPrefix(loc, "https://invalid.example") {
				t.Fatal("invalid request accepted")
			}
		})
	}
	t.Run("unknown scope filtered without privilege escalation", func(t *testing.T) {
		f, _ := begin(pub, "openid admin", nil)
		raw, e := s.AuthRequestByID(ctx, f.id)
		if e != nil {
			t.Fatal(e)
		}
		for _, scope := range raw.GetScopes() {
			if scope == "admin" {
				t.Fatal("unknown scope retained")
			}
		}
		status, c := complete(f)
		if status != 200 {
			t.Fatal(status)
		}
		status, out := exchange(f, c, "", nil)
		if status != 200 || strings.Contains(out["scope"].(string), "admin") {
			t.Fatal("unknown scope granted")
		}
		t.Log("OBSERVATION: OIDC library filters unknown scopes before storage validation")
	})
	t.Run("minimal scopes do not expose email", func(t *testing.T) {
		f, c := code(pub, "openid")
		status, out := exchange(f, c, "", nil)
		if status != 200 {
			t.Fatal(status)
		}
		status, user := info(out["access_token"].(string))
		if status != 200 || user["email"] != nil || user["name"] != nil {
			t.Fatal("scope leakage")
		}
	})
	t.Run("restricted grant and revocation", func(t *testing.T) {
		s.db.Exec(ctx, "UPDATE apps SET restricted=true WHERE id=$1", pub)
		defer s.db.Exec(ctx, "UPDATE apps SET restricted=false WHERE id=$1", pub)
		f, _ := begin(pub, "openid", nil)
		if status, _ := complete(f); status != 403 {
			t.Fatal("restricted app admitted user", status)
		}
		s.db.Exec(ctx, "INSERT INTO grants(app_id,user_id) VALUES($1,$2)", pub, uid)
		f, c := code(pub, "openid")
		status, out := exchange(f, c, "", nil)
		if status != 200 {
			t.Fatal(status)
		}
		s.db.Exec(ctx, "DELETE FROM grants WHERE app_id=$1 AND user_id=$2", pub, uid)
		if status, _ = info(out["access_token"].(string)); status == 200 {
			t.Fatal("revoked grant token still usable")
		}
	})
	t.Run("disabled project rejects exchange", func(t *testing.T) {
		f, c := code(pub, "openid")
		s.db.Exec(ctx, "UPDATE apps SET enabled=false WHERE id=$1", pub)
		defer s.db.Exec(ctx, "UPDATE apps SET enabled=true WHERE id=$1", pub)
		if status, _ := exchange(f, c, "", nil); status == 200 {
			t.Fatal("disabled project exchanged code")
		}
	})
	t.Run("expired auth request", func(t *testing.T) {
		f, _ := begin(pub, "openid", nil)
		s.db.Exec(ctx, "UPDATE auth_requests SET expires_at=now()-interval '1 second' WHERE id=$1", f.id)
		if status, _ := complete(f); status == 200 {
			t.Fatal("expired request completed")
		}
	})
	t.Run("expired access token", func(t *testing.T) {
		f, c := code(pub, "openid")
		_, out := exchange(f, c, "", nil)
		s.db.Exec(ctx, "UPDATE access_tokens SET expires_at=now()-interval '1 second' WHERE user_id=$1", uid)
		if status, _ := info(out["access_token"].(string)); status == 200 {
			t.Fatal("expired token accepted")
		}
	})
	t.Run("flow cookie prevents foreign browser completion", func(t *testing.T) {
		f, _ := begin(pub, "openid", nil)
		jar2, _ := cookiejar.New(nil)
		b := &http.Client{Jar: jar2}
		login(b)
		status, _ := request(b, "POST", "/api/auth/continue", map[string]string{"id": f.id})
		if status != 403 {
			t.Fatal("foreign flow accepted", status)
		}
	})
	t.Run("parallel tabs complete independently", func(t *testing.T) {
		a, _ := begin(pub, "openid", nil)
		b, _ := begin(other, "openid", nil)
		if status, _ := complete(a); status != 200 {
			t.Fatal("first tab cannot complete", status)
		}
		if status, _ := complete(b); status != 200 {
			t.Fatal("latest tab cannot complete", status)
		}
	})
	t.Run("reauthentication and silent login boundary", func(t *testing.T) {
		for _, change := range []func(url.Values){func(q url.Values) { q.Set("prompt", "login") }, func(q url.Values) { q.Set("max_age", "0") }} {
			f, _ := begin(pub, "openid", change)
			time.Sleep(time.Millisecond)
			if status, _ := complete(f); status != 401 {
				t.Fatal("reauthentication not enforced", status)
			}
			login(browser)
			if status, _ := complete(f); status != 200 {
				t.Fatal("fresh authentication rejected", status)
			}
		}
		f, c := code(pub, "openid")
		if status, _ := exchange(f, c, "", nil); status != 200 {
			t.Fatal("prior consent failed")
		}
		f, _ = begin(pub, "openid", func(q url.Values) { q.Set("prompt", "none") })
		if f.id == "" {
			t.Fatal("silent authentication rejected")
		}
		raw, e := s.AuthRequestByID(ctx, f.id)
		if e != nil || !raw.Done() {
			t.Fatal("silent request not completed")
		}
	})
	t.Run("refresh rotation replay scope and logout", func(t *testing.T) {
		s.db.Exec(ctx, "UPDATE apps SET refresh_enabled=true WHERE id=$1", pub)
		defer s.db.Exec(ctx, "UPDATE apps SET refresh_enabled=false WHERE id=$1", pub)
		issue := func() map[string]any {
			f, _ := begin(pub, "openid email offline_access", func(q url.Values) { q.Set("prompt", "consent") })
			status, c := complete(f)
			if status != 200 {
				t.Fatal("consent", status)
			}
			status, out := exchange(f, c, "", nil)
			if status != 200 || out["refresh_token"] == nil {
				t.Fatal("refresh not issued", status, out["error"])
			}
			return out
		}
		refresh := func(token, client, scope string) (int, map[string]any) {
			return exchange(flow{client: client}, "", "", func(q url.Values) {
				clear(q)
				q.Set("client_id", client)
				q.Set("grant_type", "refresh_token")
				q.Set("refresh_token", token)
				if scope != "" {
					q.Set("scope", scope)
				}
			})
		}
		out := issue()
		old := out["refresh_token"].(string)
		if status, _ := refresh(old, other, ""); status == 200 {
			t.Fatal("cross client refresh")
		}
		if status, _ := refresh(old, pub, "openid admin"); status == 200 {
			t.Fatal("scope escalation")
		}
		status, next := refresh(old, pub, "openid")
		if status != 200 || next["refresh_token"] == old {
			t.Fatal("rotation failed", status, next["error"])
		}
		if status, _ := info(next["access_token"].(string)); status != 200 {
			t.Fatal("rotated access rejected")
		}
		if status, _ := refresh(old, pub, ""); status == 200 {
			t.Fatal("replay accepted")
		}
		if status, _ := refresh(next["refresh_token"].(string), pub, ""); status == 200 {
			t.Fatal("family not revoked")
		}
		if status, _ := info(next["access_token"].(string)); status == 200 {
			t.Fatal("family access survived replay")
		}
		out = issue()
		raceToken := out["refresh_token"].(string)
		statuses := make(chan int, 2)
		for range 2 {
			go func() { status, _ := refresh(raceToken, pub, ""); statuses <- status }()
		}
		first, second := <-statuses, <-statuses
		if (first == 200) == (second == 200) {
			t.Fatal("concurrent refresh must have exactly one success", first, second)
		}
		out = issue()
		request(browser, "POST", "/api/auth/logout", map[string]string{})
		if status, _ := info(out["access_token"].(string)); status == 200 {
			t.Fatal("access survived logout")
		}
		if status, _ := refresh(out["refresh_token"].(string), pub, ""); status == 200 {
			t.Fatal("refresh survived logout")
		}
		_, loc := begin(pub, "openid", func(q url.Values) { q.Set("prompt", "none") })
		if !strings.Contains(loc, "login_required") {
			t.Fatal("anonymous silent login")
		}
		login(browser)
	})
	t.Run("signed persistent logout delivery and retry", func(t *testing.T) {
		calls := 0
		var eventID any
		receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			r.ParseForm()
			token, e := provider.Verifier(&verifierlib.Config{ClientID: pub, SkipExpiryCheck: true}).Verify(ctx, r.PostForm.Get("logout_token"))
			if e != nil {
				t.Error("invalid signed logout", e)
				w.WriteHeader(400)
				return
			}
			var claims map[string]any
			token.Claims(&claims)
			if calls == 1 {
				eventID = claims["jti"]
			} else if eventID != claims["jti"] {
				t.Error("retry changed event identity")
			}
			if token.Subject != uid || claims["sid"] == nil || claims["events"] == nil || claims["nonce"] != nil || claims["jti"] == nil {
				t.Error("logout claims")
			}
			if calls == 1 {
				w.WriteHeader(503)
			} else {
				w.WriteHeader(204)
			}
		}))
		defer receiver.Close()
		s.db.Exec(ctx, "UPDATE apps SET backchannel_logout_uri=$1 WHERE id=$2", receiver.URL, pub)
		defer s.db.Exec(ctx, "UPDATE apps SET backchannel_logout_uri='' WHERE id=$1", pub)
		f, c := code(pub, "openid")
		status, out := exchange(f, c, "", nil)
		if status != 200 {
			t.Fatal("issue")
		}
		request(browser, "POST", "/api/auth/logout", map[string]string{})
		if status, _ := info(out["access_token"].(string)); status == 200 {
			t.Fatal("logout access survived")
		}
		if worked, e := s.deliverLogout(ctx); e != nil || !worked || calls != 1 {
			t.Fatal("delivery missing", e, calls)
		}
		var state string
		s.db.QueryRow(ctx, "SELECT status FROM logout_jobs WHERE app_id=$1 ORDER BY id DESC LIMIT 1", pub).Scan(&state)
		if state != "queued" {
			t.Fatal("no retry", state)
		}
		s.db.Exec(ctx, "UPDATE logout_jobs SET next_attempt=now() WHERE app_id=$1", pub)
		if worked, e := s.deliverLogout(ctx); e != nil || !worked || calls != 2 {
			t.Fatal("retry missing", e, calls)
		}
		s.db.QueryRow(ctx, "SELECT status FROM logout_jobs WHERE app_id=$1 ORDER BY id DESC LIMIT 1", pub).Scan(&state)
		if state != "delivered" {
			t.Fatal("not delivered", state)
		}
		production := &Server{}
		res, e := production.logoutHTTPClient().Get(receiver.URL)
		if e == nil {
			res.Body.Close()
			t.Fatal("private callback not blocked")
		}
		login(browser)
	})
	t.Run("API key overlap rotation expiry and revocation", func(t *testing.T) {

		var originalAdmin string
		if err := s.db.QueryRow(ctx, "SELECT id FROM users WHERE admin").Scan(&originalAdmin); err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.Exec(ctx, "UPDATE users SET admin=false WHERE id=$1", originalAdmin); err != nil {
			t.Fatal(err)
		}
		defer s.db.Exec(ctx, "UPDATE users SET admin=true WHERE id=$1", originalAdmin)
		if _, err := s.db.Exec(ctx, "UPDATE users SET admin=true WHERE id=$1", uid); err != nil {
			t.Fatal(err)
		}
		defer s.db.Exec(ctx, "UPDATE users SET admin=false WHERE id=$1", uid)

		old := "dy_rotation_fixture"
		s.db.Exec(ctx, "INSERT INTO api_keys(id,name,digest,app_id,scopes) VALUES('rotation-key','rotation',$1,$2,ARRAY['app:read'])", s.mac(old), pub)
		call := func(secret string) int {
			r, _ := http.NewRequest("GET", origin+"/api/service/app", nil)
			r.Header.Set("Authorization", "Bearer "+secret)
			res, e := http.DefaultClient.Do(r)
			if e != nil {
				t.Fatal(e)
			}
			res.Body.Close()
			return res.StatusCode
		}
		status, out := request(browser, "POST", "/api/admin/keys/rotation-key/rotate", map[string]any{"overlap_hours": 1})
		if status != 200 {
			t.Fatal(status)
		}
		fresh := out["data"].(map[string]any)["secret"].(string)
		if call(old) != 200 || call(fresh) != 200 {
			t.Fatal("overlap rejected")
		}
		if status, _ := request(browser, "POST", "/api/admin/keys/rotation-key/rotate", map[string]any{"overlap_hours": 1}); status != 409 {
			t.Fatal("nested rotation")
		}
		s.db.Exec(ctx, "UPDATE api_keys SET previous_until=now()-interval '1 second' WHERE id='rotation-key'")
		if call(old) != 401 || call(fresh) != 200 {
			t.Fatal("overlap deadline")
		}
		s.db.Exec(ctx, "UPDATE api_keys SET revoked=true WHERE id='rotation-key'")
		if call(fresh) != 401 {
			t.Fatal("revocation")
		}
	})
	t.Run("operations alerts deduplicate and recover", func(t *testing.T) {
		if e := s.monitorOperations(ctx); e != nil {
			t.Fatal(e)
		}
		var before int
		s.db.QueryRow(ctx, "SELECT count(*) FROM audit WHERE action LIKE 'monitor.%' AND target='logout_delivery_failed'").Scan(&before)
		var id int64
		s.db.QueryRow(ctx, "INSERT INTO logout_jobs(app_id,user_id,session_hash,endpoint,status) VALUES($1,$2,'monitor','https://example.invalid/logout','failed') RETURNING id", pub, uid).Scan(&id)
		for range 2 {
			if e := s.monitorOperations(ctx); e != nil {
				t.Fatal(e)
			}
		}
		var count int
		s.db.QueryRow(ctx, "SELECT count(*) FROM audit WHERE action LIKE 'monitor.%' AND target='logout_delivery_failed'").Scan(&count)
		if count != before+1 {
			t.Fatal("alert not deduplicated", before, count)
		}
		s.db.Exec(ctx, "DELETE FROM logout_jobs WHERE id=$1", id)
		if e := s.monitorOperations(ctx); e != nil {
			t.Fatal(e)
		}
		s.db.QueryRow(ctx, "SELECT count(*) FROM audit WHERE action LIKE 'monitor.%' AND target='logout_delivery_failed'").Scan(&count)
		if count != before+2 {
			t.Fatal("recovery missing")
		}
	})
	t.Run("database unavailable fails closed without false logout", func(t *testing.T) {
		broken, e := pgxpool.NewWithConfig(ctx, s.db.Config())
		if e != nil {
			t.Fatal(e)
		}
		broken.Close()
		unavailable := *s
		unavailable.db = broken
		request := httptest.NewRequest("GET", origin+"/api/me", nil)
		issuerURL, _ := url.Parse(origin)
		for _, cookie := range browser.Jar.Cookies(issuerURL) {
			request.AddCookie(cookie)
		}
		response := httptest.NewRecorder()
		if _, yes := unavailable.require(response, request, false); yes || response.Code != 503 {
			t.Fatal("database outage mistaken for authentication failure", response.Code)
		}
		if _, e := unavailable.deliverLogout(ctx); e == nil {
			t.Fatal("queue failure hidden")
		}
		if e := unavailable.monitorOperations(ctx); e == nil {
			t.Fatal("monitor falsely healthy")
		}
	})

	t.Run("single super administrator and no delegated roles", func(t *testing.T) {
		if _, err := s.db.Exec(ctx, "UPDATE users SET admin=true WHERE id=$1", uid); err == nil {
			t.Fatal("second super administrator accepted")
		}
		for _, role := range []string{"auditor", "maintainer"} {
			if _, err := s.db.Exec(ctx, "UPDATE users SET management_role=$1 WHERE id=$2", role, uid); err == nil {
				t.Fatal("delegated role accepted")
			}
		}
		for _, path := range []string{"/api/admin/apps", "/api/admin/users", "/api/admin/audit", "/api/admin/diagnostics"} {
			if status, _ := request(browser, "GET", path, nil); status != 403 {
				t.Fatal("ordinary account reached management API", path, status)
			}
		}
		if status, _ := request(browser, "PUT", "/api/admin/users/"+uid+"/access", map[string]any{"role": "auditor"}); status != 403 {
			t.Fatal("role escalation")
		}
	})
	t.Run("logout failure metadata manual retry and audit correlation", func(t *testing.T) {

		var originalAdmin string
		if err := s.db.QueryRow(ctx, "SELECT id FROM users WHERE admin").Scan(&originalAdmin); err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.Exec(ctx, "UPDATE users SET admin=false WHERE id=$1", originalAdmin); err != nil {
			t.Fatal(err)
		}
		defer s.db.Exec(ctx, "UPDATE users SET admin=true WHERE id=$1", originalAdmin)
		if _, err := s.db.Exec(ctx, "UPDATE users SET admin=true WHERE id=$1", uid); err != nil {
			t.Fatal(err)
		}
		defer s.db.Exec(ctx, "UPDATE users SET admin=false WHERE id=$1", uid)

		receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
		defer receiver.Close()
		var id int64
		if e := s.db.QueryRow(ctx, "INSERT INTO logout_jobs(app_id,user_id,session_hash,endpoint,attempts) VALUES($1,$2,'test-session',$3,9) RETURNING id", pub, uid, receiver.URL).Scan(&id); e != nil {
			t.Fatal(e)
		}
		if _, e := s.deliverLogout(ctx); e != nil {
			t.Fatal(e)
		}
		var state, kind string
		var code, attempts int
		s.db.QueryRow(ctx, "SELECT status,last_error,http_status FROM logout_jobs WHERE id=$1", id).Scan(&state, &kind, &code)
		if state != "failed" || kind != "http_status" || code != 503 {
			t.Fatal(state, kind, code)
		}
		path := "/api/admin/logout-events/" + strconv.FormatInt(id, 10)
		if status, _ := request(browser, "POST", path+"/retry", map[string]any{}); status != 200 {
			t.Fatal("retry", status)
		}
		if status, _ := request(browser, "POST", path+"/retry", map[string]any{}); status != 409 {
			t.Fatal("duplicate retry", status)
		}
		s.db.QueryRow(ctx, "SELECT count(*) FROM logout_attempts WHERE job_id=$1", id).Scan(&attempts)
		if attempts != 1 {
			t.Fatal("history lost")
		}
		var requestID string
		s.db.QueryRow(ctx, "SELECT request_id FROM audit WHERE action='admin.logout.retry' AND target=$1 ORDER BY id DESC LIMIT 1", strconv.FormatInt(id, 10)).Scan(&requestID)
		if requestID == "" {
			t.Fatal("missing request correlation")
		}
		s.db.Exec(ctx, "DELETE FROM logout_jobs WHERE id=$1", id)
	})
	t.Run("server pagination searches beyond newest 200", func(t *testing.T) {

		var originalAdmin string
		if err := s.db.QueryRow(ctx, "SELECT id FROM users WHERE admin").Scan(&originalAdmin); err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.Exec(ctx, "UPDATE users SET admin=false WHERE id=$1", originalAdmin); err != nil {
			t.Fatal(err)
		}
		defer s.db.Exec(ctx, "UPDATE users SET admin=true WHERE id=$1", originalAdmin)
		if _, err := s.db.Exec(ctx, "UPDATE users SET admin=true WHERE id=$1", uid); err != nil {
			t.Fatal(err)
		}
		defer s.db.Exec(ctx, "UPDATE users SET admin=false WHERE id=$1", uid)

		s.db.Exec(ctx, "INSERT INTO users(id,email,name) SELECT 'page-'||n,'page-'||n||'@example.invalid','Pagination' FROM generate_series(1,230) n")
		status, out := request(browser, "GET", "/api/admin/users?page=12&page_size=20&q=Pagination", nil)
		if status != 200 {
			t.Fatal("pagination status", status)
		}
		data := out["data"].(map[string]any)
		if data["total"].(float64) != 230 || len(data["items"].([]any)) != 10 {
			t.Fatal("pagination totals", data["total"])
		}
	})
	t.Run("public browser CORS preflight", func(t *testing.T) {
		r, _ := http.NewRequest("OPTIONS", discovery["token_endpoint"].(string), nil)
		r.Header.Set("Origin", "http://localhost:8199")
		r.Header.Set("Access-Control-Request-Method", "POST")
		r.Header.Set("Access-Control-Request-Headers", "content-type")
		resp, e := http.DefaultClient.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		defer resp.Body.Close()
		io.Copy(io.Discard, resp.Body)
		t.Logf("OBSERVATION: token preflight status=%d allow-origin=%q", resp.StatusCode, resp.Header.Get("Access-Control-Allow-Origin"))
	})
	t.Run("native callback registration boundary", func(t *testing.T) {
		if s.validRedirect("duoyu-demo://callback") {
			t.Fatal("custom URI unexpectedly supported")
		}
		if !s.validRedirect("http://127.0.0.1:8199/callback") {
			t.Fatal("dev loopback rejected")
		}
		production := &Server{}
		if production.validRedirect("http://127.0.0.1:8199/callback") {
			t.Fatal("production loopback behavior changed")
		}
		for _, v := range []string{"http://127.0.0.1:8199/callback", "http://[::1]:8199/callback", "com.example.app:/callback"} {
			if !production.validAppRedirect(v, "native") {
				t.Fatal("native redirect rejected", v)
			}
		}
		for _, v := range []string{"http://evil.example/callback", "http://localhost:8199/callback", "javascript:alert(1)", "com.example.app://evil/callback"} {
			if production.validAppRedirect(v, "native") {
				t.Fatal("unsafe native callback", v)
			}
		}
	})
	t.Run("native loopback dynamic port full exchange", func(t *testing.T) {
		s.db.Exec(ctx, "UPDATE apps SET platform='native',redirects=$1 WHERE id=$2", []string{"http://127.0.0.1:8199/callback"}, other)
		defer s.db.Exec(ctx, "UPDATE apps SET platform='spa',redirects=$1 WHERE id=$2", []string{redirect}, other)
		actual := "http://127.0.0.1:18347/callback"
		f, _ := begin(other, "openid", func(q url.Values) { q.Set("redirect_uri", actual) })
		if f.id == "" {
			t.Fatal("dynamic port authorization rejected")
		}
		status, c := complete(f)
		if status != 200 {
			t.Fatal("native completion")
		}
		if status, _ := exchange(f, c, "", func(q url.Values) { q.Set("redirect_uri", actual) }); status != 200 {
			t.Fatal("native exchange", status)
		}
	})
	t.Run("cross-origin embedded login blocked", func(t *testing.T) {
		r, _ := http.NewRequest("POST", origin+"/api/auth/login", strings.NewReader(`{}`))
		r.Header.Set("Origin", "http://localhost:8199")
		r.Header.Set("X-Duoyu-Request", "1")
		resp, e := http.DefaultClient.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		resp.Body.Close()
		if resp.StatusCode != 403 {
			t.Fatal("cross-origin password API allowed")
		}
	})
}
