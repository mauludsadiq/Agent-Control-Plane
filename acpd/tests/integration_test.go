package tests

import (
"database/sql"
	"fmt"
"testing"
"time"

"github.com/mauludsadiq/agent-control-plane/acpd/internal/store"
"github.com/mauludsadiq/agent-control-plane/acpd/internal/testutil"
)

// ─── AUTH ─────────────────────────────────────────────────────────────────────

func TestAuthRejectsNoKey(t *testing.T) {
srv, _ := testutil.NewTestServer(t)
resp := testutil.DoUnauth(t, srv, "GET", "/workflows")
if resp["_status"] != float64(401) {
t.Errorf("expected 401, got %v", resp["_status"])
}
if resp["ok"] != false {
t.Errorf("expected ok:false, got %v", resp["ok"])
}
}

func TestAuthAcceptsValidKey(t *testing.T) {
srv, _ := testutil.NewTestServer(t)
resp := testutil.Do(t, srv, "GET", "/workflows", nil)
if resp["_status"] != float64(200) {
t.Errorf("expected 200, got %v — %v", resp["_status"], resp)
}
}

func TestHealthIsUnauthenticated(t *testing.T) {
srv, _ := testutil.NewTestServer(t)
resp := testutil.DoUnauth(t, srv, "GET", "/health")
if resp["_status"] != float64(200) {
t.Errorf("expected 200, got %v", resp["_status"])
}
if resp["ok"] != true {
t.Errorf("expected ok:true, got %v", resp["ok"])
}
}

// ─── WORKFLOW LIFECYCLE ───────────────────────────────────────────────────────

func TestCreateWorkflow(t *testing.T) {
srv, _ := testutil.NewTestServer(t)
resp := testutil.Do(t, srv, "POST", "/workflows", map[string]any{
"workflow_id": "wf_create_test",
"goal":        "integration test workflow",
"owner":       "test",
})
if resp["ok"] != true {
t.Fatalf("expected ok:true, got %v", resp)
}
if resp["workflow_id"] != "wf_create_test" {
t.Errorf("expected workflow_id wf_create_test, got %v", resp["workflow_id"])
}
if resp["seq"] != float64(0) {
t.Errorf("expected seq 0, got %v", resp["seq"])
}
}

func TestCreateWorkflowGeneratesID(t *testing.T) {
srv, _ := testutil.NewTestServer(t)
resp := testutil.Do(t, srv, "POST", "/workflows", map[string]any{
"goal":  "auto id test",
"owner": "test",
})
if resp["ok"] != true {
t.Fatalf("expected ok:true, got %v", resp)
}
if resp["workflow_id"] == nil || resp["workflow_id"] == "" {
t.Errorf("expected auto-generated workflow_id, got %v", resp["workflow_id"])
}
}

func TestGetWorkflow(t *testing.T) {
srv, _ := testutil.NewTestServer(t)
testutil.Do(t, srv, "POST", "/workflows", map[string]any{
"workflow_id": "wf_get_test",
"goal":        "get test",
"owner":       "test",
})
resp := testutil.Do(t, srv, "GET", "/workflows/wf_get_test", nil)
if resp["ok"] != true {
t.Fatalf("expected ok:true, got %v", resp)
}
if resp["stage"] != "created" {
t.Errorf("expected stage created, got %v", resp["stage"])
}
}

func TestGetWorkflowNotFound(t *testing.T) {
srv, _ := testutil.NewTestServer(t)
resp := testutil.Do(t, srv, "GET", "/workflows/wf_does_not_exist", nil)
if resp["_status"] != float64(404) {
t.Errorf("expected 404, got %v", resp["_status"])
}
}

func TestListWorkflows(t *testing.T) {
srv, _ := testutil.NewTestServer(t)
testutil.Do(t, srv, "POST", "/workflows", map[string]any{
"workflow_id": "wf_list_a", "goal": "list a", "owner": "test",
})
testutil.Do(t, srv, "POST", "/workflows", map[string]any{
"workflow_id": "wf_list_b", "goal": "list b", "owner": "test",
})
resp := testutil.Do(t, srv, "GET", "/workflows", nil)
if resp["ok"] != true {
t.Fatalf("expected ok:true, got %v", resp)
}
workflows, ok := resp["workflows"].([]any)
if !ok {
t.Fatalf("expected workflows array, got %T", resp["workflows"])
}
if len(workflows) < 2 {
t.Errorf("expected at least 2 workflows, got %d", len(workflows))
}
}

// ─── STATE EDIT ───────────────────────────────────────────────────────────────

func TestEditStateAdvancesStage(t *testing.T) {
srv, _ := testutil.NewTestServer(t)
testutil.Do(t, srv, "POST", "/workflows", map[string]any{
"workflow_id": "wf_edit_test",
"goal":        "edit test",
"owner":       "test",
})
resp := testutil.Do(t, srv, "POST", "/workflows/wf_edit_test/state/edit", map[string]any{
"patches": []any{map[string]any{"path": "stage", "value": "research"}},
})
if resp["ok"] != true {
t.Fatalf("expected ok:true, got %v", resp)
}
if resp["seq"] != float64(1) {
t.Errorf("expected seq 1, got %v", resp["seq"])
}
}

func TestEditStateCommitsReceipt(t *testing.T) {
srv, db := testutil.NewTestServer(t)
testutil.Do(t, srv, "POST", "/workflows", map[string]any{
"workflow_id": "wf_receipt_test",
"goal":        "receipt test",
"owner":       "test",
})
testutil.Do(t, srv, "POST", "/workflows/wf_receipt_test/state/edit", map[string]any{
"patches": []any{map[string]any{"path": "stage", "value": "research"}},
})
receipts, err := db.GetReceipts("wf_receipt_test")
if err != nil {
t.Fatalf("get receipts: %v", err)
}
if len(receipts) != 1 {
t.Errorf("expected 1 receipt, got %d", len(receipts))
}
if receipts[0].ReceiptDigest == "" {
t.Error("receipt digest is empty")
}
}

func TestMultipleEditsChainReceipts(t *testing.T) {
srv, db := testutil.NewTestServer(t)
testutil.Do(t, srv, "POST", "/workflows", map[string]any{
"workflow_id": "wf_chain_test",
"goal":        "chain test",
"owner":       "test",
})
for i, stage := range []string{"research", "review", "decision"} {
resp := testutil.Do(t, srv, "POST", "/workflows/wf_chain_test/state/edit", map[string]any{
"patches": []any{map[string]any{"path": "stage", "value": stage}},
})
if resp["ok"] != true {
t.Fatalf("step %d failed: %v", i, resp)
}
}
receipts, _ := db.GetReceipts("wf_chain_test")
if len(receipts) != 3 {
t.Errorf("expected 3 receipts, got %d", len(receipts))
}
for i := 1; i < len(receipts); i++ {
if receipts[i].PrevReceiptDigest != receipts[i-1].ReceiptDigest {
t.Errorf("receipt chain broken at seq %d", receipts[i].Seq)
}
}
}

// ─── RECEIPTS + CHAIN ─────────────────────────────────────────────────────────

func TestGetReceiptsVerifiesChain(t *testing.T) {
srv, _ := testutil.NewTestServer(t)
testutil.Do(t, srv, "POST", "/workflows", map[string]any{
"workflow_id": "wf_verify_test",
"goal":        "verify test",
"owner":       "test",
})
testutil.Do(t, srv, "POST", "/workflows/wf_verify_test/state/edit", map[string]any{
"patches": []any{map[string]any{"path": "stage", "value": "research"}},
})
resp := testutil.Do(t, srv, "GET", "/workflows/wf_verify_test/receipts", nil)
if resp["ok"] != true {
t.Fatalf("expected ok:true, got %v", resp)
}
if resp["chain_verified"] != true {
t.Errorf("expected chain_verified:true, got %v", resp["chain_verified"])
}
}

// ─── DASHBOARD ────────────────────────────────────────────────────────────────

func TestDashboardProjection(t *testing.T) {
srv, _ := testutil.NewTestServer(t)
testutil.Do(t, srv, "POST", "/workflows", map[string]any{
"workflow_id": "wf_dash_test",
"goal":        "dashboard test",
"owner":       "test",
})
testutil.Do(t, srv, "POST", "/workflows/wf_dash_test/state/edit", map[string]any{
"patches": []any{map[string]any{"path": "stage", "value": "research"}},
})
resp := testutil.Do(t, srv, "GET", "/workflows/wf_dash_test/dashboard", nil)
if resp["ok"] != true {
t.Fatalf("expected ok:true, got %v", resp)
}
dash, ok := resp["dashboard"].(map[string]any)
if !ok {
t.Fatalf("expected dashboard object, got %T", resp["dashboard"])
}
if dash["workflow_id"] != "wf_dash_test" {
t.Errorf("wrong workflow_id: %v", dash["workflow_id"])
}
if dash["replay_available"] != true {
t.Errorf("expected replay_available:true, got %v", dash["replay_available"])
}
}

// ─── TASK QUEUE ───────────────────────────────────────────────────────────────

func TestClaimNextTaskEmpty(t *testing.T) {
srv, _ := testutil.NewTestServer(t)
resp := testutil.Do(t, srv, "GET", "/tasks/next?agent=agent:research", nil)
if resp["ok"] != true {
t.Fatalf("expected ok:true, got %v", resp)
}
if resp["task"] != nil {
t.Errorf("expected nil task on empty queue, got %v", resp["task"])
}
}

func TestClaimNextTaskRequiresAgent(t *testing.T) {
srv, _ := testutil.NewTestServer(t)
resp := testutil.Do(t, srv, "GET", "/tasks/next", nil)
if resp["_status"] != float64(400) {
t.Errorf("expected 400, got %v", resp["_status"])
}
}

func TestTaskEnqueueAndClaim(t *testing.T) {
srv, db := testutil.NewTestServer(t)
testutil.Do(t, srv, "POST", "/workflows", map[string]any{
"workflow_id": "wf_task_enqueue",
"goal":        "task enqueue test",
"owner":       "test",
})

// Enqueue via store transaction
if err := db.Tx(func(tx *sql.Tx) error {
return db.EnqueueTask(tx, &store.Task{
TaskID:     "task_enqueue_001",
WorkflowID: "wf_task_enqueue",
NodeID:     "research",
Agent:      "agent:research",
InputJSON:  `{"task_id":"task_enqueue_001"}`,
TimeoutSec: 300,
})
}); err != nil {
t.Fatalf("enqueue task: %v", err)
}

resp := testutil.Do(t, srv, "GET", "/tasks/next?agent=agent:research", nil)
if resp["ok"] != true {
t.Fatalf("expected ok:true, got %v", resp)
}
if resp["task"] == nil {
t.Error("expected task, got nil")
}
}

// ─── GATE ─────────────────────────────────────────────────────────────────────

func TestOperatorInboxEmpty(t *testing.T) {
srv, _ := testutil.NewTestServer(t)
resp := testutil.Do(t, srv, "GET", "/operator/inbox", nil)
if resp["ok"] != true {
t.Fatalf("expected ok:true, got %v", resp)
}
}

func TestGateResumeNonexistentToken(t *testing.T) {
srv, _ := testutil.NewTestServer(t)
testutil.Do(t, srv, "POST", "/workflows", map[string]any{
"workflow_id": "wf_gate_404",
"goal":        "gate 404 test",
"owner":       "test",
})
resp := testutil.Do(t, srv, "POST",
"/workflows/wf_gate_404/gates/sha256:nonexistent/resume",
map[string]any{"resolution": "approved"},
)
if resp["_status"] != float64(404) {
t.Errorf("expected 404, got %v — %v", resp["_status"], resp)
}
}

func TestGateResumeInvalidResolution(t *testing.T) {
srv, _ := testutil.NewTestServer(t)
testutil.Do(t, srv, "POST", "/workflows", map[string]any{
"workflow_id": "wf_gate_invalid",
"goal":        "gate invalid test",
"owner":       "test",
})
resp := testutil.Do(t, srv, "POST",
"/workflows/wf_gate_invalid/gates/some_token/resume",
map[string]any{"resolution": "maybe"},
)
if resp["_status"] != float64(400) {
t.Errorf("expected 400, got %v", resp["_status"])
}
}

// ─── REPLAY ───────────────────────────────────────────────────────────────────

func TestReplayEndpoint(t *testing.T) {
srv, _ := testutil.NewTestServer(t)
testutil.Do(t, srv, "POST", "/workflows", map[string]any{
"workflow_id": "wf_replay_test",
"goal":        "replay test",
"owner":       "test",
})
testutil.Do(t, srv, "POST", "/workflows/wf_replay_test/state/edit", map[string]any{
"patches": []any{map[string]any{"path": "stage", "value": "research"}},
})
resp := testutil.Do(t, srv, "POST", "/workflows/wf_replay_test/replay", map[string]any{
"events": []any{
map[string]any{
"kind":  "human_state_edit",
"input": map[string]any{"tool": "none", "patches": []any{map[string]any{"path": "stage", "value": "research"}}},
},
},
"tool_version": "human",
})
if resp["ok"] != true {
t.Fatalf("expected ok:true, got %v", resp)
}
replay, ok := resp["replay"].(map[string]any)
if !ok {
t.Fatalf("expected replay object, got %T", resp["replay"])
}
if replay["chain_verified"] != true {
t.Errorf("expected chain_verified:true, got %v", replay["chain_verified"])
}
}

// ─── FULL LIFECYCLE ───────────────────────────────────────────────────────────

func TestFullWorkflowLifecycle(t *testing.T) {
srv, db := testutil.NewTestServer(t)
wfID := fmt.Sprintf("wf_full_%d", time.Now().UnixNano())

r := testutil.Do(t, srv, "POST", "/workflows", map[string]any{
"workflow_id": wfID,
"goal":        "full lifecycle test",
"owner":       "test",
})
if r["ok"] != true {
t.Fatalf("create failed: %v", r)
}

for _, stage := range []string{"research", "review", "decision"} {
r = testutil.Do(t, srv, "POST", "/workflows/"+wfID+"/state/edit", map[string]any{
"patches": []any{map[string]any{"path": "stage", "value": stage}},
})
if r["ok"] != true {
t.Fatalf("edit to %s failed: %v", stage, r)
}
}

r = testutil.Do(t, srv, "GET", "/workflows/"+wfID+"/dashboard", nil)
if r["ok"] != true {
t.Fatalf("dashboard failed: %v", r)
}

r = testutil.Do(t, srv, "GET", "/workflows/"+wfID+"/receipts", nil)
if r["chain_verified"] != true {
t.Errorf("chain not verified: %v", r)
}

wf, err := db.GetWorkflow(wfID)
if err != nil || wf == nil {
t.Fatalf("get workflow: %v", err)
}
if wf.Stage != "decision" {
t.Errorf("expected stage decision, got %s", wf.Stage)
}
if wf.CurrentSeq != 3 {
t.Errorf("expected seq 3, got %d", wf.CurrentSeq)
}

receipts, _ := db.GetReceipts(wfID)
if len(receipts) != 3 {
t.Errorf("expected 3 receipts, got %d", len(receipts))
}
for i := 1; i < len(receipts); i++ {
if receipts[i].PrevReceiptDigest != receipts[i-1].ReceiptDigest {
t.Errorf("receipt chain broken at seq %d", receipts[i].Seq)
}
}
}
