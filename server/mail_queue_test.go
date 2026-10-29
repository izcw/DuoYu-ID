package main

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func testMailQueue(t *testing.T, s *Server) {
	ctx := context.Background()
	if e := s.migrate(ctx); e != nil {
		t.Fatal("repeat migration", e)
	}
	var count int
	s.db.QueryRow(ctx, "SELECT count(*) FROM schema_migrations").Scan(&count)
	if count != 22 {
		t.Fatal("migration versions", count)
	}
	s.db.Exec(ctx, "UPDATE schema_migrations SET checksum='tampered' WHERE version='schema.sql'")
	if e := s.migrate(ctx); e == nil {
		t.Fatal("migration tampering accepted")
	}
	baseline, _ := assets.ReadFile("schema.sql")
	s.db.Exec(ctx, "UPDATE schema_migrations SET checksum=$1 WHERE version='schema.sql'", migrationChecksum(baseline))
	email := "queue-test@example.com"
	code := "982714"
	s.db.Exec(ctx, "INSERT INTO challenges(email,purpose,digest,expires_at) VALUES($1,'login',$2,now()+interval '5 minutes')", email, s.mac(email+"|login|"+code))
	if e := s.sendMail(email, code, "login"); e != nil {
		t.Fatal(e)
	}
	var id string
	var encrypted []byte
	s.db.QueryRow(ctx, "SELECT id,payload FROM mail_jobs WHERE lane='auth' ORDER BY created_at DESC LIMIT 1").Scan(&id, &encrypted)
	if strings.Contains(string(encrypted), code) {
		t.Fatal("plaintext OTP in queue")
	}
	if _, e := s.decryptMail("wrong-id", encrypted); e == nil {
		t.Fatal("payload binding not enforced")
	}
	// A new service object can decrypt jobs left in the database after a restart.
	restarted := &Server{db: s.db, key: s.key}
	var calls atomic.Int32
	restarted.mailTransport = func(MailMessage) error { calls.Add(1); return &deliveryError{kind: "retry"} }
	if _, e := restarted.processMail(ctx, "auth"); e != nil {
		t.Fatal(e)
	}
	var status string
	s.db.QueryRow(ctx, "SELECT status FROM mail_jobs WHERE id=$1", id).Scan(&status)
	if status != "retry" {
		t.Fatal(status)
	}
	s.db.Exec(ctx, "UPDATE mail_jobs SET available_at=now() WHERE id=$1", id)
	restarted.mailTransport = func(MailMessage) error { calls.Add(1); time.Sleep(30 * time.Millisecond); return nil }
	var wg sync.WaitGroup
	for range 3 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := restarted.processMail(ctx, "auth"); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 2 {
		t.Fatal("duplicate worker delivery", calls.Load())
	}
	var cleared bool
	s.db.QueryRow(ctx, "SELECT status,payload IS NULL FROM mail_jobs WHERE id=$1", id).Scan(&status, &cleared)
	if status != "accepted" || !cleared {
		t.Fatal("terminal payload not erased")
	}
	if e := s.sendMail(email, code, "login"); e != nil {
		t.Fatal(e)
	}
	s.db.QueryRow(ctx, "SELECT id FROM mail_jobs WHERE lane='auth' AND status='queued' ORDER BY created_at DESC LIMIT 1").Scan(&id)
	restarted.mailTransport = func(MailMessage) error { return &deliveryError{kind: "retry"} }
	for range 3 {
		s.db.Exec(ctx, "UPDATE mail_jobs SET available_at=now() WHERE id=$1", id)
		if _, e := restarted.processMail(ctx, "auth"); e != nil {
			t.Fatal(e)
		}
	}
	s.db.QueryRow(ctx, "SELECT status,payload IS NULL FROM mail_jobs WHERE id=$1", id).Scan(&status, &cleared)
	if status != "failed" || !cleared {
		t.Fatal("retry limit did not erase failed payload")
	}
	// Expiry and interrupted sends must never be replayed.
	for _, state := range []string{"queued", "sending"} {
		job := "recovery-" + state
		payload, _ := s.encryptMail(job, queuedMail{Message: MailMessage{To: email}})
		s.db.Exec(ctx, "INSERT INTO mail_jobs(id,lane,payload,status,expires_at,locked_at) VALUES($1,'auth',$2,$3,now()-interval '1 minute',now()-interval '3 minutes')", job, payload, state)
	}
	restarted.processMail(ctx, "auth")
	if calls.Load() != 2 {
		t.Fatal("expired or interrupted job resent")
	}
	s.db.QueryRow(ctx, "SELECT count(*) FROM mail_jobs WHERE id LIKE 'recovery-%' AND payload IS NULL AND status IN ('expired','unknown')").Scan(&count)
	if count != 2 {
		t.Fatal("recovery states", count)
	}
}
