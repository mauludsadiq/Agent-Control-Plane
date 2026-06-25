package anchor

import (
"context"
"crypto/sha256"
"encoding/json"
"fmt"
"time"
)

// ExternalRef is a backend-specific locator returned after anchoring.
type ExternalRef struct {
Kind        string `json:"kind"`                   // ethereum | rfc3161 | ct_log | local
TxHash      string `json:"tx_hash,omitempty"`      // ethereum
Block       int64  `json:"block,omitempty"`        // ethereum
Network     string `json:"network,omitempty"`      // ethereum
TSA         string `json:"tsa,omitempty"`          // rfc3161
TokenDigest string `json:"token_digest,omitempty"` // rfc3161
LogID       string `json:"log_id,omitempty"`       // ct_log
LeafIndex   int64  `json:"leaf_index,omitempty"`   // ct_log
TreeSize    int64  `json:"tree_size,omitempty"`    // ct_log
LocalRef    string `json:"local_ref,omitempty"`    // local/dev
AnchoredAt  string `json:"anchored_at,omitempty"`
}

// Backend submits an anchor payload to an external system.
type Backend interface {
// Name returns the backend identifier.
Name() string
// Anchor submits the payload digest to the external system
// and returns the external reference.
Anchor(ctx context.Context, payloadDigest, workflowID string, seqFrom, seqTo int) (*ExternalRef, error)
}

// LocalBackend is a dev/test backend that stores anchors locally.
// Production: replace with RFC3161Backend or EthereumBackend.
type LocalBackend struct{}

func (b *LocalBackend) Name() string { return "local" }

func (b *LocalBackend) Anchor(ctx context.Context, payloadDigest, workflowID string, seqFrom, seqTo int) (*ExternalRef, error) {
// Simulate anchoring by creating a deterministic local reference
h := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%d:%s", workflowID, seqFrom, seqTo, payloadDigest)))
return &ExternalRef{
Kind:       "local",
LocalRef:   fmt.Sprintf("local:%x", h[:8]),
AnchoredAt: time.Now().UTC().Format("2006-01-02 15:04:05"),
}, nil
}

// RFC3161Backend anchors via an RFC3161 timestamp authority.
// Implements the TSP (Time-Stamp Protocol) over HTTP.
type RFC3161Backend struct {
TSAURL string // e.g. "http://timestamp.digicert.com"
}

func (b *RFC3161Backend) Name() string { return "rfc3161" }

func (b *RFC3161Backend) Anchor(ctx context.Context, payloadDigest, workflowID string, seqFrom, seqTo int) (*ExternalRef, error) {
// TODO: implement RFC3161 TSP request
// 1. Build MessageImprint from payloadDigest
// 2. POST TimeStampReq to b.TSAURL
// 3. Parse TimeStampResp and extract token digest
return nil, fmt.Errorf("RFC3161Backend.Anchor: not implemented — set ACP_ANCHOR_TSA_URL and implement TSP client")
}

// EthereumBackend anchors by writing the payload digest to an Ethereum smart contract.
type EthereumBackend struct {
RPCURL          string // e.g. "https://mainnet.infura.io/v3/..."
ContractAddress string
Network         string
}

func (b *EthereumBackend) Name() string { return "ethereum" }

func (b *EthereumBackend) Anchor(ctx context.Context, payloadDigest, workflowID string, seqFrom, seqTo int) (*ExternalRef, error) {
// TODO: implement Ethereum anchor
// 1. ABI-encode payloadDigest
// 2. Send transaction to ContractAddress
// 3. Wait for confirmation and return tx_hash + block
return nil, fmt.Errorf("EthereumBackend.Anchor: not implemented — configure ACP_ANCHOR_ETH_RPC and implement ethclient")
}

// CTLogBackend anchors via a Certificate Transparency log.
type CTLogBackend struct {
LogURL string // e.g. "https://ct.googleapis.com/logs/argon2024/"
}

func (b *CTLogBackend) Name() string { return "ct_log" }

func (b *CTLogBackend) Anchor(ctx context.Context, payloadDigest, workflowID string, seqFrom, seqTo int) (*ExternalRef, error) {
// TODO: implement CT log submission
return nil, fmt.Errorf("CTLogBackend.Anchor: not implemented — configure ACP_ANCHOR_CT_LOG_URL")
}

// DefaultBackend returns a backend based on environment variables.
// ACP_ANCHOR_BACKEND=rfc3161|ethereum|ct_log|local (default: local)
func DefaultBackend() Backend {
// In production, read ACP_ANCHOR_BACKEND and return the right backend
// For now, always return LocalBackend for dev/test
return &LocalBackend{}
}

// ProofJSON is the JSON representation of an anchor proof for storage.
type ProofJSON struct {
ProofType   string         `json:"proof_type"`
Payload     map[string]any `json:"payload"`
ExternalRef *ExternalRef   `json:"external_ref"`
AnchoredBy  string         `json:"anchored_by"`
ProofDigest string         `json:"proof_digest"`
}

// MarshalExternalRef marshals an ExternalRef to JSON string.
func MarshalExternalRef(ref *ExternalRef) (string, error) {
b, err := json.Marshal(ref)
return string(b), err
}
