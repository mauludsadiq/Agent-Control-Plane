package security

import (
"crypto/hmac"
"crypto/rand"
"crypto/sha256"
"encoding/hex"
"errors"
"fmt"
"os"
)

// KeyProvider abstracts key material — local HMAC for dev, KMS for prod.
type KeyProvider interface {
// SignToken returns an HMAC-SHA256 signature of the token using the signing key.
SignToken(token string) (string, error)
// VerifyToken checks that sig is a valid HMAC-SHA256 of token.
VerifyToken(token, sig string) bool
// HashAPIKey returns a keyed hash of the API key for storage.
// Unlike plain sha256, this is not reversible without the master key.
HashAPIKey(key string) string
}

// LocalKeyProvider uses a master secret from the environment.
// In production, replace with KMSKeyProvider.
type LocalKeyProvider struct {
masterKey []byte
}

// NewLocalKeyProvider reads ACP_MASTER_KEY from the environment.
// If not set, generates a random ephemeral key (dev/test only).
func NewLocalKeyProvider() (*LocalKeyProvider, error) {
raw := os.Getenv("ACP_MASTER_KEY")
var key []byte
if raw != "" {
b, err := hex.DecodeString(raw)
if err != nil {
return nil, fmt.Errorf("ACP_MASTER_KEY must be hex-encoded: %w", err)
}
if len(b) < 32 {
return nil, errors.New("ACP_MASTER_KEY must be at least 32 bytes (64 hex chars)")
}
key = b
} else {
// Ephemeral key for dev/test — tokens are only valid for this process lifetime
key = make([]byte, 32)
if _, err := rand.Read(key); err != nil {
return nil, fmt.Errorf("generate ephemeral key: %w", err)
}
}
return &LocalKeyProvider{masterKey: key}, nil
}

// NewLocalKeyProviderFromKey creates a provider with an explicit key (for tests).
func NewLocalKeyProviderFromKey(key []byte) *LocalKeyProvider {
return &LocalKeyProvider{masterKey: key}
}

func (p *LocalKeyProvider) sign(data string) string {
mac := hmac.New(sha256.New, p.masterKey)
mac.Write([]byte(data))
return "hmac:" + hex.EncodeToString(mac.Sum(nil))
}

func (p *LocalKeyProvider) SignToken(token string) (string, error) {
return p.sign("gate:" + token), nil
}

func (p *LocalKeyProvider) VerifyToken(token, sig string) bool {
expected := p.sign("gate:" + token)
return hmac.Equal([]byte(expected), []byte(sig))
}

func (p *LocalKeyProvider) HashAPIKey(key string) string {
return p.sign("apikey:" + key)
}

// KMSKeyProvider is a stub for production KMS integration.
// Implement by calling your KMS (AWS KMS, GCP KMS, HashiCorp Vault).
type KMSKeyProvider struct {
keyID string
}

func NewKMSKeyProvider(keyID string) *KMSKeyProvider {
return &KMSKeyProvider{keyID: keyID}
}

func (p *KMSKeyProvider) SignToken(token string) (string, error) {
// TODO: call KMS GenerateMac or Sign API
return "", errors.New("KMSKeyProvider.SignToken: not implemented — configure ACP_KMS_KEY_ID and implement KMS client")
}

func (p *KMSKeyProvider) VerifyToken(token, sig string) bool {
// TODO: call KMS VerifyMac API
return false
}

func (p *KMSKeyProvider) HashAPIKey(key string) string {
// TODO: call KMS GenerateMac for deterministic keyed hash
return "kms:not-implemented"
}

// DefaultProvider returns a LocalKeyProvider, falling back gracefully.
// In production, wire in KMSKeyProvider via dependency injection.
func DefaultProvider() (KeyProvider, error) {
kmsKeyID := os.Getenv("ACP_KMS_KEY_ID")
if kmsKeyID != "" {
return NewKMSKeyProvider(kmsKeyID), nil
}
return NewLocalKeyProvider()
}
