package main

import (
	"net"
	"net/http/httptest"
	"testing"
)

func TestDevelopmentLANOrigin(t *testing.T) {
	ips := localIPv4s()
	if len(ips) == 0 {
		t.Skip("no LAN interface")
	}
	lan := "http://" + ips[0] + ":5252"
	for _, c := range []struct {
		name, origin, host string
		dev, want          bool
	}{
		{"LAN", lan, ips[0] + ":5252", true, true},
		{"production rejects LAN", lan, ips[0] + ":5252", false, false},
		{"canonical", "http://localhost:5252", "localhost:5252", false, true},
		{"wrong port", "http://" + ips[0] + ":5253", ips[0] + ":5253", true, false},
		{"host mismatch", lan, "attacker.example:5252", true, false},
		{"unrelated site", "http://attacker.example:5252", "attacker.example:5252", true, false},
		{"null", "null", ips[0] + ":5252", true, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := &Server{origin: "http://localhost:5252", dev: c.dev}
			r := httptest.NewRequest("POST", "http://"+c.host+"/api/auth/login", nil)
			r.Header.Set("Origin", c.origin)
			if got := s.allowedBrowserOrigin(r); got != c.want {
				t.Fatalf("got %v want %v", got, c.want)
			}
		})
	}
}

func TestLocalBrowserWithLANIssuer(t *testing.T) {
	for _, tc := range []struct {
		origin, host string
		dev, want    bool
	}{
		{"http://localhost:5252", "localhost:5252", true, true},
		{"http://127.0.0.1:5252", "127.0.0.1:5252", true, true},
		{"http://localhost:5253", "localhost:5253", true, false},
		{"http://localhost:5252", "other.example:5252", true, false},
		{"http://localhost:5252", "localhost:5252", false, false},
	} {
		s := &Server{origin: "http://192.168.1.16:5252", dev: tc.dev}
		r := httptest.NewRequest("POST", "http://"+tc.host+"/api/auth/login", nil)
		r.Header.Set("Origin", tc.origin)
		if got := s.allowedBrowserOrigin(r); got != tc.want {
			t.Fatalf("%s got %v want %v", tc.origin, got, tc.want)
		}
	}
}

func TestDevelopmentIssuerHost(t *testing.T) {
	for host, want := range map[string]bool{"localhost": true, "127.0.0.1": true, "::1": true, "example.com": false, "8.8.8.8": false, "localhost.evil.example": false} {
		if got := localDevelopmentHost(host); got != want {
			t.Fatalf("%s got %v", host, got)
		}
	}
	for _, ip := range localIPv4s() {
		if net.ParseIP(ip).IsPrivate() && !localDevelopmentHost(ip) {
			t.Fatal("local private issuer rejected", ip)
		}
	}
}

func TestLANIssuerWebAuthnUnavailable(t *testing.T) {
	s := &Server{}
	w := httptest.NewRecorder()
	if s.requireWebAuthn(w) || w.Code != 503 {
		t.Fatal("missing WebAuthn provider must fail explicitly")
	}
}
