package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type managementContextKey struct{}
type managementAuth struct {
	ID       string
	Owner    User
	All      bool
	Scopes   []string
	Revision int64
}

var managementScopes = []string{"projects:read", "projects:create", "projects:update", "projects:rotate", "app:read", "mail:send", "mail:read"}

func (s *Server) registerManagementRoutes(m *http.ServeMux) {
	m.HandleFunc("PUT /api/admin/developer-keys/{key}", s.updateDeveloperKey)
	m.HandleFunc("POST /api/admin/developer-keys/{key}/rotate", func(w http.ResponseWriter, r *http.Request) {
		r.SetPathValue("id", r.PathValue("key"))
		s.rotateAPIKey(w, r)
	})
	m.HandleFunc("GET /api/admin/developer-keys", s.listManagementKeys)
	m.HandleFunc("POST /api/admin/developer-keys", s.createManagementKey)
	m.HandleFunc("POST /api/admin/developer-keys/{key}/revoke", s.revokeManagementKey)
	m.HandleFunc("GET /api/admin/ai-integration", s.adminAIGuide)
	m.HandleFunc("GET /api/management/openapi.json", s.managementProxy("projects:read", false, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(s.serviceOpenAPI())
	}))
	m.HandleFunc("GET /api/management/projects", s.managementProxy("projects:read", false, s.managementProjects))
	m.HandleFunc("GET /api/management/projects/{id}/checks", s.managementProxy("projects:read", true, s.appChecks))
	m.HandleFunc("GET /api/management/projects/{id}", s.managementProxy("projects:read", true, s.managementProject))
	m.HandleFunc("POST /api/management/projects", s.managementProxy("projects:create", false, s.managementSaveProject))
	m.HandleFunc("PATCH /api/management/projects/{id}", s.managementProxy("projects:update", true, s.managementSaveProject))
	m.HandleFunc("PUT /api/management/projects/{id}", s.managementProxy("projects:update", true, s.managementSaveProject))
	m.HandleFunc("POST /api/management/project-logos", s.managementProxy("", false, s.uploadProjectLogo))
	m.HandleFunc("POST /api/management/projects/{id}/rotate-secret", s.managementProxy("projects:rotate", true, s.rotateApp))
	m.HandleFunc("GET /api/management/guide", s.managementProxy("projects:read", false, func(w http.ResponseWriter, r *http.Request) { ok(w, s.aiIntegrationGuide()) }))
	for _, route := range []struct {
		method, path, scope string
		handler             http.HandlerFunc
	}{
		{"GET", "roles", "projects:read", s.listAppRoles}, {"POST", "roles", "projects:update", s.saveAppRole},
		{"PUT", "roles/{role}", "projects:update", s.saveAppRole}, {"DELETE", "roles/{role}", "projects:update", s.deleteAppRole},
		{"GET", "grants", "projects:read", s.listGrants}, {"PUT", "grants", "projects:update", s.setGrants},
		{"GET", "blacklist", "projects:read", s.blacklist}, {"PUT", "blacklist", "projects:update", s.setBlacklist},
	} {
		m.HandleFunc(route.method+" /api/management/projects/{id}/"+route.path, s.managementProxy(route.scope, true, route.handler))
	}
}
func (s *Server) managementProxy(scope string, project bool, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// These credentials are for local tools and server processes, never browser JS.
		if r.Header.Get("Origin") != "" {
			fail(w, 403, "开发者接口仅供后端或工具调用，请勿在浏览器中使用凭证")
			return
		}
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") || len(header) > 512 {
			fail(w, 401, "需要开发者 API 凭证")
			return
		}
		var a managementAuth
		var owner string
		err := s.db.QueryRow(r.Context(), "SELECT id,owner_id,scopes,all_projects FROM api_keys WHERE (digest=$1 OR (previous_digest=$1 AND previous_until>now())) AND NOT revoked AND (expires_at IS NULL OR expires_at>now())", s.mac(strings.TrimPrefix(header, "Bearer "))).Scan(&a.ID, &owner, &a.Scopes, &a.All)
		if errors.Is(err, pgx.ErrNoRows) {
			fail(w, 401, "开发者凭证无效、已到期或撤销")
			return
		}
		if err != nil {
			fail(w, 503, "凭证验证暂不可用")
			return
		}
		a.Owner, err = s.user(r.Context(), owner)
		if err != nil || !a.Owner.Admin {
			fail(w, 401, "凭证所属管理员不可用")
			return
		}
		if scope != "*" && (scope != "" && !slices.Contains(a.Scopes, scope) || scope == "" && !slices.Contains(a.Scopes, "projects:create") && !slices.Contains(a.Scopes, "projects:update")) {
			fail(w, 403, "开发者凭证缺少所需权限")
			return
		}
		if s.limited(r, "management-key:"+a.ID, 120, time.Minute) {
			fail(w, 429, "管理请求过于频繁")
			return
		}
		if project && !s.managementProjectAllowed(r.Context(), &a, r.PathValue("id")) {
			fail(w, 403, "凭证无权管理该项目")
			return
		}
		if _, err = s.db.Exec(r.Context(), "UPDATE api_keys SET last_used_at=now() WHERE id=$1", a.ID); err != nil {
			fail(w, 503, "凭证暂不可用")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), managementContextKey{}, &a)))
	}
}
func (s *Server) managementProjectAllowed(ctx context.Context, a *managementAuth, id string) bool {
	var yes bool
	err := s.db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM apps WHERE id=$1 AND ($2 OR EXISTS(SELECT 1 FROM management_api_grants WHERE key_id=$3 AND app_id=$1)))", id, a.All, a.ID).Scan(&yes)
	return err == nil && yes
}
func (s *Server) managementProjects(w http.ResponseWriter, r *http.Request) {
	a := r.Context().Value(managementContextKey{}).(*managementAuth)
	s.queryList(w, r, `SELECT id,name,description,logo,public,platform,enabled,created_at,management_revision AS revision FROM apps WHERE ($1 OR EXISTS(SELECT 1 FROM management_api_grants WHERE key_id=$2 AND app_id=apps.id)) ORDER BY created_at DESC LIMIT 200`, a.All, a.ID)
}
func (s *Server) loadManagementProject(ctx context.Context, id string) (a App, err error) {
	err = s.db.QueryRow(ctx, `SELECT id,name,description,logo,public,platform,enabled,restricted,refresh_enabled,redirects,logout_redirects,allowed_origins,login_methods,backchannel_logout_uri,created_at,management_revision FROM apps WHERE id=$1`, id).Scan(&a.ID, &a.Name, &a.Description, &a.Logo, &a.Public, &a.Platform, &a.Enabled, &a.Restricted, &a.RefreshEnabled, &a.Redirects, &a.LogoutRedirects, &a.AllowedOrigins, &a.LoginMethods, &a.BackchannelLogoutURI, &a.Created, &a.Revision)
	return
}
func (s *Server) managementProject(w http.ResponseWriter, r *http.Request) {
	a, err := s.loadManagementProject(r.Context(), r.PathValue("id"))
	if err != nil {
		fail(w, 503, "项目查询暂不可用")
		return
	}
	w.Header().Set("ETag", strconv.Quote(strconv.FormatInt(a.Revision, 10)))
	ok(w, a)
}
func (s *Server) managementSaveProject(w http.ResponseWriter, r *http.Request) {
	auth := r.Context().Value(managementContextKey{}).(*managementAuth)
	var patch map[string]json.RawMessage
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&patch); err != nil || patch == nil {
		fail(w, 400, "需要 JSON 对象")
		return
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		fail(w, 400, "请求只能包含一个 JSON 对象")
		return
	}
	fields := []string{"name", "description", "logo", "public", "platform", "enabled", "restricted", "refresh_enabled", "redirects", "logout_redirects", "allowed_origins", "login_methods", "backchannel_logout_uri"}
	for k, v := range patch {
		if !slices.Contains(fields, k) || bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
			fail(w, 400, "不支持的字段或 null 值："+k)
			return
		}
	}
	merged := map[string]json.RawMessage{}
	if r.PathValue("id") == "" {
		if len(r.Header.Get("Idempotency-Key")) < 8 || len(r.Header.Get("Idempotency-Key")) > 128 {
			fail(w, 400, "创建项目必须提供 8–128 位 Idempotency-Key")
			return
		}
		merged["enabled"] = json.RawMessage("true")
	} else {
		a, err := s.loadManagementProject(r.Context(), r.PathValue("id"))
		if err != nil {
			fail(w, 503, "读取项目失败")
			return
		}
		match := strings.Trim(r.Header.Get("If-Match"), "\"")
		revision, err := strconv.ParseInt(match, 10, 64)
		if err != nil || revision < 0 {
			fail(w, 428, "修改项目必须先 GET，再携带 ETag 对应的 If-Match")
			return
		}
		if revision != a.Revision {
			fail(w, 412, "项目版本已变化，请重新 GET 后合并修改")
			return
		}
		auth.Revision = revision
		raw, _ := json.Marshal(a)
		json.Unmarshal(raw, &merged)
		delete(merged, "id")
		delete(merged, "created_at")
		delete(merged, "revision")
	}
	for k, v := range patch {
		merged[k] = v
	}
	raw, err := json.Marshal(merged)
	if err != nil {
		fail(w, 400, "项目参数错误")
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(raw))
	s.saveApp(w, r)
}
