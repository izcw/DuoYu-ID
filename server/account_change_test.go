package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func testAccountChangeChallenges(t *testing.T, s *Server, origin string) {
	ctx := context.Background()
	router := s.routes()
	t.Run("email moved during code login", func(t *testing.T) {
		email := "moving@example.invalid"
		if _, err := s.db.Exec(ctx, "INSERT INTO users(id,email,name) VALUES('moving-user',$1,'test')", email); err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.Exec(ctx, "INSERT INTO challenges(email,purpose,digest,expires_at) VALUES($1,'login',$2,now()+interval '5 minutes')", email, s.mac(email+"|login|123456")); err != nil {
			t.Fatal(err)
		}
		tx, err := s.db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		var locker int
		if err := tx.QueryRow(ctx, "SELECT pg_backend_pid() FROM challenges WHERE email=$1 FOR UPDATE", email).Scan(&locker); err != nil {
			t.Fatal(err)
		}
		done := make(chan int, 1)
		go func() {
			r := httptest.NewRequest("POST", origin+"/api/auth/login", strings.NewReader(`{"mode":"code","email":"moving@example.invalid","code":"123456"}`))
			r.Header.Set("Origin", origin)
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("X-Duoyu-Request", "1")
			r.Header.Set("X-Duoyu-Legal-Version", legalVersion)
			r.Header.Set("X-Duoyu-Adult", "true")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			done <- w.Code
		}()
		waitForDatabaseWaiters(t, s, locker, 1)
		if _, err := tx.Exec(ctx, "UPDATE users SET email='moved@example.invalid' WHERE id='moving-user'"); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if status := <-done; status != 401 {
			t.Errorf("login after ownership change: %d", status)
		}
		var count int
		if err := s.db.QueryRow(ctx, "SELECT count(*) FROM sessions WHERE user_id='moving-user'").Scan(&count); err != nil || count != 0 {
			t.Fatal("session for previous owner", count, err)
		}
	})
	for _, endpoint := range []string{"/api/me/email", "/api/me/password", "/api/auth/reset"} {
		t.Run(endpoint, func(t *testing.T) {
			uid := randomID()
			old := strings.ToLower(uid) + "@example.invalid"
			newEmail := "new-" + old
			if _, err := s.db.Exec(ctx, "INSERT INTO users(id,email,name) VALUES($1,$2,'test')", uid, old); err != nil {
				t.Fatal(err)
			}
			session := httptest.NewRecorder()
			if err := s.newSession(session, httptest.NewRequest("GET", origin, nil), uid, "pwd"); err != nil {
				t.Fatal(err)
			}
			cookie := session.Result().Cookies()[0]
			purpose := "reset"
			body := map[string]string{"code": "123456", "password": "new-password"}
			emails := []string{old}
			if endpoint == "/api/me/email" {
				purpose = "email"
				emails = append(emails, newEmail)
				body = map[string]string{"email": newEmail, "old_code": "123456", "new_code": "654321"}
			} else if endpoint == "/api/auth/reset" {
				body["email"] = old
			}
			for _, email := range emails {
				if _, err := s.db.Exec(ctx, "INSERT INTO challenges(email,purpose,digest,expires_at) VALUES($1,$2,$3,now()+interval '5 minutes')", email, purpose, s.mac(email+"|"+purpose+"|123456")); err != nil {
					t.Fatal(err)
				}
			}
			call := func() int {
				b, _ := json.Marshal(body)
				r := httptest.NewRequest("POST", origin+endpoint, bytes.NewReader(b))
				r.Header.Set("Origin", origin)
				r.Header.Set("Content-Type", "application/json")
				r.Header.Set("X-Duoyu-Request", "1")
				r.Header.Set("X-Duoyu-Legal-Version", legalVersion)
				r.Header.Set("X-Duoyu-Adult", "true")
				r.AddCookie(cookie)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, r)
				return w.Code
			}
			if purpose == "email" {
				if status := call(); status != 400 {
					t.Fatalf("wrong second code: %d", status)
				}
				var attempts int
				if err := s.db.QueryRow(ctx, "SELECT attempts FROM challenges WHERE email=$1 AND purpose='email'", newEmail).Scan(&attempts); err != nil || attempts != 1 {
					t.Fatal("wrong code not counted", attempts, err)
				}
				body["new_code"] = "123456"
			}
			if _, err := s.db.Exec(ctx, `CREATE FUNCTION fail_change_revoke() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION 'test rollback'; END$$;
				CREATE TRIGGER fail_change_revoke BEFORE DELETE ON sessions FOR EACH ROW EXECUTE FUNCTION fail_change_revoke()`); err != nil {
				t.Fatal(err)
			}
			status := call()
			if _, err := s.db.Exec(ctx, "DROP TRIGGER fail_change_revoke ON sessions; DROP FUNCTION fail_change_revoke()"); err != nil {
				t.Fatal(err)
			}
			if status < 500 {
				t.Errorf("injected revoke failure: %d", status)
			}
			var count int
			if err := s.db.QueryRow(ctx, "SELECT count(*) FROM challenges WHERE email=ANY($1) AND purpose=$2", emails, purpose).Scan(&count); err != nil || count != len(emails) {
				t.Errorf("valid codes lost after rollback: %d %v", count, err)
			}
			if status := call(); status != 200 {
				t.Fatalf("retry same codes: %d", status)
			}
			if err := s.db.QueryRow(ctx, "SELECT count(*) FROM challenges WHERE email=ANY($1) AND purpose=$2", emails, purpose).Scan(&count); err != nil || count != 0 {
				t.Fatal("codes not consumed on success", count, err)
			}
			if err := s.db.QueryRow(ctx, "SELECT count(*) FROM sessions WHERE user_id=$1", uid).Scan(&count); err != nil || count != 0 {
				t.Fatal("sessions survived change", count, err)
			}
		})
	}
}
