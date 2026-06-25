// Package adapters provides framework adapters for ingesting
// LangGraph, CrewAI, and Temporal traces into Agent Control Plane.
package adapters

import (
"context"
"encoding/json"
"fmt"

"github.com/mauludsadiq/agent-control-plane/sdk/go/acp"
)

// LangGraphRun represents a LangGraph execution trace.
type LangGraphRun struct {
RunID  string            `json:"run_id"`
Nodes  []LangGraphNode   `json:"nodes"`
Edges  []LangGraphEdge   `json:"edges"`
Events []LangGraphEvent  `json:"events"`
}

type LangGraphNode struct {
ID       string `json:"id"`
Type     string `json:"type"`
Metadata map[string]any `json:"metadata,omitempty"`
}

type LangGraphEdge struct {
From string `json:"from"`
To   string `json:"to"`
}

type LangGraphEvent struct {
Type   string `json:"type"`
NodeID string `json:"node_id"`
Output any    `json:"output,omitempty"`
Tool   string `json:"tool,omitempty"`
}

// CrewAIRun represents a CrewAI execution trace.
type CrewAIRun struct {
CrewID string         `json:"crew_id"`
Tasks  []CrewAITask   `json:"tasks"`
Events []CrewAIEvent  `json:"events"`
}

type CrewAITask struct {
ID          string `json:"id"`
Description string `json:"description"`
AgentID     string `json:"agent_id"`
}

type CrewAIEvent struct {
Type    string `json:"type"`
TaskID  string `json:"task_id"`
AgentID string `json:"agent_id"`
Output  any    `json:"output,omitempty"`
}

// TemporalRun represents a Temporal workflow execution trace.
type TemporalRun struct {
RunID      string            `json:"run_id"`
Workflow   string            `json:"workflow"`
Activities []TemporalActivity `json:"activities"`
Events     []TemporalEvent   `json:"events"`
}

type TemporalActivity struct {
ID   string `json:"id"`
Name string `json:"name"`
}

type TemporalEvent struct {
Type       string `json:"type"`
ActivityID string `json:"activity_id"`
Output     any    `json:"output,omitempty"`
}

// IngestResult is returned after ingesting a framework trace.
type IngestResult struct {
WorkflowID string   `json:"workflow_id"`
Receipts   int      `json:"receipts"`
Artifacts  []string `json:"artifacts"`
Stage      string   `json:"stage"`
}

// IngestLangGraph ingests a LangGraph run into ACP.
// Creates a workflow, maps nodes to plan nodes, and commits events as transitions.
func IngestLangGraph(ctx context.Context, client *acp.Client, owner string, run *LangGraphRun) (*IngestResult, error) {
// Build plan from LangGraph nodes and edges
plan := &acp.Plan{}
for _, n := range run.Nodes {
plan.Nodes = append(plan.Nodes, acp.PlanNode{
ID:     n.ID,
Title:  n.Type,
Agent:  "langgraph:" + n.ID,
Status: "pending",
Risk:   "medium",
})
}
for _, e := range run.Edges {
plan.Edges = append(plan.Edges, acp.PlanEdge{
From:   e.From,
To:     e.To,
Reason: "langgraph edge",
})
}

// Create workflow
wf, err := client.CreateWorkflow(ctx, "LangGraph run: "+run.RunID, owner, plan)
if err != nil {
return nil, fmt.Errorf("create workflow: %w", err)
}

// Advance through events
for i, event := range run.Events {
outputJSON, _ := json.Marshal(event.Output)
patches := []map[string]any{
{"path": "var.lg_event_" + fmt.Sprintf("%d", i), "value": map[string]any{
"type":    event.Type,
"node_id": event.NodeID,
"output":  string(outputJSON),
}},
}
if err := client.EditState(ctx, wf.WorkflowID, patches); err != nil {
return nil, fmt.Errorf("ingest event %d: %w", i, err)
}
}

// Mark complete
if err := client.EditState(ctx, wf.WorkflowID, []map[string]any{
{"path": "stage", "value": "complete"},
}); err != nil {
return nil, fmt.Errorf("mark complete: %w", err)
}

receipts, _ := client.GetReceipts(ctx, wf.WorkflowID)
return &IngestResult{
WorkflowID: wf.WorkflowID,
Receipts:   len(receipts),
Stage:      "complete",
}, nil
}

// IngestCrewAI ingests a CrewAI run into ACP.
func IngestCrewAI(ctx context.Context, client *acp.Client, owner string, run *CrewAIRun) (*IngestResult, error) {
// Build plan from CrewAI tasks
plan := &acp.Plan{}
for _, t := range run.Tasks {
plan.Nodes = append(plan.Nodes, acp.PlanNode{
ID:     t.ID,
Title:  t.Description,
Agent:  "crewai:" + t.AgentID,
Status: "pending",
Risk:   "medium",
})
}

wf, err := client.CreateWorkflow(ctx, "CrewAI run: "+run.CrewID, owner, plan)
if err != nil {
return nil, fmt.Errorf("create workflow: %w", err)
}

for i, event := range run.Events {
outputJSON, _ := json.Marshal(event.Output)
patches := []map[string]any{
{"path": "var.crew_event_" + fmt.Sprintf("%d", i), "value": map[string]any{
"type":     event.Type,
"task_id":  event.TaskID,
"agent_id": event.AgentID,
"output":   string(outputJSON),
}},
}
if err := client.EditState(ctx, wf.WorkflowID, patches); err != nil {
return nil, fmt.Errorf("ingest event %d: %w", i, err)
}
}

if err := client.EditState(ctx, wf.WorkflowID, []map[string]any{
{"path": "stage", "value": "complete"},
}); err != nil {
return nil, fmt.Errorf("mark complete: %w", err)
}

receipts, _ := client.GetReceipts(ctx, wf.WorkflowID)
return &IngestResult{
WorkflowID: wf.WorkflowID,
Receipts:   len(receipts),
Stage:      "complete",
}, nil
}

// IngestTemporal ingests a Temporal workflow run into ACP.
func IngestTemporal(ctx context.Context, client *acp.Client, owner string, run *TemporalRun) (*IngestResult, error) {
plan := &acp.Plan{}
for _, a := range run.Activities {
plan.Nodes = append(plan.Nodes, acp.PlanNode{
ID:     a.ID,
Title:  a.Name,
Agent:  "temporal:" + a.ID,
Status: "pending",
Risk:   "medium",
})
}

wf, err := client.CreateWorkflow(ctx, "Temporal run: "+run.RunID+" ("+run.Workflow+")", owner, plan)
if err != nil {
return nil, fmt.Errorf("create workflow: %w", err)
}

for i, event := range run.Events {
outputJSON, _ := json.Marshal(event.Output)
patches := []map[string]any{
{"path": "var.temporal_event_" + fmt.Sprintf("%d", i), "value": map[string]any{
"type":        event.Type,
"activity_id": event.ActivityID,
"output":      string(outputJSON),
}},
}
if err := client.EditState(ctx, wf.WorkflowID, patches); err != nil {
return nil, fmt.Errorf("ingest event %d: %w", i, err)
}
}

if err := client.EditState(ctx, wf.WorkflowID, []map[string]any{
{"path": "stage", "value": "complete"},
}); err != nil {
return nil, fmt.Errorf("mark complete: %w", err)
}

receipts, _ := client.GetReceipts(ctx, wf.WorkflowID)
return &IngestResult{
WorkflowID: wf.WorkflowID,
Receipts:   len(receipts),
Stage:      "complete",
}, nil
}
