package tests_test

import (
"encoding/json"
"fmt"
"net/http"
"testing"

"github.com/mauludsadiq/agent-control-plane/acpd/internal/store"
"github.com/mauludsadiq/agent-control-plane/acpd/internal/testutil"
)

func TestMultiModelPlanExecution(t *testing.T) {
ts, _ := testutil.NewTestServer(t)

body := map[string]any{
"goal":  "multi-model vendor selection",
"owner": "test",
"plan": map[string]any{
"nodes": []map[string]any{
{"id": "goal",     "title": "Define goal",      "agent": "manager",          "status": "done",    "risk": "medium",   "output_schema": "decision"},
{"id": "research", "title": "Research vendors", "agent": "research_agent",   "status": "pending", "risk": "medium",   "output_schema": "vendor_evidence"},
{"id": "finance",  "title": "Score vendors",    "agent": "finance_agent",    "status": "pending", "risk": "high",     "output_schema": "scorecard"},
{"id": "legal",    "title": "Legal review",     "agent": "legal_agent",      "status": "pending", "risk": "critical", "output_schema": "legal_findings"},
{"id": "decision", "title": "Final decision",   "agent": "procurement_agent","status": "pending", "risk": "critical", "output_schema": "recommendation"},
},
"edges": []map[string]any{
{"from": "goal",     "to": "research", "reason": "decompose"},
{"from": "research", "to": "finance",  "reason": "evidence required"},
{"from": "research", "to": "legal",    "reason": "evidence required"},
{"from": "finance",  "to": "decision", "reason": "score required"},
{"from": "legal",    "to": "decision", "reason": "legal clearance required"},
},
},
}

createResp := testutil.Do(t, ts, http.MethodPost, "/workflows", body)
wfID := createResp["workflow_id"].(string)
t.Logf("created workflow: %s", wfID)

routes := []store.ModelRoute{
{NodeAgent: "research_agent",    AgentID: "agent:claude-opus",   Model: "claude-opus-4-6"},
{NodeAgent: "finance_agent",     AgentID: "agent:claude-sonnet", Model: "claude-sonnet-4-6"},
{NodeAgent: "legal_agent",       AgentID: "agent:claude-opus",   Model: "claude-opus-4-6"},
{NodeAgent: "procurement_agent", AgentID: "agent:claude-sonnet", Model: "claude-sonnet-4-6"},
}

// Execute plan — only research is ready (goal is done, research has no pending blockers)
execResp := testutil.Do(t, ts, http.MethodPost,
fmt.Sprintf("/workflows/%s/plan/execute", wfID),
map[string]any{"routes": routes})

	t.Logf("execResp keys: %v", execResp)
	if execResp["enqueued"] == nil { t.Fatalf("plan/execute bad response: %v", execResp) }
	enqueued := int(execResp["enqueued"].(float64))
if enqueued != 1 {
t.Errorf("expected 1 task enqueued (research), got %d", enqueued)
}
t.Logf("step 1: %d task enqueued (research only — finance+legal blocked)", enqueued)

// Research agent claims its task
claimResp := testutil.Do(t, ts, http.MethodGet, "/tasks/next?agent=agent:claude-opus", nil)
if claimResp["task"] == nil {
t.Fatal("research agent could not claim task")
}
taskRaw, _ := json.Marshal(claimResp["task"])
var task store.Task
json.Unmarshal(taskRaw, &task)
if task.NodeID != "research" {
t.Errorf("expected research node task, got %s", task.NodeID)
}
t.Logf("research agent claimed task: %s (node=%s)", task.TaskID, task.NodeID)

// Complete research task
testutil.Do(t, ts, http.MethodPost, fmt.Sprintf("/tasks/%s/complete", task.TaskID),
map[string]any{"output": `{"vendors":["A","B"]}`, "policy_result": "pass"})

// Mark research node done in plan
markResp := testutil.Do(t, ts, http.MethodPost,
fmt.Sprintf("/workflows/%s/plan/nodes/research/done", wfID), nil)
if markResp["status"] != "done" {
t.Errorf("expected status done, got %v", markResp["status"])
}

// Execute plan again — finance AND legal now ready
execResp2 := testutil.Do(t, ts, http.MethodPost,
fmt.Sprintf("/workflows/%s/plan/execute", wfID),
map[string]any{"routes": routes})

enqueued2 := int(execResp2["enqueued"].(float64))
if enqueued2 != 2 {
t.Errorf("expected 2 tasks (finance+legal), got %d", enqueued2)
}
t.Logf("step 2: %d tasks enqueued (finance + legal unblocked)", enqueued2)

// Finance agent claims
finResp := testutil.Do(t, ts, http.MethodGet, "/tasks/next?agent=agent:claude-sonnet", nil)
if finResp["task"] == nil {
t.Fatal("finance agent could not claim task")
}
taskRaw, _ = json.Marshal(finResp["task"])
var finTask store.Task
json.Unmarshal(taskRaw, &finTask)
t.Logf("finance agent claimed: %s (node=%s)", finTask.TaskID, finTask.NodeID)

// Legal agent claims
legResp := testutil.Do(t, ts, http.MethodGet, "/tasks/next?agent=agent:claude-opus", nil)
if legResp["task"] == nil {
t.Fatal("legal agent could not claim task")
}
taskRaw, _ = json.Marshal(legResp["task"])
var legTask store.Task
json.Unmarshal(taskRaw, &legTask)
t.Logf("legal agent claimed: %s (node=%s)", legTask.TaskID, legTask.NodeID)

if finTask.NodeID == legTask.NodeID {
t.Error("finance and legal should be different nodes")
}

t.Log("=== v0.5.0 MULTI-MODEL ROUTING VERIFIED ===")
t.Log("  research  → agent:claude-opus   (claude-opus-4-6)")
t.Log("  finance   → agent:claude-sonnet (claude-sonnet-4-6)")
t.Log("  legal     → agent:claude-opus   (claude-opus-4-6)")
t.Log("  DAG gating: finance+legal blocked until research done ✓")
t.Log("  Model routing: different agents for different node types ✓")
}

func TestPlanWithNoPlan(t *testing.T) {
ts, _ := testutil.NewTestServer(t)

createResp := testutil.Do(t, ts, http.MethodPost, "/workflows",
map[string]any{"goal": "no plan", "owner": "test"})
wfID := createResp["workflow_id"].(string)

execResp := testutil.Do(t, ts, http.MethodPost,
fmt.Sprintf("/workflows/%s/plan/execute", wfID), map[string]any{})

enqueued := int(execResp["enqueued"].(float64))
if enqueued != 0 {
t.Errorf("expected 0 tasks for workflow with no plan, got %d", enqueued)
}
t.Log("workflow with no plan returns 0 tasks ✓")
}

func TestModelRouteEndpoint(t *testing.T) {
ts, _ := testutil.NewTestServer(t)

resp := testutil.Do(t, ts, http.MethodGet, "/model-routes", nil)
if resp["ok"] != true {
t.Error("expected ok:true")
}
routes := resp["routes"].([]any)
if len(routes) == 0 {
t.Error("expected at least one route")
}
t.Logf("model routes: %d configured", len(routes))
}
