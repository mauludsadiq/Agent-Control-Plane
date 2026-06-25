package store

import (
"database/sql"
"encoding/json"
"fmt"
)

// PlanNode mirrors the FARD plan.node record.
type PlanNode struct {
ID           string `json:"id"`
Title        string `json:"title"`
Agent        string `json:"agent"`
Status       string `json:"status"`
Risk         string `json:"risk"`
OutputSchema string `json:"output_schema"`
}

// PlanEdge mirrors the FARD plan.edge record.
type PlanEdge struct {
From   string `json:"from"`
To     string `json:"to"`
Reason string `json:"reason"`
}

// Plan mirrors the FARD plan.graph record.
type Plan struct {
Nodes []PlanNode `json:"nodes"`
Edges []PlanEdge `json:"edges"`
}

// ModelRoute maps a plan node agent name to an actual agent ID.
// e.g. "research_agent" -> "agent:claude-opus-4"
type ModelRoute struct {
NodeAgent string `json:"node_agent"`
AgentID   string `json:"agent_id"`
Model     string `json:"model"`
}

// EnqueuePlanTasks reads the current workflow state, finds ready plan nodes,
// and enqueues a task for each that doesn't already have a pending/claimed task.
// Returns the list of task IDs enqueued.
func (d *DB) EnqueuePlanTasks(workflowID string, routes []ModelRoute) ([]string, error) {
// Get current state
snap, err := d.GetLatestSnapshot(workflowID)
if err != nil || snap == nil {
return nil, fmt.Errorf("get snapshot: %w", err)
}

// Parse state JSON to extract plan
var state struct {
Plan Plan `json:"plan"`
}
if err := json.Unmarshal([]byte(snap.StateJSON), &state); err != nil {
return nil, fmt.Errorf("parse state: %w", err)
}

if len(state.Plan.Nodes) == 0 {
return nil, nil // no plan
}

// Find ready nodes (pending + all blockers done)
ready := readyNodes(state.Plan)
if len(ready) == 0 {
return nil, nil
}

// Build route map
routeMap := map[string]ModelRoute{}
for _, r := range routes {
routeMap[r.NodeAgent] = r
}

var enqueued []string
for _, node := range ready {
// Skip if already has an active task
existing, err := d.getActiveTaskForNode(workflowID, node.ID)
if err != nil {
return nil, err
}
if existing != nil {
continue
}

// Resolve agent
agentID := node.Agent
model := ""
if route, ok := routeMap[node.Agent]; ok {
agentID = route.AgentID
model = route.Model
}

// Build task input
taskInput, _ := json.Marshal(map[string]any{
"workflow_id":   workflowID,
"node_id":       node.ID,
"node_title":    node.Title,
"output_schema": node.OutputSchema,
"model":         model,
"state_seq":     snap.Seq,
})

taskID := fmt.Sprintf("task_%s_%s", workflowID, node.ID)
err = d.Tx(func(tx *sql.Tx) error {
return d.EnqueueTask(tx, &Task{
TaskID:       taskID,
WorkflowID:   workflowID,
NodeID:       node.ID,
Agent:        agentID,
InputJSON:    string(taskInput),
TimeoutSec:   300,
MaxAttempts:  3,
PriorityLane: riskToLane(node.Risk),
})
})
if err != nil {
// Task may already exist — skip
continue
}
enqueued = append(enqueued, taskID)
}
return enqueued, nil
}

// MarkNodeDone marks a plan node as done in the workflow state and
// re-evaluates which nodes are now ready.
func (d *DB) MarkPlanNodeDone(workflowID, nodeID string) error {
snap, err := d.GetLatestSnapshot(workflowID)
if err != nil || snap == nil {
return fmt.Errorf("get snapshot: %w", err)
}

var state map[string]any
if err := json.Unmarshal([]byte(snap.StateJSON), &state); err != nil {
return fmt.Errorf("parse state: %w", err)
}

// Update plan node status
planRaw, _ := state["plan"].(map[string]any)
nodesRaw, _ := planRaw["nodes"].([]any)
for i, n := range nodesRaw {
node, _ := n.(map[string]any)
if node["id"] == nodeID {
node["status"] = "done"
nodesRaw[i] = node
break
}
}
planRaw["nodes"] = nodesRaw
state["plan"] = planRaw

newStateJSON, err := json.Marshal(state)
if err != nil {
return fmt.Errorf("marshal state: %w", err)
}

return d.Tx(func(tx *sql.Tx) error {
return d.SaveSnapshot(tx, &WorkflowSnapshot{
WorkflowID: workflowID,
Seq:        snap.Seq + 1,
StateJSON:  string(newStateJSON),
StateHash:  "sha256:plan-node-" + nodeID + "-done",
})
})
}

func (d *DB) getActiveTaskForNode(workflowID, nodeID string) (*Task, error) {
row := d.queryRow(`
SELECT task_id, workflow_id, node_id, agent, status, priority,
       priority_lane, input_json, output_json, policy_result_json,
       claimed_by, claimed_at, last_heartbeat, timeout_sec,
       attempt_count, max_attempts, dead_lettered,
       completed_at, failed_reason, created_at, updated_at
FROM tasks
WHERE workflow_id=? AND node_id=?
  AND status IN ('pending','claimed')
  AND dead_lettered=0
LIMIT 1`, workflowID, nodeID)
return scanTask(d, row)
}

// readyNodes returns plan nodes that are pending and have all blockers done.
func readyNodes(p Plan) []PlanNode {
// Build done set
done := map[string]bool{}
for _, n := range p.Nodes {
if n.Status == "done" {
done[n.ID] = true
}
}
// Build blocker map: nodeID -> list of from-node IDs
blockers := map[string][]string{}
for _, e := range p.Edges {
blockers[e.To] = append(blockers[e.To], e.From)
}
var ready []PlanNode
for _, n := range p.Nodes {
if n.Status != "pending" {
continue
}
allDone := true
for _, from := range blockers[n.ID] {
if !done[from] {
allDone = false
break
}
}
if allDone {
ready = append(ready, n)
}
}
return ready
}

func riskToLane(risk string) string {
switch risk {
case "critical":
return PriorityLaneCritical
case "high", "medium":
return PriorityLaneNormal
default:
return PriorityLaneBackground
}
}
