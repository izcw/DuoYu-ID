// A localhost-only relying party. Run two instances with separate registered client IDs.
package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

type flow struct {
	State, Nonce, Verifier string
	Expires                time.Time
}
type session struct {
	Subject, Email, Name string
	Expires              time.Time
	Token                *oauth2.Token
	CenterSID            string
}

func random() string {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func main() {
	port := env("CLIENT_PORT", "8091")
	origin := "http://localhost:" + port
	issuer := env("DUOYU_ISSUER", "http://localhost:8080")
	clientID := os.Getenv("CLIENT_ID")
	if clientID == "" {
		log.Fatal("Register a public client in Duoyu ID, then set CLIENT_ID")
	}
	ctx := context.Background()
	provider, e := oidc.NewProvider(ctx, issuer)
	if e != nil {
		log.Fatal(e)
	}
	verifier := provider.Verifier(&oidc.Config{ClientID: clientID})
	config := oauth2.Config{ClientID: clientID, ClientSecret: os.Getenv("CLIENT_SECRET"), RedirectURL: origin + "/callback", Endpoint: provider.Endpoint(), Scopes: []string{oidc.ScopeOpenID, "profile", "email"}}
	if config.ClientSecret == "" {
		config.Endpoint.AuthStyle = oauth2.AuthStyleInParams
	} else {
		config.Endpoint.AuthStyle = oauth2.AuthStyleInHeader
	}
	var mu sync.Mutex
	flows := map[string]flow{}
	sessions := map[string]session{}
	logoutIDs := map[string]time.Time{}
	cookieName := "duoyu_demo_" + port
	setCookie := func(w http.ResponseWriter, name, value string, maxAge int) {
		http.SetCookie(w, &http.Cookie{Name: cookieName + name, Value: value, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: maxAge})
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /login", func(w http.ResponseWriter, r *http.Request) {
		f := flow{random(), random(), oauth2.GenerateVerifier(), time.Now().Add(5 * time.Minute)}
		id := random()
		mu.Lock()
		for k, v := range flows {
			if time.Now().After(v.Expires) {
				delete(flows, k)
			}
		}
		flows[id] = f
		mu.Unlock()
		setCookie(w, "_flow_"+f.State, id, 300)
		options := []oauth2.AuthCodeOption{oidc.Nonce(f.Nonce), oauth2.S256ChallengeOption(f.Verifier)}
		if r.URL.Query().Get("offline") == "1" {
			options = append(options, oauth2.SetAuthURLParam("scope", "openid profile email offline_access"), oauth2.SetAuthURLParam("prompt", "consent"))
		}
		if prompt := r.URL.Query().Get("prompt"); prompt == "login" || prompt == "none" {
			options = append(options, oauth2.SetAuthURLParam("prompt", prompt))
		}
		if r.URL.Query().Get("max_age") == "0" {
			options = append(options, oauth2.SetAuthURLParam("max_age", "0"))
		}
		http.Redirect(w, r, config.AuthCodeURL(f.State, options...), http.StatusFound)
	})
	mux.HandleFunc("GET /callback", func(w http.ResponseWriter, r *http.Request) {
		cookie, e := r.Cookie(cookieName + "_flow_" + r.URL.Query().Get("state"))
		if e != nil {
			http.Error(w, "Missing flow cookie", 400)
			return
		}
		mu.Lock()
		f, ok := flows[cookie.Value]
		delete(flows, cookie.Value)
		mu.Unlock()
		setCookie(w, "_flow_"+f.State, "", -1)
		if !ok || time.Now().After(f.Expires) || r.URL.Query().Get("state") != f.State {
			http.Error(w, "Invalid or expired state", 400)
			return
		}
		if r.URL.Query().Get("error") != "" {
			http.Error(w, "Authorization was not completed", 400)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		token, e := config.Exchange(ctx, r.URL.Query().Get("code"), oauth2.VerifierOption(f.Verifier))
		if e != nil {
			http.Error(w, "Code exchange failed", 400)
			return
		}
		raw, ok := token.Extra("id_token").(string)
		if !ok {
			http.Error(w, "Missing ID token", 400)
			return
		}
		idToken, e := verifier.Verify(ctx, raw)
		if e != nil || idToken.Nonce != f.Nonce {
			http.Error(w, "Invalid ID token", 400)
			return
		}
		var claims struct {
			Email string `json:"email"`
			Name  string `json:"name"`
			SID   string `json:"sid"`
		}
		if e = idToken.Claims(&claims); e != nil {
			http.Error(w, "Invalid claims", 400)
			return
		}
		// UserInfo verifies the access token is still active and returns allowed profile fields.
		info, e := provider.UserInfo(ctx, oauth2.StaticTokenSource(token))
		if e != nil || info.Subject != idToken.Subject {
			http.Error(w, "UserInfo rejected", 401)
			return
		}
		_ = info.Claims(&claims)
		sid := random()
		mu.Lock()
		for k, v := range sessions {
			if time.Now().After(v.Expires) {
				delete(sessions, k)
			}
		}
		sessions[sid] = session{idToken.Subject, claims.Email, claims.Name, time.Now().Add(5 * time.Minute), token, claims.SID}
		mu.Unlock()
		setCookie(w, "_session", sid, 300)
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})
	mux.HandleFunc("POST /backchannel-logout", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
		if r.ParseForm() != nil {
			http.Error(w, "Invalid form", 400)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		token, err := provider.Verifier(&oidc.Config{ClientID: clientID, SkipExpiryCheck: true}).Verify(ctx, r.PostForm.Get("logout_token"))
		if err != nil {
			http.Error(w, "Invalid signature or audience", 400)
			return
		}
		var claims struct {
			SID    string                     `json:"sid"`
			JTI    string                     `json:"jti"`
			IAT    int64                      `json:"iat"`
			Nonce  *json.RawMessage           `json:"nonce"`
			Events map[string]json.RawMessage `json:"events"`
		}
		if token.Claims(&claims) != nil || claims.SID == "" || token.Subject == "" || claims.JTI == "" || claims.Nonce != nil || claims.IAT < time.Now().Add(-5*time.Minute).Unix() || claims.IAT > time.Now().Add(time.Minute).Unix() {
			http.Error(w, "Invalid logout claims", 400)
			return
		}
		if event, ok := claims.Events["http://schemas.openid.net/event/backchannel-logout"]; !ok || string(event) != "{}" {
			http.Error(w, "Invalid logout event", 400)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		for id, expires := range logoutIDs {
			if time.Now().After(expires) {
				delete(logoutIDs, id)
			}
		}
		if _, seen := logoutIDs[claims.JTI]; !seen {
			for id, sess := range sessions {
				if sess.CenterSID == claims.SID && sess.Subject == token.Subject {
					delete(sessions, id)
				}
			}
			logoutIDs[claims.JTI] = time.Now().Add(10 * time.Minute)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /logout", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") != origin {
			http.Error(w, "Invalid origin", 403)
			return
		}
		if c, e := r.Cookie(cookieName + "_session"); e == nil {
			mu.Lock()
			delete(sessions, c.Value)
			mu.Unlock()
		}
		setCookie(w, "_session", "", -1)
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})
	view := template.Must(template.New("page").Parse(`<!doctype html><html lang="zh-CN"><meta charset="utf-8"><title>多鱼接入示例</title><style>body{font:16px system-ui;background:#edf5f3;color:#183d38;max-width:700px;margin:80px auto;padding:24px}main{padding:40px;border-radius:24px;background:white}a,button{color:#087e73}pre{white-space:pre-wrap;overflow-wrap:anywhere}</style><main><p>DUOYU ID · OIDC CLIENT {{.Port}}</p><h1>这是你的独立项目</h1>{{if .SignedIn}}<p>已验证登录身份：</p><pre>{{.JSON}}</pre><form method="post" action="/logout"><button>退出这个项目</button></form>{{else}}<p>业务数据仍由这个项目自己保存。</p><a href="/login">使用多鱼账号登录 →</a>{{end}}<p>示例本地会话 5 分钟。另一个示例项目可复用多鱼中心会话。</p></main></html>`))
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		var user session
		var ok bool
		if c, e := r.Cookie(cookieName + "_session"); e == nil {
			mu.Lock()
			user, ok = sessions[c.Value]
			mu.Unlock()
			ok = ok && time.Now().Before(user.Expires)
			if ok && os.Getenv("CLIENT_ONLINE_CHECK") != "false" {
				// Fail closed on revocation, disabled user/project, removed grant or
				// unavailable center. Production apps may apply this to sensitive routes.
				ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
				info, err := provider.UserInfo(ctx, oauth2.StaticTokenSource(user.Token))
				cancel()
				ok = err == nil && info.Subject == user.Subject
				if !ok {
					mu.Lock()
					delete(sessions, c.Value)
					mu.Unlock()
					setCookie(w, "_session", "", -1)
				}
			}
		}
		data, _ := json.MarshalIndent(map[string]string{"iss": issuer, "sub": user.Subject, "name": user.Name, "email": user.Email}, "", "  ")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_ = view.Execute(w, map[string]any{"Port": port, "SignedIn": ok, "JSON": string(data)})
	})
	fmt.Println("Demo client ready:", origin)
	log.Fatal((&http.Server{Addr: "127.0.0.1:" + port, Handler: mux, ReadHeaderTimeout: 5 * time.Second}).ListenAndServe())
}
