package config

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

func generateSerialNumber() (*big.Int, error) {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, limit)
	if err != nil {
		return nil, err
	}

	return serial, nil
}

func Init(targetDir string) error {
	if targetDir == "" {
		targetDir = DefaultConfigDir()
	}

	cleanPath := filepath.Clean(targetDir)
	mailPath := filepath.Join(cleanPath, "mailbox")
	tmpMailPath := filepath.Join(mailPath, "tmp")
	newMailPath := filepath.Join(mailPath, "new")
	curMailPath := filepath.Join(mailPath, "cur")
	spilloverPath := filepath.Join(cleanPath, "spillover")
	configPath := filepath.Join(cleanPath, "config.json")

	if _, err := os.Stat(filepath.Join(cleanPath, "ca.key")); err == nil {
		return errors.New("configuration directory already initialized (ca.key exists)")
	}

	err := os.MkdirAll(cleanPath, 0700)
	if err != nil {
		return err
	}

	err = os.MkdirAll(tmpMailPath, 0700)
	if err != nil {
		return err
	}

	err = os.MkdirAll(newMailPath, 0700)
	if err != nil {
		return err
	}

	err = os.MkdirAll(curMailPath, 0700)
	if err != nil {
		return err
	}

	err = os.MkdirAll(spilloverPath, 0700)
	if err != nil {
		return err
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {

		defaultJSON := `{
		"listen_address": ":8443",
		"peer_san": "caduceus-agent"
		}
		`

		if err := os.WriteFile(configPath, []byte(defaultJSON), 0644); err != nil {
			return err
		}
	}

	caPubKey, caPrivKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}

	marshPrivKey, err := x509.MarshalPKCS8PrivateKey(caPrivKey)
	if err != nil {
		return err
	}

	pemBlockWrappedKey := pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: marshPrivKey,
	}

	err = os.WriteFile(filepath.Join(cleanPath, "ca.key"), pem.EncodeToMemory(&pemBlockWrappedKey), 0600)
	if err != nil {
		return err
	}

	serial, err := generateSerialNumber()
	if err != nil {
		return err
	}

	caTemplate := x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "caduceus-ca",
			Organization: []string{"Caduceus"},
		},
		NotBefore:             time.Now().Add(-1 * time.Minute),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}

	caDER, err := x509.CreateCertificate(rand.Reader, &caTemplate, &caTemplate, caPubKey, caPrivKey)
	if err != nil {
		return err
	}

	pemBlockWrappedCert := pem.Block{
		Type:  "CERTIFICATE",
		Bytes: caDER,
	}

	err = os.WriteFile(filepath.Join(cleanPath, "ca.pem"), pem.EncodeToMemory(&pemBlockWrappedCert), 0644)
	if err != nil {
		return err
	}

	leafPubKey, leafPrivKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}

	marshLeafKey, err := x509.MarshalPKCS8PrivateKey(leafPrivKey)
	if err != nil {
		return err
	}

	leafPemBlock := pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: marshLeafKey,
	}

	err = os.WriteFile(filepath.Join(cleanPath, "key.pem"), pem.EncodeToMemory(&leafPemBlock), 0600)
	if err != nil {
		return err
	}

	leafSerial, err := generateSerialNumber()
	if err != nil {
		return err
	}

	leafTemplate := x509.Certificate{
		SerialNumber:          leafSerial,
		Subject:               pkix.Name{CommonName: "caduceus-agent", Organization: []string{"Caduceus"}},
		NotBefore:             time.Now().Add(-1 * time.Minute),
		NotAfter:              time.Now().AddDate(1, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		IsCA:                  false,
		BasicConstraintsValid: true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		DNSNames:              []string{"caduceus-agent", "localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.IPv6loopback},
	}

	leafDER, err := x509.CreateCertificate(rand.Reader, &leafTemplate, &caTemplate, leafPubKey, caPrivKey)
	if err != nil {
		return err
	}

	leafCertPem := pem.Block{
		Type:  "CERTIFICATE",
		Bytes: leafDER,
	}

	err = os.WriteFile(filepath.Join(cleanPath, "cert.pem"), pem.EncodeToMemory(&leafCertPem), 0644)
	if err != nil {
		return err
	}

	return nil

}
