package main

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func testLegalConsent(t *testing.T, s *Server, origin string) {
	for _, path := range []string{"/legal/terms", "/legal/privacy"} {
		r := httptest.NewRequest("GET", origin+path, nil)
		w := httptest.NewRecorder()
		s.routes().ServeHTTP(w, r)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "2405824084@qq.com") || !strings.Contains(w.Body.String(), "18周岁") {
			t.Fatalf("legal page missing required facts: %s", path)
		}
	}
	for _, path := range []string{"/api/auth/login", "/api/auth/code", "/api/auth/authenticator", "/api/auth/passkey/begin", "/api/auth/passkey/finish", "/api/auth/qr/device/finish", "/api/auth/qr/test/poll", "/api/auth/qr/test/approve", "/api/auth/qr", "/api/auth/continue", "/api/auth/qr/device/begin"} {
		r := httptest.NewRequest("POST", origin+path, strings.NewReader("{}"))
		r.Header.Set("X-Duoyu-Request", "1")
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		s.routes().ServeHTTP(w, r)
		if w.Code != 400 || !strings.Contains(w.Body.String(), "请阅读并同意服务条款和隐私政策") {
			t.Fatalf("missing consent not rejected: %s", path)
		}
	}
	old := httptest.NewRequest("POST", origin+"/api/auth/login", strings.NewReader("{}"))
	old.Header.Set("X-Duoyu-Request", "1")
	old.Header.Set("Origin", origin)
	old.Header.Set("X-Duoyu-Adult", "true")
	old.Header.Set("X-Duoyu-Legal-Version", "outdated")
	oldResult := httptest.NewRecorder()
	s.routes().ServeHTTP(oldResult, old)
	if oldResult.Code != 400 {
		t.Fatal("old legal version accepted")
	}
	for _, adult := range []string{"false", ""} {
		r := httptest.NewRequest("POST", origin+"/api/auth/login", strings.NewReader("{}"))
		r.Header.Set("X-Duoyu-Request", "1")
		r.Header.Set("Origin", origin)
		r.Header.Set("X-Duoyu-Legal-Version", legalVersion)
		r.Header.Set("X-Duoyu-Adult", adult)
		w := httptest.NewRecorder()
		s.routes().ServeHTTP(w, r)
		if w.Code != 400 {
			t.Fatal("adult declaration not required")
		}
	}
	raw, _ := json.Marshal(map[string]string{"mode": "password", "email": "admin@example.com", "password": "test-admin-password-123"})
	r := httptest.NewRequest("POST", origin+"/api/auth/login", bytes.NewReader(raw))
	r.Header.Set("Origin", origin)
	r.Header.Set("X-Duoyu-Request", "1")
	r.Header.Set("X-Duoyu-Legal-Version", legalVersion)
	r.Header.Set("X-Duoyu-Adult", "true")
	w := httptest.NewRecorder()
	s.routes().ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal("consented login failed")
	}
	var count int
	if err := s.db.QueryRow(r.Context(), "SELECT count(*) FROM user_legal_acceptances WHERE version=$1 AND adult_asserted", legalVersion).Scan(&count); err != nil || count < 1 {
		t.Fatal("consent was not persisted", err)
	}
}
