package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

// Rejection sampling guarantees all three character classes without fixed positions.
func randomRegistrationPassword() (string, error) {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	for {
		password := make([]byte, 32)
		upper, lower, digit := false, false, false
		for i := range password {
			n, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
			if err != nil {
				return "", err
			}
			c := alphabet[n.Int64()]
			password[i] = c
			upper = upper || c >= 'A' && c <= 'Z'
			lower = lower || c >= 'a' && c <= 'z'
			digit = digit || c >= '0' && c <= '9'
		}
		if upper && lower && digit {
			return string(password), nil
		}
	}
}

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
	if !utf8.ValidString(v) || utf8.RuneCountInString(v) < 6 || utf8.RuneCountInString(v) > 64 {
		return fmt.Errorf("密码须为 6–64 个字符")
	}
	return nil
}
