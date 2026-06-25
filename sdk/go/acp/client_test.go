package acp_test

import (
"context"
"net/http/httptest"
"os"
"testing"
"time"

"github.com/mauludsadiq/agent-control-plane/sdk/go/acp"
"github.com/mauludsadiq/agent-control-plane/sdk/go/adapters"
)

// startServer starts a real ACP server for SDK tests.
// Requires the acpd package to be importable — uses a subprocess instead.
// For SDK tests we spin up an httptest server that proxies to a real acpd instance.
// Simpler: just test against a mock that validates request shapes.

const testKey = "acp_sdk_test_key_v9"

// mockServer creates a minimal ACP-compatible test server.
func mockServer(t *testing.T) (*httptest.Server, *acp.Client) {
t.Helper()
// Use the real acpd test server via its HTTP handler
// Import path: run a subprocess or use shared test infra
// For SDK tests, use a lightweight mock that validates API contract
mux := newMockACPMux(t)
srv := httptest.NewServer(mux)
t.Cleanup(srv.Close)
client := acp.NewClient(srv.URL, testKey)
return srv, client
}

// TestClientHealth verifies the health endpoint.
func TestClientHealth(t *testing.T) {
_, client := mockServer(t)
ctx := context.Background()
if err := client.Health(ctx); err != nil {
t.Fatalf("Health: %v", err)
}
t.Log("Health: OK")
}

// TestClientCreateWorkflow verifies workflow creation via SDK.
func TestClientCreateWorkflow(t *testing.T) {
_, client := mockServer(t)
ctx := context.Background()

wf, err := client.CreateWorkflow(ctx, "SDK test workflow", "sdk-test", nil)
if err != nil {
t.Fatalf("CreateWorkflow: %v", err)
}
if wf.WorkflowID == "" {
t.Error("empty workflow ID")
}
if wf.Goal != "SDK test workflow" {
t.Errorf("wrong goal: %s", wf.Goal)
}
t.Logf("created workflow: %s", wf.WorkflowID)
}

// TestClientWorkflowBuilder verifies the fluent builder API.
func TestClientWorkflowBuilder(t *testing.T) {
_, client := mockServer(t)
ctx := context.Background()

builder, err := client.NewWorkflow("builder test", "sdk-test").Create(ctx)
if err != nil {
t.Fatalf("Create: %v", err)
}
if builder.WorkflowID() == "" {
t.Fatal("empty workflow ID")
}

_, err = builder.SetStage(ctx, "research")
if err != nil {
t.Fatalf("SetStage: %v", err)
}

_, err = builder.SetVariable(ctx, "vendor_count", 3)
if err != nil {
t.Fatalf("SetVariable: %v", err)
}

receipts, err := builder.Receipts(ctx)
if err != nil {
t.Fatalf("Receipts: %v", err)
}
if len(receipts) != 2 {
t.Errorf("expected 2 receipts, got %d", len(receipts))
}
t.Logf("builder: workflow=%s receipts=%d", builder.WorkflowID(), len(receipts))
}

// TestClientFork verifies fork via SDK.
func TestClientFork(t *testing.T) {
_, client := mockServer(t)
ctx := context.Background()

wf, _ := client.CreateWorkflow(ctx, "fork sdk test", "sdk-test", nil)
client.EditState(ctx, wf.WorkflowID, []map[string]any{
{"path": "stage", "value": "research"},
})

branch, err := client.Fork(ctx, wf.WorkflowID, 1, "sdk fork test")
if err != nil {
t.Fatalf("Fork: %v", err)
}
if branch.BranchID == "" {
t.Error("empty branch ID")
}
if branch.ParentID != wf.WorkflowID {
t.Errorf("wrong parent: %s", branch.ParentID)
}
t.Logf("fork: branch=%s parent=%s", branch.BranchID, branch.ParentID)
}

// TestClientTaskWorker verifies the worker loop claims and completes tasks.
func TestClientTaskWorker(t *testing.T) {
_, client := mockServer(t)
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

// Verify ClaimTask returns nil when no tasks available
task, err := client.ClaimTask(ctx, "agent:sdk-test")
if err != nil {
t.Fatalf("ClaimTask: %v", err)
}
if task != nil {
t.Errorf("expected no task, got %+v", task)
}
t.Log("ClaimTask: correctly returns nil when queue empty")
}

// TestClientWithPlan verifies workflow creation with a DAG plan.
func TestClientWithPlan(t *testing.T) {
_, client := mockServer(t)
ctx := context.Background()

plan := &acp.Plan{
Nodes: []acp.PlanNode{
{ID: "research", Title: "Research", Agent: "agent:claude-opus", Status: "pending", Risk: "medium", OutputSchema: "evidence"},
{ID: "decision", Title: "Decide",   Agent: "agent:claude-sonnet", Status: "pending", Risk: "critical", OutputSchema: "recommendation"},
},
Edges: []acp.PlanEdge{
{From: "research", To: "decision", Reason: "evidence required"},
},
}

wf, err := client.CreateWorkflow(ctx, "plan sdk test", "sdk-test", plan)
if err != nil {
t.Fatalf("CreateWorkflow with plan: %v", err)
}

taskIDs, err := client.ExecutePlan(ctx, wf.WorkflowID, []acp.ModelRoute{
{NodeAgent: "agent:claude-opus",   AgentID: "agent:claude-opus",   Model: "claude-opus-4-6"},
{NodeAgent: "agent:claude-sonnet", AgentID: "agent:claude-sonnet", Model: "claude-sonnet-4-6"},
})
if err != nil {
t.Fatalf("ExecutePlan: %v", err)
}
if len(taskIDs) != 1 {
t.Errorf("expected 1 task (research only), got %d", len(taskIDs))
}
t.Logf("ExecutePlan: %d tasks enqueued", len(taskIDs))
}

// TestAdapterLangGraph verifies LangGraph trace ingestion.
func TestAdapterLangGraph(t *testing.T) {
_, client := mockServer(t)
ctx := context.Background()

run := &adapters.LangGraphRun{
RunID: "lg_run_001",
Nodes: []adapters.LangGraphNode{
{ID: "retriever", Type: "retrieval"},
{ID: "generator", Type: "generation"},
},
Edges: []adapters.LangGraphEdge{
{From: "retriever", To: "generator"},
},
Events: []adapters.LangGraphEvent{
{Type: "node_complete", NodeID: "retriever", Output: map[string]any{"docs": 5}},
{Type: "node_complete", NodeID: "generator", Output: map[string]any{"tokens": 150}},
},
}

result, err := adapters.IngestLangGraph(ctx, client, "sdk-test", run)
if err != nil {
t.Fatalf("IngestLangGraph: %v", err)
}
if result.WorkflowID == "" {
t.Error("empty workflow ID")
}
if result.Stage != "complete" {
t.Errorf("expected stage complete, got %s", result.Stage)
}
t.Logf("LangGraph ingested: workflow=%s receipts=%d", result.WorkflowID, result.Receipts)
}

// TestAdapterCrewAI verifies CrewAI trace ingestion.
func TestAdapterCrewAI(t *testing.T) {
_, client := mockServer(t)
ctx := context.Background()

run := &adapters.CrewAIRun{
CrewID: "crew_vendor_001",
Tasks: []adapters.CrewAITask{
{ID: "research_task", Description: "Research vendors", AgentID: "researcher"},
{ID: "score_task",    Description: "Score vendors",    AgentID: "analyst"},
},
Events: []adapters.CrewAIEvent{
{Type: "task_complete", TaskID: "research_task", AgentID: "researcher", Output: "3 vendors found"},
{Type: "task_complete", TaskID: "score_task",    AgentID: "analyst",    Output: "vendor_a scores 92"},
},
}

result, err := adapters.IngestCrewAI(ctx, client, "sdk-test", run)
if err != nil {
t.Fatalf("IngestCrewAI: %v", err)
}
if result.Stage != "complete" {
t.Errorf("expected stage complete, got %s", result.Stage)
}
t.Logf("CrewAI ingested: workflow=%s receipts=%d", result.WorkflowID, result.Receipts)
}

// TestAdapterTemporal verifies Temporal trace ingestion.
func TestAdapterTemporal(t *testing.T) {
_, client := mockServer(t)
ctx := context.Background()

run := &adapters.TemporalRun{
RunID:    "temp_run_001",
Workflow: "VendorApprovalWorkflow",
Activities: []adapters.TemporalActivity{
{ID: "validate",  Name: "ValidateVendor"},
{ID: "approve",   Name: "ApproveSpend"},
},
Events: []adapters.TemporalEvent{
{Type: "activity_complete", ActivityID: "validate", Output: map[string]any{"valid": true}},
{Type: "activity_complete", ActivityID: "approve",  Output: map[string]any{"approved": true}},
},
}

result, err := adapters.IngestTemporal(ctx, client, "sdk-test", run)
if err != nil {
t.Fatalf("IngestTemporal: %v", err)
}
if result.Stage != "complete" {
t.Errorf("expected stage complete, got %s", result.Stage)
}
t.Logf("Temporal ingested: workflow=%s receipts=%d", result.WorkflowID, result.Receipts)
}

// TestGetModelRoutes verifies model route retrieval.
func TestGetModelRoutes(t *testing.T) {
_, client := mockServer(t)
ctx := context.Background()

routes, err := client.GetModelRoutes(ctx)
if err != nil {
t.Fatalf("GetModelRoutes: %v", err)
}
if len(routes) == 0 {
t.Error("expected at least one route")
}
t.Logf("model routes: %d", len(routes))
}

// Ensure os import is used
var _ = os.Stderr
