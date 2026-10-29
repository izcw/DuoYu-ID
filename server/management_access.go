package main

import (
	"net/http"
)

// Only the single super administrator may enter management APIs.
func (s *Server) managementAllowed(r *http.Request, u User) bool { return false }
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
	if _, yes := s.require(w, r, true); !yes {
		return
	}
	fail(w, 403, "系统仅允许一位超级管理员，不支持分配其他管理角色")
}
