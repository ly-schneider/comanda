package database

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const verifyFullDSN = "postgres://u:p@db.example:5432/comanda?sslmode=verify-full"

// writeTestCA writes a throwaway self-signed CA PEM to a temp file and returns
// its path.
func writeTestCA(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	path := filepath.Join(t.TempDir(), "ca.crt")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatalf("write CA: %v", err)
	}
	return path
}

func TestBuildConnConfig_PinsCA(t *testing.T) {
	cfg, err := buildConnConfig(verifyFullDSN, writeTestCA(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.TLSConfig == nil || cfg.TLSConfig.RootCAs == nil {
		t.Fatal("expected the CA pinned onto TLSConfig.RootCAs")
	}
}

func TestBuildConnConfig_EmptyCAPathLeavesRootsUnset(t *testing.T) {
	cfg, err := buildConnConfig(verifyFullDSN, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.TLSConfig != nil && cfg.TLSConfig.RootCAs != nil {
		t.Fatal("expected RootCAs left unset (system roots) when no CA path is configured")
	}
}

func TestBuildConnConfig_MissingFile(t *testing.T) {
	if _, err := buildConnConfig(verifyFullDSN, filepath.Join(t.TempDir(), "absent.crt")); err == nil {
		t.Fatal("expected an error for a missing CA file")
	}
}

func TestBuildConnConfig_InvalidPEM(t *testing.T) {
	path := filepath.Join(t.TempDir(), "junk.crt")
	if err := os.WriteFile(path, []byte("not a certificate"), 0o600); err != nil {
		t.Fatalf("write junk: %v", err)
	}
	if _, err := buildConnConfig(verifyFullDSN, path); err == nil {
		t.Fatal("expected an error when the file has no PEM certificates")
	}
}
