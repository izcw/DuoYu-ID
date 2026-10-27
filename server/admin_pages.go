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
	s.queryList(w, r, "SELECT id,name,description,redirects,logout_redirects,public,enabled,restricted,created_at,platform,refresh_enabled,backchannel_logout_uri FROM apps WHERE id=$1", r.PathValue("id"))
}
func (s *Server) adminUserDetail(w http.ResponseWriter, r *http.Request) {
	if _, yes := s.require(w, r, true); !yes {
		return
	}
	s.queryList(w, r, "SELECT id,email,name,avatar,admin,enabled,created_at FROM users WHERE id=$1", r.PathValue("id"))
}
func (s *Server) adminKeyDetail(w http.ResponseWriter, r *http.Request) {
	if _, yes := s.require(w, r, true); !yes {
		return
	}
	s.queryList(w, r, "SELECT k.id,k.name,k.app_id,a.name AS app_name,k.scopes,k.expires_at,k.revoked,k.created_at FROM api_keys k JOIN apps a ON a.id=k.app_id WHERE k.id=$1", r.PathValue("id"))
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
