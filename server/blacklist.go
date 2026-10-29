package main

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (s *Server) projectBlocked(ctx context.Context, email, app string) (bool, error) {
	var blocked bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM app_blacklist b WHERE b.app_id=$1 AND (b.email=$2 OR b.user_id=(SELECT id FROM users WHERE email=$2)))`, app, email).Scan(&blocked)
	return blocked, err
}

func (s *Server) checkLoginProject(w http.ResponseWriter, r *http.Request, email, id string) bool {
	if id == "" {
		return true
	}
	cookie, err := r.Cookie(flowCookieName(id))
	if err != nil || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(s.mac(id))) != 1 {
		fail(w, 403, "登录请求不属于当前浏览器")
		return false
	}
	request, err := s.AuthRequestByID(r.Context(), id)
	if err != nil {
		fail(w, 400, "登录请求已过期，请返回项目重新登录")
		return false
	}
	if _, err = s.app(r.Context(), request.GetClientID()); err != nil {
		fail(w, 403, "项目不可用")
		return false
	}
	blocked, err := s.projectBlocked(r.Context(), email, request.GetClientID())
	if err != nil {
		fail(w, 503, "暂时无法检查项目权限")
		return false
	}
	if blocked {
		fail(w, 403, "此账号已被该项目加入黑名单，无法注册或登录此项目")
		return false
	}
	return true
}

func (s *Server) blacklist(w http.ResponseWriter, r *http.Request) {
	if _, yes := s.require(w, r, true); !yes {
		return
	}
	s.queryList(w, r, `SELECT b.identifier,b.email,u.account_id,u.name FROM app_blacklist b LEFT JOIN users u ON u.id=b.user_id WHERE b.app_id=$1 ORDER BY b.created_at,b.identifier`, r.PathValue("id"))
}

var accountNumberPattern = regexp.MustCompile(`^[1-9][0-9]{9}$`)

func (s *Server) setBlacklist(w http.ResponseWriter, r *http.Request) {
	actor, yes := s.require(w, r, true)
	if !yes {
		return
	}
	var in struct {
		Entries []string `json:"entries"`
	}
	if decode(r, &in) != nil || len(in.Entries) > 500 {
		fail(w, 400, "最多添加 500 个账号 ID 或邮箱")
		return
	}
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		fail(w, 503, "暂时无法保存")
		return
	}
	defer tx.Rollback(r.Context())
	app := r.PathValue("id")
	var appID string
	if err = tx.QueryRow(r.Context(), "SELECT id FROM apps WHERE id=$1 FOR UPDATE", app).Scan(&appID); err != nil {
		fail(w, 404, "项目不存在")
		return
	}
	entries := []string{}
	for _, raw := range in.Entries {
		identifier := strings.TrimSpace(raw)
		var uid *string
		email := ""
		if accountNumberPattern.MatchString(identifier) {
			var id string
			if err = tx.QueryRow(r.Context(), "SELECT id FROM users WHERE account_id=$1", identifier).Scan(&id); err != nil {
				fail(w, 400, "名单中存在不存在的账号 ID")
				return
			}
			uid = &id
		} else {
			email = normalizeEmail(identifier)
			if email == "" {
				fail(w, 400, "请填写有效的 10 位账号 ID 或邮箱")
				return
			}
			identifier = email
			var id string
			err = tx.QueryRow(r.Context(), "SELECT id FROM users WHERE email=$1", email).Scan(&id)
			if err == nil {
				uid = &id
			} else if !errors.Is(err, pgx.ErrNoRows) {
				fail(w, 503, "暂时无法保存")
				return
			}
		}
		entries = append(entries, identifier)
		if _, err = tx.Exec(r.Context(), `INSERT INTO app_blacklist(app_id,identifier,email,user_id) VALUES($1,$2,$3,$4) ON CONFLICT(app_id,identifier) DO NOTHING`, app, identifier, email, uid); err != nil {
			fail(w, 500, "黑名单保存失败")
			return
		}
	}
	if _, err = tx.Exec(r.Context(), "DELETE FROM app_blacklist WHERE app_id=$1 AND NOT(identifier=ANY($2))", app, entries); err != nil {
		fail(w, 500, "黑名单保存失败")
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		fail(w, 500, "黑名单保存失败")
		return
	}
	s.audit(r.Context(), actor.ID, "admin.app.blacklist", app, true)
	ok(w, nil)
}
