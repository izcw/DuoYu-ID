package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	jose "github.com/go-jose/go-jose/v4"
)

func (s *Server) validLogoutEndpoint(raw string) bool {
	u, e := url.Parse(raw)
	return e == nil && u.Host != "" && u.User == nil && u.Fragment == "" && u.RawQuery == "" && (u.Scheme == "https" || (s.dev && u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1")))
}

// Resolve and pin the dial address to prevent DNS rebinding. Redirects and
// environment proxies cannot turn an administrator-configured callback into SSRF.
func (s *Server) logoutHTTPClient() *http.Client {
	transport := &http.Transport{DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, e := net.SplitHostPort(address)
		if e != nil {
			return nil, e
		}
		ips, e := net.DefaultResolver.LookupIPAddr(ctx, host)
		if e != nil {
			return nil, e
		}
		for _, entry := range ips {
			ip := entry.IP
			if !(s.dev && ip.IsLoopback()) && (!ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.String() == "169.254.169.254") {
				return nil, errors.New("callback address blocked")
			}
		}
		if len(ips) == 0 {
			return nil, errors.New("no callback address")
		}
		var last error
		for _, entry := range ips {
			conn, err := (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, network, net.JoinHostPort(entry.IP.String(), port))
			if err == nil {
				return conn, nil
			}
			last = err
		}
		return nil, last
	}}
	return &http.Client{Transport: transport, Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func (s *Server) logoutToken(id int64, client, uid, hash string) (string, error) {
	key := signingKey{s.signing}
	signer, e := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key.Key()}, (&jose.SignerOptions{}).WithType("logout+jwt").WithHeader("kid", key.ID()))
	if e != nil {
		return "", e
	}
	body, e := json.Marshal(map[string]any{"iss": s.origin, "aud": []string{client}, "iat": time.Now().Unix(), "jti": "logout-" + strconv.FormatInt(id, 10), "sub": uid, "sid": digest("logout:" + hash), "events": map[string]any{"http://schemas.openid.net/event/backchannel-logout": map[string]any{}}})
	if e != nil {
		return "", e
	}
	signed, e := signer.Sign(body)
	if e != nil {
		return "", e
	}
	return signed.CompactSerialize()
}

func (s *Server) deliverLogout(ctx context.Context) (bool, error) {
	tx, e := s.db.Begin(ctx)
	if e != nil {
		return false, e
	}
	defer tx.Rollback(ctx)
	var id int64
	var client, uid, hash, endpoint string
	var attempts int
	e = tx.QueryRow(ctx, `SELECT id,app_id,user_id,session_hash,endpoint,attempts FROM logout_jobs WHERE status='queued' AND next_attempt<=now() ORDER BY id FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&id, &client, &uid, &hash, &endpoint, &attempts)
	if e != nil {
		return false, nil
	}
	attempts++
	if _, e = tx.Exec(ctx, "UPDATE logout_jobs SET attempts=$2,next_attempt=now()+interval '30 seconds' WHERE id=$1", id, attempts); e != nil {
		return true, e
	}
	if e = tx.Commit(ctx); e != nil {
		return true, e
	}
	accepted := false
	if s.validLogoutEndpoint(endpoint) {
		token, err := s.logoutToken(id, client, uid, hash)
		if err == nil {
			req, err := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(url.Values{"logout_token": {token}}.Encode()))
			if err == nil {
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				httpClient := s.logoutHTTPClient()
				res, err := httpClient.Do(req)
				if err == nil {
					accepted = res.StatusCode >= 200 && res.StatusCode < 300
					io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
					res.Body.Close()
				}
				httpClient.CloseIdleConnections()
			}
		}
	}
	state := "queued"
	if accepted {
		state = "delivered"
	} else if attempts >= 10 {
		state = "failed"
	}
	delay := min(3600, 5*(1<<min(attempts, 10)))
	_, e = s.db.Exec(ctx, "UPDATE logout_jobs SET status=$2,next_attempt=now()+$3::interval WHERE id=$1", id, state, fmt.Sprintf("%d seconds", delay))
	return true, e
}

func (s *Server) startLogoutWorker(ctx context.Context, wg *sync.WaitGroup) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				for range 20 {
					worked, e := s.deliverLogout(ctx)
					if e != nil || !worked {
						break
					}
				}
			}
		}
	}()
}
