package config

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

func TestInit(t *testing.T) {
	tmpDir := t.TempDir()

	err := Init(tmpDir)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	expectedDirs := []string{
		filepath.Join(tmpDir, "mailbox", "tmp"),
		filepath.Join(tmpDir, "mailbox", "new"),
		filepath.Join(tmpDir, "mailbox", "cur"),
		filepath.Join(tmpDir, "spillover"),
	}

	for _, d := range expectedDirs {
		info, err := os.Stat(d)
		if err != nil {
			t.Errorf("expected directory %s to exist: %v", d, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("expected %s to be a directory", d)
		}
	}

	expectedFiles := []struct {
		name string
		perm os.FileMode
	}{
		{"ca.key", 0600},
		{"ca.pem", 0644},
		{"key.pem", 0600},
		{"cert.pem", 0644},
		{"config.json", 0644},
	}

	for _, f := range expectedFiles {
		p := filepath.Join(tmpDir, f.name)
		info, err := os.Stat(p)
		if err != nil {
			t.Errorf("expected file %s to exist: %v", p, err)
			continue
		}
		if info.IsDir() {
			t.Errorf("expected %s to be a file, got directory", p)
		}
		if perm := info.Mode().Perm(); perm != f.perm {
			t.Errorf("file %s has permissions %o, want %o", f.name, perm, f.perm)
		}
	}

	caPEM, err := os.ReadFile(filepath.Join(tmpDir, "ca.pem"))
	if err != nil {
		t.Fatalf("failed to read ca.pem: %v", err)
	}
	caBlock, _ := pem.Decode(caPEM)
	if caBlock == nil || caBlock.Type != "CERTIFICATE" {
		t.Fatalf("invalid CA certificate PEM")
	}
	caCert, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil {
		t.Fatalf("failed to parse CA certificate: %v", err)
	}
	if !caCert.IsCA {
		t.Errorf("expected CA certificate to have IsCA = true")
	}

	certPEM, err := os.ReadFile(filepath.Join(tmpDir, "cert.pem"))
	if err != nil {
		t.Fatalf("failed to read cert.pem: %v", err)
	}
	leafBlock, _ := pem.Decode(certPEM)
	if leafBlock == nil || leafBlock.Type != "CERTIFICATE" {
		t.Fatalf("invalid leaf certificate PEM")
	}
	leafCert, err := x509.ParseCertificate(leafBlock.Bytes)
	if err != nil {
		t.Fatalf("failed to parse leaf certificate: %v", err)
	}

	roots := x509.NewCertPool()
	roots.AddCert(caCert)
	opts := x509.VerifyOptions{
		Roots:     roots,
		DNSName:   "caduceus-agent",
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}
	if _, err := leafCert.Verify(opts); err != nil {
		t.Errorf("leaf certificate failed verification against CA: %v", err)
	}

	err = Init(tmpDir)
	if err == nil {
		t.Fatalf("expected Init on existing directory to fail, got nil")
	}
}
