package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"net/http"
	"slices"
	"strings"
	"time"
)

func (s *Server) listManagementKeys(w http.ResponseWriter, r *http.Request) {
	if _, yes := s.require(w, r, true); !yes {
		return
	}
	s.queryList(w, r, `SELECT id,name,scopes,all_projects,expires_at,revoked,created_at,last_used_at,ARRAY(SELECT app_id FROM management_api_grants WHERE key_id=k.id UNION SELECT k.app_id WHERE k.app_id IS NOT NULL) AS app_ids FROM api_keys k ORDER BY created_at DESC`)
}
func (s *Server) createManagementKey(w http.ResponseWriter, r *http.Request) {
	owner, yes := s.require(w, r, true)
	if !yes {
		return
	}
	var in struct {
		Name      string   `json:"name"`
		Scopes    []string `json:"scopes"`
		All       bool     `json:"all_projects"`
		Apps      []string `json:"app_ids"`
		Permanent bool     `json:"permanent"`
		Days      int      `json:"days"`
	}
	if decode(r, &in) != nil || len(strings.TrimSpace(in.Name)) < 1 || len([]rune(in.Name)) > 100 || len(in.Scopes) < 1 || len(in.Scopes) > len(managementScopes) || len(in.Apps) > 100 || !in.Permanent && (in.Days < 1 || in.Days > 365) {
		fail(w, 400, "请检查凭证名称、权限、项目与有效期")
		return
	}
	seen := map[string]bool{}
	for _, scope := range in.Scopes {
		if seen[scope] || !slices.Contains(managementScopes, scope) {
			fail(w, 400, "凭证权限无效或重复")
			return
		}
		seen[scope] = true
	}
	id, secret := "mgmt_"+randomID()[:20], "dy_api_"+randomID()
	var expiry *time.Time
	if !in.Permanent {
		v := time.Now().Add(time.Duration(in.Days) * 24 * time.Hour)
		expiry = &v
	}
	ctx := r.Context()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		fail(w, 503, "创建凭证失败")
		return
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "INSERT INTO api_keys(id,name,digest,owner_id,scopes,all_projects,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7)", id, strings.TrimSpace(in.Name), s.mac(secret), owner.ID, in.Scopes, in.All, expiry); err != nil {
		fail(w, 503, "创建凭证失败")
		return
	}
	for _, app := range in.Apps {
		tag, err := tx.Exec(ctx, "INSERT INTO management_api_grants(key_id,app_id) SELECT $1,id FROM apps WHERE id=$2 ON CONFLICT DO NOTHING", id, app)
		if err != nil || tag.RowsAffected() != 1 {
			fail(w, 400, "项目不存在或重复")
			return
		}
	}
	if err = tx.Commit(ctx); err != nil {
		fail(w, 503, "创建凭证失败")
		return
	}
	s.audit(ctx, owner.ID, "management.key.created", id, true)
	ok(w, map[string]string{"id": id, "secret": secret})
}
func (s *Server) revokeManagementKey(w http.ResponseWriter, r *http.Request) {
	owner, yes := s.require(w, r, true)
	if !yes {
		return
	}
	tag, err := s.db.Exec(r.Context(), "UPDATE api_keys SET revoked=true WHERE id=$1", r.PathValue("key"))
	if err != nil {
		fail(w, 503, "撤销失败")
		return
	}
	if tag.RowsAffected() != 1 {
		fail(w, 404, "凭证不存在")
		return
	}
	s.audit(r.Context(), owner.ID, "management.key.revoked", r.PathValue("key"), true)
	ok(w, nil)
}
func (s *Server) managementCipher() (cipher.AEAD, error) {
	key := sha256.Sum256(append([]byte("duoyu-management-result-v1:"), s.key[:]...))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
func (s *Server) createManagedProject(w http.ResponseWriter, r *http.Request, auth *managementAuth, a App) {
	ctx := r.Context()
	requestKey := r.Header.Get("Idempotency-Key")
	raw, err := json.Marshal(a)
	if err != nil {
		fail(w, 400, "参数错误")
		return
	}
	bodyDigest := digest(string(raw))
	binding := auth.ID + ":" + requestKey
	tx, err := s.db.Begin(ctx)
	if err != nil {
		fail(w, 503, "创建项目暂不可用")
		return
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", "management-create:"+binding); err != nil {
		fail(w, 503, "创建项目暂不可用")
		return
	}
	var stored, app string
	var encrypted []byte
	var fresh bool
	err = tx.QueryRow(ctx, "SELECT body_digest,app_id,result_cipher,created_at>now()-interval '24 hours' FROM management_create_requests WHERE key_id=$1 AND request_key=$2", auth.ID, requestKey).Scan(&stored, &app, &encrypted, &fresh)
	aead, cryptoErr := s.managementCipher()
	if cryptoErr != nil {
		fail(w, 503, "创建项目暂不可用")
		return
	}
	if err == nil {
		if stored != bodyDigest {
			fail(w, 409, "相同 Idempotency-Key 已用于不同参数")
			return
		}
		var allowed bool
		var currentHash string
		if err := tx.QueryRow(ctx, "SELECT ($2 OR EXISTS(SELECT 1 FROM management_api_grants WHERE key_id=$3 AND app_id=$1)),secret_hash FROM apps WHERE id=$1", app, auth.All, auth.ID).Scan(&allowed, &currentHash); err != nil {
			fail(w, 503, "读取已创建项目失败")
			return
		}
		if !allowed {
			fail(w, 403, "已失去该项目的管理权限")
			return
		}
		if !fresh || len(encrypted) < aead.NonceSize() {
			fail(w, 410, "项目已创建，密钥重放窗口已过期；请查询项目或显式轮换密钥")
			return
		}
		plain, err := aead.Open(nil, encrypted[:aead.NonceSize()], encrypted[aead.NonceSize():], []byte(binding))
		if err != nil {
			fail(w, 503, "读取创建结果失败")
			return
		}
		var result map[string]any
		if json.Unmarshal(plain, &result) != nil {
			fail(w, 503, "读取创建结果失败")
			return
		}
		if secret, ok := result["secret"].(string); ok && secret != "" && s.mac(secret) != currentHash {
			fail(w, 409, "客户端密钥已轮换，不能重放旧创建密钥")
			return
		}
		w.Header().Set("Idempotent-Replayed", "true")
		ok(w, result)
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		fail(w, 503, "创建项目暂不可用")
		return
	}
	app = "app_" + randomID()[:20]
	secret := ""
	secretHash := ""
	if !a.Public {
		secret = randomID()
		secretHash = s.mac(secret)
	}
	_, err = tx.Exec(ctx, `INSERT INTO apps(id,name,description,secret_hash,redirects,logout_redirects,public,enabled,restricted,refresh_enabled,platform,backchannel_logout_uri,logo,allowed_origins,login_methods) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`, app, a.Name, a.Description, secretHash, a.Redirects, a.LogoutRedirects, a.Public, a.Enabled, a.Restricted, a.RefreshEnabled, a.Platform, a.BackchannelLogoutURI, a.Logo, a.AllowedOrigins, a.LoginMethods)
	if err != nil {
		fail(w, 503, "创建项目失败")
		return
	}
	if _, err = tx.Exec(ctx, "INSERT INTO management_api_grants(key_id,app_id) VALUES($1,$2)", auth.ID, app); err != nil {
		fail(w, 503, "创建项目失败")
		return
	}
	result := map[string]any{"id": app, "client_id": app, "secret": secret, "revision": 0, "issuer": s.origin}
	plain, _ := json.Marshal(result)
	nonce := make([]byte, aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		fail(w, 503, "创建项目失败")
		return
	}
	encrypted = aead.Seal(nonce, nonce, plain, []byte(binding))
	if _, err = tx.Exec(ctx, "INSERT INTO management_create_requests(key_id,request_key,body_digest,app_id,result_cipher) VALUES($1,$2,$3,$4,$5)", auth.ID, requestKey, bodyDigest, app, encrypted); err != nil {
		fail(w, 503, "创建项目失败")
		return
	}
	if err = tx.Commit(ctx); err != nil {
		fail(w, 503, "创建项目失败")
		return
	}
	s.audit(ctx, auth.Owner.ID, "management.project.created", app, true)
	ok(w, result)
}
func (s *Server) adminAIGuide(w http.ResponseWriter, r *http.Request) {
	if _, yes := s.require(w, r, true); !yes {
		return
	}
	ok(w, map[string]string{"guide": s.aiIntegrationGuide(), "issuer": s.origin})
}
func (s *Server) aiIntegrationGuide() string {
	body, err := assets.ReadFile("templates/ai-integration.md")
	if err != nil {
		return "接入说明暂不可用"
	}
	return strings.ReplaceAll(string(body), "{{ORIGIN}}", s.origin)
}
