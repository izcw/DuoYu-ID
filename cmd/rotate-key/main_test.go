package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

func TestRotationPreservesVerification(t *testing.T) {
	dir := t.TempDir()
	key, e := rsa.GenerateKey(rand.Reader, 2048)
	if e != nil {
		t.Fatal(e)
	}
	p := filepath.Join(dir, "signing.pem")
	old := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	if e = os.WriteFile(p, old, 0600); e != nil {
		t.Fatal(e)
	}
	if e = rotate(dir); e != nil {
		t.Fatal(e)
	}
	next, _ := os.ReadFile(p)
	if string(next) == string(old) {
		t.Fatal("key unchanged")
	}
	files, _ := filepath.Glob(filepath.Join(dir, "verification-keys", "*.pem"))
	if len(files) != 1 {
		t.Fatal("missing old public key")
	}
	body, _ := os.ReadFile(files[0])
	block, _ := pem.Decode(body)
	pub, e := x509.ParsePKCS1PublicKey(block.Bytes)
	if e != nil || !pub.Equal(&key.PublicKey) {
		t.Fatal("wrong archived key")
	}
}
