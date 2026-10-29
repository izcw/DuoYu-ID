package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"
)

type protocolStateKey struct{}
type protocolState struct {
	dbFailed                        atomic.Bool
	originDenied, insufficientScope bool
}

// Observe failures before the OIDC library can collapse them into invalid_grant.
// No SQL, parameters or token values are recorded.
type protocolDBTracer struct{}

func (protocolDBTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	return ctx
}
func (protocolDBTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryEndData) {
	markProtocolFailure(ctx, d.Err)
}
func (protocolDBTracer) TraceAcquireStart(ctx context.Context, _ *pgxpool.Pool, _ pgxpool.TraceAcquireStartData) context.Context {
	return ctx
}
func (protocolDBTracer) TraceAcquireEnd(ctx context.Context, _ *pgxpool.Pool, d pgxpool.TraceAcquireEndData) {
	markProtocolFailure(ctx, d.Err)
}
func markProtocolFailure(ctx context.Context, err error) {
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		if state, ok := ctx.Value(protocolStateKey{}).(*protocolState); ok {
			state.dbFailed.Store(true)
		}
	}
}

var chromeOrigin = regexp.MustCompile(`^chrome-extension://[a-p]{32}$`)
var firefoxOrigin = regexp.MustCompile(`^moz-extension://[a-zA-Z0-9-]+$`)

func (s *Server) validOrigin(raw string) bool {
	if chromeOrigin.MatchString(raw) || firefoxOrigin.MatchString(raw) {
		return true
	}
	u, err := url.Parse(raw)
	return err == nil && u.Path == "" && u.RawQuery == "" && !u.ForceQuery && u.Fragment == "" && u.User == nil && s.validRedirect(raw) && raw == u.Scheme+"://"+u.Host
}

type protocolResponse struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func (b *protocolResponse) Header() http.Header { return b.header }
func (b *protocolResponse) WriteHeader(code int) {
	if b.status == 0 {
		b.status = code
	}
}
func (b *protocolResponse) Write(p []byte) (int, error) {
	if b.status == 0 {
		b.status = 200
	}
	return b.body.Write(p)
}
func protocolError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if code == "invalid_token" || code == "insufficient_scope" {
		w.Header().Set("WWW-Authenticate", `Bearer error="`+code+`"`)
	}
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": code})
}
func (s *Server) discovery(w http.ResponseWriter, r *http.Request) {
	d := op.CreateDiscoveryConfig(op.ContextWithIssuer(r.Context(), s.origin), s.provider, s)
	d.ResponseTypesSupported = []string{"code"}
	d.GrantTypesSupported = []oidc.GrantType{oidc.GrantTypeCode, oidc.GrantTypeRefreshToken}
	d.ScopesSupported = []string{"openid", "profile", "email", "offline_access"}
	d.ClaimsSupported = []string{"iss", "sub", "aud", "exp", "iat", "auth_time", "nonce", "acr", "amr", "azp", "at_hash", "sid", "name", "picture", "email", "email_verified"}
	d.DeviceAuthorizationEndpoint = ""
	d.CheckSessionIframe = ""
	op.Discover(w, d)
}
func (s *Server) protocolHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state := new(protocolState)
		r = r.WithContext(context.WithValue(r.Context(), protocolStateKey{}, state))
		path := r.URL.Path
		metadata := path == "/.well-known/openid-configuration" || path == "/keys"
		api := metadata || path == "/userinfo" || path == "/oauth/token" || path == "/revoke" || path == "/oauth/introspect"
		origin := r.Header.Get("Origin")
		if api && origin != "" {
			w.Header().Add("Vary", "Origin")
			allowed := metadata
			if !metadata {
				if r.Method == "OPTIONS" || path == "/userinfo" {
					err := s.db.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM apps WHERE enabled AND $1=ANY(allowed_origins))", origin).Scan(&allowed)
					if err != nil {
						protocolError(w, 503, "temporarily_unavailable")
						return
					}
				} else {
					if err := r.ParseForm(); err != nil {
						protocolError(w, 400, "invalid_request")
						return
					}
					client := r.Form.Get("client_id")
					if basic, _, ok := r.BasicAuth(); ok {
						client = basic
					}
					err := s.db.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM apps WHERE id=$1 AND enabled AND $2=ANY(allowed_origins))", client, origin).Scan(&allowed)
					if err != nil {
						protocolError(w, 503, "temporarily_unavailable")
						return
					}
				}
			}
			if !allowed {
				protocolError(w, 403, "origin_not_allowed")
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Expose-Headers", "WWW-Authenticate, Retry-After")
			if r.Method == "OPTIONS" {
				if !slices.Contains([]string{"GET", "POST"}, r.Header.Get("Access-Control-Request-Method")) {
					protocolError(w, 403, "invalid_request")
					return
				}
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
				w.WriteHeader(204)
				return
			}
		}
		if path == "/.well-known/openid-configuration" {
			s.discovery(w, r)
			return
		}
		if path == "/oauth/token" && r.Method == "POST" {
			if err := r.ParseForm(); err != nil {
				protocolError(w, 400, "invalid_request")
				return
			}
			if !slices.Contains([]string{"authorization_code", "refresh_token"}, r.Form.Get("grant_type")) {
				protocolError(w, 400, "unsupported_grant_type")
				return
			}
		}
		b := &protocolResponse{header: make(http.Header)}
		s.provider.ServeHTTP(b, r)
		if state.dbFailed.Load() {
			protocolError(w, 503, "temporarily_unavailable")
			return
		}
		if path == "/userinfo" && (b.status == 401 || b.status == 403) {
			if state.originDenied {
				w.Header().Del("Access-Control-Allow-Origin")
				protocolError(w, 403, "origin_not_allowed")
			} else if state.insufficientScope {
				protocolError(w, 403, "insufficient_scope")
			} else {
				protocolError(w, 401, "invalid_token")
			}
			return
		}
		for key, values := range b.header {
			if !strings.HasPrefix(strings.ToLower(key), "access-control-") {
				for _, v := range values {
					w.Header().Add(key, v)
				}
			}
		}
		status := b.status
		if status == 0 {
			status = 200
		}
		w.WriteHeader(status)
		w.Write(b.body.Bytes())
	})
}
