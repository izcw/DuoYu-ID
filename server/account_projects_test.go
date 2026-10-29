package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testAccountProjects(t *testing.T, s *Server, origin string) {
	ctx := context.Background()
	uid := randomID()
	var admin string
	s.db.QueryRow(ctx, "SELECT id FROM users WHERE admin").Scan(&admin)
	if _, err := s.db.Exec(ctx, "INSERT INTO users(id,email,name) VALUES($1,'account-projects@example.com','Original')", uid); err != nil {
		t.Fatal(err)
	}
	session := func(id string) *http.Cookie {
		w := httptest.NewRecorder()
		if err := s.newSession(w, httptest.NewRequest("GET", origin, nil), id, "pwd"); err != nil {
			t.Fatal(err)
		}
		return w.Result().Cookies()[0]
	}
	adminCookie, userCookie := session(admin), session(uid)
	call := func(method, path string, body any, cookie *http.Cookie) (int, map[string]any) {
		t.Helper()
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest(method, origin+path, bytes.NewReader(raw))
		r.Header.Set("Origin", origin)
		r.Header.Set("X-Duoyu-Request", "1")
		r.Header.Set("X-Duoyu-Legal-Version", legalVersion)
		r.Header.Set("X-Duoyu-Adult", "true")
		r.AddCookie(cookie)
		w := httptest.NewRecorder()
		s.routes().ServeHTTP(w, r)
		out := map[string]any{}
		json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}
	status, out := call("GET", "/api/admin/users/"+uid, nil, adminCookie)
	if status != 200 {
		t.Fatal(status, out)
	}
	if len(out["data"].([]any)[0].(map[string]any)["projects"].([]any)) != 0 {
		t.Fatal("unexpected projects")
	}
	s.db.Exec(ctx, "INSERT INTO apps(id,name,redirects,public) VALUES('account-project-a','Account A',ARRAY['http://localhost/cb'],true),('account-project-b','Account B',ARRAY['http://localhost/cb'],true)")
	if _, err := s.db.Exec(ctx, "INSERT INTO app_users(app_id,user_id,role_id) VALUES('account-project-a',$1,'admin'),('account-project-b',$1,'user')", uid); err != nil {
		t.Fatal(err)
	}
	status, out = call("GET", "/api/admin/users/"+uid, nil, adminCookie)
	if status != 200 {
		t.Fatal(status, out)
	}
	projects := out["data"].([]any)[0].(map[string]any)["projects"].([]any)
	if len(projects) != 2 || projects[0].(map[string]any)["role_key"] != "admin" {
		t.Fatal("project membership", projects)
	}
	status, out = call("GET", "/api/admin/users?page=1&page_size=20&q=Account%20A", nil, adminCookie)
	if status != 200 || out["data"].(map[string]any)["total"].(float64) != 1 {
		t.Fatal("project search", status, out)
	}
	if status, _ = call("PATCH", "/api/admin/users/"+uid, map[string]any{"name": "Admin changed"}, adminCookie); status != 403 {
		t.Fatal("admin renamed user", status)
	}
	if status, _ = call("PATCH", "/api/admin/users/"+uid, map[string]any{"name": "Admin changed", "enabled": false}, adminCookie); status != 403 {
		t.Fatal("mixed payload bypass", status)
	}
	if status, _ = call("PATCH", "/api/me", map[string]string{"name": "本人修改"}, userCookie); status != 200 {
		t.Fatal("self profile", status)
	}
	var name string
	s.db.QueryRow(ctx, "SELECT name FROM users WHERE id=$1", uid).Scan(&name)
	if name != "本人修改" {
		t.Fatal(name)
	}
	// Upload once, then simulate the old host/port persisted by earlier versions.
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("avatar", "avatar.png")
	if err != nil {
		t.Fatal(err)
	}
	png.Encode(part, image.NewRGBA(image.Rect(0, 0, 32, 32)))
	writer.Close()
	r := httptest.NewRequest("POST", origin+"/api/me/avatar", &body)
	r.AddCookie(userCookie)
	r.Header.Set("Origin", origin)
	r.Header.Set("X-Duoyu-Request", "1")
	r.Header.Set("X-Duoyu-Legal-Version", legalVersion)
	r.Header.Set("X-Duoyu-Adult", "true")
	r.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	s.routes().ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal("upload", w.Code, w.Body.String())
	}
	json.Unmarshal(w.Body.Bytes(), &out)
	path := out["data"].(map[string]any)["avatar"].(string)
	if !strings.HasPrefix(path, "/avatars/") {
		t.Fatal("avatar binds hostname", path)
	}
	s.db.Exec(ctx, "UPDATE users SET avatar=$1 WHERE id=$2", "http://localhost:8080"+path, uid)
	migration, err := assets.ReadFile("migrations/019_avatar_paths.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(ctx, string(migration)); err != nil {
		t.Fatal(err)
	}
	var stored string
	s.db.QueryRow(ctx, "SELECT avatar FROM users WHERE id=$1", uid).Scan(&stored)
	if stored != path {
		t.Fatal("legacy path not repaired", stored)
	}
	w = httptest.NewRecorder()
	s.routes().ServeHTTP(w, httptest.NewRequest("GET", origin+path, nil))
	if w.Code != 200 {
		t.Fatal("avatar unavailable", w.Code)
	}
	info := &oidc.UserInfo{}
	if err = s.fillInfo(ctx, info, uid, []string{"profile"}); err != nil || info.Picture != origin+path {
		t.Fatal("OIDC picture is not absolute", info.Picture, err)
	}
}
