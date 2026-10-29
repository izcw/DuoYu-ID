package main

import (
	"context"
	"crypto/subtle"
	"net/http"
	"slices"
	"strings"
)

func allLoginMethods() []string { return []string{"password", "code", "passkey", "qr", "totp"} }
func (s *Server) loginAMRAllowed(ctx context.Context, appID, amr string) bool {
	app, err := s.app(ctx, appID)
	if err != nil {
		return false
	}
	method := ""
	fields := strings.Fields(amr)
	switch {
	case slices.Contains(fields, "totp"):
		method = "totp"
	case slices.Contains(fields, "qr"):
		method = "qr"
	case slices.Contains(fields, "hwk"):
		method = "passkey"
	case slices.Contains(fields, "pwd"):
		method = "password"
	case slices.Contains(fields, "otp"):
		method = "code"
	}
	// Legacy credentials without AMR are usable only under an unrestricted policy.
	if method == "" {
		for _, m := range allLoginMethods() {
			if !slices.Contains(app.LoginMethods, m) {
				return false
			}
		}
		return true
	}
	return slices.Contains(app.LoginMethods, method)
}
func (s *Server) checkProjectMethod(w http.ResponseWriter, r *http.Request, id, method string) bool {
	if id == "" {
		id = r.Header.Get("X-Duoyu-Auth-Request")
	}
	if id == "" {
		return true
	}
	cookie, err := r.Cookie(flowCookieName(id))
	if err != nil || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(s.mac(id))) != 1 {
		fail(w, 403, "登录请求不属于当前浏览器")
		return false
	}
	req, err := s.AuthRequestByID(r.Context(), id)
	if err != nil {
		fail(w, 400, "登录请求已过期")
		return false
	}
	app, err := s.app(r.Context(), req.GetClientID())
	if err != nil {
		fail(w, 403, "项目不可用")
		return false
	}
	if !slices.Contains(app.LoginMethods, method) {
		fail(w, 403, "该项目未启用此登录方式")
		return false
	}
	return true
}
