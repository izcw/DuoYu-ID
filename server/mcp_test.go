package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"
)

type mcpBearerTransport struct{ token string }

func (t mcpBearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	request := r.Clone(r.Context())
	request.Header.Set("Authorization", "Bearer "+t.token)
	return http.DefaultTransport.RoundTrip(request)
}
func testDeveloperMCP(t *testing.T, s *Server, origin string) {
	ctx := context.Background()
	var owner string
	s.db.QueryRow(ctx, "SELECT id FROM users WHERE admin").Scan(&owner)
	record := httptest.NewRecorder()
	s.newSession(record, httptest.NewRequest("GET", origin, nil), owner, "pwd")
	cookie := record.Result().Cookies()[0]
	app := "mcp-" + randomID()
	other := "mcp-other-" + randomID()
	for _, id := range []string{app, other} {
		if _, err := s.db.Exec(ctx, "INSERT INTO apps(id,name,redirects) VALUES($1,'MCP test',ARRAY['http://localhost/callback'])", id); err != nil {
			t.Fatal(err)
		}
	}
	adminCall := func(method, path string, body any) map[string]any {
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest(method, origin+path, bytes.NewReader(raw))
		r.AddCookie(cookie)
		r.Header.Set("Origin", origin)
		r.Header.Set("X-Duoyu-Request", "1")
		r.Header.Set("X-Duoyu-Legal-Version", legalVersion)
		r.Header.Set("X-Duoyu-Adult", "true")
		w := httptest.NewRecorder()
		s.routes().ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("admin API status %d", w.Code)
		}
		var out map[string]any
		json.Unmarshal(w.Body.Bytes(), &out)
		if out["data"] == nil {
			return map[string]any{}
		}
		return out["data"].(map[string]any)
	}
	create := func(scopes []string) (string, string) {
		out := adminCall("POST", "/api/admin/developer-keys", map[string]any{"name": "MCP test", "scopes": scopes, "app_ids": []string{app}, "permanent": true})
		return out["id"].(string), out["secret"].(string)
	}
	id, token := create([]string{"projects:read", "projects:create", "projects:update", "app:read", "mail:send", "mail:read"})
	rpc := func(token, method string, params any) (int, map[string]any) {
		raw, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
		r := httptest.NewRequest("POST", origin+"/mcp", bytes.NewReader(raw))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Accept", "application/json, text/event-stream")
		r.Header.Set("MCP-Protocol-Version", "2025-06-18")
		w := httptest.NewRecorder()
		s.routes().ServeHTTP(w, r)
		var out map[string]any
		json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "duoyu-sdk-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: origin + "/mcp", HTTPClient: &http.Client{Transport: mcpBearerTransport{token: token}}, DisableStandaloneSSE: true}, nil)
	if err != nil {
		t.Fatal("official SDK initialize failed", err)
	}
	sdkTools, err := session.ListTools(ctx, nil)
	if err != nil || len(sdkTools.Tools) == 0 {
		t.Fatal("official SDK tools/list failed", err)
	}
	sdkCall, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "list_projects", Arguments: map[string]any{}})
	if err != nil || sdkCall.IsError {
		t.Fatal("official SDK tool call failed", err)
	}
	session.Close()
	status, init := rpc(token, "initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": "integration-test", "version": "1"}})
	if status != 200 || init["result"] == nil {
		t.Fatalf("initialize failed %d", status)
	}
	status, list := rpc(token, "tools/list", map[string]any{})
	if status != 200 || list["result"] == nil {
		t.Fatal("tool discovery failed")
	}
	tools := list["result"].(map[string]any)["tools"].([]any)
	names := map[string]bool{}
	for _, v := range tools {
		names[v.(map[string]any)["name"].(string)] = true
	}
	if !names["send_mail"] || names["rotate_project_secret"] {
		t.Fatal("tool scope filtering failed")
	}
	call := func(name string, args map[string]any) map[string]any {
		status, out := rpc(token, "tools/call", map[string]any{"name": name, "arguments": args})
		if status != 200 || out["result"] == nil {
			t.Fatalf("tool call failed %s status %d", name, status)
		}
		return out["result"].(map[string]any)
	}
	result := call("get_project", map[string]any{"project_id": app})
	if result["isError"] == true {
		t.Fatal("read tool failed")
	}
	if call("get_project", map[string]any{"project_id": other})["isError"] != true {
		t.Fatal("cross-project read allowed")
	}
	if call("send_mail", map[string]any{"project_id": other, "idempotency_key": "mcp-mail-other", "body": map[string]any{"to": "mcp@example.invalid", "subject": "test", "text": "test"}})["isError"] != true {
		t.Fatal("cross-project mail allowed")
	}
	sent := call("send_mail", map[string]any{"project_id": app, "idempotency_key": "mcp-mail-send", "body": map[string]any{"to": "mcp@example.invalid", "subject": "test", "text": "test"}})
	if sent["isError"] == true {
		t.Fatal("MCP mail enqueue failed")
	}
	var imageBytes bytes.Buffer
	png.Encode(&imageBytes, image.NewRGBA(image.Rect(0, 0, 32, 32)))
	logo := call("upload_project_logo", map[string]any{"file_base64": base64.StdEncoding.EncodeToString(imageBytes.Bytes()), "filename": "logo.png"})
	if logo["isError"] == true {
		t.Fatal("MCP logo upload failed")
	}
	body := logo["structuredContent"].(map[string]any)["response"].(map[string]any)["data"].(map[string]any)
	created := call("create_project", map[string]any{"idempotency_key": "mcp-project-create", "body": map[string]any{"name": "MCP created", "logo": body["logo"], "public": true, "platform": "spa", "redirects": []string{"http://localhost/callback"}}})
	if created["isError"] == true {
		t.Fatal("MCP project create failed")
	}
	createdID := created["structuredContent"].(map[string]any)["response"].(map[string]any)["data"].(map[string]any)["id"].(string)
	detail := call("get_project", map[string]any{"project_id": createdID})
	etag := detail["structuredContent"].(map[string]any)["etag"].(string)
	if call("update_project", map[string]any{"project_id": createdID, "if_match": etag, "body": map[string]any{"description": "MCP updated"}})["isError"] == true {
		t.Fatal("MCP update failed")
	}
	if call("update_project", map[string]any{"project_id": createdID, "if_match": etag, "body": map[string]any{"description": "stale"}})["isError"] != true {
		t.Fatal("MCP stale ETag accepted")
	}
	for _, name := range []string{"check_project", "list_project_roles", "get_project_grants", "get_project_blacklist", "get_project_service"} {
		if call(name, map[string]any{"project_id": createdID})["isError"] == true {
			t.Fatalf("MCP %s failed", name)
		}
	}
	roleBody := map[string]any{"name": "Tester", "role_key": "tester", "description": "MCP role"}
	role := call("save_project_role", map[string]any{"project_id": createdID, "body": roleBody})
	if role["isError"] == true {
		t.Fatal("MCP role create failed")
	}
	roleID := role["structuredContent"].(map[string]any)["response"].(map[string]any)["data"].(map[string]any)["id"].(string)
	if call("update_project_role", map[string]any{"project_id": createdID, "body": roleBody})["isError"] != true {
		t.Fatal("MCP missing role ID accepted")
	}
	roleBody["name"] = "Updated tester"
	if call("update_project_role", map[string]any{"project_id": createdID, "role_id": roleID, "body": roleBody})["isError"] == true {
		t.Fatal("MCP role update failed")
	}
	var replacement string
	s.db.QueryRow(ctx, "SELECT id FROM app_roles WHERE app_id=$1 AND is_default", createdID).Scan(&replacement)
	if call("delete_project_role", map[string]any{"project_id": createdID, "role_id": roleID, "body": map[string]any{"replacement_id": replacement}})["isError"] == true {
		t.Fatal("MCP role deletion failed")
	}
	if call("set_project_grants", map[string]any{"project_id": createdID, "body": map[string]any{"emails": []string{}}})["isError"] == true {
		t.Fatal("MCP grant update failed")
	}
	if call("set_project_blacklist", map[string]any{"project_id": createdID, "body": map[string]any{"entries": []string{}}})["isError"] == true {
		t.Fatal("MCP blacklist update failed")
	}
	mailID := sent["structuredContent"].(map[string]any)["response"].(map[string]any)["data"].(map[string]any)["id"].(string)
	if call("get_mail_status", map[string]any{"project_id": app, "mail_id": mailID})["isError"] == true {
		t.Fatal("MCP mail query failed")
	}
	_, readToken := create([]string{"projects:read"})
	_, readList := rpc(readToken, "tools/list", map[string]any{})
	for _, tool := range readList["result"].(map[string]any)["tools"].([]any) {
		if tool.(map[string]any)["name"] == "send_mail" {
			t.Fatal("mail exposed to read key")
		}
	}
	status, _ = rpc("invalid", "tools/list", map[string]any{})
	if status != 401 {
		t.Fatal("invalid key accepted")
	}
	adminCall("PUT", "/api/admin/developer-keys/"+id, map[string]any{"name": "updated", "scopes": []string{"projects:read"}, "app_ids": []string{app}, "permanent": true})
	_, after := rpc(token, "tools/list", map[string]any{})
	for _, tool := range after["result"].(map[string]any)["tools"].([]any) {
		if tool.(map[string]any)["name"] == "send_mail" {
			t.Fatal("scope removal not immediate")
		}
	}
	rotated := adminCall("POST", "/api/admin/developer-keys/"+id+"/rotate", map[string]any{"overlap_hours": 0})
	status, _ = rpc(token, "tools/list", map[string]any{})
	if status != 401 {
		t.Fatal("old key survives rotation")
	}
	token = rotated["secret"].(string)
	adminCall("POST", "/api/admin/developer-keys/"+id+"/revoke", map[string]any{})
	status, _ = rpc(token, "tools/list", map[string]any{})
	if status != 401 {
		t.Fatal("revoked key accepted")
	}
	_ = http.MethodPost
}

func testDeveloperMigration(t *testing.T, s *Server) {
	ctx := context.Background()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	schema := "migration_developer_" + digest(randomID())[:12]
	if _, err = tx.Exec(ctx, "CREATE SCHEMA "+schema+";SET LOCAL search_path TO "+schema); err != nil {
		t.Fatal(err)
	}
	baseline, _ := assets.ReadFile("schema.sql")
	if _, err = tx.Exec(ctx, string(baseline)); err != nil {
		t.Fatal(err)
	}
	entries, _ := assets.ReadDir("migrations")
	for _, entry := range entries {
		if entry.Name() >= "021_developer_keys.sql" {
			continue
		}
		body, _ := assets.ReadFile("migrations/" + entry.Name())
		if _, err = tx.Exec(ctx, string(body)); err != nil {
			t.Fatalf("old migration %s: %v", entry.Name(), err)
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO users(id,email,name,admin) VALUES('migration-owner','migration@example.invalid','owner',true);
 INSERT INTO apps(id,name,redirects) VALUES('migration-project','project',ARRAY['https://example.invalid/callback']);
 INSERT INTO api_keys(id,name,digest,app_id,scopes) VALUES('migration-service','service','service-hash','migration-project',ARRAY['mail:read']);
 INSERT INTO management_api_keys(id,name,digest,owner_id,scopes) VALUES('migration-manager','manager','manager-hash','migration-owner',ARRAY['projects:read']);
 INSERT INTO management_api_grants(key_id,app_id) VALUES('migration-manager','migration-project');
 INSERT INTO management_create_requests(key_id,request_key,body_digest,app_id) VALUES('migration-manager','request','body','migration-project');`)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := assets.ReadFile("migrations/021_developer_keys.sql")
	if _, err = tx.Exec(ctx, string(body)); err != nil {
		t.Fatal(err)
	}
	var count int
	tx.QueryRow(ctx, "SELECT count(*) FROM api_keys WHERE owner_id='migration-owner'").Scan(&count)
	if count != 2 {
		t.Fatal("migration lost credentials")
	}
	tx.QueryRow(ctx, "SELECT count(*) FROM management_api_grants").Scan(&count)
	if count != 2 {
		t.Fatal("migration lost grants")
	}
	if _, err = tx.Exec(ctx, "DELETE FROM management_create_requests;DELETE FROM api_keys WHERE id='migration-manager'"); err != nil {
		t.Fatal("foreign key migration failed")
	}
}
