package main

import (
	"context"
	"encoding/json"
	"time"

	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"
)

// Deliberately does not implement op.AuthRequest: refreshing with narrower
// scopes must still rotate the refresh credential.
type refreshRequest struct{ Auth AuthRequest }

func (r *refreshRequest) GetAMR() []string                 { return r.Auth.GetAMR() }
func (r *refreshRequest) GetAudience() []string            { return r.Auth.GetAudience() }
func (r *refreshRequest) GetAuthTime() time.Time           { return r.Auth.Authenticated }
func (r *refreshRequest) GetClientID() string              { return r.Auth.GetClientID() }
func (r *refreshRequest) GetScopes() []string              { return r.Auth.GetScopes() }
func (r *refreshRequest) GetSubject() string               { return r.Auth.UserID }
func (r *refreshRequest) SetCurrentScopes(scopes []string) { r.Auth.Request.Scopes = scopes }

func (s *Server) TokenRequestByRefreshToken(ctx context.Context, token string) (op.RefreshTokenRequest, error) {
	var body []byte
	// Used tokens remain readable until client authorization is checked by the
	// provider. Replay is acted on inside the locked rotation transaction.
	err := s.db.QueryRow(ctx, `SELECT t.body FROM refresh_tokens t JOIN refresh_families f ON f.id=t.family_id JOIN sessions ss ON ss.hash=f.session_hash WHERE t.hash=$1 AND NOT f.revoked AND f.expires_at>now() AND ss.expires_at>now()`, digest(token)).Scan(&body)
	if err != nil {
		return nil, op.ErrInvalidRefreshToken
	}
	r := new(refreshRequest)
	if json.Unmarshal(body, &r.Auth) != nil || !s.allowed(ctx, r.Auth.UserID, r.Auth.GetClientID()) {
		return nil, op.ErrInvalidRefreshToken
	}
	return r, nil
}

func (s *Server) GetRefreshTokenInfo(ctx context.Context, client, token string) (string, string, error) {
	var uid string
	err := s.db.QueryRow(ctx, `SELECT f.user_id FROM refresh_tokens t JOIN refresh_families f ON f.id=t.family_id WHERE t.hash=$1 AND f.app_id=$2`, digest(token), client).Scan(&uid)
	if err != nil {
		return "", "", op.ErrInvalidRefreshToken
	}
	return uid, token, nil
}

func (s *Server) CreateAccessAndRefreshTokens(ctx context.Context, request op.TokenRequest, current string) (string, string, time.Time, error) {
	invalid := oidc.ErrInvalidGrant()
	var a AuthRequest
	switch r := request.(type) {
	case *AuthRequest:
		a = *r
	case *refreshRequest:
		a = r.Auth
	default:
		return "", "", time.Time{}, invalid
	}
	app, err := s.app(ctx, a.GetClientID())
	if err != nil || !app.RefreshEnabled || !s.allowed(ctx, a.UserID, a.GetClientID()) || a.SessionHash == "" {
		return "", "", time.Time{}, invalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return "", "", time.Time{}, err
	}
	defer tx.Rollback(ctx)
	family := randomID()
	if current == "" {
		// Absolute lifetime is the existing center session; refresh never extends it.
		tag, e := tx.Exec(ctx, `INSERT INTO refresh_families(id,user_id,app_id,session_hash,expires_at) SELECT $1,$2,$3,hash,expires_at FROM sessions WHERE hash=$4 AND user_id=$2 AND expires_at>now()`, family, a.UserID, a.GetClientID(), a.SessionHash)
		if e != nil {
			return "", "", time.Time{}, e
		}
		if tag.RowsAffected() != 1 {
			return "", "", time.Time{}, invalid
		}
	} else {
		var used, revoked bool
		var expiry time.Time
		var client, uid string
		err = tx.QueryRow(ctx, `SELECT f.id,t.used,f.revoked,f.expires_at,f.app_id,f.user_id FROM refresh_tokens t JOIN refresh_families f ON f.id=t.family_id WHERE t.hash=$1 FOR UPDATE OF f,t`, digest(current)).Scan(&family, &used, &revoked, &expiry, &client, &uid)
		if err != nil || revoked || time.Now().After(expiry) || client != a.GetClientID() || uid != a.UserID {
			return "", "", time.Time{}, invalid
		}
		if used {
			if _, err = tx.Exec(ctx, "UPDATE refresh_families SET revoked=true WHERE id=$1", family); err != nil {
				return "", "", time.Time{}, err
			}
			if err = tx.Commit(ctx); err != nil {
				return "", "", time.Time{}, err
			}
			s.audit(ctx, a.UserID, "token.refresh_reuse", a.GetClientID(), false)
			return "", "", time.Time{}, invalid
		}
		if _, err = tx.Exec(ctx, "UPDATE refresh_tokens SET used=true WHERE hash=$1", digest(current)); err != nil {
			return "", "", time.Time{}, err
		}
	}
	access, next, expiry := randomID(), randomID(), time.Now().Add(5*time.Minute)
	body, err := json.Marshal(a)
	if err != nil {
		return "", "", time.Time{}, err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO refresh_tokens(hash,family_id,body) VALUES($1,$2,$3)", digest(next), family, body); err != nil {
		return "", "", time.Time{}, err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO access_tokens(id,user_id,app_id,scopes,expires_at,session_hash,family_id) VALUES($1,$2,$3,$4,$5,$6,$7)", access, a.UserID, a.GetClientID(), a.GetScopes(), expiry, a.SessionHash, family); err != nil {
		return "", "", time.Time{}, err
	}
	if current == "" {
		if _, err = tx.Exec(ctx, "INSERT INTO app_users(app_id,user_id) VALUES($1,$2) ON CONFLICT(app_id,user_id) DO UPDATE SET last_login_at=now(),login_count=app_users.login_count+1", a.GetClientID(), a.UserID); err != nil {
			return "", "", time.Time{}, err
		}
	}
	return access, next, expiry, tx.Commit(ctx)
}
