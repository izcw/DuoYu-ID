package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
)

type developerTool struct {
	name, description, method, path, scope string
	project                                bool
	handler                                http.HandlerFunc
}

func (s *Server) developerTools() []developerTool {
	return []developerTool{
		{"list_projects", "查询凭证可访问的项目", "GET", "/api/management/projects", "projects:read", false, s.managementProjects},
		{"get_project", "读取项目，返回 ETag 供修改使用", "GET", "/api/management/projects/{id}", "projects:read", true, s.managementProject},
		{"create_project", "创建项目。body 为项目配置，必须提供 idempotency_key；Logo 必须先上传", "POST", "/api/management/projects", "projects:create", false, s.managementSaveProject},
		{"update_project", "局部修改项目。必须提供 get_project 返回的 if_match", "PATCH", "/api/management/projects/{id}", "projects:update", true, s.managementSaveProject},
		{"check_project", "检查项目回调和接入配置", "GET", "/api/management/projects/{id}/checks", "projects:read", true, s.appChecks},
		{"list_project_roles", "读取项目角色", "GET", "/api/management/projects/{id}/roles", "projects:read", true, s.listAppRoles},
		{"save_project_role", "添加项目角色，body 与 HTTP API 相同", "POST", "/api/management/projects/{id}/roles", "projects:update", true, s.saveAppRole},
		{"update_project_role", "修改角色，role_id 指定角色", "PUT", "/api/management/projects/{id}/roles/{role}", "projects:update", true, s.saveAppRole},
		{"delete_project_role", "删除角色，role_id 指定角色，body.replacement_id 必须指定同项目替代角色", "DELETE", "/api/management/projects/{id}/roles/{role}", "projects:update", true, s.deleteAppRole},
		{"get_project_grants", "查询项目准入名单", "GET", "/api/management/projects/{id}/grants", "projects:read", true, s.listGrants},
		{"set_project_grants", "设置准入名单", "PUT", "/api/management/projects/{id}/grants", "projects:update", true, s.setGrants},
		{"get_project_blacklist", "查询项目黑名单", "GET", "/api/management/projects/{id}/blacklist", "projects:read", true, s.blacklist},
		{"set_project_blacklist", "设置项目黑名单", "PUT", "/api/management/projects/{id}/blacklist", "projects:update", true, s.setBlacklist},
		{"rotate_project_secret", "立即轮换客户端密钥，旧密钥失效", "POST", "/api/management/projects/{id}/rotate-secret", "projects:rotate", true, s.rotateApp},
		{"get_integration_guide", "读取完整认证接入说明", "GET", "/api/management/guide", "projects:read", false, func(w http.ResponseWriter, r *http.Request) { ok(w, s.aiIntegrationGuide()) }},
		{"get_project_service", "读取目标项目服务资料", "GET", "/api/service/app", "app:read", true, s.serviceApp},
		{"send_mail", "发送项目邮件，必须提供 idempotency_key。queued/accepted 不表示实际送达", "POST", "/api/service/mail", "mail:send", true, s.serviceMail},
		{"get_mail_status", "查询目标项目邮件状态，mail_id 指定邮件", "GET", "/api/service/mail/{mail}", "mail:read", true, s.serviceMailStatus},
		{"upload_project_logo", "上传 Logo。file_base64 为图片文件内容，filename 为文件名", "POST", "/api/management/project-logos", "", false, s.uploadProjectLogo},
	}
}

type toolInput struct {
	Project     string          `json:"project_id"`
	Role        string          `json:"role_id"`
	Mail        string          `json:"mail_id"`
	Body        json.RawMessage `json:"body"`
	Match       string          `json:"if_match"`
	Idempotency string          `json:"idempotency_key"`
	File        string          `json:"file_base64"`
	Filename    string          `json:"filename"`
}
type toolCapture struct {
	headers http.Header
	status  int
	bytes.Buffer
}

func (w *toolCapture) Header() http.Header { return w.headers }
func (w *toolCapture) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}
func (w *toolCapture) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = 200
	}
	return w.Buffer.Write(b)
}
func (s *Server) serveMCP(w http.ResponseWriter, r *http.Request) {
	auth := r.Context().Value(managementContextKey{}).(*managementAuth)
	server := mcp.NewServer(&mcp.Implementation{Name: "duoyu-id", Version: "1.0.0"}, nil)
	for _, tool := range s.developerTools() {
		allowed := false
		for _, scope := range auth.Scopes {
			if scope == tool.scope || tool.scope == "" && (scope == "projects:create" || scope == "projects:update") {
				allowed = true
			}
		}
		if !allowed {
			continue
		}
		properties := map[string]any{"project_id": map[string]string{"type": "string"}, "role_id": map[string]string{"type": "string"}, "mail_id": map[string]string{"type": "string"}, "body": map[string]string{"type": "object"}, "if_match": map[string]string{"type": "string"}, "idempotency_key": map[string]string{"type": "string"}, "file_base64": map[string]string{"type": "string"}, "filename": map[string]string{"type": "string"}}
		required := []string{}
		if tool.project {
			required = append(required, "project_id")
		}
		if strings.Contains(tool.path, "{role}") {
			required = append(required, "role_id")
		}
		if strings.Contains(tool.path, "{mail}") {
			required = append(required, "mail_id")
		}
		if tool.name == "create_project" || tool.name == "send_mail" {
			required = append(required, "idempotency_key", "body")
		}
		if tool.name == "update_project" {
			required = append(required, "if_match", "body")
		}
		if tool.name == "upload_project_logo" {
			required = append(required, "file_base64", "filename")
		}
		server.AddTool(&mcp.Tool{Name: tool.name, Description: tool.description, Annotations: &mcp.ToolAnnotations{ReadOnlyHint: tool.method == "GET", IdempotentHint: tool.method == "GET" || tool.name == "create_project" || tool.name == "send_mail"}, InputSchema: map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			var in toolInput
			decoder := json.NewDecoder(bytes.NewReader(req.Params.Arguments))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&in); err != nil {
				return nil, err
			}

			values := map[string]string{"project_id": in.Project, "role_id": in.Role, "mail_id": in.Mail, "if_match": in.Match, "idempotency_key": in.Idempotency, "file_base64": in.File, "filename": in.Filename}
			for _, field := range required {
				missing := field == "body" && (len(in.Body) == 0 || bytes.Equal(bytes.TrimSpace(in.Body), []byte("null"))) || field != "body" && strings.TrimSpace(values[field]) == ""
				if missing {
					return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "缺少必要参数：" + field}}}, nil
				}
			}
			path := strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(tool.path, "{id}", url.PathEscape(in.Project)), "{role}", url.PathEscape(in.Role)), "{mail}", url.PathEscape(in.Mail))
			body := in.Body
			if len(body) == 0 {
				body = []byte("{}")
			}
			contentType := "application/json"
			if tool.name == "upload_project_logo" {
				file, err := base64.StdEncoding.DecodeString(in.File)
				if err != nil || len(file) > 2<<20 {
					return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "Logo 必须是有效 Base64 图片且不超过 2 MiB"}}}, nil
				}
				var buf bytes.Buffer
				form := multipart.NewWriter(&buf)
				part, err := form.CreateFormFile("logo", in.Filename)
				if err != nil {
					return nil, err
				}
				part.Write(file)
				form.Close()
				body = buf.Bytes()
				contentType = form.FormDataContentType()
			}
			request, _ := http.NewRequestWithContext(ctx, tool.method, path, bytes.NewReader(body))
			request.Header.Set("Authorization", r.Header.Get("Authorization"))
			request.Header.Set("Content-Type", contentType)
			request.Header.Set("X-Duoyu-Project", in.Project)
			request.Header.Set("If-Match", in.Match)
			request.Header.Set("Idempotency-Key", in.Idempotency)
			request.RemoteAddr = r.RemoteAddr
			request.SetPathValue("id", in.Project)
			request.SetPathValue("role", in.Role)
			if tool.name == "get_mail_status" {
				request.SetPathValue("id", in.Mail)
			}
			capture := &toolCapture{headers: make(http.Header)}
			capture.Header().Set("X-Request-ID", w.Header().Get("X-Request-ID"))
			handler := tool.handler
			if strings.HasPrefix(tool.path, "/api/management/") {
				handler = s.managementProxy(tool.scope, tool.project, handler)
			}
			handler(capture, request)
			s.audit(ctx, auth.Owner.ID, "mcp."+tool.name, in.Project, capture.status < 400)
			var data any
			if json.Unmarshal(capture.Bytes(), &data) != nil {
				data = map[string]string{"message": "服务响应格式异常"}
			}
			result := map[string]any{"status": capture.status, "etag": capture.Header().Get("ETag"), "response": data}
			raw, _ := json.Marshal(result)
			return &mcp.CallToolResult{IsError: capture.status >= 400, Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}}, StructuredContent: result}, nil
		})
	}
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	handler.ServeHTTP(w, r)
}

var _ io.Writer = (*toolCapture)(nil)
