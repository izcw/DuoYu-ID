package main

import (
	"net/http/httptest"
	"testing"
)

func TestTrustedProxy(t *testing.T) {
	prefixes, e := parseProxies("10.0.0.0/8,127.0.0.1/32")
	if e != nil {
		t.Fatal(e)
	}
	s := &Server{proxies: prefixes}
	for _, v := range []struct{ peer, header, want string }{
		{"203.0.113.2:123", "1.1.1.1", "203.0.113.2"},
		{"127.0.0.1:123", "1.1.1.1, 203.0.113.3, 10.0.0.3", "203.0.113.3"},
		{"127.0.0.1:123", "invalid", "127.0.0.1"},
		{"127.0.0.1:123", "2001:db8::1", "2001:db8::1"},
	} {
		r := httptest.NewRequest("GET", "http://localhost", nil)
		r.RemoteAddr = v.peer
		r.Header.Set("X-Forwarded-For", v.header)
		if got := remoteIP(s.withClientIP(r)); got != v.want {
			t.Errorf("got %s want %s", got, v.want)
		}
	}
	if _, e = parseProxies("0.0.0.0/0"); e == nil {
		t.Fatal("unrestricted proxy accepted")
	}
}
