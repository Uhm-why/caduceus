// Package trust provides the trust store management engine for Caduceus,
// including certificate validation, bundle loading, and atomic trust modification.
package trust

import (
	"bytes"
	"crypto"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// TrustedCert represents a parsed CA certificate within the trust bundle,
// including its identity metadata, validity window, fingerprint, and whether
// it corresponds to the local node's private key.
type TrustedCert struct {
	SubjectCN    string
	IssuerCN     string
	SerialNumber string
	NotBefore    time.Time
	NotAfter     time.Time
	Fingerprint  string
	IsLocalRoot  bool
}

var (
	ErrInvalidPEM            = errors.New("the supplied bytes do not contain a valid PEM block with type `CERTIFICATE`")
	ErrNotCA                 = errors.New("the certificate has `BasicConstraints.IsCA` == false or lacks x509.KeyUsageCertSign in its KeyUsage bitmask")
	ErrCertExpired           = errors.New("the certificates validity window (`NotBefore` / `NotAfter`) does not encompass `time.Now()`")
	ErrCertAlreadyExists     = errors.New("a certificate with an identical SHA-256 fingerprint is already present in the bundle")
	ErrCertNotFound          = errors.New("attempting to remove a fingerprint that does not exist in the bundle")
	ErrCannotRemoveLocalRoot = errors.New("attempting to remove the root CA corresponding to the local node's private key")
)

// Fingerprint computes and returns the lowercase hex-encoded SHA-256 fingerprint
// of the certificate's raw DER bytes.
func Fingerprint(cert *x509.Certificate) string {
	fpSum := sha256.Sum256(cert.Raw)
	encoded := hex.EncodeToString(fpSum[:])
	return encoded
}

// loadLocalCAPublicKey parses the local node's private key at keyPath and returns
// its public key for identity comparison against trusted certificates. If keyPath
// is empty, it returns nil without error.
func loadLocalCAPublicKey(keyPath string) (crypto.PublicKey, error) {
	if keyPath == "" {
		return nil, nil
	}

	pemBytes, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}

	pemBlock, _ := pem.Decode(pemBytes)

	if pemBlock == nil {
		return nil, ErrInvalidPEM
	}

	if pemBlock.Bytes == nil {
		return nil, fmt.Errorf("PEM Block is empty")
	}

	if pemBlock.Type != "PRIVATE KEY" {
		return nil, fmt.Errorf("expected PEM block type of `PRIVATE KEY`, got %s", pemBlock.Type)
	}

	privKey, err := x509.ParsePKCS8PrivateKey(pemBlock.Bytes)
	if err != nil {
		return nil, err
	}

	signer, ok := privKey.(crypto.Signer)
	if !ok {
		return nil, fmt.Errorf("error retrieving public key from signer")
	}

	return signer.Public(), nil
}

// LoadCertificates parses all CA certificates from the PEM bundle at caPath,
// determining whether each certificate matches the local node's private key at keyPath.
// If caPath does not exist, an empty slice is returned without error.
func LoadCertificates(caPath, keyPath string) ([]TrustedCert, error) {
	tcSlice := make([]TrustedCert, 0)
	pubKey, err := loadLocalCAPublicKey(keyPath)
	if err != nil {
		return nil, err
	}

	pemBytes, err := os.ReadFile(caPath)
	if err != nil {
		if os.IsNotExist(err) {
			return make([]TrustedCert, 0), nil
		}
		return nil, err
	}

	data := pemBytes

	for len(data) > 0 {
		block, rest := pem.Decode(data)
		if block == nil {
			break
		}
		data = rest

		if block.Type != "CERTIFICATE" {
			continue
		}

		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, err
		}

		var isLocalRoot bool
		if eq, ok := pubKey.(interface{ Equal(crypto.PublicKey) bool }); ok {
			isLocalRoot = eq.Equal(cert.PublicKey)
		}

		tcSlice = append(tcSlice, TrustedCert{
			SubjectCN:    cert.Subject.CommonName,
			IssuerCN:     cert.Issuer.CommonName,
			SerialNumber: cert.SerialNumber.Text(16),
			NotBefore:    cert.NotBefore,
			NotAfter:     cert.NotAfter,
			Fingerprint:  Fingerprint(cert),
			IsLocalRoot:  isLocalRoot,
		})

	}

	return tcSlice, nil

}

// ValidateCertificate verifies that certPEM contains a valid PEM-encoded CA certificate,
// enforcing basic constraints (IsCA), key usage (KeyUsageCertSign), and current temporal validity.
func ValidateCertificate(certPEM []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, ErrInvalidPEM
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}

	if !cert.IsCA || (cert.KeyUsage&x509.KeyUsageCertSign) == 0 {
		return nil, ErrNotCA
	}

	now := time.Now()
	if now.Before(cert.NotBefore) || now.After(cert.NotAfter) {
		return nil, ErrCertExpired
	}

	return cert, nil
}

// AddCertificate validates certPEM, checks for duplicate fingerprints in caPath,
// and atomically appends the certificate to the PEM bundle file.
func AddCertificate(caPath string, certPEM []byte) (*TrustedCert, error) {
	cert, err := ValidateCertificate(certPEM)
	if err != nil {
		return nil, err
	}

	fp := Fingerprint(cert)

	curCerts, err := LoadCertificates(caPath, "")
	if err != nil {
		return nil, err
	}

	for _, c := range curCerts {
		if c.Fingerprint == fp {
			return nil, ErrCertAlreadyExists
		}
	}

	readCerts, err := os.ReadFile(caPath)
	if err != nil {
		if os.IsNotExist(err) {
			readCerts = make([]byte, 0)
		} else {
			return nil, err
		}
	}

	block := pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	}

	encodedCert := pem.EncodeToMemory(&block)

	dir := filepath.Dir(caPath)
	tmpFile, err := os.CreateTemp(dir, "ca-*.pem.tmp")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpFile.Name())

	nByte := []byte("\n")
	endsWithNewLine := bytes.HasSuffix(readCerts, nByte)

	if len(readCerts) > 0 && !endsWithNewLine {
		readCerts = append(readCerts, nByte...)
	}

	combinedCerts := append(readCerts, encodedCert...)

	if _, err = tmpFile.Write(combinedCerts); err != nil {
		tmpFile.Close()
		return nil, err
	}

	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		return nil, err
	}

	if err := tmpFile.Close(); err != nil {
		return nil, err
	}

	if err := os.Rename(tmpFile.Name(), caPath); err != nil {
		return nil, err
	}

	return &TrustedCert{
		SubjectCN:    cert.Subject.CommonName,
		IssuerCN:     cert.Issuer.CommonName,
		SerialNumber: cert.SerialNumber.Text(16),
		NotBefore:    cert.NotBefore,
		NotAfter:     cert.NotAfter,
		Fingerprint:  fp,
		IsLocalRoot:  false,
	}, nil
}

// RemoveCertificate removes the certificate with the matching SHA-256 fingerprint
// from caPath. It refuses to remove the local node's root CA and commits the update
// atomically via a temporary file and rename.
func RemoveCertificate(caPath, keyPath, fingerprint string) error {
	if keyPath == "" {
		return fmt.Errorf("key path must be provided when removing certificates")
	}

	_, err := os.Stat(keyPath)
	if os.IsNotExist(err) {
		return fmt.Errorf("provided key does not exist")
	}

	curCerts, err := LoadCertificates(caPath, keyPath)
	if err != nil {
		return err
	}
	var target TrustedCert
	var found bool
	for _, cert := range curCerts {
		if cert.Fingerprint == fingerprint {
			target = cert
			found = true
			break
		}
	}

	if !found {
		return ErrCertNotFound
	}

	if target.IsLocalRoot {
		return ErrCannotRemoveLocalRoot
	}

	caPEM, err := os.ReadFile(caPath)
	if err != nil {
		return err
	}

	data := caPEM

	retainedPEM := make([]byte, 0)

	for len(data) > 0 {
		block, rest := pem.Decode(data)
		if block == nil {
			break
		}
		data = rest

		if block.Type == "CERTIFICATE" {
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return fmt.Errorf("error parsing certificate: %w", err)
			}

			fp := Fingerprint(cert)

			if target.Fingerprint == fp {
				continue
			}

			encodedCert := pem.EncodeToMemory(block)

			retainedPEM = append(retainedPEM, encodedCert...)

		} else {
			retainedPEM = append(retainedPEM, pem.EncodeToMemory(block)...)
		}
	}

	tmpFile, err := os.CreateTemp(filepath.Dir(caPath), "ca-*.pem.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.Write(retainedPEM)
	if err != nil {
		tmpFile.Close()
		return err
	}

	err = tmpFile.Sync()
	if err != nil {
		tmpFile.Close()
		return err
	}

	if err = tmpFile.Close(); err != nil {
		return err
	}

	if err = os.Rename(tmpFile.Name(), caPath); err != nil {
		return err
	}

	return nil
}
