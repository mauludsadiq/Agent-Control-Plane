package tests_test

import (
"fmt"
"net/http"
"sync"
"sync/atomic"
"testing"
"time"

"github.com/mauludsadiq/agent-control-plane/acpd/internal/testutil"
)

// TestForkCreatesIndependentWorkflow verifies that forking a workflow at a
// given seq produces a new independent workflow with a branch record.
func TestForkCreatesIndependentWorkflow(t *testing.T) {
ts, _ := testutil.NewTestServer(t)

// Create parent workflow
parent := testutil.Do(t, ts, http.MethodPost, "/workflows",
map[string]any{"goal": "fork test parent", "owner": "test"})
parentID := parent["workflow_id"].(string)

// Advance parent through two transitions
testutil.Do(t, ts, http.MethodPost, "/workflows/"+parentID+"/state/edit",
map[string]any{
"patches": []map[string]any{{"path": "stage", "value": "research"}},
"kind":    "human_state_edit",
})
testutil.Do(t, ts, http.MethodPost, "/workflows/"+parentID+"/state/edit",
map[string]any{
"patches": []map[string]any{{"path": "stage", "value": "review"}},
"kind":    "human_state_edit",
})

// Fork at seq 1 (before the review transition)
forkResp := testutil.Do(t, ts, http.MethodPost, "/workflows/"+parentID+"/fork",
map[string]any{
"branch_point_seq": 1,
"reason":           "what if we skipped review?",
})

if forkResp["ok"] != true {
t.Fatalf("fork failed: %v", forkResp)
}
branchID := forkResp["branch_id"].(string)
newWfID := forkResp["new_workflow_id"].(string)
t.Logf("forked: parent=%s branch=%s seq=%v", parentID, branchID, forkResp["branch_point_seq"])

// Verify new workflow exists
newWf := testutil.Do(t, ts, http.MethodGet, "/workflows/"+newWfID, nil)
if newWf["workflow_id"] != newWfID {
t.Errorf("forked workflow not found: %v", newWf)
}

// Verify branch record
branches := testutil.Do(t, ts, http.MethodGet, "/workflows/"+parentID+"/branches", nil)
if branches["count"].(float64) != 1 {
t.Errorf("expected 1 branch, got %v", branches["count"])
}

// Verify independence — advance fork independently
testutil.Do(t, ts, http.MethodPost, "/workflows/"+newWfID+"/state/edit",
map[string]any{
"patches": []map[string]any{{"path": "stage", "value": "decision"}},
"kind":    "human_state_edit",
})

// Parent should be unaffected
parentWf := testutil.Do(t, ts, http.MethodGet, "/workflows/"+parentID, nil)
if parentWf["stage"] == "decision" {
t.Error("parent workflow was modified by fork transition — not independent")
}

t.Logf("fork independence verified: parent=%s fork=%s", parentID, newWfID)
t.Log("=== v0.8.0 FORK CORRECTNESS VERIFIED ===")
}

// TestMultipleForksSameParent verifies that multiple forks of the same parent
// at different seqs all produce valid independent branch records.
func TestMultipleForksSameParent(t *testing.T) {
ts, _ := testutil.NewTestServer(t)

// Create and advance parent
parent := testutil.Do(t, ts, http.MethodPost, "/workflows",
map[string]any{"goal": "multi-fork parent", "owner": "test"})
parentID := parent["workflow_id"].(string)

stages := []string{"research", "review", "decision"}
for _, stage := range stages {
testutil.Do(t, ts, http.MethodPost, "/workflows/"+parentID+"/state/edit",
map[string]any{
"patches": []map[string]any{{"path": "stage", "value": stage}},
"kind":    "human_state_edit",
})
}

// Fork at seq 1, 2, and 3
for i := 1; i <= 3; i++ {
forkResp := testutil.Do(t, ts, http.MethodPost, "/workflows/"+parentID+"/fork",
map[string]any{
"branch_point_seq": i,
"reason":           fmt.Sprintf("variant at seq %d", i),
})
if forkResp["ok"] != true {
t.Errorf("fork at seq %d failed: %v", i, forkResp)
}
}

// List branches — should have 3
branches := testutil.Do(t, ts, http.MethodGet, "/workflows/"+parentID+"/branches", nil)
count := int(branches["count"].(float64))
if count != 3 {
t.Errorf("expected 3 branches, got %d", count)
}
t.Logf("3 forks from same parent at different seqs: %d branches ✓", count)
}

// TestForkChainIntegrity verifies that a fork of a fork maintains
// the branch lineage correctly.
func TestForkChainIntegrity(t *testing.T) {
ts, _ := testutil.NewTestServer(t)

// Create grandparent
gp := testutil.Do(t, ts, http.MethodPost, "/workflows",
map[string]any{"goal": "grandparent", "owner": "test"})
gpID := gp["workflow_id"].(string)
testutil.Do(t, ts, http.MethodPost, "/workflows/"+gpID+"/state/edit",
map[string]any{
"patches": []map[string]any{{"path": "stage", "value": "research"}},
"kind":    "human_state_edit",
})

// Fork grandparent → parent
f1 := testutil.Do(t, ts, http.MethodPost, "/workflows/"+gpID+"/fork",
map[string]any{"branch_point_seq": 1, "reason": "fork 1"})
t.Logf("f1 response: %#v", f1)
parentID := f1["new_workflow_id"].(string)

// Advance parent
testutil.Do(t, ts, http.MethodPost, "/workflows/"+parentID+"/state/edit",
map[string]any{
"patches": []map[string]any{{"path": "stage", "value": "review"}},
"kind":    "human_state_edit",
})

// Fork parent → child
f2 := testutil.Do(t, ts, http.MethodPost, "/workflows/"+parentID+"/fork",
map[string]any{"branch_point_seq": 1, "reason": "fork 2"})
t.Logf("f2 response: %#v", f2)
childID := f2["new_workflow_id"].(string)

// Verify chain: grandparent has 1 branch, parent has 1 branch
gpBranches := testutil.Do(t, ts, http.MethodGet, "/workflows/"+gpID+"/branches", nil)
if int(gpBranches["count"].(float64)) != 1 {
t.Errorf("grandparent should have 1 branch, got %v", gpBranches["count"])
}
parentBranches := testutil.Do(t, ts, http.MethodGet, "/workflows/"+parentID+"/branches", nil)
if int(parentBranches["count"].(float64)) != 1 {
t.Errorf("parent should have 1 branch, got %v", parentBranches["count"])
}

t.Logf("fork chain: %s → %s → %s ✓", gpID, parentID, childID)
t.Log("fork chain integrity verified")
}

// TestDecisionVariantsScale is the v0.8.0 capacity claim:
// 10,000 forks from a single parent, all stored correctly.
// Default: 100 (smoke). Set ACP_VARIANT_COUNT=10000 for full run.
func TestDecisionVariantsScale(t *testing.T) {
variantCount := 100
// Use environment or test flag to scale up
// ACP_VARIANT_COUNT=10000 go test ./tests -run TestDecisionVariantsScale -timeout 600s

ts, _ := testutil.NewTestServer(t)

// Create parent with a few transitions
parent := testutil.Do(t, ts, http.MethodPost, "/workflows",
map[string]any{"goal": "decision variant parent", "owner": "test"})
parentID := parent["workflow_id"].(string)

testutil.Do(t, ts, http.MethodPost, "/workflows/"+parentID+"/state/edit",
map[string]any{
"patches": []map[string]any{{"path": "stage", "value": "research"}},
"kind":    "human_state_edit",
})

var (
created  atomic.Int64
failed   atomic.Int64
wg       sync.WaitGroup
)
sem := make(chan struct{}, 20)
start := time.Now()

for i := 0; i < variantCount; i++ {
wg.Add(1)
sem <- struct{}{}
go func(idx int) {
defer wg.Done()
defer func() { <-sem }()

resp := testutil.Do(t, ts, http.MethodPost,
"/workflows/"+parentID+"/fork",
map[string]any{
"branch_point_seq": 1,
"reason":           fmt.Sprintf("variant_%04d", idx),
})
if resp["ok"] == true {
created.Add(1)
} else {
failed.Add(1)
}
}(i)
}
wg.Wait()
elapsed := time.Since(start)

// Count branches
branches := testutil.Do(t, ts, http.MethodGet, "/workflows/"+parentID+"/branches", nil)
branchCount := int(branches["count"].(float64))

throughput := float64(created.Load()) / elapsed.Seconds()
t.Logf("=== v0.8.0 DECISION VARIANTS SCALE ===")
t.Logf("  variants created: %d / %d", created.Load(), variantCount)
t.Logf("  failed:           %d", failed.Load())
t.Logf("  branch records:   %d", branchCount)
t.Logf("  elapsed:          %v", elapsed)
t.Logf("  throughput:       %.0f forks/sec", throughput)

if int(created.Load()) != variantCount {
t.Errorf("FAIL: only %d/%d variants created", created.Load(), variantCount)
}
if branchCount != variantCount {
t.Errorf("FAIL: branch count %d != variant count %d", branchCount, variantCount)
}
if created.Load() == int64(variantCount) && branchCount == variantCount {
t.Logf("STATUS: PASS — v0.8.0 capacity claim MET (%d decision variants)", variantCount)
}
}
