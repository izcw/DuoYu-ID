package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func testProjectMail(t *testing.T, s *Server, origin string) {
	ctx := context.Background()
	appID := "mail-test-" + randomID()
	otherApp := "mail-other-" + randomID()
	for _, id := range []string{appID, otherApp} {
		if _, e := s.db.Exec(ctx, "INSERT INTO apps(id,name,redirects) VALUES($1,'邮件测试',$2)", id, []string{"http://localhost/callback"}); e != nil {
			t.Fatal(e)
		}
	}
	createKey := func(app string, scopes []string, expiry any) string {
		t.Helper()
		secret := randomID()
		if _, e := s.db.Exec(ctx, "INSERT INTO api_keys(id,name,digest,app_id,scopes,expires_at) VALUES($1,'mail test',$2,$3,$4,$5)", randomID(), s.mac(secret), app, scopes, expiry); e != nil {
			t.Fatal(e)
		}
		return secret
	}
	token := createKey(appID, []string{"mail:send", "mail:read"}, nil)
	other := createKey(otherApp, []string{"mail:send", "mail:read"}, nil)
	readOnly := createKey(appID, []string{"app:read"}, nil)
	expired := createKey(appID, []string{"mail:send"}, time.Now().Add(-time.Hour))
	var calls atomic.Int32
	var messages []MailMessage
	var mu sync.Mutex
	var failSend atomic.Bool
	s.mailTransport = func(m MailMessage) error {
		calls.Add(1)
		mu.Lock()
		messages = append(messages, m)
		mu.Unlock()
		if failSend.Load() {
			return errors.New("simulated ambiguous transport")
		}
		return nil
	}
	defer func() { s.mailTransport = nil }()
	request := func(method, path, key, idem string, body any) (int, map[string]any) {
		t.Helper()
		b, _ := json.Marshal(body)
		r, _ := http.NewRequest(method, origin+path, bytes.NewReader(b))
		r.Header.Set("Authorization", "Bearer "+key)
		r.Header.Set("Idempotency-Key", idem)
		r.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Error(err)
			return 0, nil
		}
		defer res.Body.Close()
		var out map[string]any
		json.NewDecoder(res.Body).Decode(&out)
		return res.StatusCode, out
	}
	send := func(key, idem string, body any) (int, map[string]any) {
		return request("POST", "/api/service/mail", key, idem, body)
	}
	input := ProjectMail{To: "mail-test@example.com", Subject: "任务通知", Text: "hello <script>alert(1)</script>\n第二行"}
	for _, pair := range []struct {
		key  string
		want int
	}{{"", 401}, {readOnly, 403}, {expired, 401}} {
		status, _ := send(pair.key, "scope-check", input)
		if status != pair.want {
			t.Fatalf("scope status %d want %d", status, pair.want)
		}
	}
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			status, _ := send(token, "concurrent-mail", input)
			if status != 200 {
				t.Errorf("concurrent send %d", status)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 0 {
		t.Fatal("HTTP handler sent SMTP instead of enqueueing")
	}
	if worked, e := s.processMail(ctx, "project"); e != nil || !worked {
		t.Fatal("queue not processed", e)
	}
	if calls.Load() != 1 {
		t.Fatalf("duplicate deliveries: %d", calls.Load())
	}
	status, out := send(token, "concurrent-mail", input)
	if status != 200 {
		t.Fatal(status)
	}
	id := out["data"].(map[string]any)["id"].(string)
	if !strings.Contains(messages[0].HTML, "&lt;script&gt;") || strings.Contains(messages[0].HTML, "<script>") {
		t.Fatal("default template must escape user text")
	}
	if status, _ = request("GET", "/api/service/mail/"+id, other, "", nil); status != 404 {
		t.Fatal("cross-project mail visibility", status)
	}
	if status, out = request("GET", "/api/service/mail/"+id, token, "", nil); status != 200 || out["data"].(map[string]any)["status"] != "accepted" {
		t.Fatal("status lookup", status)
	}
	input.Subject = "different"
	if status, _ = send(token, "concurrent-mail", input); status != 409 {
		t.Fatal("idempotency conflict", status)
	}
	input.HTML = "<!doctype html><h1>项目自己的模板</h1>"
	input.Text = "plain fallback"
	if status, _ = send(token, "custom-html-mail", input); status != 200 {
		t.Fatal(status)
	}
	s.processMail(ctx, "project")
	if messages[1].HTML != input.HTML {
		t.Fatal("custom HTML modified")
	}
	input.Subject = "bad\r\nBcc: victim@example.com"
	if status, _ = send(token, "invalid-header", input); status != 400 {
		t.Fatal("header injection", status)
	}
	input.Subject = "valid"
	input.HTML = strings.Repeat("x", 65537)
	if status, _ = send(token, "oversize-html", input); status != 400 {
		t.Fatal("oversized HTML", status)
	}
	input.HTML = "<p>test</p>"
	input.To = "unknown@example.com"
	failSend.Store(true)
	status, out = send(token, "unknown-smtp-result", input)
	s.processMail(ctx, "project")
	status, out = send(token, "unknown-smtp-result", input)
	if status != 200 || out["data"].(map[string]any)["status"] != "unknown" {
		t.Fatal("unknown delivery result", out)
	}
	before := calls.Load()
	send(token, "unknown-smtp-result", input)
	if calls.Load() != before {
		t.Fatal("unknown SMTP automatically retried")
	}
	s.db.Exec(ctx, "UPDATE apps SET enabled=false WHERE id=$1", appID)
	if status, _ = send(token, "disabled-project", input); status != 403 {
		t.Fatal("disabled app", status)
	}
	s.db.Exec(ctx, "UPDATE apps SET enabled=true WHERE id=$1", appID)
	s.db.Exec(ctx, "UPDATE api_keys SET revoked=true WHERE digest=$1", s.mac(token))
	if status, _ = send(token, "revoked-api-key", input); status != 401 {
		t.Fatal("revoked key", status)
	}
	t.Setenv("MAIL_APP_PER_MINUTE", "1")
	input.To = "quota@example.com"
	failSend.Store(false)
	if status, _ = send(other, "quota-first", input); status != 200 {
		t.Fatal(status)
	}
	if status, _ = send(other, "quota-second", input); status != 429 {
		t.Fatal("quota not enforced", status)
	}
	before = calls.Load()
	s.db.Exec(ctx, "UPDATE api_keys SET revoked=true WHERE digest=$1", s.mac(other))
	s.processMail(ctx, "project")
	if calls.Load() != before {
		t.Fatal("revoked credential's queued mail was sent")
	}
	// No body/HTML/code is persisted in the metadata table.
	var columns int
	s.db.QueryRow(ctx, "SELECT count(*) FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='mail_messages' AND column_name IN ('text','html','body','code')").Scan(&columns)
	if columns != 0 {
		t.Fatal("mail body persisted")
	}
}
