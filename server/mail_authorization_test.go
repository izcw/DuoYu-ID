package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func testQueuedMailAuthorization(t *testing.T, s *Server, origin string) {
	ctx := context.Background()
	var owner string
	if err := s.db.QueryRow(ctx, "SELECT id FROM users WHERE admin").Scan(&owner); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, mutation       string
		legacy, all, allowed bool
	}{
		{name: "explicit grant", allowed: true},
		{name: "all projects", all: true, allowed: true},
		{name: "legacy project key", legacy: true, allowed: true},
		{name: "removed grant", mutation: "DELETE FROM management_api_grants WHERE key_id=$1"},
		{name: "removed all projects", all: true, mutation: "UPDATE api_keys SET all_projects=false WHERE id=$1"},
		{name: "disabled owner", mutation: "UPDATE users SET enabled=false WHERE id=(SELECT owner_id FROM api_keys WHERE id=$1)"},
		{name: "demoted owner", mutation: "UPDATE users SET admin=false WHERE id=(SELECT owner_id FROM api_keys WHERE id=$1)"},
		{name: "removed mail scope", mutation: "UPDATE api_keys SET scopes=ARRAY['app:read'] WHERE id=$1"},
		{name: "revoked key", mutation: "UPDATE api_keys SET revoked=true WHERE id=$1"},
		{name: "expired key", mutation: "UPDATE api_keys SET expires_at=now()-interval '1 second' WHERE id=$1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, key, secret := "queue-app-"+randomID(), "queue-key-"+randomID(), randomID()
			if _, err := s.db.Exec(ctx, "INSERT INTO apps(id,name,redirects) VALUES($1,'queue authorization',$2)", app, []string{"http://localhost/callback"}); err != nil {
				t.Fatal(err)
			}
			var keyOwner, legacyApp any = owner, nil
			if tc.legacy {
				keyOwner, legacyApp = nil, app
			}
			if _, err := s.db.Exec(ctx, "INSERT INTO api_keys(id,name,digest,owner_id,app_id,scopes,all_projects) VALUES($1,'queue authorization',$2,$3,$4,ARRAY['mail:send'],$5)", key, s.mac(secret), keyOwner, legacyApp, tc.all); err != nil {
				t.Fatal(err)
			}
			if !tc.legacy && !tc.all {
				if _, err := s.db.Exec(ctx, "INSERT INTO management_api_grants(key_id,app_id) VALUES($1,$2)", key, app); err != nil {
					t.Fatal(err)
				}
			}
			recipient := strings.ToLower(randomID()) + "@example.invalid"
			body, err := json.Marshal(ProjectMail{To: recipient, Subject: "Queue authorization", Text: "isolated fixture"})
			if err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequest("POST", origin+"/api/service/mail", bytes.NewReader(body))
			r.Header.Set("Authorization", "Bearer "+secret)
			r.Header.Set("X-Duoyu-Project", app)
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Idempotency-Key", randomID())
			w := httptest.NewRecorder()
			s.routes().ServeHTTP(w, r)
			if w.Code != 200 {
				t.Fatalf("enqueue status %d", w.Code)
			}
			defer s.db.Exec(ctx, "UPDATE users SET enabled=true,admin=true WHERE id=$1", owner)
			if tc.mutation != "" {
				if _, err := s.db.Exec(ctx, tc.mutation, key); err != nil {
					t.Fatal(err)
				}
			}
			calls := 0
			previous := s.mailTransport
			s.mailTransport = func(m MailMessage) error {
				if m.To == recipient {
					calls++
				}
				return nil
			}
			defer func() { s.mailTransport = previous }()
			var status string
			var cleared bool
			for attempts := 0; attempts < 100; attempts++ {
				if _, err := s.processMail(ctx, "project"); err != nil {
					t.Fatal(err)
				}
				if err := s.db.QueryRow(ctx, "SELECT j.status,j.payload IS NULL FROM mail_jobs j JOIN mail_messages m ON m.id=j.message_id WHERE m.key_id=$1", key).Scan(&status, &cleared); err != nil {
					t.Fatal(err)
				}
				if status != "queued" && status != "sending" {
					break
				}
			}
			want, count := "cancelled", 0
			if tc.allowed {
				want, count = "accepted", 1
			}
			if status != want || calls != count || !cleared {
				t.Fatalf("status=%s transport calls=%d cleared=%v; want %s/%d/true", status, calls, cleared, want, count)
			}
		})
	}
}
