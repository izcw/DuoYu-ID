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
	"github.com/jackc/pgx/v5"
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
	if errors.Is(e, pgx.ErrNoRows) {
		return false, nil
	}
	if e != nil {
		return false, e
	}
	attempts++
	if _, e = tx.Exec(ctx, "UPDATE logout_jobs SET attempts=$2,next_attempt=now()+interval '30 seconds' WHERE id=$1", id, attempts); e != nil {
		return true, e
	}
	if e = tx.Commit(ctx); e != nil {
		return true, e
	}
	started := time.Now()
	statusCode, errorClass := s.sendLogout(ctx, id, client, uid, hash, endpoint)
	state := "queued"
	if errorClass == "" {
		state = "delivered"
	} else if attempts >= 10 {
		state = "failed"
	}
	delay := min(3600, 5*(1<<min(attempts, 10)))
	resultTx, e := s.db.Begin(ctx)
	if e != nil {
		return true, e
	}
	defer resultTx.Rollback(ctx)
	_, e = resultTx.Exec(ctx, "INSERT INTO logout_attempts(job_id,http_status,error_class,duration_ms) VALUES($1,$2,$3,$4)", id, statusCode, errorClass, time.Since(started).Milliseconds())
	if e != nil {
		return true, e
	}
	_, e = resultTx.Exec(ctx, "UPDATE logout_jobs SET status=$2,next_attempt=now()+$3::interval,last_error=$4,http_status=$5,completed_at=CASE WHEN $2='delivered' THEN now() ELSE NULL END WHERE id=$1", id, state, fmt.Sprintf("%d seconds", delay), errorClass, statusCode)
	if e != nil {
		return true, e
	}
	e = resultTx.Commit(ctx)
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

// Persist only bounded classifications; response bodies and signed tokens are never logged.
func (s *Server) sendLogout(ctx context.Context, id int64, client, uid, hash, endpoint string) (int, string) {
	if !s.validLogoutEndpoint(endpoint) {
		return 0, "invalid_endpoint"
	}
	token, e := s.logoutToken(id, client, uid, hash)
	if e != nil {
		return 0, "signing"
	}
	req, e := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(url.Values{"logout_token": {token}}.Encode()))
	if e != nil {
		return 0, "invalid_endpoint"
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	c := s.logoutHTTPClient()
	defer c.CloseIdleConnections()
	res, e := c.Do(req)
	if e != nil {
		var timeout net.Error
		if errors.As(e, &timeout) && timeout.Timeout() {
			return 0, "timeout"
		}
		return 0, "transport"
	}
	defer res.Body.Close()
	io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return res.StatusCode, "http_status"
	}
	return res.StatusCode, ""
}
func (s *Server) retryLogout(w http.ResponseWriter, r *http.Request) {
	u, yes := s.require(w, r, true)
	if !yes {
		return
	}
	id, e := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if e != nil || id < 1 {
		fail(w, 400, "事件标识无效")
		return
	}
	result, e := s.db.Exec(r.Context(), "UPDATE logout_jobs SET status='queued',attempts=0,next_attempt=now(),retry_count=retry_count+1 WHERE id=$1 AND status='failed'", id)
	if e != nil {
		fail(w, 503, "暂时无法重试")
		return
	}
	if result.RowsAffected() != 1 {
		fail(w, 409, "仅失败事件可重新投递，请刷新状态")
		return
	}
	s.audit(r.Context(), u.ID, "admin.logout.retry", strconv.FormatInt(id, 10), true)
	ok(w, nil)
}
func (s *Server) logoutAttempts(w http.ResponseWriter, r *http.Request) {
	if _, yes := s.require(w, r, true); !yes {
		return
	}
	id, e := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if e != nil || id < 1 {
		fail(w, 400, "事件标识无效")
		return
	}
	s.queryList(w, r, "SELECT id,created_at,http_status,error_class,duration_ms FROM logout_attempts WHERE job_id=$1 ORDER BY id DESC LIMIT 200", id)
}
func requestID(ctx context.Context) string { v, _ := ctx.Value(requestIDKey{}).(string); return v }
