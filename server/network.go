package main

import (
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
)

func localIPv4s() []string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []string
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addresses, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, address := range addresses {
			ip, _, err := net.ParseCIDR(address.String())
			if err != nil || ip.To4() == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				continue
			}
			out = append(out, ip.String())
		}
	}
	return out
}

func (s *Server) allowedBrowserOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == s.origin {
		return true
	}
	if !s.dev {
		return false
	}
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "http" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.Host != r.Host {
		return false
	}
	canonical, err := url.Parse(s.origin)
	if err != nil || canonical.Scheme != "http" {
		return false
	}
	port := u.Port()
	if port == "" {
		port = "80"
	}
	canonicalPort := canonical.Port()
	if canonicalPort == "" {
		canonicalPort = "80"
	}
	if port != canonicalPort {
		return false
	}
	// A LAN issuer must still allow same-origin localhost development pages.
	if u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1" {
		return true
	}
	ip := net.ParseIP(u.Hostname())
	if ip == nil {
		return false
	}
	for _, local := range localIPv4s() {
		if ip.Equal(net.ParseIP(local)) {
			return true
		}
	}
	return false
}

func (s *Server) logAccessAddresses(address string) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return
	}
	if host == "" || host == "0.0.0.0" || host == "::" || host == "127.0.0.1" || host == "localhost" {
		slog.Info("Local", "url", "http://localhost:"+port+"/login")
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		for _, ip := range localIPv4s() {
			slog.Info("Network", "url", "http://"+net.JoinHostPort(ip, port)+"/login")
		}
	} else if !strings.HasPrefix(host, "127.") && net.ParseIP(host) != nil {
		slog.Info("Network", "url", "http://"+net.JoinHostPort(host, port)+"/login")
	}
}

// HTTP development issuers must identify this machine, never an arbitrary site.
func localDevelopmentHost(host string) bool {
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsPrivate() {
		return false
	}
	for _, local := range localIPv4s() {
		if ip.Equal(net.ParseIP(local)) {
			return true
		}
	}
	return false
}
