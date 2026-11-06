package main

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zitadel/oidc/v3/pkg/oidc"
)

func testProjectPolicyReplacement(t *testing.T, s *Server, origin string) {
	ctx := context.Background()
	if _, err := s.db.Exec(ctx, `INSERT INTO apps(id,name,redirects) VALUES('policy-app','Policy','{}');
		INSERT INTO users(id,email,name) VALUES('policy-a','policy-a@example.invalid','A'),('policy-b','policy-b@example.invalid','B')`); err != nil {
		t.Fatal(err)
	}
	var owner string
	if err := s.db.QueryRow(ctx, "SELECT id FROM users WHERE admin").Scan(&owner); err != nil {
		t.Fatal(err)
	}
	session := httptest.NewRecorder()
	if err := s.newSession(session, httptest.NewRequest("GET", origin, nil), owner, "pwd"); err != nil {
		t.Fatal(err)
	}
	cookie := session.Result().Cookies()[0]
	router := s.routes()
	put := func(project, list, body string) int {
		r := httptest.NewRequest("PUT", origin+"/api/admin/apps/"+project+"/"+list, strings.NewReader(body))
		r.AddCookie(cookie)
		r.Header.Set("Origin", origin)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Duoyu-Request", "1")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w.Code
	}
	for _, tc := range []struct{ list, field, table string }{{"grants", "emails", "grants"}, {"blacklist", "entries", "app_blacklist"}} {
		t.Run(tc.list, func(t *testing.T) {
			for _, body := range []string{"null", "{}", `{"` + tc.field + `":null}`} {
				if status := put("policy-app", tc.list, `{"`+tc.field+`":["policy-a@example.invalid"]}`); status != 200 {
					t.Fatal("seed", status)
				}
				if status := put("policy-app", tc.list, body); status != 400 {
					t.Errorf("invalid payload %s: got %d", body, status)
				}
				var count int
				if err := s.db.QueryRow(ctx, "SELECT count(*) FROM "+tc.table+" WHERE app_id='policy-app' AND user_id='policy-a'").Scan(&count); err != nil || count != 1 {
					t.Errorf("existing policy lost: rows=%d err=%v", count, err)
				}
			}
			if tc.list == "grants" {
				if status := put("policy-app", tc.list, `{"emails":[" Policy-A@Example.Invalid ", "policy-a@example.invalid"]}`); status != 200 {
					t.Fatal("normalized duplicate", status)
				}
				if status := put("policy-app", tc.list, `{"emails":["policy-b@example.invalid","absent@example.invalid"]}`); status != 400 {
					t.Fatal("unregistered account", status)
				}
				var kept bool
				if err := s.db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM grants WHERE app_id='policy-app' AND user_id='policy-a')").Scan(&kept); err != nil || !kept {
					t.Fatal("invalid edit lost original grant", err)
				}
			}
			if status := put("policy-app", tc.list, `{"`+tc.field+`":[]}`); status != 200 {
				t.Fatal("explicit clear", status)
			}
			var remaining int
			if err := s.db.QueryRow(ctx, "SELECT count(*) FROM "+tc.table+" WHERE app_id='policy-app'").Scan(&remaining); err != nil || remaining != 0 {
				t.Fatal("clear did not apply", remaining, err)
			}
			if status := put("missing-project", tc.list, `{"`+tc.field+`":[]}`); status != 404 {
				t.Errorf("missing project: %d", status)
			}
		})
	}
	t.Run("concurrent whole-list replacements", func(t *testing.T) {
		// Gate both real requests on the same project. Release only when both
		// have reached a database lock, independent of machine speed.
		tx, err := s.db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		var locker int
		if err := tx.QueryRow(ctx, "SELECT pg_backend_pid() FROM apps WHERE id='policy-app' FOR UPDATE").Scan(&locker); err != nil {
			t.Fatal(err)
		}
		results := make(chan int, 2)
		for _, email := range []string{"policy-a@example.invalid", "policy-b@example.invalid"} {
			go func(email string) { results <- put("policy-app", "grants", `{"emails":["`+email+`"]}`) }(email)
		}
		waitForDatabaseWaiters(t, s, locker, 2)
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		for range 2 {
			if status := <-results; status != 200 {
				t.Fatal("replacement", status)
			}
		}
		var count int
		if err := s.db.QueryRow(ctx, "SELECT count(*) FROM grants WHERE app_id='policy-app'").Scan(&count); err != nil || count != 1 {
			t.Fatalf("two replacements must leave exactly one user's grant; rows=%d err=%v", count, err)
		}
	})
	t.Run("blacklist during token issuance", func(t *testing.T) {
		if _, err := s.db.Exec(ctx, `UPDATE apps SET refresh_enabled=true WHERE id='policy-app';
			INSERT INTO sessions(hash,user_id,expires_at) VALUES('policy-session','policy-a',now()+interval '1 hour')`); err != nil {
			t.Fatal(err)
		}
		request := &AuthRequest{ID: "policy-auth", Completed: true, Request: oidc.AuthRequest{ClientID: "policy-app", Scopes: []string{"openid", "offline_access"}}, UserID: "policy-a", SessionHash: "policy-session", AMR: "pwd"}
		body, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.Exec(ctx, "INSERT INTO auth_requests(id,body,expires_at) VALUES($1,$2,now()+interval '10 minutes')", request.ID, body); err != nil {
			t.Fatal(err)
		}
		tx, err := s.db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		var locker int
		if err := tx.QueryRow(ctx, "SELECT pg_backend_pid() FROM apps WHERE id='policy-app' FOR UPDATE").Scan(&locker); err != nil {
			t.Fatal(err)
		}
		errs := make(chan error, 2)
		go func() { _, _, err := s.CreateAccessToken(ctx, request); errs <- err }()
		go func() { _, _, _, err := s.CreateAccessAndRefreshTokens(ctx, request, ""); errs <- err }()
		waitForDatabaseWaiters(t, s, locker, 2)
		if _, err := tx.Exec(ctx, `INSERT INTO app_blacklist(app_id,identifier,user_id) VALUES('policy-app','policy-a','policy-a')`); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		for range 2 {
			if err := <-errs; err == nil {
				t.Error("issued credentials after blacklist commit")
			}
		}
		if _, err := s.db.Exec(ctx, "DELETE FROM app_blacklist WHERE app_id='policy-app'"); err != nil {
			t.Fatal(err)
		}
		var count int
		if err := s.db.QueryRow(ctx, `SELECT (SELECT count(*) FROM access_tokens WHERE app_id='policy-app') + (SELECT count(*) FROM refresh_families WHERE app_id='policy-app' AND NOT revoked)`).Scan(&count); err != nil || count != 0 {
			t.Fatalf("unblocking left racing credentials active: %d, %v", count, err)
		}
	})
}

func waitForDatabaseWaiters(t *testing.T, s *Server, locker, minimum int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		var waiting int
		err := s.db.QueryRow(context.Background(), `WITH RECURSIVE waiters(pid) AS (
				SELECT $1::int UNION SELECT a.pid FROM pg_stat_activity a JOIN waiters w ON w.pid=ANY(pg_blocking_pids(a.pid))
			) SELECT count(*)-1 FROM waiters`, locker).Scan(&waiting)
		if err != nil {
			t.Fatal(err)
		}
		if waiting >= minimum {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("requests did not reach database lock")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
