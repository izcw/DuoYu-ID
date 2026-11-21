package main

import (
	"encoding/hex"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
)

// Confidential projects may check only identities that previously signed into that project.
func (s *Server) memberStatus(w http.ResponseWriter, r *http.Request) {
	app, secret, basic := r.BasicAuth()
	if !basic {
		fail(w, 401, "客户端不可用")
		return
	}
	client, err := s.app(r.Context(), app)
	if err != nil && !errors.Is(err, errAppDisabled) && !errors.Is(err, pgx.ErrNoRows) {
		fail(w, 503, "暂时无法核验准入")
		return
	}
	if errors.Is(err, pgx.ErrNoRows) || !s.matchesClientSecret(client, secret) {
		fail(w, 401, "客户端不可用")
		return
	}
	if errors.Is(err, errAppDisabled) {
		ok(w, map[string]any{"active": false, "generation": 0})
		return
	}
	var input struct {
		Owner string `json:"owner"`
		SID   string `json:"sid"`
	}
	if decode(r, &input) != nil || len(input.Owner) != 64 || len(input.SID) > 128 {
		fail(w, 400, "身份参数无效")
		return
	}
	if _, err := hex.DecodeString(input.Owner); err != nil {
		fail(w, 400, "身份参数无效")
		return
	}
	var active bool
	var generation int64
	err = s.db.QueryRow(r.Context(), `SELECT (`+projectAccessCondition+`) AND ($4='' OR EXISTS (
 SELECT 1 FROM sessions ss WHERE ss.user_id=u.id AND ss.expires_at>now()
 AND encode(sha256(convert_to('logout:'||ss.hash,'UTF8')),'hex')=$4)), au.access_generation
 FROM app_users au JOIN users u ON u.id=au.user_id JOIN apps a ON a.id=au.app_id
 WHERE au.app_id=$2 AND encode(sha256(convert_to('['||to_json($3::text)::text||','||to_json(au.user_id)::text||']','UTF8')),'hex')=$1`,
		input.Owner, app, s.origin, input.SID).Scan(&active, &generation)
	if errors.Is(err, pgx.ErrNoRows) {
		active = false
		generation = 0
	} else if err != nil {
		fail(w, 503, "暂时无法核验准入")
		return
	}
	ok(w, map[string]any{"active": active, "generation": generation})
}
