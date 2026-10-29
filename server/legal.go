package main

import (
	"net/http"
	"strings"
)

const legalVersion = "2026-10-29"

func legalAccepted(r *http.Request) bool {
	return r.Header.Get("X-Duoyu-Legal-Version") == legalVersion && r.Header.Get("X-Duoyu-Adult") == "true"
}
func legalProtected(r *http.Request) bool {
	if r.Method != "POST" {
		return false
	}
	switch r.URL.Path {
	case "/api/auth/code", "/api/auth/login", "/api/auth/reset", "/api/auth/authenticator", "/api/auth/passkey/begin", "/api/auth/passkey/finish", "/api/auth/qr/device/finish", "/api/auth/qr/device/begin", "/api/auth/qr", "/api/auth/continue":
		return true
	}
	return strings.HasPrefix(r.URL.Path, "/api/auth/qr/") && (strings.HasSuffix(r.URL.Path, "/poll") || strings.HasSuffix(r.URL.Path, "/approve"))
}
func (s *Server) legalPage(w http.ResponseWriter, r *http.Request) {
	privacy := strings.HasSuffix(r.URL.Path, "privacy")
	title := "服务条款"
	if privacy {
		title = "隐私政策"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	s.templates.ExecuteTemplate(w, "legal.html", map[string]any{"Title": title, "Privacy": privacy, "Version": legalVersion, "Operator": "多鱼账号个人项目运营者", "Contact": "2405824084@qq.com"})
}
