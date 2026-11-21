package main

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func testMemberStatus(t *testing.T, s *Server) {
	ctx := context.Background()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, e := s.db.Exec(ctx, sql, args...); e != nil {
			t.Fatal(e)
		}
	}
	exec("INSERT INTO apps(id,name,redirects,secret_hash) VALUES('member-app','QA','{}',$1)", s.mac("test-secret"))
	exec("INSERT INTO users(id,email,name) VALUES('member-qa','member-qa@example.invalid','QA')")
	exec("INSERT INTO app_users(app_id,user_id) VALUES('member-app','member-qa')")
	exec("INSERT INTO sessions(hash,user_id,expires_at) VALUES('qa-device','member-qa',now()+interval '1 hour')")
	owner := digest("[\"" + s.origin + "\",\"member-qa\"]")
	check := func(sid string, expected bool, generation int) {
		t.Helper()
		body, _ := json.Marshal(map[string]string{"owner": owner, "sid": sid})
		r := httptest.NewRequest("POST", "/api/service/member-status", strings.NewReader(string(body)))
		r.SetBasicAuth("member-app", "test-secret")
		w := httptest.NewRecorder()
		s.routes().ServeHTTP(w, r)
		var out struct {
			Data struct {
				Active     bool
				Generation int
			}
		}
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil || out.Data.Active != expected || out.Data.Generation != generation {
			t.Fatalf("status %d response %s", w.Code, w.Body.String())
		}
	}
	check("", true, 0)
	check(digest("logout:qa-device"), true, 0)
	exec("DELETE FROM sessions WHERE hash='qa-device'")
	check(digest("logout:qa-device"), false, 0)
	check("", true, 0)
	exec("UPDATE users SET enabled=false WHERE id='member-qa'")
	check("", false, 1)
	exec("UPDATE users SET enabled=true WHERE id='member-qa'")
	check("", true, 1)
	exec("INSERT INTO grants VALUES('member-app','member-qa')")
	exec("UPDATE apps SET restricted=true WHERE id='member-app'")
	check("", true, 1)
	exec("DELETE FROM grants WHERE app_id='member-app'")
	check("", false, 2)
	exec("INSERT INTO grants VALUES('member-app','member-qa')")
	check("", true, 2)
	exec("INSERT INTO app_blacklist(app_id,identifier,user_id) VALUES('member-app','member-qa','member-qa')")
	check("", false, 3)
	exec("DELETE FROM app_blacklist WHERE app_id='member-app'")
	check("", true, 3)
	exec("UPDATE apps SET enabled=false WHERE id='member-app'")
	check("", false, 0)
	exec("UPDATE apps SET enabled=true WHERE id='member-app'")
	check("", true, 4)
	exec("DELETE FROM app_users WHERE app_id='member-app'")
	check("", false, 0)
	exec("DELETE FROM apps WHERE id='member-app'")
	exec("DELETE FROM users WHERE id='member-qa'")
}
