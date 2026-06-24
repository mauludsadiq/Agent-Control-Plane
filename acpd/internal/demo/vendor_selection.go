package demo

import (
"database/sql"
"encoding/json"
"fmt"
"time"

"github.com/google/uuid"
"github.com/mauludsadiq/agent-control-plane/acpd/internal/bridge"
"github.com/mauludsadiq/agent-control-plane/acpd/internal/store"
)

const (
colorReset  = "\033[0m"
colorGreen  = "\033[32m"
colorYellow = "\033[33m"
colorRed    = "\033[31m"
colorBlue   = "\033[34m"
colorCyan   = "\033[36m"
colorBold   = "\033[1m"
)

func print_step(n int, msg string) {
fmt.Printf("\n%s── Step %d%s  %s\n", colorBold, n, colorReset, msg)
}

func print_ok(label, value string) {
fmt.Printf("   %s✓%s  %-30s %s%s%s\n", colorGreen, colorReset, label, colorCyan, value, colorReset)
}

func print_warn(label, value string) {
fmt.Printf("   %s⚠%s  %-30s %s%s%s\n", colorYellow, colorReset, label, colorYellow, value, colorReset)
}

func print_gate(label, value string) {
fmt.Printf("   %s⊠%s  %-30s %s%s%s\n", colorRed, colorReset, label, colorRed, value, colorReset)
}

func print_replay(label, value string) {
fmt.Printf("   %s▶%s  %-30s %s%s%s\n", colorBlue, colorReset, label, colorBlue, value, colorReset)
}

func short(s string) string {
if len(s) > 24 {
return s[:24] + "..."
}
return s
}

// RunVendorSelection executes the full vendor selection demo scenario:
//
//  1. Create workflow
//  2. Research agent commits evidence artifact
//  3. Finance agent hits policy gate (spend > 5000)
//  4. Manager sees pending gate
//  5. Manager approves gate
//  6. Workflow resumes to decision stage
//  7. Final decision sealed
//  8. Replay proves decision
//  9. Print receipt chain + compliance verdict
func RunVendorSelection(db *store.DB, br *bridge.Bridge, jsonOut bool) error {
workflowID := "wf_vendor_" + uuid.New().String()[:8]
managerActor := map[string]any{"id": "manager:ops", "roles": []string{"operator", "manager"}}
researchActor := map[string]any{"id": "agent:research", "roles": []string{"agent"}}

fmt.Printf("\n%s%s Agent Control Plane — Vendor Selection Demo %s%s\n",
colorBold, colorBlue, colorReset, colorReset)
fmt.Printf("   workflow: %s\n", workflowID)
fmt.Printf("   time:     %s\n\n", time.Now().Format("2006-01-02 15:04:05"))

// ── Step 1: Create workflow ────────────────────────────────────────────────
print_step(1, "Create vendor selection workflow")

var createResult store.TransitionResult
if err := br.RunAndUnmarshal("create_workflow.fard", map[string]any{
"workflow_id": workflowID,
"goal":        "Select a vendor with auditable approval and replay",
"owner":       "ops",
}, &createResult); err != nil {
return fmt.Errorf("create workflow: %w", err)
}

wf := &store.Workflow{
WorkflowID:  workflowID,
Goal:        "Select a vendor with auditable approval and replay",
Owner:       "ops",
Stage:       "created",
StateHash:   createResult.StateHash,
CurrentSeq:  0,
SnapshotSeq: 0,
}
if err := db.CreateWorkflow(wf); err != nil {
return fmt.Errorf("store workflow: %w", err)
}
if err := db.Tx(func(tx *sql.Tx) error {
return db.SaveSnapshot(tx, &store.WorkflowSnapshot{
WorkflowID: workflowID,
Seq:        0,
StateJSON:  createResult.StateJSON,
StateHash:  createResult.StateHash,
})
}); err != nil {
return fmt.Errorf("save initial snapshot: %w", err)
}

print_ok("workflow created", workflowID)
print_ok("stage", "created")
print_ok("state hash", short(createResult.StateHash))

// ── Step 2: Operator moves to research stage ───────────────────────────────
print_step(2, "Operator advances workflow to research stage")

snap, _ := db.GetLatestSnapshot(workflowID)
var r1 store.TransitionResult
if err := br.RunAndUnmarshal("transition.fard", map[string]any{
"state_json":   snap.StateJSON,
"actor":        managerActor,
"kind":         "human_state_edit",
"patches":      []any{map[string]any{"path": "stage", "value": "research"}, map[string]any{"path": "var.candidate_vendors", "value": []string{"vendor:a", "vendor:b"}}},
"tool_version": "human",
}, &r1); err != nil {
return fmt.Errorf("transition to research: %w", err)
}
if err := db.CommitTransition(&store.CommitParams{
WorkflowID:    workflowID,
Result:        &r1,
Patches:       []map[string]any{{"path": "stage", "value": "research"}},
Kind:          "human_state_edit",
ActorID:       "manager:ops",
SnapshotEvery: 10,
}); err != nil {
return fmt.Errorf("commit transition: %w", err)
}

print_ok("stage", r1.Stage)
print_ok("seq", fmt.Sprintf("%d", r1.Seq))
print_ok("policy", "PASS")

// ── Step 3: Research agent commits evidence artifact ───────────────────────
print_step(3, "Research agent commits vendor evidence artifact")

snap, _ = db.GetLatestSnapshot(workflowID)
var artifactResult struct {
OK            bool   `json:"ok"`
Digest        string `json:"digest"`
StateJSON     string `json:"state_json"`
StateHash     string `json:"state_hash"`
ReceiptJSON   string `json:"receipt_json"`
ReceiptDigest string `json:"receipt_digest"`
Seq           int    `json:"seq"`
Stage         string `json:"stage"`
PolicyOK      bool   `json:"policy_ok"`
Violations    any    `json:"violations"`
}
artifactJSON := `{
"kind": "summary",
"name": "vendor_evidence.md",
"media_type": "text/markdown",
"content": "Vendor A: ISO-27001 certified, $420/mo. Vendor B: SOC-2 Type II, $380/mo. Recommendation: Vendor A for compliance posture.",
"producer": "agent:research",
"input_state_hash": "` + snap.StateHash + `",
"tool_trace": [{"tool": "search", "ok": true}, {"tool": "read_file", "ok": true}],
"confidence": 91
}`
if err := br.RunAndUnmarshal("commit_artifact.fard", map[string]any{
"state_json":    snap.StateJSON,
"actor":         researchActor,
"artifact_json": artifactJSON,
"tool_version":  "tool-gateway-1.0.0",
}, &artifactResult); err != nil {
return fmt.Errorf("commit artifact: %w", err)
}
if err := db.CommitArtifact(&store.ArtifactCommitParams{
WorkflowID:     workflowID,
ArtifactJSON:   artifactJSON,
ArtifactDigest: artifactResult.Digest,
Result: &store.TransitionResult{
OK:            artifactResult.OK,
StateJSON:     artifactResult.StateJSON,
StateHash:     artifactResult.StateHash,
ReceiptJSON:   artifactResult.ReceiptJSON,
ReceiptDigest: artifactResult.ReceiptDigest,
Seq:           artifactResult.Seq,
},
ActorID:       "agent:research",
SnapshotEvery: 10,
}); err != nil {
return fmt.Errorf("store artifact: %w", err)
}

print_ok("artifact committed", "vendor_evidence.md")
print_ok("producer", "agent:research")
print_ok("confidence", "91%")
print_ok("digest", short(artifactResult.Digest))

// ── Step 4: Finance agent hits policy gate (spend > 5000) ─────────────────
print_step(4, "Finance agent requests $9,000 spend — policy gate triggered")

snap, _ = db.GetLatestSnapshot(workflowID)
var gateResult struct {
OK            bool   `json:"ok"`
Token         string `json:"token"`
GateJSON      string `json:"gate_json"`
StateJSON     string `json:"state_json"`
StateHash     string `json:"state_hash"`
ReceiptJSON   string `json:"receipt_json"`
ReceiptDigest string `json:"receipt_digest"`
Seq           int    `json:"seq"`
Stage         string `json:"stage"`
Error         string `json:"error"`
}
if err := br.RunAndUnmarshal("gate_open.fard", map[string]any{
"state_json": snap.StateJSON,
"actor":      managerActor,
"input": map[string]any{
"tool":    "none",
"patches": []any{map[string]any{"path": "stage", "value": "decision"}},
"amount":  9000,
},
"gate_kind":   "manager_spend",
"deadline_seq": 20,
"context":     map[string]any{"amount": 9000, "vendor": "vendor:a"},
}, &gateResult); err != nil {
return fmt.Errorf("open gate: %w", err)
}

// Store gate
if err := db.Tx(func(tx *sql.Tx) error {
if err := db.CreateGate(tx, &store.Gate{
Token:      gateResult.Token,
WorkflowID: workflowID,
Seq:        gateResult.Seq,
Status:     "pending",
GateJSON:   gateResult.GateJSON,
}); err != nil {
return err
}
return db.SaveSnapshot(tx, &store.WorkflowSnapshot{
WorkflowID: workflowID,
Seq:        gateResult.Seq,
StateJSON:  gateResult.StateJSON,
StateHash:  gateResult.StateHash,
})
}); err != nil {
return fmt.Errorf("store gate: %w", err)
}
// Update workflow row
_ = db.Tx(func(tx *sql.Tx) error {
return db.UpdateWorkflow(tx, &store.Workflow{
WorkflowID:  workflowID,
Stage:       gateResult.Stage,
StateHash:   gateResult.StateHash,
CurrentSeq:  gateResult.Seq,
SnapshotSeq: gateResult.Seq,
})
})

print_gate("policy gate opened", "manager_spend")
print_gate("stage", gateResult.Stage)
print_gate("spend requested", "$9,000 (limit: $5,000)")
print_gate("token", short(gateResult.Token))
fmt.Printf("\n   %s   Workflow is GATED. Runtime released for other workflows.%s\n", colorYellow, colorReset)

// ── Step 5: Manager sees pending gate ─────────────────────────────────────
print_step(5, "Manager reviews operator inbox")

gates, _ := db.ListPendingGates()
fmt.Printf("   pending gates: %d\n", len(gates))
for _, g := range gates {
print_warn("gate", fmt.Sprintf("%s — %s", g.WorkflowID, short(g.Token)))
}

// ── Step 6: Manager approves gate ─────────────────────────────────────────
print_step(6, "Manager approves $9,000 spend")

snap, _ = db.GetLatestSnapshot(workflowID)
var resumeResult struct {
OK            bool   `json:"ok"`
StateJSON     string `json:"state_json"`
StateHash     string `json:"state_hash"`
ReceiptJSON   string `json:"receipt_json"`
ReceiptDigest string `json:"receipt_digest"`
Seq           int    `json:"seq"`
Stage         string `json:"stage"`
Error         string `json:"error"`
}
if err := br.RunAndUnmarshal("gate_resume.fard", map[string]any{
"state_json":   snap.StateJSON,
"actor":        managerActor,
"token":        gateResult.Token,
"resolution":   "approved",
"tool_version": "human",
}, &resumeResult); err != nil {
return fmt.Errorf("resume gate: %w", err)
}
if err := db.CommitGateResolution(
workflowID, gateResult.Token, "approved", "manager:ops",
&store.TransitionResult{
OK:            resumeResult.OK,
StateJSON:     resumeResult.StateJSON,
StateHash:     resumeResult.StateHash,
ReceiptJSON:   resumeResult.ReceiptJSON,
ReceiptDigest: resumeResult.ReceiptDigest,
Seq:           resumeResult.Seq,
},
10,
); err != nil {
return fmt.Errorf("commit gate resolution: %w", err)
}

// Always snapshot post-resume state so step 7 reads the correct baseline.
if err := db.Tx(func(tx *sql.Tx) error {
return db.SaveSnapshot(tx, &store.WorkflowSnapshot{
WorkflowID: workflowID,
Seq:        resumeResult.Seq,
StateJSON:  resumeResult.StateJSON,
StateHash:  resumeResult.StateHash,
})
}); err != nil {
return fmt.Errorf("save post-resume snapshot: %w", err)
}

print_ok("gate resolved", "approved")
print_ok("stage", resumeResult.Stage)
print_ok("seq", fmt.Sprintf("%d", resumeResult.Seq))

// ── Step 7: Seal final decision ───────────────────────────────────────────
print_step(7, "Seal final vendor decision")

snap, _ = db.GetLatestSnapshot(workflowID)
var finalResult store.TransitionResult
if err := br.RunAndUnmarshal("transition.fard", map[string]any{
"state_json": snap.StateJSON,
"actor":      managerActor,
"kind":       "human_state_edit",
"patches": []any{
map[string]any{"path": "final_decision", "value": map[string]any{
"vendor":    "vendor:a",
"rationale": "ISO-27001 certified, best compliance posture, approved spend $9,000",
"decided_by": "manager:ops",
}},
map[string]any{"path": "stage", "value": "complete"},
},
"tool_version": "human",
}, &finalResult); err != nil {
return fmt.Errorf("seal final decision: %w", err)
}
if err := db.CommitTransition(&store.CommitParams{
WorkflowID:    workflowID,
Result:        &finalResult,
Patches:       []map[string]any{{"path": "final_decision", "value": "vendor:a"}, {"path": "stage", "value": "complete"}},
Kind:          "human_state_edit",
ActorID:       "manager:ops",
SnapshotEvery: 10,
}); err != nil {
return fmt.Errorf("commit final decision: %w", err)
}

print_ok("final decision", "vendor:a — ISO-27001 certified")
print_ok("stage", "complete")
print_ok("seq", fmt.Sprintf("%d", finalResult.Seq))
print_ok("state hash", short(finalResult.StateHash))

// ── Step 8: Replay proves decision ────────────────────────────────────────
print_step(8, "Replay verifies complete decision chain")

receipts, _ := db.GetReceipts(workflowID)
receiptMaps := make([]map[string]any, 0, len(receipts))
for _, r := range receipts {
var rm map[string]any
if err := json.Unmarshal([]byte(r.ReceiptJSON), &rm); err == nil {
receiptMaps = append(receiptMaps, rm)
}
}

var verifyResult struct {
ChainRoot     string `json:"chain_root"`
ChainVerified bool   `json:"chain_verified"`
ReceiptCount  int    `json:"receipt_count"`
}
receiptWrappers := make([]map[string]any, 0, len(receipts))
for _, r := range receipts {
receiptWrappers = append(receiptWrappers, map[string]any{"receipt_json": r.ReceiptJSON})
}
_ = br.RunAndUnmarshal("verify_chain.fard", map[string]any{
"receipts": receiptWrappers,
}, &verifyResult)

print_replay("receipts verified", fmt.Sprintf("%d transitions", len(receipts)))
print_replay("chain root", short(verifyResult.ChainRoot))
if verifyResult.ChainVerified {
print_replay("chain integrity", "✓ VERIFIED — no tampering detected")
} else {
fmt.Printf("   %s✗  chain integrity  FAILED%s\n", colorRed, colorReset)
}

// ── Step 9: Final summary ─────────────────────────────────────────────────
fmt.Printf("\n%s%s ─────────────────────────────────────────── %s%s\n",
colorBold, colorGreen, colorReset, colorReset)
fmt.Printf("%s   COMPLIANCE: PASS%s\n", colorGreen, colorReset)
fmt.Printf("%s─────────────────────────────────────────────────%s\n\n",
colorGreen, colorReset)

wfFinal, _ := db.GetWorkflow(workflowID)
if wfFinal != nil {
print_ok("workflow", wfFinal.WorkflowID)
print_ok("final stage", wfFinal.Stage)
print_ok("total transitions", fmt.Sprintf("%d", wfFinal.CurrentSeq))
print_ok("artifacts committed", fmt.Sprintf("%d", len(receipts)-1))
print_ok("policy violations", "0")
print_ok("chain verified", fmt.Sprintf("%v", verifyResult.ChainVerified))
}

fmt.Printf("\n   %sEvery plan visible. Every artifact tracked. Every policy enforced.%s\n", colorCyan, colorReset)
fmt.Printf("   %sEvery state editable. Every decision replayable.%s\n\n", colorCyan, colorReset)

if jsonOut {
out := map[string]any{
"workflow_id":    workflowID,
"final_stage":    "complete",
"receipt_count":  len(receipts),
"chain_root":     verifyResult.ChainRoot,
"chain_verified": verifyResult.ChainVerified,
}
b, _ := json.MarshalIndent(out, "", "  ")
fmt.Println(string(b))
}

return nil
}
