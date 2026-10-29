package main

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/zitadel/oidc/v3/pkg/oidc"
)

var projectRoleKey = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)

type projectRoleInput struct {
	Name        string `json:"name"`
	Key         string `json:"role_key"`
	Description string `json:"description"`
	Default     bool   `json:"is_default"`
}

func roleDBError(w http.ResponseWriter, err error) {
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		fail(w, 409, "角色字段已存在，请使用其他字段")
		return
	}
	fail(w, 503, "角色配置暂不可用，请重试")
}
func (s *Server) listAppRoles(w http.ResponseWriter, r *http.Request) {
	if _, yes := s.require(w, r, true); !yes {
		return
	}
	s.queryList(w, r, `SELECT r.id,r.name,r.role_key,r.description,r.is_default,r.created_at,(SELECT count(*) FROM app_users u WHERE u.app_id=r.app_id AND u.role_id=r.id) AS user_count FROM app_roles r WHERE r.app_id=$1 ORDER BY r.is_default DESC,r.created_at,r.id`, r.PathValue("id"))
}
func (s *Server) saveAppRole(w http.ResponseWriter, r *http.Request) {
	actor, yes := s.require(w, r, true)
	if !yes {
		return
	}
	var in projectRoleInput
	if decode(r, &in) != nil {
		fail(w, 400, "请求格式错误")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Key = strings.TrimSpace(in.Key)
	in.Description = strings.TrimSpace(in.Description)
	if len([]rune(in.Name)) < 1 || len([]rune(in.Name)) > 40 || !projectRoleKey.MatchString(in.Key) || len([]rune(in.Description)) < 1 || len([]rune(in.Description)) > 500 {
		fail(w, 400, "名称须为 1–40 字；字段以小写字母开头，仅含小写字母、数字、下划线，最多 40 位；描述须为 1–500 字")
		return
	}
	ctx := r.Context()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		roleDBError(w, err)
		return
	}
	defer tx.Rollback(ctx)
	app, id := r.PathValue("id"), r.PathValue("role")
	var exists string
	if err = tx.QueryRow(ctx, "SELECT id FROM apps WHERE id=$1 FOR UPDATE", app).Scan(&exists); err != nil {
		fail(w, 404, "项目不存在")
		return
	}
	if id != "" {
		var wasDefault bool
		err = tx.QueryRow(ctx, "SELECT is_default FROM app_roles WHERE app_id=$1 AND id=$2", app, id).Scan(&wasDefault)
		if err != nil {
			fail(w, 404, "角色不存在")
			return
		}
		if wasDefault && !in.Default {
			fail(w, 400, "请先将其他角色设为默认，项目必须保留默认角色")
			return
		}
	} else {
		var count int
		err = tx.QueryRow(ctx, "SELECT count(*) FROM app_roles WHERE app_id=$1", app).Scan(&count)
		if err != nil {
			roleDBError(w, err)
			return
		}
		if count >= 100 {
			fail(w, 400, "每个项目最多配置 100 个角色")
			return
		}
	}
	if in.Default {
		if _, err = tx.Exec(ctx, "UPDATE app_roles SET is_default=false WHERE app_id=$1 AND is_default", app); err != nil {
			roleDBError(w, err)
			return
		}
	}
	if id == "" {
		id = randomID()
		_, err = tx.Exec(ctx, "INSERT INTO app_roles(app_id,id,name,role_key,description,is_default) VALUES($1,$2,$3,$4,$5,$6)", app, id, in.Name, in.Key, in.Description, in.Default)
	} else {
		_, err = tx.Exec(ctx, "UPDATE app_roles SET name=$3,role_key=$4,description=$5,is_default=$6 WHERE app_id=$1 AND id=$2", app, id, in.Name, in.Key, in.Description, in.Default)
	}
	if err != nil {
		roleDBError(w, err)
		return
	}
	if err = tx.Commit(ctx); err != nil {
		roleDBError(w, err)
		return
	}
	s.audit(ctx, actor.ID, "admin.app.role.saved", app+":"+id, true)
	ok(w, map[string]string{"id": id})
}
func (s *Server) deleteAppRole(w http.ResponseWriter, r *http.Request) {
	actor, yes := s.require(w, r, true)
	if !yes {
		return
	}
	var in struct {
		Replacement string `json:"replacement_id"`
	}
	if decode(r, &in) != nil {
		fail(w, 400, "请指定替代角色")
		return
	}
	app, id := r.PathValue("id"), r.PathValue("role")
	if id == in.Replacement || in.Replacement == "" {
		fail(w, 400, "必须选择另一个替代角色")
		return
	}
	ctx := r.Context()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		roleDBError(w, err)
		return
	}
	defer tx.Rollback(ctx)
	var key string
	if err = tx.QueryRow(ctx, "SELECT id FROM apps WHERE id=$1 FOR UPDATE", app).Scan(&key); err != nil {
		fail(w, 404, "项目不存在")
		return
	}
	var wasDefault bool
	if err = tx.QueryRow(ctx, "SELECT is_default FROM app_roles WHERE app_id=$1 AND id=$2", app, id).Scan(&wasDefault); err != nil {
		fail(w, 404, "角色不存在")
		return
	}
	if err = tx.QueryRow(ctx, "SELECT id FROM app_roles WHERE app_id=$1 AND id=$2", app, in.Replacement).Scan(&key); err != nil {
		fail(w, 400, "替代角色不存在或不属于该项目")
		return
	}
	if _, err = tx.Exec(ctx, "UPDATE app_users SET role_id=$3 WHERE app_id=$1 AND role_id=$2", app, id, in.Replacement); err != nil {
		roleDBError(w, err)
		return
	}
	if _, err = tx.Exec(ctx, "DELETE FROM app_roles WHERE app_id=$1 AND id=$2", app, id); err != nil {
		roleDBError(w, err)
		return
	}
	if wasDefault {
		if _, err = tx.Exec(ctx, "UPDATE app_roles SET is_default=true WHERE app_id=$1 AND id=$2", app, in.Replacement); err != nil {
			roleDBError(w, err)
			return
		}
	}
	if err = tx.Commit(ctx); err != nil {
		roleDBError(w, err)
		return
	}
	s.audit(ctx, actor.ID, "admin.app.role.deleted", app+":"+id, true)
	ok(w, nil)
}
func (s *Server) setAppUserRole(w http.ResponseWriter, r *http.Request) {
	actor, yes := s.require(w, r, true)
	if !yes {
		return
	}
	var in struct {
		Role string `json:"role_id"`
	}
	if decode(r, &in) != nil || in.Role == "" {
		fail(w, 400, "请选择项目角色")
		return
	}
	app, uid := r.PathValue("id"), r.PathValue("user")
	tag, err := s.db.Exec(r.Context(), "UPDATE app_users SET role_id=$3 WHERE app_id=$1 AND user_id=$2 AND EXISTS(SELECT 1 FROM app_roles WHERE app_id=$1 AND id=$3)", app, uid, in.Role)
	if err != nil {
		roleDBError(w, err)
		return
	}
	if tag.RowsAffected() != 1 {
		fail(w, 400, "项目用户或角色不存在")
		return
	}
	s.audit(r.Context(), actor.ID, "admin.app.user.role", app+":"+uid, true)
	ok(w, nil)
}
func (s *Server) fillProjectRole(ctx context.Context, info *oidc.UserInfo, uid, app string, scopes []string) error {
	if !slices.Contains(scopes, "profile") {
		return nil
	}
	var id, name, key, description string
	err := s.db.QueryRow(ctx, `SELECT r.id,r.name,r.role_key,r.description FROM app_roles r WHERE r.app_id=$1 AND r.id=COALESCE((SELECT role_id FROM app_users WHERE app_id=$1 AND user_id=$2),(SELECT id FROM app_roles WHERE app_id=$1 AND is_default))`, app, uid).Scan(&id, &name, &key, &description)
	if err != nil {
		return err
	}
	info.AppendClaims("roles", []string{key})
	info.AppendClaims("project_role", map[string]string{"id": id, "name": name, "field": key, "description": description})
	return nil
}
