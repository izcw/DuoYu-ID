package main

import (
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestDecodeSingleJSON(t *testing.T) {
	for _, body := range []string{`{"name":"ok"} {"name":"extra"}`, `{"name":"ok"} trailing`, `{"unknown":1}`, `null {}`} {
		var value struct {
			Name string `json:"name"`
		}
		if decode(httptest.NewRequest("POST", "/", strings.NewReader(body)), &value) == nil {
			t.Errorf("accepted ambiguous JSON: %s", body)
		}
	}
	var value struct {
		Name string `json:"name"`
	}
	if err := decode(httptest.NewRequest("POST", "/", strings.NewReader("{\"name\":\"ok\"} \n")), &value); err != nil {
		t.Fatal(err)
	}
}
func TestTruncateUTF8Boundary(t *testing.T) {
	for _, n := range []int{-1, 0, 1, 2, 3, 4, 5, 6, 7, 20} {
		got := truncate("中文abc", n)
		if !utf8.ValidString(got) {
			t.Fatalf("invalid UTF-8 at %d", n)
		}
		if n >= 0 && len(got) > n {
			t.Fatalf("exceeded byte limit %d", n)
		}
	}
}
