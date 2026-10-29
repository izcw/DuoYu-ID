package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
)

func testQRLogin(t *testing.T, s *Server, origin string) {
	t.Helper()
	newClient := func() *http.Client { jar, _ := cookiejar.New(nil); return &http.Client{Jar: jar} }
	desktop, phone, stranger := newClient(), newClient(), newClient()
	call := func(c *http.Client, path string, data any) (int, map[string]any) {
		t.Helper()
		b, _ := json.Marshal(data)
		r, _ := http.NewRequest("POST", origin+path, bytes.NewReader(b))
		r.Header.Set("Origin", origin)
		r.Header.Set("X-Duoyu-Request", "1")
		r.Header.Set("X-Duoyu-Legal-Version", legalVersion)
		r.Header.Set("X-Duoyu-Adult", "true")
		r.Header.Set("Content-Type", "application/json")
		resp, e := c.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		defer resp.Body.Close()
		var out map[string]any
		if e = json.NewDecoder(resp.Body).Decode(&out); e != nil {
			t.Fatal(e)
		}
		return resp.StatusCode, out
	}
	status, out := call(desktop, "/api/auth/qr", map[string]any{})
	if status != 200 {
		t.Fatal(out)
	}
	id := out["data"].(map[string]any)["id"].(string)
	// Seed a known approval capability; production stores only its hash.
	ticket := randomID()
	ctx := context.Background()
	if _, e := s.db.Exec(ctx, "UPDATE qr_logins SET ticket_hash=$1 WHERE id=$2", digest(ticket), id); e != nil {
		t.Fatal(e)
	}
	status, _ = call(stranger, "/api/auth/qr/"+id+"/poll", map[string]any{})
	if status != 403 {
		t.Fatal("unbound browser polled", status)
	}
	status, _ = call(phone, "/api/auth/qr/context", map[string]any{"ticket": ticket})
	if status != 200 {
		t.Fatal("scan failed")
	}
	status, _ = call(phone, "/api/auth/qr/approve", map[string]any{"ticket": ticket})
	if status != 401 {
		t.Fatal("anonymous approval accepted")
	}
	var uid string
	s.db.QueryRow(ctx, "SELECT id FROM users WHERE email='admin@example.com'").Scan(&uid)
	token := randomID()
	_, e := s.db.Exec(ctx, "INSERT INTO sessions(hash,user_id,expires_at,amr) VALUES($1,$2,now()+interval '5 minutes','pwd')", digest(token), uid)
	if e != nil {
		t.Fatal(e)
	}
	u, _ := url.Parse(origin)
	phone.Jar.SetCookies(u, []*http.Cookie{{Name: "duoyu_session", Value: token, Path: "/"}})
	status, _ = call(phone, "/api/auth/qr/approve", map[string]any{"ticket": ticket})
	if status != 403 {
		t.Fatal("password session approved QR")
	}
	s.db.Exec(ctx, "UPDATE sessions SET amr='hwk user' WHERE hash=$1", digest(token))
	status, out = call(phone, "/api/auth/qr/approve", map[string]any{"ticket": ticket})
	if status != 403 {
		t.Fatal("unenrolled passkey session approved QR")
	}
	_, e = s.db.Exec(ctx, "INSERT INTO passkeys(id,user_id,name,credential,qr_enabled) VALUES('qr-test-key',$1,'phone','{}',false)", uid)
	if e != nil {
		t.Fatal(e)
	}
	s.db.Exec(ctx, "UPDATE sessions SET passkey_id='qr-test-key' WHERE hash=$1", digest(token))
	status, _ = call(phone, "/api/auth/qr/approve", map[string]any{"ticket": ticket})
	if status != 403 {
		t.Fatal("ordinary passkey approved QR")
	}
	s.db.Exec(ctx, "UPDATE passkeys SET qr_enabled=true WHERE id='qr-test-key'")
	defer s.db.Exec(ctx, "DELETE FROM passkeys WHERE id='qr-test-key'")
	status, out = call(phone, "/api/auth/qr/approve", map[string]any{"ticket": ticket})
	if status != 200 {
		t.Fatal(out)
	}
	status, _ = call(phone, "/api/auth/qr/approve", map[string]any{"ticket": ticket})
	if status != 403 {
		t.Fatal("approval replay accepted")
	}
	var wins atomic.Int32
	// Re-check enrollment when the computer claims an already approved QR.
	s.db.Exec(ctx, "UPDATE passkeys SET qr_enabled=false WHERE id='qr-test-key'")
	status, _ = call(desktop, "/api/auth/qr/"+id+"/poll", map[string]any{})
	if status != 403 {
		t.Fatal("revoked enrollment still issued a desktop session")
	}
	s.db.Exec(ctx, "UPDATE passkeys SET qr_enabled=true WHERE id='qr-test-key'")
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			status, out := call(desktop, "/api/auth/qr/"+id+"/poll", map[string]any{})
			if status == 200 && out["data"].(map[string]any)["status"] == "approved" {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatal("QR grant consumed more than once", wins.Load())
	}
	resp, e := desktop.Get(origin + "/api/me")
	if e != nil {
		t.Fatal(e)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatal("desktop session missing")
	}
	status, out = call(desktop, "/api/auth/qr", map[string]any{})
	if status != 200 {
		t.Fatal(out)
	}
	id = out["data"].(map[string]any)["id"].(string)
	s.db.Exec(ctx, "UPDATE qr_logins SET expires_at=now()-interval '1 second' WHERE id=$1", id)
	_, out = call(desktop, "/api/auth/qr/"+id+"/poll", map[string]any{})
	if out["data"].(map[string]any)["status"] != "expired" {
		t.Fatal("expiry failed")
	}
	_, out = call(desktop, "/api/auth/qr", map[string]any{})
	id = out["data"].(map[string]any)["id"].(string)
	call(desktop, "/api/auth/qr/"+id+"/cancel", map[string]any{})
	_, out = call(desktop, "/api/auth/qr/"+id+"/poll", map[string]any{})
	if out["data"].(map[string]any)["status"] != "expired" {
		t.Fatal("cancellation failed")
	}
}
