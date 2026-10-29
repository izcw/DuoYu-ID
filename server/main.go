package main

import (
	"context"
	"crypto/rsa"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/mail"
	"net/netip"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zitadel/oidc/v3/pkg/op"
)

//go:embed schema.sql templates/* migrations/*
var assets embed.FS

type Server struct {
	db            *pgxpool.Pool
	proxies       []netip.Prefix
	key           [32]byte
	origin, data  string
	dev           bool
	signing       *rsa.PrivateKey
	provider      *op.Provider
	wa            *webauthn.WebAuthn
	templates     *template.Template
	mailTransport func(MailMessage) error // Replaced only by isolated tests; production uses SMTP.
}
type User struct {
	AccountID      string    `json:"account_id"`
	ManagementRole string    `json:"management_role"`
	ID             string    `json:"id"`
	Email          string    `json:"email"`
	Name           string    `json:"name"`
	Avatar         string    `json:"avatar"`
	Admin          bool      `json:"admin"`
	Enabled        bool      `json:"enabled"`
	Created        time.Time `json:"created_at"`
}

var errDisabledUser = errors.New("disabled user")

type sessionContextKey struct{}

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func main() {
	s, e := newServer()
	if e != nil {
		slog.Error("startup failed", "error", e)
		os.Exit(1)
	}
	defer s.db.Close()
	if os.Getenv("DUOYU_MIGRATE_ONLY") == "true" {
		slog.Info("database migrations complete")
		return
	}
	h := s.routes()
	srv := &http.Server{Addr: env("DUOYU_ADDR", "0.0.0.0:5252"), Handler: h, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 16}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var workers sync.WaitGroup
	role := env("DUOYU_ROLE", "combined")
	if role != "api" && role != "worker" && role != "combined" {
		slog.Error("invalid DUOYU_ROLE")
		return
	}
	if role != "api" {
		s.startMailWorkers(ctx, &workers)
		s.startLogoutWorker(ctx, &workers)
		s.startMaintenance(ctx, &workers)
	}
	defer func() { stop(); workers.Wait() }()
	if role == "worker" {
		<-ctx.Done()
		return
	}
	go func() {
		<-ctx.Done()
		c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(c)
	}()
	listener, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		slog.Error("listen failed", "address", srv.Addr, "error", err)
		return
	}
	s.logAccessAddresses(srv.Addr)
	slog.Info("DuoYu ID ready", "origin", s.origin)
	if e = srv.Serve(listener); e != nil && !errors.Is(e, http.ErrServerClosed) {
		slog.Error("server stopped", "error", e)
	}
}
func newServer() (*Server, error) {
	s := &Server{origin: strings.TrimRight(env("DUOYU_ORIGIN", "http://localhost:5252"), "/"), data: env("DUOYU_DATA_DIR", "data"), dev: os.Getenv("DUOYU_DEV") == "true"}
	var e error
	s.proxies, e = parseProxies(os.Getenv("DUOYU_TRUSTED_PROXIES"))
	if e != nil {
		return nil, e
	}
	u, e := url.Parse(s.origin)
	if e != nil || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, errors.New("DUOYU_ORIGIN must be an origin")
	}
	if !s.dev && u.Scheme != "https" {
		return nil, errors.New("HTTPS required")
	}
	if s.dev && !localDevelopmentHost(u.Hostname()) {
		return nil, errors.New("development mode requires localhost or a local network interface IP")
	}
	key, e := hex.DecodeString(os.Getenv("DUOYU_SECRET"))
	if e != nil || len(key) != 32 {
		return nil, errors.New("DUOYU_SECRET requires 64 random hex characters")
	}
	copy(s.key[:], key)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	poolConfig, e := pgxpool.ParseConfig(os.Getenv("DATABASE_URL"))
	if e != nil {
		return nil, e
	}
	poolConfig.ConnConfig.Tracer = protocolDBTracer{}
	s.db, e = pgxpool.NewWithConfig(ctx, poolConfig)
	if e != nil {
		return nil, e
	}
	if e = s.db.Ping(ctx); e != nil {
		return nil, e
	}
	if e = s.migrate(ctx); e != nil {
		return nil, e
	}
	if os.Getenv("DUOYU_MIGRATE_ONLY") == "true" {
		return s, nil
	}
	if e = os.MkdirAll(filepath.Join(s.data, "avatars"), 0700); e != nil {
		return nil, e
	}
	if e = s.loadSigningKey(); e != nil {
		return nil, e
	}
	s.templates = template.Must(template.New("").Funcs(template.FuncMap{"assetURL": authAssetURL}).ParseFS(assets, "templates/*.html"))
	// WebAuthn requires a domain RP ID. A development LAN-IP issuer can
	// still serve OIDC, password, email and TOTP without a WebAuthn provider.
	if net.ParseIP(u.Hostname()) == nil {
		s.wa, e = webauthn.New(&webauthn.Config{RPDisplayName: "多鱼账号", RPID: u.Hostname(), RPOrigins: []string{s.origin}})
		if e != nil {
			return nil, e
		}
	}
	opts := []op.Option{}
	if s.dev {
		opts = append(opts, op.WithAllowInsecure())
	}
	s.provider, e = op.NewProvider(&op.Config{CryptoKey: s.key, CodeMethodS256: true, GrantTypeRefreshToken: true, BackChannelLogoutSupported: true, BackChannelLogoutSessionSupported: true, DefaultLogoutRedirectURI: s.origin + "/login"}, s, op.StaticIssuer(s.origin), opts...)
	if e != nil {
		return nil, e
	}
	if p := os.Getenv("DUOYU_ADMIN_PASSWORD"); p != "" {
		if e = validPassword(p); e != nil {
			return nil, e
		}
		email := normalizeEmail(os.Getenv("DUOYU_ADMIN_EMAIL"))
		if email == "" {
			return nil, errors.New("valid admin email required")
		}
		_, e = s.db.Exec(ctx, "INSERT INTO users(id,email,name,password_hash,admin) SELECT $1,$2,'超级管理员',$3,true WHERE NOT EXISTS(SELECT 1 FROM users WHERE admin) ON CONFLICT DO NOTHING", randomID(), email, hashPassword(p))
		if e != nil {
			return nil, e
		}
	}
	return s, nil
}
func (s *Server) routes() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if s.db.Ping(r.Context()) != nil {
			fail(w, 503, "数据库不可用")
			return
		}
		ok(w, map[string]bool{"healthy": true})
	})
	m.HandleFunc("GET /login", s.loginPage)
	m.HandleFunc("POST /api/auth/code", s.sendCode)
	m.HandleFunc("POST /api/auth/login", s.login)
	m.HandleFunc("POST /api/auth/reset", s.resetPassword)
	m.HandleFunc("POST /api/auth/logout", s.logout)
	m.HandleFunc("GET /api/auth/session", s.sessionStatus)
	m.HandleFunc("POST /api/service/mail", s.serviceMail)
	m.HandleFunc("GET /api/service/mail/{id}", s.serviceMailStatus)
	m.HandleFunc("GET /api/admin/mail", s.adminMail)
	m.HandleFunc("GET /api/me/authenticator", s.authenticatorStatus)
	m.HandleFunc("POST /api/me/authenticator/begin", s.beginAuthenticator)
	m.HandleFunc("POST /api/me/authenticator/finish", s.finishAuthenticator)
	m.HandleFunc("DELETE /api/me/authenticator", s.deleteAuthenticator)
	m.HandleFunc("POST /api/auth/authenticator", s.loginAuthenticator)
	m.HandleFunc("GET /api/me", s.me)
	m.HandleFunc("GET /api/user/info", s.artUser)
	m.HandleFunc("PATCH /api/me", s.profile)
	m.HandleFunc("POST /api/me/password", s.changePassword)
	m.HandleFunc("POST /api/me/email", s.changeEmail)
	m.HandleFunc("POST /api/me/avatar", s.avatar)
	m.HandleFunc("GET /avatars/{id}", s.serveAvatar)
	m.HandleFunc("POST /api/admin/project-logos", s.uploadProjectLogo)
	m.HandleFunc("GET /project-logos/{id}", s.serveProjectLogo)
	m.HandleFunc("GET /api/me/sessions", s.sessions)
	m.HandleFunc("DELETE /api/me/sessions/{id}", s.revokeSession)
	m.HandleFunc("GET /api/auth/context", s.authContext)
	m.HandleFunc("POST /api/auth/continue", s.completeAuth)
	m.HandleFunc("GET /api/admin/overview", s.overview)
	m.HandleFunc("GET /api/admin/diagnostics", s.diagnostics)
	m.HandleFunc("GET /api/admin/apps/{id}/checks", s.appChecks)
	m.HandleFunc("POST /api/admin/keys/{id}/rotate", s.rotateAPIKey)
	m.HandleFunc("POST /api/admin/logout-events/{id}/retry", s.retryLogout)
	m.HandleFunc("GET /api/admin/logout-events/{id}/attempts", s.logoutAttempts)
	m.HandleFunc("GET /api/admin/logout-events", func(w http.ResponseWriter, r *http.Request) {
		if _, yes := s.require(w, r, true); !yes {
			return
		}
		s.queryList(w, r, "SELECT id,app_id,user_id,created_at,next_attempt,attempts,status,last_error,http_status,completed_at,retry_count FROM logout_jobs ORDER BY id DESC LIMIT 200")
	})
	m.HandleFunc("GET /api/admin/users", s.users)
	m.HandleFunc("PATCH /api/admin/users/{id}", s.updateUser)
	m.HandleFunc("PUT /api/admin/users/{id}/access", s.updateManagementAccess)
	m.HandleFunc("GET /api/admin/users/{id}/access", s.managementAccess)
	m.HandleFunc("POST /api/admin/users/{id}/revoke", s.adminRevoke)
	m.HandleFunc("GET /api/admin/apps", s.apps)
	m.HandleFunc("GET /api/admin/apps/{id}/blacklist", s.blacklist)
	m.HandleFunc("PUT /api/admin/apps/{id}/blacklist", s.setBlacklist)
	m.HandleFunc("POST /api/admin/apps", s.saveApp)
	m.HandleFunc("PUT /api/admin/apps/{id}", s.saveApp)
	m.HandleFunc("POST /api/admin/apps/{id}/rotate", s.rotateApp)
	m.HandleFunc("GET /api/admin/apps/{id}/grants", s.listGrants)
	m.HandleFunc("PUT /api/admin/apps/{id}/grants", s.setGrants)
	m.HandleFunc("GET /api/admin/audit", s.auditList)
	m.HandleFunc("GET /api/admin/keys", s.keys)
	m.HandleFunc("POST /api/admin/keys", s.createKey)
	m.HandleFunc("DELETE /api/admin/keys/{id}", s.revokeKey)
	m.HandleFunc("GET /api/admin/settings", s.settings)
	m.HandleFunc("GET /api/admin/analytics", s.analytics)
	m.HandleFunc("GET /api/admin/apps/{id}", s.adminAppDetail)
	m.HandleFunc("GET /api/admin/users/{id}", s.adminUserDetail)
	m.HandleFunc("GET /api/admin/keys/{id}", s.adminKeyDetail)
	m.HandleFunc("PUT /api/admin/keys/{id}", s.updateKey)
	m.HandleFunc("GET /api/admin/apps/{id}/roles", s.listAppRoles)
	m.HandleFunc("POST /api/admin/apps/{id}/roles", s.saveAppRole)
	m.HandleFunc("PUT /api/admin/apps/{id}/roles/{role}", s.saveAppRole)
	m.HandleFunc("DELETE /api/admin/apps/{id}/roles/{role}", s.deleteAppRole)
	m.HandleFunc("PUT /api/admin/apps/{id}/users/{user}/role", s.setAppUserRole)
	m.HandleFunc("GET /api/admin/apps/{id}/users", s.appUsers)
	m.HandleFunc("PUT /api/admin/settings", s.saveSettings)
	m.HandleFunc("GET /api/admin/mail-preview", s.mailPreview)
	m.HandleFunc("GET /api/service/app", s.serviceApp)
	m.HandleFunc("GET /api/me/passkeys", s.listPasskeys)
	m.HandleFunc("POST /api/me/passkeys/begin", s.beginPasskey)
	m.HandleFunc("POST /api/me/passkeys/finish", s.finishPasskey)
	m.HandleFunc("DELETE /api/me/passkeys/{id}", s.deletePasskey)
	m.HandleFunc("POST /api/auth/qr/device/begin", s.beginPasskeyLogin)
	m.HandleFunc("POST /api/auth/qr/device/finish", s.finishPasskeyLogin)
	m.HandleFunc("POST /api/auth/passkey/begin", s.beginPasskeyLogin)
	m.HandleFunc("POST /api/auth/passkey/finish", s.finishPasskeyLogin)
	m.HandleFunc("POST /api/auth/qr", s.createQR)
	m.HandleFunc("POST /api/auth/qr/{id}/poll", s.pollQR)
	m.HandleFunc("POST /api/auth/qr/{id}/cancel", s.cancelQR)
	m.HandleFunc("POST /api/auth/qr/context", s.qrContext)
	m.HandleFunc("POST /api/auth/qr/approve", s.approveQR)
	m.HandleFunc("GET /scan", s.scanPage)
	s.registerManagementRoutes(m)
	m.HandleFunc("GET /api/openapi.json", s.openapi)
	m.Handle("/oidc/", s.protocolHandler())
	m.Handle("/.well-known/", s.protocolHandler())
	// OIDC endpoints are discovered by clients; do not guess paths in client code.
	for _, p := range []string{"/authorize", "/authorize/", "/oauth/", "/userinfo", "/keys", "/end_session", "/revoke", "/introspect"} {
		m.Handle(p, s.protocolHandler())
	}
	m.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			fail(w, 404, "接口不存在")
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			http.NotFound(w, r)
			return
		}
		// Authentication assets use fixed paths; revalidate them after frontend updates.
		if strings.HasPrefix(r.URL.Path, "/auth/") || strings.HasPrefix(r.URL.Path, "/login-ui/") {
			w.Header().Set("Cache-Control", "no-cache")
		}
		http.FileServer(http.Dir("admin/dist")).ServeHTTP(w, r)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := randomID()
		w.Header().Set("X-Request-ID", requestID)
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("X-Duoyu-API-Version", "1")
		}
		r = r.WithContext(context.WithValue(r.Context(), requestIDKey{}, requestID))
		r = s.withClientIP(r)
		if c, e := r.Cookie("duoyu_session"); e == nil {
			r = r.WithContext(context.WithValue(r.Context(), sessionContextKey{}, digest(c.Value)))
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/login" {
			w.Header().Set("Cache-Control", "no-store")
		}
		if !s.dev {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000")
		}
		// Server-to-server sending authenticates exclusively with a scoped Bearer key.
		serviceMail := (r.Method == "POST" && r.URL.Path == "/api/service/mail") || strings.HasPrefix(r.URL.Path, "/api/management/")
		if strings.HasPrefix(r.URL.Path, "/api/") && r.Method != "GET" && r.Method != "HEAD" && !serviceMail {
			if !s.allowedBrowserOrigin(r) || r.Header.Get("X-Duoyu-Request") != "1" {
				fail(w, 403, "请求来源不受信任")
				return
			}
		}
		r.Body = http.MaxBytesReader(w, r.Body, 3<<20)
		m.ServeHTTP(w, r)
	})
}
func decode(r *http.Request, v any) error {
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return errors.New("request must contain a single JSON value")
	}
	return nil
}
func normalizeEmail(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	a, e := mail.ParseAddress(v)
	if e != nil || a.Address != v || len(v) > 254 {
		return ""
	}
	return v
}

type requestIDKey struct{}

func (s *Server) audit(ctx context.Context, uid, action, target string, success bool) {
	slog.Info("audit event", "request_id", requestID(ctx), "action", action, "success", success)
	if _, e := s.db.Exec(ctx, "INSERT INTO audit(user_id,action,target,success,request_id) VALUES($1,$2,$3,$4,$5)", uid, action, target, success, requestID(ctx)); e != nil {
		slog.Error("audit write failed")
	}
}
func (s *Server) limited(r *http.Request, key string, max int, period time.Duration) bool {
	var n int
	e := s.db.QueryRow(r.Context(), `INSERT INTO rate_limits(key,count,until_at) VALUES($1,1,$2) ON CONFLICT(key) DO UPDATE SET count=CASE WHEN rate_limits.until_at<now() THEN 1 ELSE rate_limits.count+1 END, until_at=CASE WHEN rate_limits.until_at<now() THEN EXCLUDED.until_at ELSE rate_limits.until_at END RETURNING count`, key, time.Now().Add(period)).Scan(&n)
	return e != nil || n > max
}
func remoteIP(r *http.Request) string {
	if ip, ok := r.Context().Value(clientIPKey{}).(string); ok {
		return ip
	}
	h, _, _ := net.SplitHostPort(r.RemoteAddr)
	return h
}
func (s *Server) user(ctx context.Context, id string) (u User, e error) {
	e = s.db.QueryRow(ctx, "SELECT id,email,name,avatar,admin,enabled,created_at,management_role,account_id FROM users WHERE id=$1", id).Scan(&u.ID, &u.Email, &u.Name, &u.Avatar, &u.Admin, &u.Enabled, &u.Created, &u.ManagementRole, &u.AccountID)
	if e == nil && !u.Enabled {
		e = errDisabledUser
	}
	return
}
func (s *Server) current(r *http.Request) (u User, e error) {
	if management, ok := r.Context().Value(managementContextKey{}).(*managementAuth); ok {
		return management.Owner, nil
	}
	c, e := r.Cookie("duoyu_session")
	if e != nil {
		return u, e
	}
	var id string
	e = s.db.QueryRow(r.Context(), "SELECT user_id FROM sessions WHERE hash=$1 AND expires_at>now()", digest(c.Value)).Scan(&id)
	if e != nil {
		return u, e
	}
	return s.user(r.Context(), id)
}
func (s *Server) require(w http.ResponseWriter, r *http.Request, admin bool) (User, bool) {
	u, e := s.current(r)
	if e != nil {
		if errors.Is(e, http.ErrNoCookie) || errors.Is(e, pgx.ErrNoRows) || errors.Is(e, errDisabledUser) {
			fail(w, 401, "请先登录")
		} else {
			fail(w, 503, "认证服务暂不可用，请稍后重试")
		}
		return u, false
	}
	if admin && !u.Admin && !s.managementAllowed(r, u) {
		fail(w, 403, "需要管理员权限")
		return u, false
	}
	return u, true
}
func (s *Server) cookie(w http.ResponseWriter, name, val string, age int) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: val, Path: "/", HttpOnly: true, Secure: !s.dev, SameSite: http.SameSiteLaxMode, MaxAge: age})
}
func (s *Server) newSession(w http.ResponseWriter, r *http.Request, id, amr string, credentialIDs ...string) error {
	credentialID := ""
	if len(credentialIDs) > 0 {
		credentialID = credentialIDs[0]
	}
	v := randomID()
	tx, e := s.db.Begin(r.Context())
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	// Serialize authenticator session creation with unbinding and account disable.
	if strings.Contains(" "+amr+" ", " totp ") {
		var current string
		if err := tx.QueryRow(r.Context(), "SELECT u.id FROM users u JOIN authenticators a ON a.user_id=u.id WHERE u.id=$1 AND u.enabled AND a.active FOR SHARE OF u,a", id).Scan(&current); err != nil {
			return err
		}
	}

	if c, e := r.Cookie("duoyu_session"); e == nil {
		if _, err := tx.Exec(r.Context(), "DELETE FROM sessions WHERE hash=$1", digest(c.Value)); err != nil {
			return err
		}
	}
	_, e = tx.Exec(r.Context(), "INSERT INTO sessions(hash,user_id,expires_at,agent,amr,passkey_id) VALUES($1,$2,$3,$4,$5,$6)", digest(v), id, time.Now().Add(12*time.Hour), truncate(r.UserAgent(), 300), amr, credentialID)
	if e == nil {
		e = tx.Commit(r.Context())
	}
	if e == nil {
		s.cookie(w, "duoyu_session", v, 43200)
	}
	return e
}
func truncate(v string, n int) string {
	if n <= 0 {
		return ""
	}
	if len(v) > n {
		for n > 0 && !utf8.RuneStart(v[n]) {
			n--
		}
		return v[:n]
	}
	return v
}
