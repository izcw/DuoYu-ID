package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/jackc/pgx/v5"
)

func testPasskeyChallenges(t *testing.T, s *Server, origin string) {
	ctx := context.Background()
	u := User{ID: randomID(), Email: "enroll@example.invalid"}
	if _, err := s.db.Exec(ctx, "INSERT INTO users(id,email,name) VALUES($1,$2,'enroll')", u.ID, u.Email); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", origin, nil)
	login := httptest.NewRecorder()
	if err := s.newSession(login, r, u.ID, "pwd"); err != nil {
		t.Fatal(err)
	}
	r.AddCookie(login.Result().Cookies()[0])
	if _, err := s.db.Exec(ctx, "INSERT INTO challenges(email,purpose,digest,expires_at) VALUES($1,'login',$2,now()+interval '2 minutes')", u.Email, s.mac(u.Email+"|login|123456")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(ctx, `CREATE FUNCTION fail_wa_begin() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION 'fixture'; END$$;
		CREATE TRIGGER fail_wa_begin BEFORE INSERT ON webauthn_challenges FOR EACH ROW EXECUTE FUNCTION fail_wa_begin()`); err != nil {
		t.Fatal(err)
	}
	data := webauthn.SessionData{Challenge: "fixture"}
	w := httptest.NewRecorder()
	if s.storeWA(w, r, &u, "123456", "register", &data, true) || w.Code != 503 || len(w.Result().Cookies()) != 0 {
		t.Fatal("failed challenge published", w.Code)
	}
	if _, err := s.db.Exec(ctx, "DROP TRIGGER fail_wa_begin ON webauthn_challenges; DROP FUNCTION fail_wa_begin()"); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	if !s.storeWA(w, r, &u, "123456", "register", &data, true) {
		t.Fatal("same code could not retry", w.Code)
	}
	challenge := w.Result().Cookies()[0]
	other := httptest.NewRequest("POST", origin, nil)
	secondLogin := httptest.NewRecorder()
	if err := s.newSession(secondLogin, other, u.ID, "pwd"); err != nil {
		t.Fatal(err)
	}
	other.AddCookie(secondLogin.Result().Cookies()[0])
	other.AddCookie(challenge)
	if _, _, err := s.takeWA(httptest.NewRecorder(), other, u.ID, "register"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("different live session consumed registration", err)
	}
	r.AddCookie(challenge)
	got, qr, err := s.takeWA(httptest.NewRecorder(), r, u.ID, "register")
	if err != nil || !qr || got.Challenge != data.Challenge {
		t.Fatal("original session could not finish", err, qr)
	}
	if _, _, err := s.takeWA(httptest.NewRecorder(), r, u.ID, "register"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("replay accepted", err)
	}

	// Existing in-flight registrations carried the unqualified purpose.
	legacy := randomID()
	if _, err := s.db.Exec(ctx, `INSERT INTO webauthn_challenges(id,user_id,purpose,data,expires_at,qr_enabled) VALUES($1,$2,'register','{}',now()+interval '2 minutes',true)`, digest(legacy), u.ID); err != nil {
		t.Fatal(err)
	}
	old := httptest.NewRequest("POST", origin, nil)
	old.AddCookie(login.Result().Cookies()[0])
	old.AddCookie(&http.Cookie{Name: "duoyu_webauthn", Value: legacy})
	if _, qr, err := s.takeWA(httptest.NewRecorder(), old, u.ID, "register"); err != nil || !qr {
		t.Fatal("legacy registration rejected", err)
	}
}
