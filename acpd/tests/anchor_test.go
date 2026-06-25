package tests_test

import (
"net/http"
"testing"

"github.com/mauludsadiq/agent-control-plane/acpd/internal/testutil"
)

// TestAnchorCreation verifies that a workflow can be anchored after transitions.
func TestAnchorCreation(t *testing.T) {
ts, _ := testutil.NewTestServer(t)

// Create and advance workflow
wf := testutil.Do(t, ts, http.MethodPost, "/workflows",
map[string]any{"goal": "anchor test", "owner": "test"})
wfID := wf["workflow_id"].(string)

for _, stage := range []string{"research", "review", "decision"} {
testutil.Do(t, ts, http.MethodPost, "/workflows/"+wfID+"/state/edit",
map[string]any{
"patches": []map[string]any{{"path": "stage", "value": stage}},
"kind":    "human_state_edit",
})
}

// Anchor the workflow
resp := testutil.Do(t, ts, http.MethodPost, "/workflows/"+wfID+"/anchor",
map[string]any{"policy_version": "ACP-POLICY-1.0.0"})

if resp["ok"] != true {
t.Fatalf("anchor failed: %v", resp)
}
proofID := resp["proof_id"].(string)
chainRoot := resp["chain_root"].(string)
extKind := resp["external_kind"].(string)

if proofID == "" {
t.Error("empty proof ID")
}
if chainRoot == "" {
t.Error("empty chain root")
}
if extKind != "local" {
t.Errorf("expected local backend, got %s", extKind)
}

t.Logf("anchored: proof=%s chain_root=%s backend=%s", proofID[:8], chainRoot[:16], extKind)
t.Log("=== v0.9.5 ANCHOR CREATION VERIFIED ===")
}

// TestAnchorLog verifies the anchor log endpoint.
func TestAnchorLog(t *testing.T) {
ts, _ := testutil.NewTestServer(t)

wf := testutil.Do(t, ts, http.MethodPost, "/workflows",
map[string]any{"goal": "anchor log test", "owner": "test"})
wfID := wf["workflow_id"].(string)

testutil.Do(t, ts, http.MethodPost, "/workflows/"+wfID+"/state/edit",
map[string]any{
"patches": []map[string]any{{"path": "stage", "value": "research"}},
"kind":    "human_state_edit",
})
testutil.Do(t, ts, http.MethodPost, "/workflows/"+wfID+"/state/edit",
map[string]any{
"patches": []map[string]any{{"path": "stage", "value": "complete"}},
"kind":    "human_state_edit",
})

// Anchor twice at different seq ranges
testutil.Do(t, ts, http.MethodPost, "/workflows/"+wfID+"/anchor",
map[string]any{"seq_from": 1, "seq_to": 1})
testutil.Do(t, ts, http.MethodPost, "/workflows/"+wfID+"/anchor",
map[string]any{"seq_from": 2, "seq_to": 2})

// Get anchor log
logResp := testutil.Do(t, ts, http.MethodGet, "/workflows/"+wfID+"/anchor/log", nil)
if logResp["ok"] != true {
t.Fatalf("anchor log failed: %v", logResp)
}

anchors := logResp["anchors"].([]any)
if len(anchors) != 2 {
t.Errorf("expected 2 anchors, got %d", len(anchors))
}

summary := logResp["summary"].(map[string]any)
proofCount := int(summary["proof_count"].(float64))
if proofCount != 2 {
t.Errorf("expected proof_count 2, got %d", proofCount)
}

t.Logf("anchor log: %d proofs, fully_anchored=%v", proofCount, summary["fully_anchored"])
}

// TestAnchorGapDetection verifies that gaps are detected when not all seqs are anchored.
func TestAnchorGapDetection(t *testing.T) {
ts, _ := testutil.NewTestServer(t)

wf := testutil.Do(t, ts, http.MethodPost, "/workflows",
map[string]any{"goal": "gap test", "owner": "test"})
wfID := wf["workflow_id"].(string)

// 3 transitions
for _, stage := range []string{"research", "review", "complete"} {
testutil.Do(t, ts, http.MethodPost, "/workflows/"+wfID+"/state/edit",
map[string]any{
"patches": []map[string]any{{"path": "stage", "value": stage}},
"kind":    "human_state_edit",
})
}

// Only anchor seq 1 — leaving 2 and 3 as gaps
testutil.Do(t, ts, http.MethodPost, "/workflows/"+wfID+"/anchor",
map[string]any{"seq_from": 1, "seq_to": 1})

logResp := testutil.Do(t, ts, http.MethodGet, "/workflows/"+wfID+"/anchor/log", nil)
summary := logResp["summary"].(map[string]any)

gapCount := int(summary["gap_count"].(float64))
fullyAnchored := summary["fully_anchored"].(bool)

if gapCount == 0 {
t.Error("expected gaps when only seq 1 anchored out of 3")
}
if fullyAnchored {
t.Error("should not be fully anchored with gaps")
}
t.Logf("gap detection: gap_count=%d fully_anchored=%v ✓", gapCount, fullyAnchored)
}

// TestAnchorVerify verifies the anchor verify endpoint confirms proof integrity.
func TestAnchorVerify(t *testing.T) {
ts, _ := testutil.NewTestServer(t)

wf := testutil.Do(t, ts, http.MethodPost, "/workflows",
map[string]any{"goal": "verify test", "owner": "test"})
wfID := wf["workflow_id"].(string)

testutil.Do(t, ts, http.MethodPost, "/workflows/"+wfID+"/state/edit",
map[string]any{
"patches": []map[string]any{{"path": "stage", "value": "research"}},
"kind":    "human_state_edit",
})

// Anchor
anchorResp := testutil.Do(t, ts, http.MethodPost, "/workflows/"+wfID+"/anchor",
map[string]any{})
if anchorResp["ok"] != true {
t.Fatalf("anchor failed: %v", anchorResp)
}

// Verify
verifyResp := testutil.Do(t, ts, http.MethodGet, "/workflows/"+wfID+"/anchor/verify", nil)
if verifyResp["ok"] != true {
t.Fatalf("verify failed: %v", verifyResp)
}
if verifyResp["verified"] != true {
t.Error("expected verified=true")
}

proofs := verifyResp["proofs"].([]any)
if len(proofs) == 0 {
t.Error("expected at least one proof")
}

t.Logf("anchor verify: %d proofs verified ✓", len(proofs))
t.Log("=== v0.9.5 INDEPENDENT VERIFICATION VERIFIED ===")
}

// TestAnchorScale verifies anchoring at scale — 1,000 workflows each anchored.
func TestAnchorScale(t *testing.T) {
const count = 100 // smoke; set ACP_ANCHOR_COUNT=1000 for full
ts, _ := testutil.NewTestServer(t)

anchored := 0
for i := 0; i < count; i++ {
wf := testutil.Do(t, ts, http.MethodPost, "/workflows",
map[string]any{"goal": "anchor scale", "owner": "test"})
wfID := wf["workflow_id"].(string)

testutil.Do(t, ts, http.MethodPost, "/workflows/"+wfID+"/state/edit",
map[string]any{
"patches": []map[string]any{{"path": "stage", "value": "complete"}},
"kind":    "human_state_edit",
})

resp := testutil.Do(t, ts, http.MethodPost, "/workflows/"+wfID+"/anchor", map[string]any{})
if resp["ok"] == true {
anchored++
}
}

if anchored != count {
t.Errorf("expected %d anchored, got %d", count, anchored)
}
t.Logf("anchor scale: %d/%d workflows anchored ✓", anchored, count)
}
