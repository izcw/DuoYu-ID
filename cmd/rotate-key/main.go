// Rotate an issuer signing key without removing the previous verification key.
package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func rotate(directory string) error {
	current := filepath.Join(directory, "signing.pem")
	old, e := os.ReadFile(current)
	if e != nil {
		return e
	}
	block, _ := pem.Decode(old)
	if block == nil {
		return fmt.Errorf("invalid current key")
	}
	previous, e := x509.ParsePKCS1PrivateKey(block.Bytes)
	if e != nil {
		return e
	}
	next, e := rsa.GenerateKey(rand.Reader, 3072)
	if e != nil {
		return e
	}
	archive := filepath.Join(directory, "verification-keys")
	if e = os.MkdirAll(archive, 0700); e != nil {
		return e
	}
	public := x509.MarshalPKCS1PublicKey(&previous.PublicKey)
	sum := sha256.Sum256(public)
	filename := filepath.Join(archive, hex.EncodeToString(sum[:])[:16]+".pem")
	if e = os.WriteFile(filename, pem.EncodeToMemory(&pem.Block{Type: "RSA PUBLIC KEY", Bytes: public}), 0600); e != nil {
		return e
	}
	temp, e := os.OpenFile(current+".next", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	_, e = temp.Write(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(next)}))
	if e == nil {
		e = temp.Sync()
	}
	closeErr := temp.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(current+".next", current)
}
func main() {
	directory := flag.String("data", "data", "issuer data directory; back up before rotation")
	flag.Parse()
	if e := rotate(*directory); e != nil {
		fmt.Fprintln(os.Stderr, "Rotation failed:", e)
		os.Exit(1)
	}
	fmt.Println("Key rotated on disk. Restart all API/worker instances; retain previous public keys until old tokens and callback retries have expired.")
}
