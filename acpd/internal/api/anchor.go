package api

import (
"encoding/json"
"net/http"

"github.com/google/uuid"
"github.com/mauludsadiq/agent-control-plane/acpd/internal/anchor"
"github.com/mauludsadiq/agent-control-plane/acpd/internal/auth"
"github.com/mauludsadiq/agent-control-plane/acpd/internal/store"
)

// anchorWorkflow handles POST /workflows/:id/anchor
// Creates an anchor payload from the receipt chain and submits to external backend.
func (h *Handlers) anchorWorkflow(w http.ResponseWriter, r *http.Request, workflowID string) {
if r.Method != http.MethodPost {
writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
return
}
actor := auth.ActorFromContext(r.Context())
if actor == nil {
writeErr(w, http.StatusUnauthorized, "unauthorized")
return
}

var req struct {
SeqFrom       int    `json:"seq_from"`
SeqTo         int    `json:"seq_to"`
PolicyVersion string `json:"policy_version"`
}
json.NewDecoder(r.Body).Decode(&req)
if req.PolicyVersion == "" {
req.PolicyVersion = "ACP-POLICY-1.0.0"
}

// Get receipts
receipts, err := h.db.GetReceipts(workflowID)
if err != nil || len(receipts) == 0 {
writeErr(w, http.StatusBadRequest, "no receipts found for workflow")
return
}

// Determine seq range
if req.SeqTo == 0 {
req.SeqTo = receipts[len(receipts)-1].Seq
}
if req.SeqFrom == 0 {
req.SeqFrom = 1
}

	// Parse receipts into maps for FARD chain_root computation
	var receiptMaps []map[string]any
	for _, rec := range receipts {
		var rm map[string]any
		json.Unmarshal([]byte(rec.ReceiptJSON), &rm)
		receiptMaps = append(receiptMaps, rm)
	}

	// Call FARD anchor_payload bridge — FARD computes chain_root
	var payloadResult struct {
		OK            bool   `json:"ok"`
		PayloadJSON   string `json:"payload_json"`
		PayloadDigest string `json:"payload_digest"`
		ChainRoot     string `json:"chain_root"`
	}
	if err := h.br.RunAndUnmarshal("anchor_payload.fard", map[string]any{
		"workflow_id":    workflowID,
		"seq_from":       req.SeqFrom,
		"seq_to":         req.SeqTo,
		"receipts":       receiptMaps,
		"policy_version": req.PolicyVersion,
	}, &payloadResult); err != nil {
		writeErr(w, http.StatusInternalServerError, "anchor payload bridge: "+err.Error())
		return
	}

// Submit to external backend
backend := anchor.DefaultBackend()
extRef, err := backend.Anchor(r.Context(), payloadResult.PayloadDigest,
workflowID, req.SeqFrom, req.SeqTo)
if err != nil {
writeErr(w, http.StatusInternalServerError, "anchor backend: "+err.Error())
return
}

extRefJSON, _ := anchor.MarshalExternalRef(extRef)

	// Build proof via FARD bridge — gets correct proof_digest
	var proofResult struct {
		OK          bool   `json:"ok"`
		ProofJSON   string `json:"proof_json"`
		ProofDigest string `json:"proof_digest"`
	}
	if err := h.br.RunAndUnmarshal("anchor_proof.fard", map[string]any{
		"payload_json":  payloadResult.PayloadJSON,
		"external_ref":  extRef,
		"anchored_by":   actor.ActorID,
	}, &proofResult); err != nil {
		writeErr(w, http.StatusInternalServerError, "anchor proof bridge: "+err.Error())
		return
	}
	proofID := uuid.New().String()

// Store anchor
if err := h.db.CreateAnchor(&store.Anchor{
ProofID:       proofID,
WorkflowID:    workflowID,
SeqFrom:       req.SeqFrom,
SeqTo:         req.SeqTo,
ChainRoot:     payloadResult.ChainRoot,
PayloadDigest: payloadResult.PayloadDigest,
ProofDigest:   proofResult.ProofDigest,
ExternalKind:  extRef.Kind,
ExternalRef:   extRefJSON,
AnchoredBy:    actor.ActorID,
PayloadJSON:   payloadResult.PayloadJSON,
ProofJSON:     proofResult.ProofJSON,
}); err != nil {
writeErr(w, http.StatusInternalServerError, "store anchor: "+err.Error())
return
}

writeJSON(w, http.StatusOK, map[string]any{
"ok":             true,
"proof_id":       proofID,
"workflow_id":    workflowID,
"seq_from":       req.SeqFrom,
"seq_to":         req.SeqTo,
"chain_root":     payloadResult.ChainRoot,
"payload_digest": payloadResult.PayloadDigest,
"external_kind":  extRef.Kind,
"external_ref":   extRef,
})
}

// getAnchorLog handles GET /workflows/:id/anchor/log
func (h *Handlers) getAnchorLog(w http.ResponseWriter, r *http.Request, workflowID string) {
if r.Method != http.MethodGet {
writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
return
}
anchors, err := h.db.ListAnchors(workflowID)
if err != nil {
writeErr(w, http.StatusInternalServerError, err.Error())
return
}
summary, _ := h.db.AnchorSummary(workflowID)
writeJSON(w, http.StatusOK, map[string]any{
"ok":      true,
"anchors": anchors,
"summary": summary,
})
}

// verifyAnchor handles GET /workflows/:id/anchor/verify
func (h *Handlers) verifyAnchor(w http.ResponseWriter, r *http.Request, workflowID string) {
if r.Method != http.MethodGet {
writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
return
}

anchors, err := h.db.ListAnchors(workflowID)
if err != nil || len(anchors) == 0 {
writeErr(w, http.StatusNotFound, "no anchors found")
return
}

receipts, err := h.db.GetReceipts(workflowID)
if err != nil {
writeErr(w, http.StatusInternalServerError, err.Error())
return
}

// Convert receipts to map format for FARD
var receiptMaps []map[string]any
for _, rec := range receipts {
var rm map[string]any
json.Unmarshal([]byte(rec.ReceiptJSON), &rm)
receiptMaps = append(receiptMaps, rm)
}

// Verify each anchor proof via FARD
results := make([]map[string]any, 0, len(anchors))
allOK := true
for _, a := range anchors {
var proof map[string]any
json.Unmarshal([]byte(a.ProofJSON), &proof)

var verifyResult struct {
OK                 bool           `json:"ok"`
PayloadOK          bool           `json:"payload_ok"`
ProofDigestMatches bool           `json:"proof_digest_matches"`
RootMatches        bool           `json:"root_matches"`
DigestMatches      bool           `json:"digest_matches"`
ExternalRef        map[string]any `json:"external_ref"`
}
if err := h.br.RunAndUnmarshal("anchor_verify.fard", map[string]any{
"proof":    proof,
"receipts": receiptMaps,
}, &verifyResult); err != nil {
allOK = false
results = append(results, map[string]any{
"proof_id": a.ProofID, "ok": false, "error": err.Error(),
})
continue
}
if !verifyResult.OK {
allOK = false
}
results = append(results, map[string]any{
"proof_id":     a.ProofID,
"seq_from":     a.SeqFrom,
"seq_to":       a.SeqTo,
"ok":           verifyResult.OK,
"root_matches": verifyResult.RootMatches,
"external_ref": a.ExternalRef,
})
}

writeJSON(w, http.StatusOK, map[string]any{
"ok":          allOK,
"workflow_id": workflowID,
"proofs":      results,
"verified":    allOK,
})
}
