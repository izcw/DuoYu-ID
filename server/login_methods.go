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
	return amrAllowedByMethods(amr, app.LoginMethods)
}

func amrAllowedByMethods(amr string, methods []string) bool {
	method := loginMethodForAMR(amr)
	if method != "" {
		return slices.Contains(methods, method)
	}
	// Legacy credentials without AMR require the unrestricted policy.
	for _, m := range allLoginMethods() {
		if !slices.Contains(methods, m) {
			return false
		}
	}
	return true
}

func loginMethodForAMR(amr string) string {
	fields := strings.Fields(amr)
	switch {
	case slices.Contains(fields, "totp"):
		return "totp"
	case slices.Contains(fields, "qr"):
		return "qr"
	case slices.Contains(fields, "hwk"):
		return "passkey"
	case slices.Contains(fields, "pwd"):
		return "password"
	case slices.Contains(fields, "otp"):
		return "code"
	}
	return ""
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
