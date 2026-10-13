package peer

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/Uhm-why/caduceus/internal/config"
)

func setupTestPipe(t *testing.T) (*tls.Conn, *tls.Conn) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate ecdsa key: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test-peer"},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(1 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		DNSNames:     []string{"localhost", "caduceus-agent"},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}

	cert := tls.Certificate{
		Certificate: [][]byte{derBytes},
		PrivateKey:  key,
	}

	serverConfig := tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAnyClientCert,
		MinVersion:   tls.VersionTLS13,
	}

	clientConfig := tls.Config{
		Certificates:       []tls.Certificate{cert},
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS13,
	}

	c1, c2 := net.Pipe()

	clientTLS := tls.Client(c1, &clientConfig)
	serverTLS := tls.Server(c2, &serverConfig)

	errChan := make(chan error, 1)

	go func() {
		errChan <- serverTLS.Handshake()
	}()
	if err := clientTLS.Handshake(); err != nil {
		t.Fatalf("client TLS handshake: %v", err)
	}
	if err := <-errChan; err != nil {
		t.Fatalf("server TLS handshake: %v", err)
	}

	return clientTLS, serverTLS
}

func setupTestSessionPair(t *testing.T) (*Session, *Session, *tls.Conn, *tls.Conn) {
	serverConfig := config.DefaultConfig()
	clientConfig := config.DefaultConfig()

	serverSession := NewSession(serverConfig)

	clientSession := NewSession(clientConfig)

	t.Cleanup(func() {
		clientSession.Close()
		serverSession.Close()
	})
	clientConn, serverConn := setupTestPipe(t)

	return clientSession, serverSession, clientConn, serverConn

}

func TestHandshake_Success(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	clientSession, serverSession, clientConn, serverConn := setupTestSessionPair(t)

	if err := serverSession.Attach(ctx, serverConn); err != nil {
		t.Fatalf("server attach: %v", err)
	}

	if err := clientSession.Attach(ctx, clientConn); err != nil {
		t.Fatalf("client attach: %v", err)
	}

	if err := clientSession.PerformHandshake(ctx); err != nil {
		t.Fatalf("perform handshake: %v", err)
	}

	if clientSession.State() != StateActive {
		t.Fatalf("client state not StateActive after handshake")
	}

	if serverSession.State() != StateActive {
		t.Fatalf("server state not StateActive after handshake")
	}

	if clientSession.RemoteCapabilities() == nil {
		t.Fatalf("client does not report any capabilities")
	}

	if !clientSession.HasRemoteFeature("messaging") {
		t.Fatalf("client does not report messaging as a capability")
	}

	if serverSession.RemoteCapabilities() == nil {
		t.Fatalf("server does not report any capabilities")
	}

	if !serverSession.HasRemoteFeature("messaging") {
		t.Fatalf("server does not report messaging as a capability")
	}
}

func TestHandshake_VersionMismatch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	clientSession, serverSession, clientConn, serverConn := setupTestSessionPair(t)

	clientSession.localCaps.Version = "2.0.0"

	serverSession.Attach(ctx, serverConn)
	clientSession.Attach(ctx, clientConn)

	err := clientSession.PerformHandshake(ctx)

	if err == nil {
		t.Fatalf("expected handshake to fail due to version mismatch, but got nil")
	}

	if clientSession.State() == StateActive {
		t.Fatalf("client state should not be StateActive after handshake failure")
	}

	if serverSession.State() == StateActive {
		t.Fatalf("server state should not be StateActive after handshake failure")
	}

}
