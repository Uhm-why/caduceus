package trust

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func generateTestCA(t *testing.T, cn string, isCA bool, keyUsage x509.KeyUsage, notBefore, notAfter time.Time) (certPEM, keyPEM []byte, pub ed25519.PublicKey, priv ed25519.PrivateKey) {
	t.Helper()

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate ed25519 key: %v", err)
	}

	privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("failed to marshal private key: %v", err)
	}

	keyPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privBytes,
	})

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		t.Fatalf("failed to generate serial number: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: cn,
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		IsCA:                  isCA,
		BasicConstraintsValid: true,
		KeyUsage:              keyUsage,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, pub, priv)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	return certPEM, keyPEM, pub, priv
}

func TestFingerprint(t *testing.T) {
	now := time.Now()
	certPEM, _, _, _ := generateTestCA(t, "test-ca", true, x509.KeyUsageCertSign, now.Add(-time.Hour), now.Add(time.Hour))

	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Fatal("failed to decode generated PEM")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("failed to parse certificate: %v", err)
	}

	fp := Fingerprint(cert)
	if len(fp) != 64 {
		t.Fatalf("expected 64-character SHA-256 hex string, got %d chars: %s", len(fp), fp)
	}
}

func TestLoadLocalCAPublicKey(t *testing.T) {
	t.Run("empty key path returns nil", func(t *testing.T) {
		pub, err := loadLocalCAPublicKey("")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if pub != nil {
			t.Fatalf("expected nil public key, got %v", pub)
		}
	})

	t.Run("non-existent key path returns error", func(t *testing.T) {
		_, err := loadLocalCAPublicKey("/path/does/not/exist/ca.key")
		if err == nil {
			t.Fatal("expected error for non-existent key path, got nil")
		}
	})

	t.Run("invalid PEM bytes", func(t *testing.T) {
		dir := t.TempDir()
		badKeyPath := filepath.Join(dir, "bad.key")
		if err := os.WriteFile(badKeyPath, []byte("not valid pem data"), 0600); err != nil {
			t.Fatalf("failed to write bad key: %v", err)
		}

		_, err := loadLocalCAPublicKey(badKeyPath)
		if !errors.Is(err, ErrInvalidPEM) {
			t.Fatalf("expected ErrInvalidPEM, got %v", err)
		}
	})

	t.Run("wrong PEM block type", func(t *testing.T) {
		dir := t.TempDir()
		wrongTypePath := filepath.Join(dir, "wrong.key")
		data := pem.EncodeToMemory(&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: []byte{0x01, 0x02},
		})
		if err := os.WriteFile(wrongTypePath, data, 0600); err != nil {
			t.Fatalf("failed to write key file: %v", err)
		}

		_, err := loadLocalCAPublicKey(wrongTypePath)
		if err == nil {
			t.Fatal("expected error for non-PRIVATE KEY block type, got nil")
		}
	})

	t.Run("valid private key", func(t *testing.T) {
		dir := t.TempDir()
		keyPath := filepath.Join(dir, "ca.key")
		now := time.Now()
		_, keyPEM, expectedPub, _ := generateTestCA(t, "test-ca", true, x509.KeyUsageCertSign, now.Add(-time.Hour), now.Add(time.Hour))
		if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
			t.Fatalf("failed to write ca.key: %v", err)
		}

		pub, err := loadLocalCAPublicKey(keyPath)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		edPub, ok := pub.(ed25519.PublicKey)
		if !ok {
			t.Fatalf("expected ed25519.PublicKey, got %T", pub)
		}
		if !edPub.Equal(expectedPub) {
			t.Fatal("retrieved public key does not match expected key")
		}
	})
}

func TestValidateCertificate(t *testing.T) {
	now := time.Now()

	t.Run("non-PEM data", func(t *testing.T) {
		_, err := ValidateCertificate([]byte("not pem"))
		if !errors.Is(err, ErrInvalidPEM) {
			t.Fatalf("expected ErrInvalidPEM, got %v", err)
		}
	})

	t.Run("wrong block type", func(t *testing.T) {
		data := pem.EncodeToMemory(&pem.Block{
			Type:  "PRIVATE KEY",
			Bytes: []byte{0x01},
		})
		_, err := ValidateCertificate(data)
		if !errors.Is(err, ErrInvalidPEM) {
			t.Fatalf("expected ErrInvalidPEM, got %v", err)
		}
	})

	t.Run("corrupted DER bytes", func(t *testing.T) {
		data := pem.EncodeToMemory(&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: []byte("corrupt der"),
		})
		_, err := ValidateCertificate(data)
		if err == nil {
			t.Fatal("expected error parsing corrupted DER, got nil")
		}
	})

	t.Run("not a CA", func(t *testing.T) {
		certPEM, _, _, _ := generateTestCA(t, "non-ca", false, x509.KeyUsageCertSign, now.Add(-time.Hour), now.Add(time.Hour))
		_, err := ValidateCertificate(certPEM)
		if !errors.Is(err, ErrNotCA) {
			t.Fatalf("expected ErrNotCA, got %v", err)
		}
	})

	t.Run("lacks KeyUsageCertSign", func(t *testing.T) {
		certPEM, _, _, _ := generateTestCA(t, "ca-no-certsign", true, x509.KeyUsageDigitalSignature, now.Add(-time.Hour), now.Add(time.Hour))
		_, err := ValidateCertificate(certPEM)
		if !errors.Is(err, ErrNotCA) {
			t.Fatalf("expected ErrNotCA, got %v", err)
		}
	})

	t.Run("expired certificate", func(t *testing.T) {
		certPEM, _, _, _ := generateTestCA(t, "expired-ca", true, x509.KeyUsageCertSign, now.Add(-2*time.Hour), now.Add(-time.Hour))
		_, err := ValidateCertificate(certPEM)
		if !errors.Is(err, ErrCertExpired) {
			t.Fatalf("expected ErrCertExpired, got %v", err)
		}
	})

	t.Run("not yet valid certificate", func(t *testing.T) {
		certPEM, _, _, _ := generateTestCA(t, "future-ca", true, x509.KeyUsageCertSign, now.Add(time.Hour), now.Add(2*time.Hour))
		_, err := ValidateCertificate(certPEM)
		if !errors.Is(err, ErrCertExpired) {
			t.Fatalf("expected ErrCertExpired, got %v", err)
		}
	})

	t.Run("valid CA certificate", func(t *testing.T) {
		certPEM, _, _, _ := generateTestCA(t, "valid-ca", true, x509.KeyUsageCertSign|x509.KeyUsageCRLSign, now.Add(-time.Hour), now.Add(time.Hour))
		cert, err := ValidateCertificate(certPEM)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cert.Subject.CommonName != "valid-ca" {
			t.Fatalf("expected CN 'valid-ca', got %q", cert.Subject.CommonName)
		}
	})
}

func TestLoadCertificates(t *testing.T) {
	dir := t.TempDir()
	caPath := filepath.Join(dir, "ca.pem")
	keyPath := filepath.Join(dir, "ca.key")
	now := time.Now()

	t.Run("non-existent caPath returns empty slice", func(t *testing.T) {
		certs, err := LoadCertificates(caPath, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(certs) != 0 {
			t.Fatalf("expected 0 certs, got %d", len(certs))
		}
	})

	t.Run("non-existent keyPath returns error", func(t *testing.T) {
		_, err := LoadCertificates(caPath, "/nonexistent/key.pem")
		if err == nil {
			t.Fatal("expected error for non-existent keyPath, got nil")
		}
	})

	t.Run("populates certificates and marks local root correctly", func(t *testing.T) {
		localCertPEM, localKeyPEM, _, _ := generateTestCA(t, "local-root-ca", true, x509.KeyUsageCertSign, now.Add(-time.Hour), now.Add(time.Hour))
		peerCertPEM, _, _, _ := generateTestCA(t, "peer-ca", true, x509.KeyUsageCertSign, now.Add(-time.Hour), now.Add(time.Hour))

		if err := os.WriteFile(keyPath, localKeyPEM, 0600); err != nil {
			t.Fatalf("failed to write local ca.key: %v", err)
		}

		combinedPEM := append(localCertPEM, peerCertPEM...)
		if err := os.WriteFile(caPath, combinedPEM, 0644); err != nil {
			t.Fatalf("failed to write ca.pem: %v", err)
		}

		certs, err := LoadCertificates(caPath, keyPath)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(certs) != 2 {
			t.Fatalf("expected 2 certs, got %d", len(certs))
		}

		var localCount, peerCount int
		for _, c := range certs {
			if c.IsLocalRoot {
				localCount++
				if c.SubjectCN != "local-root-ca" {
					t.Fatalf("expected local root CN 'local-root-ca', got %q", c.SubjectCN)
				}
			} else {
				peerCount++
				if c.SubjectCN != "peer-ca" {
					t.Fatalf("expected peer CN 'peer-ca', got %q", c.SubjectCN)
				}
			}
		}

		if localCount != 1 || peerCount != 1 {
			t.Fatalf("expected 1 local root and 1 peer, got %d local, %d peer", localCount, peerCount)
		}
	})
}

func TestAddCertificate(t *testing.T) {
	dir := t.TempDir()
	caPath := filepath.Join(dir, "ca.pem")
	now := time.Now()

	cert1PEM, _, _, _ := generateTestCA(t, "ca-one", true, x509.KeyUsageCertSign, now.Add(-time.Hour), now.Add(time.Hour))
	cert2PEM, _, _, _ := generateTestCA(t, "ca-two", true, x509.KeyUsageCertSign, now.Add(-time.Hour), now.Add(time.Hour))

	t.Run("rejects invalid certificate PEM", func(t *testing.T) {
		_, err := AddCertificate(caPath, []byte("invalid pem"))
		if !errors.Is(err, ErrInvalidPEM) {
			t.Fatalf("expected ErrInvalidPEM, got %v", err)
		}
	})

	t.Run("adds certificate to fresh caPath", func(t *testing.T) {
		tc, err := AddCertificate(caPath, cert1PEM)
		if err != nil {
			t.Fatalf("unexpected error adding first cert: %v", err)
		}
		if tc.SubjectCN != "ca-one" {
			t.Fatalf("expected SubjectCN 'ca-one', got %q", tc.SubjectCN)
		}

		// Verify on-disk contents
		certs, err := LoadCertificates(caPath, "")
		if err != nil {
			t.Fatalf("failed to load certificates: %v", err)
		}
		if len(certs) != 1 {
			t.Fatalf("expected 1 cert on disk, got %d", len(certs))
		}
		if certs[0].Fingerprint != tc.Fingerprint {
			t.Fatalf("fingerprint mismatch: got %s, expected %s", certs[0].Fingerprint, tc.Fingerprint)
		}
	})

	t.Run("prevents duplicate certificate", func(t *testing.T) {
		_, err := AddCertificate(caPath, cert1PEM)
		if !errors.Is(err, ErrCertAlreadyExists) {
			t.Fatalf("expected ErrCertAlreadyExists, got %v", err)
		}
	})

	t.Run("appends second certificate cleanly", func(t *testing.T) {
		tc2, err := AddCertificate(caPath, cert2PEM)
		if err != nil {
			t.Fatalf("unexpected error adding second cert: %v", err)
		}
		if tc2.SubjectCN != "ca-two" {
			t.Fatalf("expected SubjectCN 'ca-two', got %q", tc2.SubjectCN)
		}

		certs, err := LoadCertificates(caPath, "")
		if err != nil {
			t.Fatalf("failed to load certificates: %v", err)
		}
		if len(certs) != 2 {
			t.Fatalf("expected 2 certs on disk, got %d", len(certs))
		}
	})
}

func TestRemoveCertificate(t *testing.T) {
	dir := t.TempDir()
	caPath := filepath.Join(dir, "ca.pem")
	keyPath := filepath.Join(dir, "ca.key")
	now := time.Now()

	localCertPEM, localKeyPEM, _, _ := generateTestCA(t, "local-root-ca", true, x509.KeyUsageCertSign, now.Add(-time.Hour), now.Add(time.Hour))
	peer1PEM, _, _, _ := generateTestCA(t, "peer-ca-1", true, x509.KeyUsageCertSign, now.Add(-time.Hour), now.Add(time.Hour))
	peer2PEM, _, _, _ := generateTestCA(t, "peer-ca-2", true, x509.KeyUsageCertSign, now.Add(-time.Hour), now.Add(time.Hour))

	if err := os.WriteFile(keyPath, localKeyPEM, 0600); err != nil {
		t.Fatalf("failed to write key: %v", err)
	}

	// Initialize bundle with all 3 certs
	bundlePEM := append(append(localCertPEM, peer1PEM...), peer2PEM...)
	if err := os.WriteFile(caPath, bundlePEM, 0644); err != nil {
		t.Fatalf("failed to write bundle: %v", err)
	}

	initialCerts, err := LoadCertificates(caPath, keyPath)
	if err != nil {
		t.Fatalf("failed to load initial certs: %v", err)
	}
	if len(initialCerts) != 3 {
		t.Fatalf("expected 3 initial certs, got %d", len(initialCerts))
	}

	var localFP, peer1FP, peer2FP string
	for _, c := range initialCerts {
		switch c.SubjectCN {
		case "local-root-ca":
			localFP = c.Fingerprint
		case "peer-ca-1":
			peer1FP = c.Fingerprint
		case "peer-ca-2":
			peer2FP = c.Fingerprint
		}
	}

	t.Run("empty keyPath returns error", func(t *testing.T) {
		err := RemoveCertificate(caPath, "", peer1FP)
		if err == nil {
			t.Fatal("expected error for empty keyPath, got nil")
		}
	})

	t.Run("non-existent keyPath returns error", func(t *testing.T) {
		err := RemoveCertificate(caPath, "/nonexistent/key", peer1FP)
		if err == nil {
			t.Fatal("expected error for non-existent keyPath, got nil")
		}
	})

	t.Run("non-existent fingerprint returns ErrCertNotFound", func(t *testing.T) {
		err := RemoveCertificate(caPath, keyPath, "0000000000000000000000000000000000000000000000000000000000000000")
		if !errors.Is(err, ErrCertNotFound) {
			t.Fatalf("expected ErrCertNotFound, got %v", err)
		}
	})

	t.Run("refuses to remove local root CA", func(t *testing.T) {
		err := RemoveCertificate(caPath, keyPath, localFP)
		if !errors.Is(err, ErrCannotRemoveLocalRoot) {
			t.Fatalf("expected ErrCannotRemoveLocalRoot, got %v", err)
		}
	})

	t.Run("successfully removes peer certificate", func(t *testing.T) {
		err := RemoveCertificate(caPath, keyPath, peer1FP)
		if err != nil {
			t.Fatalf("unexpected error removing peer cert: %v", err)
		}

		remaining, err := LoadCertificates(caPath, keyPath)
		if err != nil {
			t.Fatalf("failed to reload certificates: %v", err)
		}

		if len(remaining) != 2 {
			t.Fatalf("expected 2 certs remaining, got %d", len(remaining))
		}

		for _, c := range remaining {
			if c.Fingerprint == peer1FP {
				t.Fatalf("found removed fingerprint %s in remaining bundle", peer1FP)
			}
		}

		// Ensure peer2 and local root are still present
		foundLocal := false
		foundPeer2 := false
		for _, c := range remaining {
			if c.Fingerprint == localFP {
				foundLocal = true
			}
			if c.Fingerprint == peer2FP {
				foundPeer2 = true
			}
		}

		if !foundLocal || !foundPeer2 {
			t.Fatalf("expected local root and peer2 to be preserved, foundLocal=%v, foundPeer2=%v", foundLocal, foundPeer2)
		}
	})
}
