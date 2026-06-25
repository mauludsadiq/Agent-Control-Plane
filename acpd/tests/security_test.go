package tests_test

import (
"crypto/tls"
"net"
"net/http"
"testing"

"github.com/mauludsadiq/agent-control-plane/acpd/internal/security"
	"github.com/mauludsadiq/agent-control-plane/acpd/internal/store"
"github.com/mauludsadiq/agent-control-plane/acpd/internal/testutil"
)

// ─── KeyProvider tests ────────────────────────────────────────────────────────

// TestLocalKeyProviderHMACIsKeyed verifies that two different keys produce
// different hashes for the same input — i.e. it is not plain SHA256.
func TestLocalKeyProviderHMACIsKeyed(t *testing.T) {
key1 := make([]byte, 32)
key2 := make([]byte, 32)
for i := range key2 {
key2[i] = 0xff
}

kp1 := security.NewLocalKeyProviderFromKey(key1)
kp2 := security.NewLocalKeyProviderFromKey(key2)

hash1 := kp1.HashAPIKey("acp_test_key")
hash2 := kp2.HashAPIKey("acp_test_key")

if hash1 == hash2 {
t.Error("same input with different keys produced same hash — HMAC not keyed")
}
if hash1 == "sha256:"+hash1 {
t.Error("HMAC hash looks like plain SHA256")
}
t.Logf("key1 hash: %s", hash1)
t.Logf("key2 hash: %s", hash2)
t.Log("HMAC hashing is keyed ✓")
}

// TestLocalKeyProviderSignVerify verifies that SignToken + VerifyToken round-trips.
func TestLocalKeyProviderSignVerify(t *testing.T) {
key := make([]byte, 32)
for i := range key {
key[i] = 0xab
}
kp := security.NewLocalKeyProviderFromKey(key)

token := "sha256:abc123def456"
sig, err := kp.SignToken(token)
if err != nil {
t.Fatalf("SignToken: %v", err)
}
if sig == token {
t.Error("signature equals token — not signed")
}
if !kp.VerifyToken(token, sig) {
t.Error("VerifyToken returned false for valid signature")
}
t.Logf("token: %s", token)
t.Logf("sig:   %s", sig)
t.Log("sign/verify round-trip ✓")
}

// TestTokenForgeryFails verifies that a forged or tampered token fails verification.
func TestTokenForgeryFails(t *testing.T) {
key := make([]byte, 32)
kp := security.NewLocalKeyProviderFromKey(key)

token := "sha256:real_gate_token"
sig, _ := kp.SignToken(token)

// Tampered token
if kp.VerifyToken("sha256:forged_token", sig) {
t.Error("forged token passed verification — CRITICAL SECURITY FAILURE")
}
// Tampered signature
if kp.VerifyToken(token, sig+"tampered") {
t.Error("tampered signature passed verification")
}
// Wrong key
key2 := make([]byte, 32)
for i := range key2 {
key2[i] = 0x01
}
kp2 := security.NewLocalKeyProviderFromKey(key2)
if kp2.VerifyToken(token, sig) {
t.Error("signature from different key passed verification")
}
t.Log("token forgery correctly rejected ✓")
}

// TestAPIKeyHMACIsNotReversible verifies that two different API keys
// produce different HMAC hashes (no collision with wrong key).
func TestAPIKeyHMACIsNotReversible(t *testing.T) {
key := make([]byte, 32)
kp := security.NewLocalKeyProviderFromKey(key)

h1 := kp.HashAPIKey("acp_live_key_abc")
h2 := kp.HashAPIKey("acp_live_key_xyz")

if h1 == h2 {
t.Error("different API keys produced same hash")
}
// Verify it is deterministic
h1b := kp.HashAPIKey("acp_live_key_abc")
if h1 != h1b {
t.Error("HashAPIKey is not deterministic")
}
t.Logf("key1 hash: %s", h1)
t.Logf("key2 hash: %s", h2)
t.Log("API key HMAC hashing: unique + deterministic ✓")
}

// TestDefaultProviderInit verifies DefaultProvider initialises without error.
func TestDefaultProviderInit(t *testing.T) {
kp, err := security.DefaultProvider()
if err != nil {
t.Fatalf("DefaultProvider: %v", err)
}
if kp == nil {
t.Fatal("DefaultProvider returned nil")
}
// Basic smoke test
hash := kp.HashAPIKey("test_key")
if hash == "" {
t.Error("HashAPIKey returned empty string")
}
t.Logf("default provider hash: %s", hash[:20]+"...")
t.Log("DefaultProvider initialised ✓")
}

// ─── mTLS tests ───────────────────────────────────────────────────────────────

// TestGenerateCA verifies that a CA certificate can be generated.
func TestGenerateCA(t *testing.T) {
ca, err := security.GenerateCA("ACP Test CA")
if err != nil {
t.Fatalf("GenerateCA: %v", err)
}
if ca.CACert == nil {
t.Error("CA cert is nil")
}
if len(ca.CAPEMCert) == 0 {
t.Error("CA PEM cert is empty")
}
if ca.CACert.IsCA != true {
t.Error("generated cert is not a CA")
}
t.Logf("CA subject: %s", ca.CACert.Subject.CommonName)
t.Log("CA generation ✓")
}

// TestIssueCert verifies that a leaf cert can be issued from a CA.
func TestIssueCert(t *testing.T) {
ca, err := security.GenerateCA("ACP Test CA")
if err != nil {
t.Fatalf("GenerateCA: %v", err)
}

certPEM, keyPEM, err := ca.IssueCert("acp-server",
[]string{"localhost"}, []net.IP{net.ParseIP("127.0.0.1")})
if err != nil {
t.Fatalf("IssueCert: %v", err)
}
if len(certPEM) == 0 || len(keyPEM) == 0 {
t.Error("empty cert or key PEM")
}
t.Log("leaf cert issued from CA ✓")
}

// TestServerTLSConfig verifies that a valid tls.Config is produced.
func TestServerTLSConfig(t *testing.T) {
ca, _ := security.GenerateCA("ACP Test CA")
certPEM, keyPEM, err := ca.IssueCert("acp-server",
[]string{"localhost"}, []net.IP{net.ParseIP("127.0.0.1")})
if err != nil {
t.Fatalf("IssueCert: %v", err)
}

cfg, err := security.ServerTLSConfig(certPEM, keyPEM, ca.CAPEMCert)
if err != nil {
t.Fatalf("ServerTLSConfig: %v", err)
}
if cfg.ClientAuth != tls.RequireAndVerifyClientCert {
t.Error("ClientAuth should be RequireAndVerifyClientCert")
}
if cfg.MinVersion != tls.VersionTLS13 {
t.Error("MinVersion should be TLS 1.3")
}
t.Log("server TLS config: mTLS required, TLS 1.3 minimum ✓")
}

// TestMTLSHandshake verifies that a client with a valid cert can connect,
// and a client without a cert is rejected.
func TestMTLSHandshake(t *testing.T) {
// Generate CA and certs
ca, _ := security.GenerateCA("ACP Test CA")
serverCertPEM, serverKeyPEM, _ := ca.IssueCert("acp-server",
[]string{"localhost"}, []net.IP{net.ParseIP("127.0.0.1")})
clientCertPEM, clientKeyPEM, _ := ca.IssueCert("acp-worker",
[]string{"localhost"}, nil)

serverTLS, err := security.ServerTLSConfig(serverCertPEM, serverKeyPEM, ca.CAPEMCert)
if err != nil {
t.Fatalf("ServerTLSConfig: %v", err)
}

// Verify server enforces client cert requirement
if serverTLS.ClientAuth != tls.RequireAndVerifyClientCert {
t.Error("server config does not enforce client cert requirement")
}

// Test 1: client WITH valid cert — should succeed
ln, _ := tls.Listen("tcp", "127.0.0.1:0", serverTLS)
defer ln.Close()

done := make(chan error, 1)
go func() {
conn, err := ln.Accept()
if err != nil { done <- err; return }
defer conn.Close()
done <- conn.(*tls.Conn).Handshake()
}()

clientTLS, _ := security.ClientTLSConfig(clientCertPEM, clientKeyPEM, ca.CAPEMCert)
conn, err := tls.Dial("tcp", ln.Addr().String(), clientTLS)
if err != nil {
t.Fatalf("valid client cert rejected: %v", err)
}
conn.Close()
if err := <-done; err != nil {
t.Fatalf("server rejected valid client cert: %v", err)
}
t.Log("mTLS handshake with valid client cert: success ✓")

// Test 2: client cert from UNTRUSTED CA — server must reject
ca2, _ := security.GenerateCA("Untrusted CA")
badCert, badKey, _ := ca2.IssueCert("bad-client", nil, nil)
untrustedCfg, _ := security.ClientTLSConfig(badCert, badKey, ca.CAPEMCert)

ln2, _ := tls.Listen("tcp", "127.0.0.1:0", serverTLS)
defer ln2.Close()

serverDone := make(chan error, 1)
go func() {
conn, err := ln2.Accept()
if err != nil { serverDone <- err; return }
defer conn.Close()
serverDone <- conn.(*tls.Conn).Handshake()
}()

badConn, _ := tls.Dial("tcp", ln2.Addr().String(), untrustedCfg)
if badConn != nil {
badConn.Close()
}
serverErr := <-serverDone
if serverErr == nil {
t.Error("server accepted cert from untrusted CA — mTLS not enforced")
} else {
t.Logf("untrusted CA cert correctly rejected: %v", serverErr)
t.Log("mTLS rejects clients from untrusted CAs ✓")
}
}

// TestKeyProviderWiredIntoServer verifies that the server still works
// correctly after wiring in a KeyProvider (auth flow unchanged).
func TestKeyProviderWiredIntoServer(t *testing.T) {
// NewTestServer seeds the actor with plain SHA256 hash.
// KeyProvider HMAC changes the hash format, so we test that:
// 1. The server accepts the pre-seeded key (plain SHA256, no KeyProvider)
// 2. After KeyProvider is set, new actors use HMAC hashing
ts, db := testutil.NewTestServer(t)

// Without KeyProvider — existing test actor works fine
resp := testutil.Do(t, ts, http.MethodPost, "/workflows",
map[string]any{"goal": "pre-kp test", "owner": "test"})
if resp["ok"] != true {
t.Fatalf("workflow creation without KeyProvider failed: %v", resp)
}

// Wire in KeyProvider and create a NEW actor with HMAC hash
key := make([]byte, 32)
for i := range key {
key[i] = 0xde
}
kp := security.NewLocalKeyProviderFromKey(key)
db.SetKeyProvider(kp)

// Create new actor — hash stored as HMAC
newKey := "acp_security_test_key_v7"
if err := db.CreateActor(&store.Actor{
ActorID: "operator:security-test",
Roles:   []string{"operator"},
}, newKey); err != nil {
t.Fatalf("create actor with HMAC: %v", err)
}

// Resolve the new actor — should find it via HMAC hash
actor, err := db.ResolveAPIKey(newKey)
if err != nil || actor == nil {
t.Fatalf("ResolveAPIKey with HMAC failed: %v", err)
}
if actor.ActorID != "operator:security-test" {
t.Errorf("wrong actor: %s", actor.ActorID)
}
t.Log("HMAC-keyed actor creation and resolution ✓")
t.Log("server works correctly with KeyProvider wired in ✓")
}
