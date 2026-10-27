package main

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// Opt-in response envelope preserves array responses for existing integrations.
func (s *Server) paginatedList(w http.ResponseWriter, r *http.Request, sql string, args []any) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	page = max(1, min(page, 1000000))
	size, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if size < 1 {
		size = 20
	}
	size = min(size, 100)
	base := strings.TrimSuffix(sql, " LIMIT 200")
	query := fmt.Sprintf(" FROM (%s) records WHERE to_jsonb(records)::text ILIKE $%d", base, len(args)+1)
	args = append(args, "%"+truncate(r.URL.Query().Get("q"), 100)+"%")
	if status := r.URL.Query().Get("status"); status != "" {
		args = append(args, status)
		query += fmt.Sprintf(" AND to_jsonb(records)->>'status'=$%d", len(args))
	}
	var total int64
	if err := s.db.QueryRow(r.Context(), "SELECT count(*)"+query, args...).Scan(&total); err != nil {
		fail(w, 500, "查询失败")
		return
	}
	args = append(args, size, (page-1)*size)
	// Timestamp text is ISO formatted by PostgreSQL. ID provides a stable tie break.
	order := " ORDER BY coalesce(to_jsonb(records)->>'last_login_at',to_jsonb(records)->>'created_at') DESC, to_jsonb(records)->>'id' DESC"
	rows, err := s.db.Query(r.Context(), "SELECT records.*"+query+order+fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)-1, len(args)), args...)
	if err != nil {
		fail(w, 500, "查询失败")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		values, e := rows.Values()
		if e != nil {
			fail(w, 500, "读取失败")
			return
		}
		item := map[string]any{}
		for i, f := range rows.FieldDescriptions() {
			item[f.Name] = values[i]
		}
		items = append(items, item)
	}
	if rows.Err() != nil {
		fail(w, 500, "读取失败")
		return
	}
	ok(w, map[string]any{"items": items, "total": total, "page": page, "page_size": size})
}
