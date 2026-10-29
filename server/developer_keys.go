package main

import (
	"net/http"
	"strings"
	"time"
)

func (s *Server) updateDeveloperKey(w http.ResponseWriter, r *http.Request) {
	owner, yes := s.require(w, r, true)
	if !yes {
		return
	}
	var in struct {
		Name      string   `json:"name"`
		Scopes    []string `json:"scopes"`
		All       bool     `json:"all_projects"`
		Apps      []string `json:"app_ids"`
		Permanent bool     `json:"permanent"`
		Days      int      `json:"days"`
	}
	if decode(r, &in) != nil || len(strings.TrimSpace(in.Name)) == 0 || len([]rune(in.Name)) > 100 || !validKeyScopes(in.Scopes) || len(in.Apps) > 100 || !in.Permanent && (in.Days < 1 || in.Days > 365) {
		fail(w, 400, "请检查凭证名称、权限、项目和有效期")
		return
	}
	var expiry *time.Time
	if !in.Permanent {
		v := time.Now().Add(time.Duration(in.Days) * 24 * time.Hour)
		expiry = &v
	}
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		fail(w, 503, "修改凭证失败")
		return
	}
	defer tx.Rollback(r.Context())
	tag, err := tx.Exec(r.Context(), "UPDATE api_keys SET name=$2,scopes=$3,all_projects=$4,expires_at=$5,app_id=NULL WHERE id=$1 AND NOT revoked", r.PathValue("key"), strings.TrimSpace(in.Name), in.Scopes, in.All, expiry)
	if err != nil {
		fail(w, 503, "修改凭证失败")
		return
	}
	if tag.RowsAffected() != 1 {
		fail(w, 404, "凭证不存在或已撤销")
		return
	}
	if _, err = tx.Exec(r.Context(), "DELETE FROM management_api_grants WHERE key_id=$1", r.PathValue("key")); err != nil {
		fail(w, 503, "修改范围失败")
		return
	}
	seen := map[string]bool{}
	for _, id := range in.Apps {
		if seen[id] {
			fail(w, 400, "项目重复")
			return
		}
		seen[id] = true
		tag, err := tx.Exec(r.Context(), "INSERT INTO management_api_grants(key_id,app_id) SELECT $1,id FROM apps WHERE id=$2", r.PathValue("key"), id)
		if err != nil || tag.RowsAffected() != 1 {
			fail(w, 400, "项目不存在")
			return
		}
	}
	if tx.Commit(r.Context()) != nil {
		fail(w, 503, "修改凭证失败")
		return
	}
	s.audit(r.Context(), owner.ID, "developer.key.updated", r.PathValue("key"), true)
	ok(w, nil)
}
