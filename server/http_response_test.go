package main

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestResponseContract(t *testing.T) {
	for _, status := range []int{200, 400, 401, 403, 404, 409, 412, 429, 500, 503} {
		w := httptest.NewRecorder()
		w.Header().Set("X-Request-ID", "request-test")
		if status == 200 {
			ok(w, map[string]string{"value": "ok"})
		} else {
			fail(w, status, "message")
		}
		var body APIResponse
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if w.Code != status || body.Code != status || body.RequestID != "request-test" || body.APIVersion != "1" {
			t.Fatalf("contract mismatch: %d", status)
		}
		if (body.Error == nil) != (status == 200) {
			t.Fatal("error shape mismatch")
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("API response must not be cached")
		}
	}
}
