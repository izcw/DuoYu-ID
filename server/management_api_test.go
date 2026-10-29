package main

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func testManagementAPI(t *testing.T, s *Server, origin string) {
	ctx := context.Background()
	var owner string
	s.db.QueryRow(ctx, "SELECT id FROM users WHERE admin").Scan(&owner)
	session := httptest.NewRecorder()
	if err := s.newSession(session, httptest.NewRequest("GET", origin, nil), owner, "pwd"); err != nil {
		t.Fatal(err)
	}
	cookie := session.Result().Cookies()[0]
	call := func(method, path string, body any, token string, admin bool, headers map[string]string) (int, map[string]any, http.Header) {
		t.Helper()
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest(method, origin+path, bytes.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
		if admin {
			r.AddCookie(cookie)
			r.Header.Set("Origin", origin)
			r.Header.Set("X-Duoyu-Request", "1")
		}
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		for k, v := range headers {
			r.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		s.routes().ServeHTTP(w, r)
		out := map[string]any{}
		json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out, w.Header()
	}
	key := func(scopes []string, apps []string) (string, string) {
		t.Helper()
		status, out, _ := call("POST", "/api/admin/project-api-keys", map[string]any{"name": "API test", "scopes": scopes, "app_ids": apps, "permanent": true}, "", true, nil)
		if status != 200 {
			t.Fatal(status, out)
		}
		d := out["data"].(map[string]any)
		return d["id"].(string), d["secret"].(string)
	}
	keyID, token := key([]string{"projects:read", "projects:create", "projects:update"}, nil)
	_, readOnly := key([]string{"projects:read"}, nil)
	if status, _, _ := call("GET", "/api/management/projects", nil, "", true, nil); status != 403 {
		t.Fatal("browser management call", status)
	}
	if status, _, _ := call("GET", "/api/management/projects", nil, "", false, nil); status != 401 {
		t.Fatal("cookie/token required")
	}
	if status, _, _ := call("GET", "/api/admin/apps", nil, token, false, nil); status != 401 {
		t.Fatal("management credential grants admin session")
	}
	if status, _, _ := call("POST", "/api/management/projects", map[string]string{}, readOnly, false, nil); status != 403 {
		t.Fatal("scope bypass")
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, _ := writer.CreateFormFile("logo", "logo.png")
	png.Encode(part, image.NewNRGBA(image.Rect(0, 0, 64, 64)))
	writer.Close()
	r := httptest.NewRequest("POST", origin+"/api/management/project-logos", &body)
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	s.routes().ServeHTTP(w, r)
	var logoResult map[string]any
	json.Unmarshal(w.Body.Bytes(), &logoResult)
	if w.Code != 200 {
		t.Fatal("logo upload", w.Code, logoResult)
	}
	logo := logoResult["data"].(map[string]any)["logo"].(string)
	payload := map[string]any{"name": "Managed project", "logo": logo, "redirects": []string{"http://localhost:9101/callback"}, "public": false}
	if status, _, _ := call("POST", "/api/management/projects", payload, token, false, nil); status != 400 {
		t.Fatal("idempotency required")
	}
	headers := map[string]string{"Idempotency-Key": "management-create-fixture"}
	status, out, _ := call("POST", "/api/management/projects", payload, token, false, headers)
	if status != 200 {
		t.Fatal("create", status, out)
	}
	data := out["data"].(map[string]any)
	id, secret := data["id"].(string), data["secret"].(string)
	if len(secret) < 32 {
		t.Fatal("missing secret")
	}
	status, out, h := call("POST", "/api/management/projects", payload, token, false, headers)
	if status != 200 || out["data"].(map[string]any)["secret"] != secret || h.Get("Idempotent-Replayed") != "true" {
		t.Fatal("idempotent replay", status, out)
	}
	var cipher []byte
	s.db.QueryRow(ctx, "SELECT result_cipher FROM management_create_requests WHERE key_id=$1", keyID).Scan(&cipher)
	if bytes.Contains(cipher, []byte(secret)) {
		t.Fatal("plaintext secret in replay table")
	}
	payload["name"] = "Changed body"
	if status, _, _ := call("POST", "/api/management/projects", payload, token, false, headers); status != 409 {
		t.Fatal("idempotency conflict")
	}
	payload["name"] = "Managed project"
	// Concurrent retries return the original project and secret, not duplicates.
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			st, result, _ := call("POST", "/api/management/projects", payload, token, false, headers)
			if st != 200 || result["data"].(map[string]any)["id"] != id {
				t.Error("concurrent replay", st)
			}
		}()
	}
	wg.Wait()
	base := "/api/management/projects/" + id
	if status, _, _ := call("GET", base, nil, readOnly, false, nil); status != 403 {
		t.Fatal("cross-key project leak")
	}
	status, out, h = call("GET", base, nil, token, false, nil)
	if status != 200 {
		t.Fatal(status, out)
	}
	cfg := out["data"].(map[string]any)
	if cfg["enabled"] != true || cfg["public"] != false || cfg["revision"].(float64) != 0 || len(cfg["login_methods"].([]any)) != 5 {
		t.Fatal("defaults", cfg)
	}
	if _, yes := cfg["secret"]; yes {
		t.Fatal("secret leak")
	}
	if status, _, _ := call("PATCH", base, map[string]any{"name": "Renamed"}, token, false, nil); status != 428 {
		t.Fatal("If-Match required")
	}
	status, out, _ = call("PATCH", base, map[string]any{"name": "Renamed"}, token, false, map[string]string{"If-Match": h.Get("ETag")})
	if status != 200 {
		t.Fatal("patch", status, out)
	}
	if status, _, _ := call("PATCH", base, map[string]any{"name": "Stale"}, token, false, map[string]string{"If-Match": "\"0\""}); status != 412 {
		t.Fatal("stale overwrite")
	}
	status, out, h = call("GET", base, nil, token, false, nil)
	cfg = out["data"].(map[string]any)
	if cfg["name"] != "Renamed" || cfg["logo"] != logo || len(cfg["redirects"].([]any)) != 1 {
		t.Fatal("patch erased omitted fields")
	}
	if status, _, _ := call("PATCH", base, map[string]any{"public": true}, token, false, map[string]string{"If-Match": h.Get("ETag")}); status != 400 {
		t.Fatal("client type changed")
	}
	if status, _, _ := call("PATCH", base, map[string]any{"secret": "evil"}, token, false, map[string]string{"If-Match": h.Get("ETag")}); status != 400 {
		t.Fatal("readonly field accepted")
	}
	var hash string
	s.db.QueryRow(ctx, "SELECT secret_hash FROM apps WHERE id=$1", id).Scan(&hash)
	if hash != s.mac(secret) {
		t.Fatal("patch changed secret")
	}
	if status, _, _ := call("POST", base+"/rotate-secret", nil, token, false, nil); status != 403 {
		t.Fatal("implicit rotation scope")
	}
	_, rotate := key([]string{"projects:read", "projects:rotate"}, []string{id})
	status, out, _ = call("POST", base+"/rotate-secret", nil, rotate, false, nil)
	if status != 200 || out["data"].(map[string]any)["secret"] == secret {
		t.Fatal("explicit rotation")
	}
	if status, _, _ := call("POST", "/api/management/projects", payload, token, false, headers); status != 409 {
		t.Fatal("rotated secret was replayed")
	}
	if status, _, _ := call("POST", base+"/roles", map[string]any{"name": "编辑", "role_key": "editor", "description": "管理内容"}, token, false, nil); status != 200 {
		t.Fatal("role API", status)
	}
	if status, _, _ := call("PUT", base+"/blacklist", map[string]any{"entries": []string{"blocked@example.com"}}, token, false, nil); status != 200 {
		t.Fatal("blacklist API", status)
	}
	status, out, _ = call("GET", "/api/management/openapi.json", nil, token, false, nil)
	if status != 200 || out["openapi"] != "3.0.3" {
		t.Fatal("OpenAPI", status)
	}
	status, out, _ = call("GET", "/api/management/guide", nil, token, false, nil)
	if status != 200 || !strings.Contains(out["data"].(string), origin) || strings.Contains(out["data"].(string), "{{ORIGIN}}") {
		t.Fatal("guide issuer")
	}
	s.db.Exec(ctx, "UPDATE management_create_requests SET created_at=now()-interval '25 hours' WHERE key_id=$1", keyID)
	if status, _, _ := call("POST", "/api/management/projects", payload, token, false, headers); status != 410 {
		t.Fatal("expired result created another app")
	}
	if status, _, _ := call("POST", "/api/admin/project-api-keys/"+keyID+"/revoke", map[string]any{}, "", true, nil); status != 200 {
		t.Fatal("revoke")
	}
	if status, _, _ := call("GET", base, nil, token, false, nil); status != 401 {
		t.Fatal("revoked key usable")
	}
	s.db.Exec(ctx, "UPDATE management_api_keys SET expires_at=now()-interval '1 second' WHERE digest=$1", s.mac(readOnly))
	if status, _, _ := call("GET", "/api/management/projects", nil, readOnly, false, nil); status != 401 {
		t.Fatal("expired key usable")
	}
}
