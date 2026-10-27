package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

func randomID() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
func digest(v string) string { h := sha256.Sum256([]byte(v)); return hex.EncodeToString(h[:]) }
func (s *Server) mac(v string) string {
	h := hmac.New(sha256.New, s.key[:])
	h.Write([]byte(v))
	return hex.EncodeToString(h.Sum(nil))
}
func hashPassword(v string) string {
	salt := make([]byte, 16)
	if _, e := rand.Read(salt); e != nil {
		panic(e)
	}
	h := argon2.IDKey([]byte(v), salt, 2, 64*1024, 2, 32)
	return "a2$" + base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(h)
}
func checkPassword(v, h string) bool {
	if h == "" {
		checkPassword(v, dummyPasswordHash)
		return false
	}
	p := strings.Split(h, "$")
	if len(p) != 3 || p[0] != "a2" {
		return false
	}
	salt, e := base64.RawStdEncoding.DecodeString(p[1])
	if e != nil || len(salt) != 16 {
		return false
	}
	want, e := base64.RawStdEncoding.DecodeString(p[2])
	if e != nil || len(want) != 32 {
		return false
	}
	got := argon2.IDKey([]byte(v), salt, 2, 64*1024, 2, 32)
	return subtle.ConstantTimeCompare(got, want) == 1
}

var dummyPasswordHash = hashPassword(randomID())

func validPassword(v string) error {
	if len(v) < 12 || len(v) > 128 {
		return fmt.Errorf("密码须为 12–128 字节")
	}
	return nil
}
