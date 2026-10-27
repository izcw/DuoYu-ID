package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/subtle"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"
)

type App struct {
	ID                   string    `json:"id"`
	Name                 string    `json:"name"`
	Description          string    `json:"description"`
	Secret               string    `json:"-"`
	Redirects            []string  `json:"redirects"`
	LogoutRedirects      []string  `json:"logout_redirects"`
	Public               bool      `json:"public"`
	Enabled              bool      `json:"enabled"`
	Restricted           bool      `json:"restricted"`
	Created              time.Time `json:"created_at"`
	Development          bool      `json:"-"`
	RefreshEnabled       bool      `json:"refresh_enabled"`
	Platform             string    `json:"platform"`
	BackchannelLogoutURI string    `json:"backchannel_logout_uri"`
}

func (c *App) GetID() string                    { return c.ID }
func (c *App) RedirectURIs() []string           { return c.Redirects }
func (c *App) PostLogoutRedirectURIs() []string { return c.LogoutRedirects }
func (c *App) ApplicationType() op.ApplicationType {
	if c.Platform == "native" {
		return op.ApplicationTypeNative
	}
	if c.Public {
		return op.ApplicationTypeUserAgent
	}
	return op.ApplicationTypeWeb
}
func (c *App) AuthMethod() oidc.AuthMethod {
	if c.Public {
		return oidc.AuthMethodNone
	}
	return oidc.AuthMethodBasic
}
func (c *App) ResponseTypes() []oidc.ResponseType { return []oidc.ResponseType{oidc.ResponseTypeCode} }
func (c *App) GrantTypes() []oidc.GrantType {
	if c.RefreshEnabled {
		return []oidc.GrantType{oidc.GrantTypeCode, oidc.GrantTypeRefreshToken}
	}
	return []oidc.GrantType{oidc.GrantTypeCode}
}
func (c *App) LoginURL(id string) string           { return "/login?authRequestID=" + url.QueryEscape(id) }
func (c *App) AccessTokenType() op.AccessTokenType { return op.AccessTokenTypeBearer }
func (c *App) IDTokenLifetime() time.Duration      { return 5 * time.Minute }
func (c *App) DevMode() bool                       { return c.Development }
func (c *App) RestrictAdditionalIdTokenScopes() func([]string) []string {
	return func(scopes []string) []string {
		if slices.Contains(scopes, "openid") {
			return []string{"openid"}
		}
		return nil
	}
}
func (c *App) RestrictAdditionalAccessTokenScopes() func([]string) []string {
	return func([]string) []string { return nil }
}
func (c *App) IsScopeAllowed(scope string) bool     { return false }
func (c *App) IDTokenUserinfoClaimsAssertion() bool { return false }
func (c *App) ClockSkew() time.Duration             { return 0 }

type AuthRequest struct {
	Request                oidc.AuthRequest
	ID, UserID, AMR        string
	Created, Authenticated time.Time
	Completed              bool
	SessionHash            string
}

func (a *AuthRequest) GetID() string          { return a.ID }
func (a *AuthRequest) GetACR() string         { return "" }
func (a *AuthRequest) GetAMR() []string       { return strings.Fields(a.AMR) }
func (a *AuthRequest) GetAudience() []string  { return []string{a.Request.ClientID} }
func (a *AuthRequest) GetAuthTime() time.Time { return a.Authenticated }
func (a *AuthRequest) GetClientID() string    { return a.Request.ClientID }
func (a *AuthRequest) GetCodeChallenge() *oidc.CodeChallenge {
	return &oidc.CodeChallenge{Challenge: a.Request.CodeChallenge, Method: a.Request.CodeChallengeMethod}
}
func (a *AuthRequest) GetNonce() string                   { return a.Request.Nonce }
func (a *AuthRequest) GetRedirectURI() string             { return a.Request.RedirectURI }
func (a *AuthRequest) GetResponseType() oidc.ResponseType { return a.Request.ResponseType }
func (a *AuthRequest) GetResponseMode() oidc.ResponseMode { return a.Request.ResponseMode }
func (a *AuthRequest) GetScopes() []string                { return a.Request.Scopes }
func (a *AuthRequest) GetState() string                   { return a.Request.State }
func (a *AuthRequest) GetSubject() string                 { return a.UserID }
func (a *AuthRequest) Done() bool                         { return a.Completed }
func (s *Server) app(ctx context.Context, id string) (a *App, e error) {
	a = &App{Development: s.dev}
	e = s.db.QueryRow(ctx, "SELECT id,name,description,secret_hash,redirects,logout_redirects,public,enabled,restricted,created_at,refresh_enabled,platform,backchannel_logout_uri FROM apps WHERE id=$1", id).Scan(&a.ID, &a.Name, &a.Description, &a.Secret, &a.Redirects, &a.LogoutRedirects, &a.Public, &a.Enabled, &a.Restricted, &a.Created, &a.RefreshEnabled, &a.Platform, &a.BackchannelLogoutURI)
	if e == nil && !a.Enabled {
		e = errors.New("application disabled")
	}
	return
}
func (s *Server) GetClientByClientID(ctx context.Context, id string) (op.Client, error) {
	return s.app(ctx, id)
}
func (s *Server) AuthorizeClientIDSecret(ctx context.Context, id, secret string) error {
	a, e := s.app(ctx, id)
	if e != nil || a.Public || subtle.ConstantTimeCompare([]byte(a.Secret), []byte(s.mac(secret))) != 1 {
		return errors.New("invalid client")
	}
	return nil
}
func (s *Server) allowed(ctx context.Context, uid, aid string) bool {
	u, e := s.user(ctx, uid)
	if e != nil || !u.Enabled {
		return false
	}
	a, e := s.app(ctx, aid)
	if e != nil {
		return false
	}
	if !a.Restricted {
		return true
	}
	var yes bool
	s.db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM grants WHERE user_id=$1 AND app_id=$2)", uid, aid).Scan(&yes)
	return yes
}
func (s *Server) CreateAuthRequest(ctx context.Context, r *oidc.AuthRequest, _ string) (op.AuthRequest, error) {
	if r.ResponseType != oidc.ResponseTypeCode || r.CodeChallengeMethod != oidc.CodeChallengeMethodS256 || len(r.CodeChallenge) != 43 {
		return nil, oidc.ErrInvalidRequest().WithDescription("S256 PKCE is required")
	}
	for _, scope := range r.Scopes {
		if !slices.Contains([]string{"openid", "profile", "email", "offline_access"}, scope) {
			return nil, oidc.ErrInvalidScope()
		}
	}
	if slices.Contains(r.Scopes, "offline_access") {
		app, e := s.app(ctx, r.ClientID)
		if e != nil || !app.RefreshEnabled {
			return nil, oidc.ErrInvalidScope().WithDescription("offline_access is not enabled for this client")
		}
	}
	a := &AuthRequest{Request: *r, ID: randomID(), Created: time.Now()}
	if slices.Contains(r.Prompt, "none") {
		hash, _ := ctx.Value(sessionContextKey{}).(string)
		e := s.db.QueryRow(ctx, "SELECT user_id,created_at,amr FROM sessions WHERE hash=$1 AND expires_at>now()", hash).Scan(&a.UserID, &a.Authenticated, &a.AMR)
		if e != nil || needsReauthentication(a, a.Authenticated) {
			return nil, oidc.ErrLoginRequired()
		}
		if !s.allowed(ctx, a.UserID, r.ClientID) {
			return nil, oidc.ErrAccessDenied()
		}
		// Silent authorization requires prior successful authorization to this app.
		var prior bool
		s.db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM access_tokens WHERE user_id=$1 AND app_id=$2 AND scopes @> $3::text[])", a.UserID, r.ClientID, []string(r.Scopes)).Scan(&prior)
		if !prior || slices.Contains(r.Scopes, "offline_access") {
			return nil, oidc.ErrInteractionRequired()
		}
		a.Completed = true
		a.SessionHash = hash
	}
	b, e := json.Marshal(a)
	if e != nil {
		return nil, e
	}
	_, e = s.db.Exec(ctx, "INSERT INTO auth_requests(id,body,expires_at) VALUES($1,$2,$3)", a.ID, b, time.Now().Add(10*time.Minute))
	return a, e
}
func (s *Server) AuthRequestByID(ctx context.Context, id string) (op.AuthRequest, error) {
	var b []byte
	e := s.db.QueryRow(ctx, "SELECT body FROM auth_requests WHERE id=$1 AND expires_at>now()", id).Scan(&b)
	if e != nil {
		return nil, e
	}
	a := new(AuthRequest)
	e = json.Unmarshal(b, a)
	return a, e
}
func (s *Server) AuthRequestByCode(ctx context.Context, code string) (op.AuthRequest, error) {
	var b []byte
	e := s.db.QueryRow(ctx, "UPDATE auth_requests SET code_hash=NULL WHERE code_hash=$1 AND expires_at>now() RETURNING body", digest(code)).Scan(&b)
	if e != nil {
		return nil, e
	}
	a := new(AuthRequest)
	e = json.Unmarshal(b, a)
	if e == nil && !s.allowed(ctx, a.UserID, a.Request.ClientID) {
		e = errors.New("access denied")
	}
	return a, e
}
func (s *Server) SaveAuthCode(ctx context.Context, id, code string) error {
	tag, e := s.db.Exec(ctx, "UPDATE auth_requests SET code_hash=$1 WHERE id=$2 AND expires_at>now()", digest(code), id)
	if e == nil && tag.RowsAffected() != 1 {
		return errors.New("expired request")
	}
	return e
}
func (s *Server) DeleteAuthRequest(ctx context.Context, id string) error {
	_, e := s.db.Exec(ctx, "DELETE FROM auth_requests WHERE id=$1", id)
	return e
}
func (s *Server) CreateAccessToken(ctx context.Context, r op.TokenRequest) (string, time.Time, error) {
	id := randomID()
	expiry := time.Now().Add(5 * time.Minute)
	aud := r.GetAudience()
	if len(aud) != 1 || !s.allowed(ctx, r.GetSubject(), aud[0]) {
		return "", expiry, errors.New("access denied")
	}
	tx, e := s.db.Begin(ctx)
	if e != nil {
		return "", expiry, e
	}
	defer tx.Rollback(ctx)
	var sessionHash any
	if a, ok := r.(*AuthRequest); ok && a.SessionHash != "" {
		sessionHash = a.SessionHash
	}
	if _, e = tx.Exec(ctx, "INSERT INTO access_tokens(id,user_id,app_id,scopes,expires_at,session_hash) VALUES($1,$2,$3,$4,$5,$6)", id, r.GetSubject(), aud[0], r.GetScopes(), expiry, sessionHash); e != nil {
		return "", expiry, e
	}
	if _, e = tx.Exec(ctx, "INSERT INTO app_users(app_id,user_id) VALUES($1,$2) ON CONFLICT(app_id,user_id) DO UPDATE SET last_login_at=now(),login_count=app_users.login_count+1", aud[0], r.GetSubject()); e != nil {
		return "", expiry, e
	}
	return id, expiry, tx.Commit(ctx)
}

func (s *Server) TerminateSession(ctx context.Context, uid, cid string) error {
	if hash, ok := ctx.Value(sessionContextKey{}).(string); ok {
		if _, e := s.db.Exec(ctx, "DELETE FROM sessions WHERE hash=$1 AND user_id=$2", hash, uid); e != nil {
			return e
		}
	}
	_, e := s.db.Exec(ctx, "DELETE FROM access_tokens WHERE user_id=$1 AND app_id=$2", uid, cid)
	return e
}
func (s *Server) RevokeToken(ctx context.Context, id, uid, cid string) *oidc.Error {
	if _, e := s.db.Exec(ctx, "UPDATE refresh_families SET revoked=true WHERE id IN(SELECT family_id FROM refresh_tokens WHERE hash=$1) AND app_id=$2", digest(id), cid); e != nil {
		return oidc.ErrServerError()
	}
	_, e := s.db.Exec(ctx, "DELETE FROM access_tokens WHERE id=$1 AND app_id=$2", id, cid)
	if e != nil {
		return oidc.ErrServerError()
	}
	return nil
}
func (s *Server) Health(ctx context.Context) error { return s.db.Ping(ctx) }
func (s *Server) SetUserinfoFromScopes(context.Context, *oidc.UserInfo, string, string, []string) error {
	return nil
}
func (s *Server) fillInfo(ctx context.Context, info *oidc.UserInfo, uid string, scopes []string) error {
	u, e := s.user(ctx, uid)
	if e != nil {
		return e
	}
	info.Subject = u.ID
	for _, sc := range scopes {
		switch sc {
		case "profile":
			info.Name = u.Name
			info.Picture = u.Avatar
		case "email":
			info.Email = u.Email
			info.EmailVerified = oidc.Bool(true)
		}
	}
	return nil
}
func (s *Server) SetUserinfoFromRequest(ctx context.Context, info *oidc.UserInfo, r op.IDTokenRequest, scopes []string) error {
	if err := s.fillInfo(ctx, info, r.GetSubject(), scopes); err != nil {
		return err
	}
	var hash string
	switch req := r.(type) {
	case *AuthRequest:
		hash = req.SessionHash
	case *refreshRequest:
		hash = req.Auth.SessionHash
	}
	if hash != "" {
		info.AppendClaims("sid", digest("logout:"+hash))
	}
	return nil
}
func (s *Server) token(ctx context.Context, id, subject string) (string, []string, time.Time, error) {
	var app string
	var scopes []string
	var expires time.Time
	e := s.db.QueryRow(ctx, "SELECT app_id,scopes,expires_at FROM access_tokens WHERE id=$1 AND user_id=$2 AND expires_at>now() AND (session_hash IS NULL OR EXISTS(SELECT 1 FROM sessions WHERE hash=session_hash AND expires_at>now()))", id, subject).Scan(&app, &scopes, &expires)
	var revoked bool
	if e == nil {
		e = s.db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM refresh_families f JOIN access_tokens t ON t.family_id=f.id WHERE t.id=$1 AND f.revoked)", id).Scan(&revoked)
	}
	if e == nil && (revoked || !s.allowed(ctx, subject, app)) {
		e = errors.New("access denied")
	}
	return app, scopes, expires, e
}
func (s *Server) SetUserinfoFromToken(ctx context.Context, info *oidc.UserInfo, id, subject, origin string) error {
	app, scopes, _, e := s.token(ctx, id, subject)
	if e != nil {
		return e
	}
	if origin != "" {
		a, e := s.app(ctx, app)
		if e != nil {
			return e
		}
		allowed := false
		for _, raw := range a.Redirects {
			u, _ := url.Parse(raw)
			if u.Scheme+"://"+u.Host == origin {
				allowed = true
			}
		}
		if !allowed {
			return errors.New("origin denied")
		}
	}
	return s.fillInfo(ctx, info, subject, scopes)
}
func (s *Server) SetIntrospectionFromToken(ctx context.Context, info *oidc.IntrospectionResponse, id, subject, client string) error {
	app, scopes, expires, e := s.token(ctx, id, subject)
	if e != nil || app != client {
		return errors.New("invalid token")
	}
	u := new(oidc.UserInfo)
	if e = s.fillInfo(ctx, u, subject, scopes); e != nil {
		return e
	}
	info.SetUserInfo(u)
	info.ClientID = app
	info.Scope = scopes
	info.Expiration = oidc.FromTime(expires)
	return nil
}
func (s *Server) GetPrivateClaimsFromScopes(context.Context, string, string, []string) (map[string]any, error) {
	return nil, nil
}
func (s *Server) GetKeyByIDAndClientID(context.Context, string, string) (*jose.JSONWebKey, error) {
	return nil, errors.New("JWT profile not supported")
}
func (s *Server) ValidateJWTProfileScopes(context.Context, string, []string) ([]string, error) {
	return nil, errors.New("JWT profile not supported")
}

type signingKey struct{ k *rsa.PrivateKey }

func (k signingKey) ID() string {
	return digest(string(x509.MarshalPKCS1PublicKey(&k.k.PublicKey)))[:16]
}
func (k signingKey) Key() any                                    { return k.k }
func (k signingKey) SignatureAlgorithm() jose.SignatureAlgorithm { return jose.RS256 }

type publicKey struct{ signingKey }

func (k publicKey) Key() any                           { return &k.k.PublicKey }
func (k publicKey) Algorithm() jose.SignatureAlgorithm { return jose.RS256 }
func (k publicKey) Use() string                        { return "sig" }
func (s *Server) SigningKey(context.Context) (op.SigningKey, error) {
	return signingKey{s.signing}, nil
}
func (s *Server) SignatureAlgorithms(context.Context) ([]jose.SignatureAlgorithm, error) {
	return []jose.SignatureAlgorithm{jose.RS256}, nil
}
func (s *Server) KeySet(context.Context) ([]op.Key, error) {
	keys := []op.Key{publicKey{signingKey{s.signing}}}
	paths, e := filepath.Glob(filepath.Join(s.data, "verification-keys", "*.pem"))
	if e != nil {
		return nil, e
	}
	for _, path := range paths {
		body, e := os.ReadFile(path)
		if e != nil {
			return nil, e
		}
		block, _ := pem.Decode(body)
		if block == nil {
			return nil, errors.New("invalid verification key")
		}
		pub, e := x509.ParsePKCS1PublicKey(block.Bytes)
		if e != nil {
			return nil, e
		}
		key := publicKey{signingKey{&rsa.PrivateKey{PublicKey: *pub}}}
		if key.ID() != (signingKey{s.signing}).ID() {
			keys = append(keys, key)
		}
	}
	return keys, nil
}
func (s *Server) loadSigningKey() error {
	p := filepath.Join(s.data, "signing.pem")
	b, e := os.ReadFile(p)
	if errors.Is(e, os.ErrNotExist) {
		k, e := rsa.GenerateKey(rand.Reader, 3072)
		if e != nil {
			return e
		}
		b = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)})
		f, e := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return e
		}
		_, e = f.Write(b)
		f.Close()
		if e != nil {
			return e
		}
	} else if e != nil {
		return e
	}
	block, _ := pem.Decode(b)
	if block == nil {
		return errors.New("invalid signing key")
	}
	s.signing, e = x509.ParsePKCS1PrivateKey(block.Bytes)
	return e
}
func (s *Server) loginPage(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("authRequestID")
	app := "多鱼账号中心"
	if id != "" {
		a, e := s.AuthRequestByID(r.Context(), id)
		if e != nil {
			http.Error(w, "登录请求已过期，请返回应用重新登录。", 400)
			return
		}
		c, e := s.app(r.Context(), a.GetClientID())
		if e != nil {
			http.Error(w, "应用不可用", 403)
			return
		}
		app = c.Name
		if a.Done() && slices.Contains(a.(*AuthRequest).Request.Prompt, "none") {
			hash, _ := r.Context().Value(sessionContextKey{}).(string)
			if hash == "" || hash != a.(*AuthRequest).SessionHash {
				http.Error(w, "登录状态已变化", 401)
				return
			}
			http.Redirect(w, r, s.origin+"/authorize/callback?id="+url.QueryEscape(id), http.StatusFound)
			return
		}
		// One short-lived cookie per request also works when two initial tabs open
		// concurrently, without a race to establish a shared browser cookie.
		name := flowCookieName(id)
		count := 0
		for _, cookie := range r.Cookies() {
			if strings.HasPrefix(cookie.Name, "duoyu_flow_") && cookie.Name != name {
				count++
				if count >= 8 {
					s.cookie(w, cookie.Name, "", -1)
				}
			}
		}
		s.cookie(w, name, s.mac(id), 600)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	s.templates.ExecuteTemplate(w, "login.html", map[string]string{"App": app, "RequestID": id})
}
func (s *Server) authContext(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	a, e := s.AuthRequestByID(r.Context(), id)
	if e != nil {
		fail(w, 400, "登录请求已过期")
		return
	}
	c, e := s.app(r.Context(), a.GetClientID())
	if e != nil {
		fail(w, 403, "应用不可用")
		return
	}
	var created time.Time
	if cookie, err := r.Cookie("duoyu_session"); err == nil {
		s.db.QueryRow(r.Context(), "SELECT created_at FROM sessions WHERE hash=$1 AND expires_at>now()", digest(cookie.Value)).Scan(&created)
	}
	ok(w, map[string]any{"name": c.Name, "scopes": a.GetScopes(), "reauthenticate": needsReauthentication(a.(*AuthRequest), created), "consent": slices.Contains(a.(*AuthRequest).Request.Prompt, "consent") || slices.Contains(a.GetScopes(), "offline_access")})
}

func flowCookieName(id string) string { return "duoyu_flow_" + digest(id)[:24] }

func needsReauthentication(a *AuthRequest, created time.Time) bool {
	if created.IsZero() {
		return true
	}
	if slices.Contains(a.Request.Prompt, "login") && created.Before(a.Created) {
		return true
	}
	if a.Request.MaxAge != nil {
		// max_age=0 requires a fresh authentication after this request, not an
		// impossible zero elapsed time at the subsequent completion request.
		if *a.Request.MaxAge == 0 {
			return created.Before(a.Created)
		}
		return time.Since(created) > time.Duration(*a.Request.MaxAge)*time.Second
	}
	return false
}
func (s *Server) completeAuth(w http.ResponseWriter, r *http.Request) {
	u, yes := s.require(w, r, false)
	if !yes {
		return
	}
	var in struct {
		ID string `json:"id"`
	}
	if decode(r, &in) != nil {
		fail(w, 400, "请求错误")
		return
	}
	c, e := r.Cookie(flowCookieName(in.ID))
	if e != nil || subtle.ConstantTimeCompare([]byte(c.Value), []byte(s.mac(in.ID))) != 1 {
		fail(w, 403, "登录请求不属于当前浏览器")
		return
	}
	raw, e := s.AuthRequestByID(r.Context(), in.ID)
	if e != nil {
		fail(w, 400, "登录请求已过期")
		return
	}
	a := raw.(*AuthRequest)
	if !s.allowed(r.Context(), u.ID, a.GetClientID()) {
		fail(w, 403, "你尚无权访问该项目")
		return
	}
	cookie, _ := r.Cookie("duoyu_session")
	var created time.Time
	var amr string
	e = s.db.QueryRow(r.Context(), "SELECT created_at,amr FROM sessions WHERE hash=$1", digest(cookie.Value)).Scan(&created, &amr)
	if e != nil {
		fail(w, 401, "会话已失效")
		return
	}
	if needsReauthentication(a, created) {
		fail(w, 401, "该应用要求重新验证，请重新登录")
		return
	}
	a.UserID = u.ID
	a.Completed = true
	a.Authenticated = created
	a.AMR = amr
	a.SessionHash = digest(cookie.Value)
	b, _ := json.Marshal(a)
	tag, e := s.db.Exec(r.Context(), "UPDATE auth_requests SET body=$1 WHERE id=$2 AND expires_at>now() AND body->>'Completed'='false'", b, a.ID)
	if e != nil || tag.RowsAffected() != 1 {
		fail(w, 400, "请求已使用或过期")
		return
	}
	s.audit(r.Context(), u.ID, "oidc.authorized", a.GetClientID(), true)
	s.cookie(w, flowCookieName(in.ID), "", -1)
	ok(w, map[string]string{"redirect": s.origin + "/authorize/callback?id=" + url.QueryEscape(a.ID)})
}
