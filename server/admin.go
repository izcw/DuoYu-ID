package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"slices"

	"strings"
	"time"
)

func (s *Server) queryList(w http.ResponseWriter, r *http.Request, sql string, args ...any) {
	if r.URL.Path == "/api/admin/apps" {
		u, e := s.current(r)
		if e != nil {
			fail(w, 401, "请先登录")
			return
		}
		if !u.Admin && u.ManagementRole == "maintainer" {
			args = append(args, u.ID)
			sql = fmt.Sprintf("SELECT scoped.* FROM (%s) scoped WHERE scoped.id IN(SELECT app_id FROM app_maintainers WHERE user_id=$%d) ORDER BY scoped.created_at DESC LIMIT 200", strings.TrimSuffix(sql, " LIMIT 200"), len(args))
		}
	}
	if r.URL.Query().Has("page") {
		s.paginatedList(w, r, sql, args)
		return
	}
	rows, e := s.db.Query(r.Context(), sql, args...)
	if e != nil {
		fail(w, 500, "查询失败")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		v, e := rows.Values()
		if e != nil {
			fail(w, 500, "读取失败")
			return
		}
		item := map[string]any{}
		for i, f := range rows.FieldDescriptions() {
			item[f.Name] = v[i]
		}
		out = append(out, item)
	}
	if rows.Err() != nil {
		fail(w, 500, "读取失败")
		return
	}
	ok(w, out)
}
func (s *Server) overview(w http.ResponseWriter, r *http.Request) {
	if _, yes := s.require(w, r, true); !yes {
		return
	}
	var users, apps, sessions, success, failed int64
	e := s.db.QueryRow(r.Context(), `SELECT (SELECT count(*) FROM users),(SELECT count(*) FROM apps),(SELECT count(*) FROM sessions WHERE expires_at>now()),(SELECT count(*) FROM audit WHERE action='login.success' AND created_at>now()-interval '24 hours'),(SELECT count(*) FROM audit WHERE action='login.failed' AND created_at>now()-interval '24 hours')`).Scan(&users, &apps, &sessions, &success, &failed)
	if e != nil {
		fail(w, 500, "查询失败")
		return
	}
	ok(w, map[string]any{"users": users, "apps": apps, "sessions": sessions, "success": success, "failed": failed, "issuer": s.origin})
}
func (s *Server) users(w http.ResponseWriter, r *http.Request) {
	if _, yes := s.require(w, r, true); !yes {
		return
	}
	s.queryList(w, r, "SELECT id,account_id,email,name,avatar,admin,enabled,created_at,password_locked_until,password_lock_level,"+accountProjectsSQL+" AS projects FROM users WHERE email ILIKE $1 OR name ILIKE $1 OR account_id ILIKE $1 OR EXISTS(SELECT 1 FROM app_users au JOIN apps a ON a.id=au.app_id WHERE au.user_id=users.id AND a.name ILIKE $1) ORDER BY created_at DESC LIMIT 200", "%"+truncate(r.URL.Query().Get("q"), 100)+"%")
}
func (s *Server) updateUser(w http.ResponseWriter, r *http.Request) {
	u, yes := s.require(w, r, true)
	if !yes {
		return
	}
	var in struct {
		Enabled *bool   `json:"enabled"`
		Name    *string `json:"name"`
	}
	if decode(r, &in) != nil {
		fail(w, 400, "请求错误")
		return
	}
	id := r.PathValue("id")
	if in.Name != nil {
		fail(w, 403, "昵称只能由账号本人在账号与安全中修改")
		return
	}
	if in.Enabled == nil {
		fail(w, 400, "请选择需要更新的字段")
		return
	}
	if id == u.ID {
		fail(w, 400, "不能停用当前管理员账号")
		return
	}
	tx, e := s.db.Begin(r.Context())
	if e != nil {
		fail(w, 500, "更新失败")
		return
	}
	defer tx.Rollback(r.Context())
	tag, e := tx.Exec(r.Context(), "UPDATE users SET enabled=$1 WHERE id=$2 AND NOT admin", in.Enabled, id)
	if e != nil || tag.RowsAffected() != 1 {
		fail(w, 400, "账号不存在或为受保护的管理员")
		return
	}
	if !*in.Enabled {
		tx.Exec(r.Context(), "DELETE FROM sessions WHERE user_id=$1", id)
		tx.Exec(r.Context(), "DELETE FROM access_tokens WHERE user_id=$1", id)
	}
	if tx.Commit(r.Context()) != nil {
		fail(w, 500, "更新失败")
		return
	}
	s.audit(r.Context(), u.ID, "admin.user.status", id, true)
	ok(w, nil)
}
func (s *Server) adminRevoke(w http.ResponseWriter, r *http.Request) {
	u, yes := s.require(w, r, true)
	if !yes {
		return
	}
	id := r.PathValue("id")
	tx, e := s.db.Begin(r.Context())
	if e != nil {
		fail(w, 500, "操作失败")
		return
	}
	defer tx.Rollback(r.Context())
	tx.Exec(r.Context(), "DELETE FROM sessions WHERE user_id=$1", id)
	tx.Exec(r.Context(), "DELETE FROM access_tokens WHERE user_id=$1", id)
	if tx.Commit(r.Context()) != nil {
		fail(w, 500, "操作失败")
		return
	}
	s.audit(r.Context(), u.ID, "admin.user.revoke", id, true)
	ok(w, nil)
}
func (s *Server) apps(w http.ResponseWriter, r *http.Request) {
	if _, yes := s.require(w, r, true); !yes {
		return
	}
	s.queryList(w, r, "SELECT id,name,description,redirects,logout_redirects,public,enabled,restricted,created_at,platform,refresh_enabled,backchannel_logout_uri,logo FROM apps ORDER BY created_at DESC LIMIT 200")
}
func (s *Server) appUsers(w http.ResponseWriter, r *http.Request) {
	if _, yes := s.require(w, r, true); !yes {
		return
	}
	s.queryList(w, r, `SELECT u.id,u.email,u.name,u.avatar,u.enabled,a.first_login_at,a.last_login_at,a.login_count,a.role_id,ar.name AS role_name,ar.role_key FROM app_users a JOIN users u ON u.id=a.user_id JOIN app_roles ar ON ar.app_id=a.app_id AND ar.id=a.role_id WHERE a.app_id=$1 ORDER BY a.last_login_at DESC LIMIT 200`, r.PathValue("id"))
}
func (s *Server) validRedirect(v string) bool {
	u, e := url.Parse(v)
	if e != nil || u.Host == "" || u.User != nil || u.Fragment != "" || strings.Contains(v, "*") {
		return false
	}
	return u.Scheme == "https" || (s.dev && u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1"))
}
func (s *Server) validAppRedirect(v, platform string) bool {
	if s.validRedirect(v) {
		return true
	}
	if platform != "native" {
		return false
	}
	u, e := url.Parse(v)
	if e != nil || u.User != nil || u.Fragment != "" || strings.Contains(v, "*") {
		return false
	}
	if u.Scheme == "http" {
		return u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"
	}
	// Private schemes must use reverse-domain names and hierarchical paths.
	return strings.Contains(u.Scheme, ".") && u.Host == "" && strings.HasPrefix(u.Path, "/") && u.Opaque == ""
}
func (s *Server) saveApp(w http.ResponseWriter, r *http.Request) {
	u, yes := s.require(w, r, true)
	if !yes {
		return
	}
	var a App
	if decode(r, &a) != nil || len(strings.TrimSpace(a.Name)) < 1 || len([]rune(a.Name)) > 60 || len(a.Description) > 1000 || len(a.Redirects) == 0 || len(a.Redirects) > 10 || len(a.LogoutRedirects) > 10 {
		fail(w, 400, "检查应用名称和回调地址（最多 10 个）")
		return
	}
	if a.LoginMethods == nil {
		a.LoginMethods = allLoginMethods()
		if id := r.PathValue("id"); id != "" {
			if err := s.db.QueryRow(r.Context(), "SELECT login_methods FROM apps WHERE id=$1", id).Scan(&a.LoginMethods); err != nil {
				fail(w, 400, "项目不存在")
				return
			}
		}
	}
	for _, method := range a.LoginMethods {
		if !slices.Contains(allLoginMethods(), method) {
			fail(w, 400, "不支持的登录方式")
			return
		}
	}
	a.Logo = strings.TrimSpace(a.Logo)
	if !s.validProjectLogo(a.Logo) {
		fail(w, 400, "请先上传项目 Logo，再保存项目")
		return
	}
	if a.BackchannelLogoutURI != "" && !s.validLogoutEndpoint(a.BackchannelLogoutURI) {
		fail(w, 400, "后台退出通知地址须为无查询参数的 HTTPS URL")
		return
	}
	if a.Platform == "" {
		a.Platform = "web"
		if a.Public {
			a.Platform = "spa"
		}
	}
	if !slices.Contains([]string{"web", "spa", "native", "extension"}, a.Platform) || (a.Platform != "web" && !a.Public) {
		fail(w, 400, "SPA、原生应用和浏览器扩展必须使用公共客户端")
		return
	}
	if len(a.AllowedOrigins) > 20 {
		fail(w, 400, "最多填写 20 个请求来源")
		return
	}
	for _, origin := range a.AllowedOrigins {
		if !s.validOrigin(origin) {
			fail(w, 400, "请求来源须为精确 origin，不含路径、通配符或查询参数")
			return
		}
	}
	if a.AllowedOrigins == nil {
		a.AllowedOrigins = []string{}
	}
	for _, v := range a.Redirects {
		if !s.validAppRedirect(v, a.Platform) {
			fail(w, 400, "回调地址须为精确的 HTTPS URL，开发模式允许本机 HTTP")
			return
		}
	}
	for _, v := range a.LogoutRedirects {
		if !s.validRedirect(v) {
			fail(w, 400, "退出回调必须使用 HTTPS，开发环境允许本机 HTTP")
			return
		}
	}
	if a.LogoutRedirects == nil {
		a.LogoutRedirects = []string{}
	}
	secret := ""
	id := r.PathValue("id")
	var e error
	if id == "" {
		if management, ok := r.Context().Value(managementContextKey{}).(*managementAuth); ok {
			s.createManagedProject(w, r, management, a)
			return
		}
		id = "app_" + randomID()[:20]
		if !a.Public {
			secret = randomID()
			a.Secret = s.mac(secret)
		}
		_, e = s.db.Exec(r.Context(), "INSERT INTO apps(id,name,description,secret_hash,redirects,logout_redirects,public,enabled,restricted,refresh_enabled,platform,backchannel_logout_uri,logo,allowed_origins,login_methods) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)", id, a.Name, a.Description, a.Secret, a.Redirects, a.LogoutRedirects, a.Public, a.Enabled, a.Restricted, a.RefreshEnabled, a.Platform, a.BackchannelLogoutURI, a.Logo, a.AllowedOrigins, a.LoginMethods)
	} else {
		var wasPublic bool
		e = s.db.QueryRow(r.Context(), "SELECT public FROM apps WHERE id=$1", id).Scan(&wasPublic)
		if e != nil || wasPublic != a.Public {
			fail(w, 400, "客户端类型不可修改，请创建新应用")
			return
		}
		var expected any
		if management, ok := r.Context().Value(managementContextKey{}).(*managementAuth); ok {
			expected = management.Revision
		}
		tag, err := s.db.Exec(r.Context(), "UPDATE apps SET name=$1,description=$2,redirects=$3,logout_redirects=$4,enabled=$5,restricted=$6,refresh_enabled=$8,platform=$9,backchannel_logout_uri=$10,logo=$11,allowed_origins=$12,login_methods=$13,management_revision=management_revision+1 WHERE id=$7 AND ($14::bigint IS NULL OR management_revision=$14)", a.Name, a.Description, a.Redirects, a.LogoutRedirects, a.Enabled, a.Restricted, id, a.RefreshEnabled, a.Platform, a.BackchannelLogoutURI, a.Logo, a.AllowedOrigins, a.LoginMethods, expected)
		e = err
		if e == nil && tag.RowsAffected() != 1 {
			fail(w, 412, "项目已被修改，请重新 GET 项目再提交 If-Match")
			return
		}
	}
	if e != nil {
		fail(w, 500, "应用保存失败")
		return
	}
	s.audit(r.Context(), u.ID, "admin.app.saved", id, true)
	ok(w, map[string]string{"id": id, "secret": secret})
}
func (s *Server) rotateApp(w http.ResponseWriter, r *http.Request) {
	u, yes := s.require(w, r, true)
	if !yes {
		return
	}
	secret := randomID()
	tag, e := s.db.Exec(r.Context(), "UPDATE apps SET secret_hash=$1 WHERE id=$2 AND NOT public", s.mac(secret), r.PathValue("id"))
	if e != nil || tag.RowsAffected() != 1 {
		fail(w, 400, "应用不存在或是公共客户端")
		return
	}
	s.audit(r.Context(), u.ID, "admin.app.rotated", r.PathValue("id"), true)
	ok(w, map[string]string{"secret": secret})
}
func (s *Server) listGrants(w http.ResponseWriter, r *http.Request) {
	if _, yes := s.require(w, r, true); !yes {
		return
	}
	s.queryList(w, r, "SELECT u.id,u.email,u.name FROM grants g JOIN users u ON u.id=g.user_id WHERE g.app_id=$1", r.PathValue("id"))
}
func (s *Server) setGrants(w http.ResponseWriter, r *http.Request) {
	u, yes := s.require(w, r, true)
	if !yes {
		return
	}
	var in struct {
		Emails []string `json:"emails"`
	}
	if decode(r, &in) != nil || len(in.Emails) > 500 {
		fail(w, 400, "最多设置 500 个账号")
		return
	}
	tx, e := s.db.Begin(r.Context())
	if e != nil {
		fail(w, 500, "更新失败")
		return
	}
	defer tx.Rollback(r.Context())
	id := r.PathValue("id")
	emails := make([]string, 0, len(in.Emails))
	for _, email := range in.Emails {
		emails = append(emails, normalizeEmail(email))
	}
	if _, e = tx.Exec(r.Context(), "DELETE FROM grants WHERE app_id=$1 AND user_id NOT IN(SELECT id FROM users WHERE email=ANY($2))", id, emails); e != nil {
		fail(w, 500, "更新失败")
		return
	}
	for _, email := range in.Emails {
		var uid string
		if e = tx.QueryRow(r.Context(), "SELECT id FROM users WHERE email=$1", normalizeEmail(email)).Scan(&uid); e != nil {
			fail(w, 400, "名单中存在未注册邮箱")
			return
		}
		if _, e = tx.Exec(r.Context(), "INSERT INTO grants(app_id,user_id) VALUES($1,$2) ON CONFLICT DO NOTHING", id, uid); e != nil {
			fail(w, 400, "授权失败")
			return
		}
	}
	if tx.Commit(r.Context()) != nil {
		fail(w, 500, "更新失败")
		return
	}
	s.audit(r.Context(), u.ID, "admin.app.grants", id, true)
	ok(w, nil)
}
func (s *Server) auditList(w http.ResponseWriter, r *http.Request) {
	if _, yes := s.require(w, r, true); !yes {
		return
	}
	s.queryList(w, r, "SELECT a.id,a.action,a.target,a.success,a.created_at,a.request_id,coalesce(u.email,'系统 / 匿名') AS actor FROM audit a LEFT JOIN users u ON u.id=a.user_id ORDER BY a.id DESC LIMIT 200")
}
func (s *Server) keys(w http.ResponseWriter, r *http.Request) {
	if _, yes := s.require(w, r, true); !yes {
		return
	}
	s.queryList(w, r, "SELECT k.id,k.name,k.app_id,a.name AS app_name,k.scopes,k.expires_at,k.revoked,k.created_at,k.last_used_at FROM api_keys k JOIN apps a ON a.id=k.app_id ORDER BY k.created_at DESC LIMIT 200")
}
func (s *Server) createKey(w http.ResponseWriter, r *http.Request) {
	u, yes := s.require(w, r, true)
	if !yes {
		return
	}
	var in struct {
		Name      string   `json:"name"`
		AppID     string   `json:"app_id"`
		Days      int      `json:"days"`
		Permanent bool     `json:"permanent"`
		Scopes    []string `json:"scopes"`
	}
	if decode(r, &in) != nil || len(strings.TrimSpace(in.Name)) < 1 || len(in.Name) > 100 || (!in.Permanent && (in.Days < 1 || in.Days > 365)) {
		fail(w, 400, "填写名称，选择永久有效或 1–365 天")
		return
	}
	if _, e := s.app(r.Context(), in.AppID); e != nil {
		fail(w, 400, "应用不可用")
		return
	}
	if in.Scopes == nil {
		in.Scopes = []string{"app:read"}
	}
	if !validKeyScopes(in.Scopes) {
		fail(w, 400, "请选择有效的凭证权限")
		return
	}
	id := "key_" + randomID()[:16]
	secret := "dy_" + randomID()
	var expiry *time.Time
	if !in.Permanent {
		v := time.Now().Add(time.Duration(in.Days) * 24 * time.Hour)
		expiry = &v
	}
	_, e := s.db.Exec(r.Context(), "INSERT INTO api_keys(id,name,digest,app_id,scopes,expires_at,owner_id) VALUES($1,$2,$3,$4,$5,$6,$7)", id, strings.TrimSpace(in.Name), s.mac(secret), in.AppID, in.Scopes, expiry, u.ID)
	if e != nil {
		fail(w, 500, "创建失败")
		return
	}
	s.audit(r.Context(), u.ID, "admin.key.created", id, true)
	ok(w, map[string]string{"id": id, "secret": secret})
}
func (s *Server) revokeKey(w http.ResponseWriter, r *http.Request) {
	u, yes := s.require(w, r, true)
	if !yes {
		return
	}
	if _, e := s.db.Exec(r.Context(), "UPDATE api_keys SET revoked=true WHERE id=$1", r.PathValue("id")); e != nil {
		fail(w, 500, "撤销失败")
		return
	}
	s.audit(r.Context(), u.ID, "admin.key.revoked", r.PathValue("id"), true)
	ok(w, nil)
}
func (s *Server) serviceApp(w http.ResponseWriter, r *http.Request) {
	key, app, yes := s.requireService(w, r, "app:read")
	if !yes {
		return
	}
	s.audit(r.Context(), "", "api.app.read", key, true)
	ok(w, app)
}
func (s *Server) registration(ctx context.Context) bool {
	var v string
	s.db.QueryRow(ctx, "SELECT value FROM settings WHERE key='registration'").Scan(&v)
	return v == "true"
}
func (s *Server) settings(w http.ResponseWriter, r *http.Request) {
	if _, yes := s.require(w, r, true); !yes {
		return
	}
	ok(w, map[string]any{"registration": s.registration(r.Context()), "issuer": s.origin, "smtp_host": env("SMTP_HOST", "smtp.qq.com"), "smtp_port": env("SMTP_PORT", "465"), "smtp_from": os.Getenv("SMTP_FROM"), "smtp_configured": os.Getenv("SMTP_PASSWORD") != "" || s.localMail(), "local_mail": s.localMail(), "development": s.dev})
}
func (s *Server) saveSettings(w http.ResponseWriter, r *http.Request) {
	u, yes := s.require(w, r, true)
	if !yes {
		return
	}
	var in struct {
		Registration bool `json:"registration"`
	}
	if decode(r, &in) != nil {
		fail(w, 400, "请求错误")
		return
	}
	v := "false"
	if in.Registration {
		v = "true"
	}
	if _, e := s.db.Exec(r.Context(), "UPDATE settings SET value=$1 WHERE key='registration'", v); e != nil {
		fail(w, 500, "更新失败")
		return
	}
	s.audit(r.Context(), u.ID, "admin.registration", v, true)
	ok(w, nil)
}
func (s *Server) mailPreview(w http.ResponseWriter, r *http.Request) {
	if _, yes := s.require(w, r, true); !yes {
		return
	}
	v, e := s.emailHTML("123456", r.URL.Query().Get("purpose"))
	if e != nil {
		fail(w, 500, "模板错误")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(v))
}
func (s *Server) openapi(w http.ResponseWriter, r *http.Request) {
	if _, yes := s.require(w, r, true); !yes {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.serviceOpenAPI())
}
