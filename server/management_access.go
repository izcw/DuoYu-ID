package main

import (
	"net/http"
	"strings"
)

// Explicit allowlists: new administration endpoints default to super-admin only.
func (s *Server) managementAllowed(r *http.Request, u User) bool {
	p := r.URL.Path
	if u.ManagementRole == "auditor" {
		return r.Method == "GET" && (p == "/api/admin/audit" || p == "/api/admin/diagnostics" || p == "/api/admin/logout-events" || strings.HasPrefix(p, "/api/admin/logout-events/") && strings.HasSuffix(p, "/attempts"))
	}
	if u.ManagementRole != "maintainer" {
		return false
	}
	if p == "/api/admin/apps" {
		return r.Method == "GET"
	}
	id := r.PathValue("id")
	base := "/api/admin/apps/" + id
	if id == "" {
		return false
	}
	allowed := (p == base+"/checks" && r.Method == "GET") || (p == base && (r.Method == "GET" || r.Method == "PUT")) || (p == base+"/grants" && (r.Method == "GET" || r.Method == "PUT")) || (p == base+"/users" && r.Method == "GET") || (p == base+"/rotate" && r.Method == "POST")
	if !allowed {
		return false
	}
	var assigned bool
	e := s.db.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM app_maintainers WHERE user_id=$1 AND app_id=$2)", u.ID, id).Scan(&assigned)
	return e == nil && assigned
}
func (s *Server) managementAccess(w http.ResponseWriter, r *http.Request) {
	u, yes := s.require(w, r, true)
	if !yes {
		return
	}
	if !u.Admin {
		fail(w, 403, "需要系统管理员权限")
		return
	}
	s.queryList(w, r, "SELECT management_role,ARRAY(SELECT app_id FROM app_maintainers WHERE user_id=users.id) AS app_ids FROM users WHERE id=$1", r.PathValue("id"))
}
func (s *Server) updateManagementAccess(w http.ResponseWriter, r *http.Request) {
	u, yes := s.require(w, r, true)
	if !yes {
		return
	}
	if !u.Admin {
		fail(w, 403, "需要系统管理员权限")
		return
	}
	var in struct {
		Role string   `json:"role"`
		Apps []string `json:"app_ids"`
	}
	if decode(r, &in) != nil || (in.Role != "none" && in.Role != "auditor" && in.Role != "maintainer") || len(in.Apps) > 100 {
		fail(w, 400, "权限配置无效")
		return
	}
	tx, e := s.db.Begin(r.Context())
	if e != nil {
		fail(w, 503, "暂时无法保存")
		return
	}
	defer tx.Rollback(r.Context())
	tag, e := tx.Exec(r.Context(), "UPDATE users SET management_role=$2 WHERE id=$1 AND NOT admin", r.PathValue("id"), in.Role)
	if e != nil || tag.RowsAffected() != 1 {
		fail(w, 400, "账号不存在或为系统管理员")
		return
	}
	if _, e = tx.Exec(r.Context(), "DELETE FROM app_maintainers WHERE user_id=$1", r.PathValue("id")); e != nil {
		fail(w, 503, "暂时无法保存")
		return
	}
	if in.Role == "maintainer" {
		for _, id := range in.Apps {
			if _, e = tx.Exec(r.Context(), "INSERT INTO app_maintainers(user_id,app_id) VALUES($1,$2) ON CONFLICT DO NOTHING", r.PathValue("id"), id); e != nil {
				fail(w, 400, "接入项目不存在")
				return
			}
		}
	}
	if tx.Commit(r.Context()) != nil {
		fail(w, 503, "暂时无法保存")
		return
	}
	s.audit(r.Context(), u.ID, "admin.management.access", r.PathValue("id"), true)
	ok(w, nil)
}
