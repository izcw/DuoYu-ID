package main

import (
	"net/http"
	"strings"
	"time"
)

// Detail pages fetch by ID instead of depending on the latest 200 list rows.
func (s *Server) adminAppDetail(w http.ResponseWriter, r *http.Request) {
	if _, yes := s.require(w, r, true); !yes {
		return
	}
	s.queryList(w, r, "SELECT id,name,description,redirects,logout_redirects,public,enabled,restricted,created_at,platform,refresh_enabled,backchannel_logout_uri,logo,allowed_origins,login_methods FROM apps WHERE id=$1", r.PathValue("id"))
}
func (s *Server) adminUserDetail(w http.ResponseWriter, r *http.Request) {
	if _, yes := s.require(w, r, true); !yes {
		return
	}
	s.queryList(w, r, "SELECT id,account_id,email,name,avatar,admin,enabled,created_at,password_locked_until,password_lock_level,"+accountProjectsSQL+" AS projects FROM users WHERE id=$1", r.PathValue("id"))
}
func (s *Server) adminKeyDetail(w http.ResponseWriter, r *http.Request) {
	if _, yes := s.require(w, r, true); !yes {
		return
	}
	s.queryList(w, r, "SELECT k.id,k.name,k.app_id,a.name AS app_name,k.scopes,k.expires_at,k.revoked,k.created_at,k.previous_until,k.last_ip,k.last_used_at FROM api_keys k JOIN apps a ON a.id=k.app_id WHERE k.id=$1", r.PathValue("id"))
}
func (s *Server) updateKey(w http.ResponseWriter, r *http.Request) {
	u, yes := s.require(w, r, true)
	if !yes {
		return
	}
	var in struct {
		Name         string   `json:"name"`
		Days         int      `json:"days"`
		Permanent    bool     `json:"permanent"`
		ChangeExpiry bool     `json:"change_expiry"`
		Scopes       []string `json:"scopes"`
	}
	if decode(r, &in) != nil || len(strings.TrimSpace(in.Name)) < 1 || len(in.Name) > 100 || in.ChangeExpiry && !in.Permanent && (in.Days < 1 || in.Days > 365) {
		fail(w, 400, "请检查名称与有效期")
		return
	}
	var expiry *time.Time
	if in.Scopes != nil && !validKeyScopes(in.Scopes) {
		fail(w, 400, "请选择有效的凭证权限")
		return
	}
	if !in.Permanent {
		v := time.Now().Add(time.Duration(in.Days) * 24 * time.Hour)
		expiry = &v
	}
	tag, e := s.db.Exec(r.Context(), "UPDATE api_keys SET name=$1,expires_at=CASE WHEN $2 THEN $3 ELSE expires_at END,scopes=COALESCE($5::text[],scopes) WHERE id=$4 AND NOT revoked", strings.TrimSpace(in.Name), in.ChangeExpiry, expiry, r.PathValue("id"), in.Scopes)
	if e != nil || tag.RowsAffected() != 1 {
		fail(w, 400, "凭证不存在或已撤销")
		return
	}
	s.audit(r.Context(), u.ID, "admin.key.updated", r.PathValue("id"), true)
	ok(w, nil)
}
func (s *Server) analytics(w http.ResponseWriter, r *http.Request) {
	if _, yes := s.require(w, r, true); !yes {
		return
	}
	days := 14
	switch r.URL.Query().Get("days") {
	case "7":
		days = 7
	case "30":
		days = 30
	}
	rows, e := s.db.Query(r.Context(), `WITH dates AS (SELECT generate_series((now() AT TIME ZONE 'Asia/Shanghai')::date-($1::int-1),(now() AT TIME ZONE 'Asia/Shanghai')::date,interval '1 day')::date AS day), events AS (SELECT (created_at AT TIME ZONE 'Asia/Shanghai')::date AS day,count(*) FILTER(WHERE action='login.success') AS success,count(*) FILTER(WHERE action='login.failed') AS failed FROM audit WHERE created_at>=(((now() AT TIME ZONE 'Asia/Shanghai')::date-($1::int-1))::timestamp AT TIME ZONE 'Asia/Shanghai') GROUP BY 1), registrations AS (SELECT (created_at AT TIME ZONE 'Asia/Shanghai')::date AS day,count(*) AS total FROM users WHERE created_at>=(((now() AT TIME ZONE 'Asia/Shanghai')::date-($1::int-1))::timestamp AT TIME ZONE 'Asia/Shanghai') GROUP BY 1) SELECT to_char(d.day,'YYYY-MM-DD'),coalesce(e.success,0),coalesce(e.failed,0),coalesce(r.total,0) FROM dates d LEFT JOIN events e USING(day) LEFT JOIN registrations r USING(day) ORDER BY d.day`, days)
	if e != nil {
		fail(w, 500, "统计暂不可用")
		return
	}
	type Point struct {
		Date          string `json:"date"`
		Success       int64  `json:"success"`
		Failed        int64  `json:"failed"`
		Registrations int64  `json:"registrations"`
	}
	series := []Point{}
	for rows.Next() {
		var p Point
		if rows.Scan(&p.Date, &p.Success, &p.Failed, &p.Registrations) != nil {
			rows.Close()
			fail(w, 500, "统计读取失败")
			return
		}
		series = append(series, p)
	}
	err := rows.Err()
	rows.Close()
	if err != nil {
		fail(w, 500, "统计读取失败")
		return
	}
	projects, e := s.db.Query(r.Context(), "SELECT a.name,count(u.user_id),coalesce(sum(u.login_count),0)::bigint FROM apps a LEFT JOIN app_users u ON u.app_id=a.id GROUP BY a.id ORDER BY coalesce(sum(u.login_count),0) DESC,a.name LIMIT 6")
	if e != nil {
		fail(w, 500, "项目统计不可用")
		return
	}
	defer projects.Close()
	type Project struct {
		Name   string `json:"name"`
		Users  int64  `json:"users"`
		Logins int64  `json:"logins"`
	}
	ranking := []Project{}
	for projects.Next() {
		var p Project
		if projects.Scan(&p.Name, &p.Users, &p.Logins) != nil {
			fail(w, 500, "项目统计读取失败")
			return
		}
		ranking = append(ranking, p)
	}
	if projects.Err() != nil {
		fail(w, 500, "项目统计读取失败")
		return
	}
	ok(w, map[string]any{"series": series, "projects": ranking, "timezone": "Asia/Shanghai"})
}

func (s *Server) rotateAPIKey(w http.ResponseWriter, r *http.Request) {
	u, yes := s.require(w, r, true)
	if !yes {
		return
	}
	var in struct {
		Hours int `json:"overlap_hours"`
	}
	if decode(r, &in) != nil || in.Hours < 0 || in.Hours > 168 {
		fail(w, 400, "过渡期为 0–168 小时")
		return
	}
	secret := "dy_api_" + randomID()
	until := time.Now().Add(time.Duration(in.Hours) * time.Hour)
	tag, e := s.db.Exec(r.Context(), "UPDATE api_keys SET previous_digest=digest,previous_until=$3,digest=$2 WHERE id=$1 AND NOT revoked AND (expires_at IS NULL OR expires_at>now()) AND (previous_until IS NULL OR previous_until<=now())", r.PathValue("id"), s.mac(secret), until)
	if e != nil {
		fail(w, 503, "暂时无法轮换")
		return
	}
	if tag.RowsAffected() != 1 {
		fail(w, 409, "凭证失效或仍在过渡期，暂不能再次轮换")
		return
	}
	s.audit(r.Context(), u.ID, "admin.key.rotated", r.PathValue("id"), true)
	ok(w, map[string]any{"secret": secret, "previous_until": until})
}

func (s *Server) appChecks(w http.ResponseWriter, r *http.Request) {
	if _, yes := s.require(w, r, true); !yes {
		return
	}
	a, e := s.app(r.Context(), r.PathValue("id"))
	if e != nil {
		fail(w, 400, "项目不存在或已停用")
		return
	}
	checks := []map[string]any{}
	add := func(label string, passed bool) {
		checks = append(checks, map[string]any{"label": label, "passed": passed})
	}
	add("客户端类型与平台匹配", a.Platform == "web" || a.Public)
	valid := len(a.Redirects) > 0
	for _, v := range a.Redirects {
		valid = valid && s.validAppRedirect(v, a.Platform)
	}
	add("登录回调符合当前环境规则", valid)
	add("S256 PKCE 已强制启用", true)
	add("后台退出通知已配置", a.BackchannelLogoutURI != "" && s.validLogoutEndpoint(a.BackchannelLogoutURI))
	add("使用生产模式", !s.dev)
	var failureID int64
	if e = s.db.QueryRow(r.Context(), "SELECT coalesce(max(id),0) FROM logout_jobs WHERE app_id=$1 AND last_error<>''", a.ID).Scan(&failureID); e != nil {
		fail(w, 503, "检查暂不可用")
		return
	}
	ok(w, map[string]any{"last_logout_failure_id": failureID, "checks": checks, "note": "仅校验保存的配置，不代表回调网络可达或项目验签通过。"})
}

// A central account can belong to multiple projects. Membership reflects a
// successful project authorization, not exclusive ownership or a login grant.
const accountProjectsSQL = `COALESCE((SELECT jsonb_agg(jsonb_build_object('id',a.id,'name',a.name,'logo',a.logo,'role_name',ar.name,'role_key',ar.role_key) ORDER BY a.name,a.id) FROM app_users au JOIN apps a ON a.id=au.app_id JOIN app_roles ar ON ar.app_id=au.app_id AND ar.id=au.role_id WHERE au.user_id=users.id),'[]'::jsonb)`
