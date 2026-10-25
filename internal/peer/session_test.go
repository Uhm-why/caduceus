package peer

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/Uhm-why/caduceus/internal/config"
	"github.com/Uhm-why/caduceus/internal/mailbox"
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

	serverMB, err := mailbox.New(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create server mailbox: %v", err)
	}
	clientMB, err := mailbox.New(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create client mailbox: %v", err)
	}
	serverSession := NewSession(serverConfig, serverMB)

	clientSession := NewSession(clientConfig, clientMB)

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

func TestHandshake_MissingFeature(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	clientSession, serverSession, clientConn, serverConn := setupTestSessionPair(t)

	clientSession.localCaps.Features = []string{}

	serverSession.Attach(ctx, serverConn)
	clientSession.Attach(ctx, clientConn)

	err := clientSession.PerformHandshake(ctx)

	if err == nil {
		t.Fatalf("expected handshake to fail due to missing messaging feature, but got nil")
	}

	if clientSession.State() == StateActive {
		t.Fatalf("client state should not be StateActive after handshake failure")
	}

	if serverSession.State() == StateActive {
		t.Fatalf("server state should not be StateActive after handshake failure")
	}
}

func TestHandshake_PrematureRPCRejected(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	clientSession, serverSession, clientConn, serverConn := setupTestSessionPair(t)

	serverSession.Attach(ctx, serverConn)
	clientSession.Attach(ctx, clientConn)

	resBytes, err := clientSession.SendRequest(ctx, "agent/message", []byte(`{"message":"premature"}`))

	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}

	res := &Response{}

	if err := json.Unmarshal(resBytes, &res); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if res.Error == nil {
		t.Fatalf("expected error response for premature RPC, got nil")
	}

	if res.Error.Code != -32602 {
		t.Fatalf("expected error code -32602, got %d", res.Error.Code)
	}

	if res.Error.Message != "handshake required" {
		t.Fatalf("expected error message 'handshake required', got %s", res.Error.Message)
	}
	msgs, err := serverSession.DrainInbox()
	if err != nil {
		t.Fatalf("error draining inbox: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("server should not have delivered premature message to inbox")
	}
}

func TestConsultative_TicketFlow(t *testing.T) {
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

	if !clientSession.HasRemoteFeature("consultative") {
		t.Fatalf("client does not report consultative capability")
	}
	if !serverSession.HasRemoteFeature("consultative") {
		t.Fatalf("server does not report consultative capability")
	}

	_, err := clientSession.SendRequest(ctx, "agent/message", []byte(`{"message":"standard ping"}`))
	if err != nil {
		t.Fatalf("send standard message: %v", err)
	}

	msgs, err := serverSession.DrainInbox()
	if err != nil {
		t.Fatalf("drain server inbox: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message in server inbox, got %d", len(msgs))
	}
	if msgs[0].Kind != "message" {
		t.Fatalf("expected kind 'message', got '%s'", msgs[0].Kind)
	}
	if msgs[0].CorrelationID != "" {
		t.Fatalf("expected empty correlation ID, got '%s'", msgs[0].CorrelationID)
	}
	if msgs[0].Content != "standard ping" {
		t.Fatalf("expected content 'standard ping', got '%s'", msgs[0].Content)
	}

	consultPayload := []byte(`{
		"correlation_id": "tkt-test-1001",
		"kind": "consult_request",
		"message": "how should we architect this?"
	}`)
	_, err = clientSession.SendRequest(ctx, "agent/message", consultPayload)
	if err != nil {
		t.Fatalf("send consult request: %v", err)
	}

	msgs, err = serverSession.DrainInbox()
	if err != nil {
		t.Fatalf("drain server inbox for consult: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 consult message in server inbox, got %d", len(msgs))
	}
	if msgs[0].Kind != "consult_request" {
		t.Fatalf("expected kind 'consult_request', got '%s'", msgs[0].Kind)
	}
	if msgs[0].CorrelationID != "tkt-test-1001" {
		t.Fatalf("expected correlation ID 'tkt-test-1001', got '%s'", msgs[0].CorrelationID)
	}
	if msgs[0].Content != "how should we architect this?" {
		t.Fatalf("expected consult content, got '%s'", msgs[0].Content)
	}

	replyPayload := []byte(`{
		"correlation_id": "tkt-test-1001",
		"kind": "reply",
		"message": "[completed] use pattern B"
	}`)
	_, err = serverSession.SendRequest(ctx, "agent/message", replyPayload)
	if err != nil {
		t.Fatalf("send reply: %v", err)
	}

	clientMsgs, err := clientSession.DrainInbox()
	if err != nil {
		t.Fatalf("drain client inbox for reply: %v", err)
	}
	if len(clientMsgs) != 1 {
		t.Fatalf("expected 1 reply message in client inbox, got %d", len(clientMsgs))
	}
	if clientMsgs[0].Kind != "reply" {
		t.Fatalf("expected kind 'reply', got '%s'", clientMsgs[0].Kind)
	}
	if clientMsgs[0].CorrelationID != "tkt-test-1001" {
		t.Fatalf("expected correlation ID 'tkt-test-1001', got '%s'", clientMsgs[0].CorrelationID)
	}
	if clientMsgs[0].Content != "[completed] use pattern B" {
		t.Fatalf("expected reply content, got '%s'", clientMsgs[0].Content)
	}
}

func TestTaskDelegation_WireFlow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	clientSession, serverSession, clientConn, serverConn := setupTestSessionPair(t)

	serverSession.Attach(ctx, serverConn)
	clientSession.Attach(ctx, clientConn)

	clientSession.PerformHandshake(ctx)

	if !clientSession.HasRemoteFeature("tasks") {
		t.Fatalf("client missing remote tasks feature")
	}

	if !serverSession.HasRemoteFeature("tasks") {
		t.Fatalf("server missing remote tasks feature")
	}

	req := &TaskDispatchRequest{
		TaskID:     "tkt-wire-001",
		Objective:  "Inspect RF trace",
		Context:    "Spectrum analyzer 10 GHz",
		TimeoutSec: 60,
	}

	res, err := clientSession.DispatchTask(ctx, *req)
	if err != nil {
		t.Fatalf("error dispatching task: %v", err)
	}

	if res.Accepted != true {
		t.Fatalf("task not accepted by server")
	}
	if res.InitialState != "running" {
		t.Fatalf("initial state from server not `running`, current state: %s", res.InitialState)
	}
	if res.TaskID != "tkt-wire-001" {
		t.Fatalf("task id does not match the one sent to the server, received: %s", res.TaskID)
	}

	srvTask, ok := serverSession.TaskManager().Get("tkt-wire-001")
	if !ok {
		t.Fatalf("server does not have a task with id `tkt-wire-001`")
	}
	if srvTask.State != TaskStateRunning {
		t.Fatalf("server task not in TaskStateRunning, got %s", srvTask.State.String())
	}
	if srvTask.Objective != "Inspect RF trace" {
		t.Fatalf("expected server task to have objective `Inspect RF trace`, but got %s", srvTask.Objective)
	}

	status, err := clientSession.QueryTaskStatus(ctx, "tkt-wire-001")
	if err != nil {
		t.Fatalf("error when sending task status query from client: %v", err)
	}
	if status.TaskID != "tkt-wire-001" {
		t.Fatalf("queried task id set to %s, expected `tkt-wire-001`", status.TaskID)
	}
	if status.State != "running" {
		t.Fatalf("queried task state not `running`, got %s", status.State)
	}

	serverSession.TaskManager().UpdateProgress("tkt-wire-001", "trace sweep complete")
	artifacts := []string{"/logs/trace.png"}
	serverSession.TaskManager().Complete("tkt-wire-001", "impedance matched 50 ohms", artifacts)
	statusDone, err := clientSession.QueryTaskStatus(ctx, "tkt-wire-001")
	if err != nil {
		t.Fatalf("error querying status after server completed task: %v", err)
	}
	if statusDone.State != "completed" {
		t.Fatalf("expected completed task status after server completed task, got %s", statusDone.State)
	}
	if statusDone.Progress != "trace sweep complete" {
		t.Fatalf("expected progress to be `trace sweep complete`, got %s", statusDone.Progress)
	}
	if statusDone.Result != "impedance matched 50 ohms" {
		t.Fatalf("expected result to be `impedance matched 50 ohms`, got %s", statusDone.Result)
	}
	if len(statusDone.Artifacts) != 1 || statusDone.Artifacts[0] != "/logs/trace.png" {
		t.Fatalf("expected there to be 1 artifact `/logs/trace.png`, got %v", statusDone.Artifacts)
	}

	req2 := TaskDispatchRequest{
		TaskID:    "tkt-wire-002",
		Objective: "Long burn-in",
	}

	clientSession.DispatchTask(ctx, req2)
	cancelRes, err := clientSession.CancelTask(ctx, "tkt-wire-002", "emergency shutdown")
	if err != nil {
		t.Fatalf("error while client attempted to cancel task on server: %v", err)
	}
	if cancelRes.Canceled != true {
		t.Fatalf("server did not cancel task")
	}
	if cancelRes.FinalState != "aborted" {
		t.Fatalf("expected cancelled task final state to be `aborted`, got %s", cancelRes.FinalState)
	}

	statusAborted, err := clientSession.QueryTaskStatus(ctx, "tkt-wire-002")
	if err != nil {
		t.Fatalf("error while client queried state of cancelled task: %v", err)
	}
	if statusAborted.State != "aborted" {
		t.Fatalf("expected state returned from server for an aborted task to be `aborted`, got %s", statusAborted.State)
	}

	_, err = clientSession.QueryTaskStatus(ctx, "tkt-nonexistent")

	if err == nil {
		t.Fatalf("expected error when client requested status of non-existent task, got nil")
	}
}
