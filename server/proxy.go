package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

type clientIPKey struct{}

func parseProxies(value string) ([]netip.Prefix, error) {
	var prefixes []netip.Prefix
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		p, e := netip.ParsePrefix(part)
		if e != nil {
			return nil, fmt.Errorf("invalid trusted proxy CIDR")
		}
		if p.Bits() == 0 {
			return nil, fmt.Errorf("unrestricted proxy CIDR is forbidden")
		}
		prefixes = append(prefixes, p)
	}
	return prefixes, nil
}
func (s *Server) trusted(ip netip.Addr) bool {
	for _, p := range s.proxies {
		if p.Contains(ip.Unmap()) {
			return true
		}
	}
	return false
}
func (s *Server) clientIP(r *http.Request) string {
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	peer, e := netip.ParseAddr(host)
	if e != nil {
		return host
	}
	peer = peer.Unmap()
	if !s.trusted(peer) {
		return peer.String()
	}
	raw := r.Header.Get("X-Forwarded-For")
	if raw == "" || len(raw) > 2048 {
		return peer.String()
	}
	chain := strings.Split(raw, ",")
	if len(chain) > 20 {
		return peer.String()
	}
	ips := make([]netip.Addr, len(chain))
	for i, part := range chain {
		ip, e := netip.ParseAddr(strings.TrimSpace(part))
		if e != nil {
			return peer.String()
		}
		ips[i] = ip.Unmap()
	}
	for i := len(ips) - 1; i >= 0; i-- {
		if !s.trusted(ips[i]) {
			return ips[i].String()
		}
	}
	return ips[0].String()
}
func (s *Server) withClientIP(r *http.Request) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), clientIPKey{}, s.clientIP(r)))
}
