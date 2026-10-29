package main

import (
	"context"
	"encoding/json"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"net/http/httptest"
	"strings"
	"testing"
)

func testProjectRoles(t *testing.T, s *Server, origin string) {
	ctx := context.Background()
	app := "roles-test"
	other := "roles-other"
	if _, err := s.db.Exec(ctx, "INSERT INTO apps(id,name,redirects,public) VALUES($1,'Roles',ARRAY['http://localhost/callback'],true),($2,'Other',ARRAY['http://localhost/callback'],true)", app, other); err != nil {
		t.Fatal(err)
	}
	var admin string
	s.db.QueryRow(ctx, "SELECT id FROM users WHERE admin").Scan(&admin)
	rec := httptest.NewRecorder()
	if err := s.newSession(rec, httptest.NewRequest("GET", origin, nil), admin, "pwd"); err != nil {
		t.Fatal(err)
	}
	cookie := rec.Result().Cookies()[0]
	call := func(method, path string, body any, auth bool) (int, map[string]any) {
		t.Helper()
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest(method, origin+path, strings.NewReader(string(raw)))
		r.Header.Set("Origin", origin)
		r.Header.Set("X-Duoyu-Request", "1")
		if auth {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		s.routes().ServeHTTP(w, r)
		out := map[string]any{}
		json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}
	base := "/api/admin/apps/" + app + "/roles"
	if status, _ := call("GET", base, nil, false); status != 401 {
		t.Fatal("unauthorized", status)
	}
	status, out := call("GET", base, nil, true)
	if status != 200 || len(out["data"].([]any)) != 2 {
		t.Fatal("defaults", status, out)
	}
	if _, err := s.db.Exec(ctx, "INSERT INTO app_users(app_id,user_id) VALUES($1,$2)", app, admin); err != nil {
		t.Fatal(err)
	}
	var role string
	s.db.QueryRow(ctx, "SELECT role_id FROM app_users WHERE app_id=$1 AND user_id=$2", app, admin).Scan(&role)
	if role != "user" {
		t.Fatal("center admin granted project admin")
	}
	create := map[string]any{"name": "编辑", "role_key": "editor", "description": "内容维护", "is_default": true}
	status, out = call("POST", base, create, true)
	if status != 200 {
		t.Fatal(status, out)
	}
	editor := out["data"].(map[string]any)["id"].(string)
	if status, _ = call("POST", base, create, true); status != 409 {
		t.Fatal("duplicate", status)
	}
	create["role_key"] = "INVALID FIELD"
	if status, _ = call("POST", base, create, true); status != 400 {
		t.Fatal("invalid field", status)
	}
	s.db.QueryRow(ctx, "SELECT role_id FROM app_users WHERE app_id=$1 AND user_id=$2", app, admin).Scan(&role)
	if role != "user" {
		t.Fatal("existing role changed")
	}
	uid := randomID()
	s.db.Exec(ctx, "INSERT INTO users(id,email,name) VALUES($1,'project-roles@example.com','Roles')", uid)
	if _, err := s.db.Exec(ctx, "INSERT INTO app_users(app_id,user_id) VALUES($1,$2)", app, uid); err != nil {
		t.Fatal(err)
	}
	s.db.QueryRow(ctx, "SELECT role_id FROM app_users WHERE app_id=$1 AND user_id=$2", app, uid).Scan(&role)
	if role != editor {
		t.Fatal("default assignment")
	}
	info := &oidc.UserInfo{}
	if err := s.fillProjectRole(ctx, info, uid, app, []string{"profile"}); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(info)
	var claims map[string]any
	json.Unmarshal(raw, &claims)
	if claims["roles"].([]any)[0] != "editor" {
		t.Fatal("role claims", claims)
	}
	status, out = call("PUT", base+"/"+editor, map[string]any{"name": "编辑员", "description": "内容维护", "role_key": "content_editor", "is_default": true}, true)
	if status != 200 {
		t.Fatal(status, out)
	}
	info = &oidc.UserInfo{}
	s.fillProjectRole(ctx, info, uid, app, []string{"openid"})
	raw, _ = json.Marshal(info)
	if strings.Contains(string(raw), "project_role") {
		t.Fatal("scope leak")
	}
	if status, _ = call("PUT", base+"/"+editor, map[string]any{"name": "编辑员", "description": "内容维护", "role_key": "content_editor", "is_default": false}, true); status != 400 {
		t.Fatal("missing default", status)
	}
	userPath := "/api/admin/apps/" + app + "/users/" + uid + "/role"
	if status, _ = call("PUT", userPath, map[string]string{"role_id": "missing"}, true); status != 400 {
		t.Fatal("missing role")
	}
	if status, _ = call("PUT", userPath, map[string]string{"role_id": "admin"}, true); status != 200 {
		t.Fatal("assign role", status)
	}
	if status, _ = call("DELETE", base+"/"+editor, map[string]string{"replacement_id": ""}, true); status != 400 {
		t.Fatal("delete default without replacement")
	}
	if status, _ = call("DELETE", base+"/"+editor, map[string]string{"replacement_id": "admin"}, true); status != 200 {
		t.Fatal("delete default", status)
	}
	var def string
	s.db.QueryRow(ctx, "SELECT id FROM app_roles WHERE app_id=$1 AND is_default", app).Scan(&def)
	if def != "admin" {
		t.Fatal("replacement default")
	}
	if status, _ = call("DELETE", base+"/user", map[string]string{"replacement_id": "admin"}, true); status != 200 {
		t.Fatal("delete built-in user", status)
	}
	s.db.QueryRow(ctx, "SELECT role_id FROM app_users WHERE app_id=$1 AND user_id=$2", app, admin).Scan(&role)
	if role != "admin" {
		t.Fatal("member migration")
	}
	if status, _ = call("DELETE", base+"/admin", map[string]string{"replacement_id": "admin"}, true); status != 400 {
		t.Fatal("last role deletion")
	}
	var count int
	s.db.QueryRow(ctx, "SELECT count(*) FROM app_roles WHERE app_id=$1", other).Scan(&count)
	if count != 2 {
		t.Fatal("cross project modification")
	}

	ordinary := httptest.NewRecorder()
	if err := s.newSession(ordinary, httptest.NewRequest("GET", origin, nil), uid, "pwd"); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", origin+base, nil)
	req.AddCookie(ordinary.Result().Cookies()[0])
	response := httptest.NewRecorder()
	s.routes().ServeHTTP(response, req)
	if response.Code != 403 {
		t.Fatal("project admin entered center role API", response.Code)
	}
	// Role membership is never a grant of center administrator privileges.
	var elevated bool
	s.db.QueryRow(ctx, "SELECT admin FROM users WHERE id=$1", uid).Scan(&elevated)
	if elevated {
		t.Fatal("center escalation")
	}
}
