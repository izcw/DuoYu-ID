package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRandomRegistrationPassword(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		password, err := randomRegistrationPassword()
		if err != nil {
			t.Fatal(err)
		}
		if !regexp.MustCompile(`^[A-Za-z0-9]{32}$`).MatchString(password) ||
			!strings.ContainsAny(password, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") ||
			!strings.ContainsAny(password, "abcdefghijklmnopqrstuvwxyz") ||
			!strings.ContainsAny(password, "0123456789") || seen[password] {
			t.Fatal("invalid or duplicated generated password")
		}
		seen[password] = true
	}
}

func TestPasswordLengthPolicy(t *testing.T) {
	for _, password := range []string{"123456", "      ", "中文密码测试", strings.Repeat("😀", 64), strings.Repeat("a", 64)} {
		if err := validPassword(password); err != nil {
			t.Errorf("valid length %d: %v", len([]rune(password)), err)
		}
	}
	for _, password := range []string{"", "12345", strings.Repeat("😀", 65), strings.Repeat("a", 65), string([]byte{255, 255, 255, 255, 255, 255})} {
		if validPassword(password) == nil {
			t.Error("invalid password length accepted")
		}
	}
	for level, minutes := range []int{10, 60, 360, 2160, 12960} {
		if passwordLockDuration(level) != time.Duration(minutes)*time.Minute {
			t.Fatal("wrong escalation", level)
		}
	}
	if passwordLockDuration(100) <= 0 {
		t.Fatal("duration overflow")
	}
}

func TestRetryWaitText(t *testing.T) {
	for _, tc := range []struct {
		seconds int64
		want    string
	}{{0, "1 秒"}, {1, "1 秒"}, {59, "59 秒"}, {60, "1 分钟"}, {600, "10 分钟"}, {3600, "1 小时"}, {21600, "6 小时"}, {90061, "1 天 1 小时 1 分钟 1 秒"}} {
		if got := retryWaitText(tc.seconds); got != tc.want {
			t.Fatalf("%d: %q", tc.seconds, got)
		}
	}
}

func testAccountPolicy(t *testing.T, s *Server) {
	ctx := context.Background()
	password := "密码 😀 !"
	_, err := s.db.Exec(ctx, `INSERT INTO users(id,email,name,password_hash) VALUES('policy-user','policy@example.com','昵称',$1)`, hashPassword(password))
	if err != nil {
		t.Fatal(err)
	}
	verify := func(input string) (bool, time.Time) {
		t.Helper()
		ok, until, err := s.verifyAccountPassword(ctx, "policy@example.com", input)
		if err != nil {
			t.Fatal(err)
		}
		return ok, until
	}
	for level, minutes := range []int{10, 60, 360, 2160} {
		for attempt := 1; attempt <= 5; attempt++ {
			ok, until := verify("wrong")
			if ok || (attempt < 5 && !until.IsZero()) {
				t.Fatal("early lock or accepted password", level, attempt)
			}
			if attempt == 5 && (time.Until(until) > time.Duration(minutes)*time.Minute || time.Until(until) < time.Duration(minutes)*time.Minute-5*time.Second) {
				t.Fatal("wrong lock duration", level, until)
			}
		}
		if ok, until := verify(password); ok || until.IsZero() {
			t.Fatal("correct password bypassed lock")
		}
		var stored int
		s.db.QueryRow(ctx, "SELECT password_lock_level FROM users WHERE id='policy-user'").Scan(&stored)
		if stored != level+1 {
			t.Fatal("locked attempt changed level", stored)
		}
		if _, err = s.db.Exec(ctx, "UPDATE users SET password_locked_until=now()-interval '1 second' WHERE id='policy-user'"); err != nil {
			t.Fatal(err)
		}
	}
	if ok, _ := verify(password); !ok {
		t.Fatal("expired lock did not allow correct password")
	}
	var failures, level int
	s.db.QueryRow(ctx, "SELECT password_failures,password_lock_level FROM users WHERE id='policy-user'").Scan(&failures, &level)
	if failures != 0 || level != 0 {
		t.Fatal("success did not reset history")
	}
	// Concurrent failures must trigger exactly one lock and never lose updates.
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := s.verifyAccountPassword(ctx, "policy@example.com", "wrong")
			if err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	s.db.QueryRow(ctx, "SELECT password_failures,password_lock_level FROM users WHERE id='policy-user'").Scan(&failures, &level)
	if failures != 0 || level != 1 {
		t.Fatal("concurrent lock failed", failures, level)
	}
	call := func(handler http.HandlerFunc, body any) *httptest.ResponseRecorder {
		t.Helper()
		data, _ := json.Marshal(body)
		r := httptest.NewRequest("POST", "/", bytes.NewReader(data))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler(w, r)
		return w
	}
	lockedResponse := call(s.login, map[string]string{"email": "policy@example.com", "password": password, "mode": "password"})
	if lockedResponse.Code != 429 || lockedResponse.Header().Get("Retry-After") == "" {
		t.Fatal("missing HTTP lock response", lockedResponse.Code)
	}
	if !strings.Contains(lockedResponse.Body.String(), "后重试") || !strings.Contains(lockedResponse.Body.String(), "分钟") {
		t.Fatal("lock wait time missing", lockedResponse.Body.String())
	}
	for i := 0; i < 16; i++ {
		response := call(s.login, map[string]string{"email": "policy@example.com", "password": password, "mode": "password"})
		if response.Code != 429 || !strings.Contains(response.Body.String(), "后重试") || response.Header().Get("Retry-After") == "" {
			t.Fatal("rate limit hid lock duration")
		}
	}
	// Clear only this fixture's request limits before checking password recovery.
	_, err = s.db.Exec(ctx, "DELETE FROM rate_limits WHERE key=$1", "login-email:policy@example.com")
	if err != nil {
		t.Fatal(err)
	}
	// Recovery uses a seeded challenge, never a real email transport.
	_, err = s.db.Exec(ctx, `INSERT INTO challenges(email,purpose,digest,expires_at) VALUES('policy@example.com','reset',$1,now()+interval '1 minute')`, s.mac("policy@example.com|reset|123456"))
	if err != nil {
		t.Fatal(err)
	}
	resetResponse := call(s.resetPassword, map[string]string{"email": "policy@example.com", "password": strings.Repeat("😀", 64), "code": "123456"})
	if resetResponse.Code != 200 {
		t.Fatal("reset failed", resetResponse.Body.String())
	}
	if ok, until := verify(strings.Repeat("😀", 64)); !ok || !until.IsZero() {
		t.Fatal("reset did not unlock Unicode password")
	}
	loginResponse := call(s.login, map[string]string{"email": "policy@example.com", "password": strings.Repeat("😀", 64), "mode": "password"})
	if loginResponse.Code != 200 {
		t.Fatal("64 Unicode character login failed", loginResponse.Body.String())
	}
	if response := call(s.resetPassword, map[string]string{"email": "policy@example.com", "password": strings.Repeat("😀", 65), "code": "123456"}); response.Code != 400 {
		t.Fatal("oversized reset accepted")
	}
	// Existing users were backfilled; concurrent registrations receive unique IDs.
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("policy-new-%d", i)
			_, err := s.db.Exec(ctx, "INSERT INTO users(id,email,name) VALUES($1,$2,'new')", id, id+"@example.com")
			if err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	rows, err := s.db.Query(ctx, "SELECT account_id FROM users")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	seen := map[string]bool{}
	pattern := regexp.MustCompile(`^[1-9][0-9]{9}$`)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		if !pattern.MatchString(id) || seen[id] {
			t.Fatal("invalid or duplicate account ID", id)
		}
		seen[id] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}
