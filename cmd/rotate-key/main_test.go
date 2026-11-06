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

func TestRotationReservation(t *testing.T) {
	dir := t.TempDir()
	reserved := filepath.Join(dir, "signing.pem.next")
	if err := os.WriteFile(reserved, []byte("another rotation"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := rotate(dir); !os.IsExist(err) {
		t.Fatalf("rotation must reserve the destination before reading the current key: %v", err)
	}
	if got, err := os.ReadFile(reserved); err != nil || string(got) != "another rotation" {
		t.Fatal("existing reservation was modified")
	}
	if err := os.Remove(reserved); err != nil {
		t.Fatal(err)
	}
	if err := rotate(dir); !os.IsNotExist(err) {
		t.Fatalf("missing signing key: %v", err)
	}
	if _, err := os.Stat(reserved); !os.IsNotExist(err) {
		t.Fatal("failed rotation left a reservation")
	}
}
