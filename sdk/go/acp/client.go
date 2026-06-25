// Package acp provides a Go SDK for the Agent Control Plane API.
package acp

import (
"bytes"
"context"
"encoding/json"
"fmt"
"net/http"
"time"
)

// Client is the ACP API client.
type Client struct {
baseURL    string
apiKey     string
httpClient *http.Client
}

// NewClient creates a new ACP client.
func NewClient(baseURL, apiKey string) *Client {
return &Client{
baseURL: baseURL,
apiKey:  apiKey,
httpClient: &http.Client{Timeout: 30 * time.Second},
}
}

// --- Types ---

type Workflow struct {
WorkflowID string         `json:"workflow_id"`
Goal       string         `json:"goal"`
Owner      string         `json:"owner"`
Stage      string         `json:"stage"`
StateHash  string         `json:"state_hash"`
Plan       *Plan          `json:"plan,omitempty"`
}

type Plan struct {
Nodes []PlanNode `json:"nodes"`
Edges []PlanEdge `json:"edges"`
}

type PlanNode struct {
ID           string `json:"id"`
Title        string `json:"title"`
Agent        string `json:"agent"`
Status       string `json:"status"`
Risk         string `json:"risk"`
OutputSchema string `json:"output_schema"`
}

type PlanEdge struct {
From   string `json:"from"`
To     string `json:"to"`
Reason string `json:"reason,omitempty"`
}

type Task struct {
TaskID     string          `json:"task_id"`
WorkflowID string          `json:"workflow_id"`
NodeID     string          `json:"node_id"`
Agent      string          `json:"agent"`
Status     string          `json:"status"`
InputJSON  json.RawMessage `json:"input_json"`
OutputJSON json.RawMessage `json:"output_json,omitempty"`
AttemptCount int           `json:"attempt_count"`
MaxAttempts  int           `json:"max_attempts"`
PriorityLane string        `json:"priority_lane"`
}

type Receipt struct {
Seq           int    `json:"seq"`
ReceiptDigest string `json:"receipt_digest"`
ReceiptJSON   string `json:"receipt_json"`
}

type Branch struct {
BranchID        string `json:"branch_id"`
ParentID        string `json:"parent_id"`
BranchPointSeq  int    `json:"branch_point_seq"`
BranchPointHash string `json:"branch_point_hash"`
Reason          string `json:"reason"`
Kind            string `json:"kind"`
}

type ModelRoute struct {
NodeAgent string `json:"node_agent"`
AgentID   string `json:"agent_id"`
Model     string `json:"model"`
}

// --- Workflow operations ---

// CreateWorkflow creates a new workflow.
func (c *Client) CreateWorkflow(ctx context.Context, goal, owner string, plan *Plan) (*Workflow, error) {
body := map[string]any{"goal": goal, "owner": owner}
if plan != nil {
body["plan"] = plan
}
var resp struct {
OK         bool   `json:"ok"`
WorkflowID string `json:"workflow_id"`
Stage      string `json:"stage"`
StateHash  string `json:"state_hash"`
}
if err := c.post(ctx, "/workflows", body, &resp); err != nil {
return nil, err
}
return &Workflow{
WorkflowID: resp.WorkflowID,
Goal:       goal,
Owner:      owner,
Stage:      resp.Stage,
StateHash:  resp.StateHash,
Plan:       plan,
}, nil
}

// GetWorkflow retrieves a workflow by ID.
func (c *Client) GetWorkflow(ctx context.Context, workflowID string) (*Workflow, error) {
var wf Workflow
if err := c.get(ctx, "/workflows/"+workflowID, &wf); err != nil {
return nil, err
}
return &wf, nil
}

// ListWorkflows lists workflows, optionally filtered by stage.
func (c *Client) ListWorkflows(ctx context.Context, stage string) ([]*Workflow, error) {
path := "/workflows"
if stage != "" {
path += "?stage=" + stage
}
var resp struct {
Workflows []*Workflow `json:"workflows"`
}
if err := c.get(ctx, path, &resp); err != nil {
return nil, err
}
return resp.Workflows, nil
}

// EditState commits a human state edit to a workflow.
func (c *Client) EditState(ctx context.Context, workflowID string, patches []map[string]any) error {
body := map[string]any{
"patches": patches,
"kind":    "human_state_edit",
}
var resp map[string]any
return c.post(ctx, "/workflows/"+workflowID+"/state/edit", body, &resp)
}

// GetReceipts returns the receipt chain for a workflow.
func (c *Client) GetReceipts(ctx context.Context, workflowID string) ([]*Receipt, error) {
var resp struct {
Receipts []*Receipt `json:"receipts"`
Verified bool       `json:"chain_verified"`
}
if err := c.get(ctx, "/workflows/"+workflowID+"/receipts", &resp); err != nil {
return nil, err
}
return resp.Receipts, nil
}

// Fork forks a workflow at a given seq.
func (c *Client) Fork(ctx context.Context, workflowID string, branchPointSeq int, reason string) (*Branch, error) {
body := map[string]any{
"branch_point_seq": branchPointSeq,
"reason":           reason,
}
var resp struct {
OK              bool   `json:"ok"`
BranchID        string `json:"branch_id"`
NewWorkflowID   string `json:"new_workflow_id"`
ParentID        string `json:"parent_id"`
BranchPointSeq  int    `json:"branch_point_seq"`
BranchPointHash string `json:"branch_point_hash"`
}
if err := c.post(ctx, "/workflows/"+workflowID+"/fork", body, &resp); err != nil {
return nil, err
}
return &Branch{
BranchID:        resp.BranchID,
ParentID:        resp.ParentID,
BranchPointSeq:  resp.BranchPointSeq,
BranchPointHash: resp.BranchPointHash,
Reason:          reason,
Kind:            "fork",
}, nil
}

// ListBranches lists all branches of a workflow.
func (c *Client) ListBranches(ctx context.Context, workflowID string) ([]*Branch, error) {
var resp struct {
Branches []*Branch `json:"branches"`
Count    int       `json:"count"`
}
if err := c.get(ctx, "/workflows/"+workflowID+"/branches", &resp); err != nil {
return nil, err
}
return resp.Branches, nil
}

// ExecutePlan enqueues ready plan nodes as tasks.
func (c *Client) ExecutePlan(ctx context.Context, workflowID string, routes []ModelRoute) ([]string, error) {
body := map[string]any{"routes": routes}
var resp struct {
OK       bool     `json:"ok"`
Enqueued int      `json:"enqueued"`
TaskIDs  []string `json:"task_ids"`
}
if err := c.post(ctx, "/workflows/"+workflowID+"/plan/execute", body, &resp); err != nil {
return nil, err
}
return resp.TaskIDs, nil
}

// MarkNodeDone marks a plan node as done.
func (c *Client) MarkNodeDone(ctx context.Context, workflowID, nodeID string) error {
var resp map[string]any
return c.post(ctx, fmt.Sprintf("/workflows/%s/plan/nodes/%s/done", workflowID, nodeID), nil, &resp)
}

// --- Task operations ---

// ClaimTask claims the next available task for an agent.
// Returns nil, nil if no task is available.
func (c *Client) ClaimTask(ctx context.Context, agentID string) (*Task, error) {
var resp struct {
Task *Task `json:"task"`
}
if err := c.get(ctx, "/tasks/next?agent="+agentID, &resp); err != nil {
return nil, err
}
return resp.Task, nil
}

// Heartbeat resets the expiry clock for a claimed task.
func (c *Client) Heartbeat(ctx context.Context, taskID, agentID string) error {
var resp map[string]any
return c.post(ctx, "/tasks/"+taskID+"/heartbeat", nil, &resp)
}

// CompleteTask submits the output of a completed task.
func (c *Client) CompleteTask(ctx context.Context, taskID, output, policyResult string) error {
body := map[string]any{
"output":        output,
"policy_result": policyResult,
}
var resp map[string]any
return c.post(ctx, "/tasks/"+taskID+"/complete", body, &resp)
}

// FailTask reports a task failure.
func (c *Client) FailTask(ctx context.Context, taskID, reason string) error {
body := map[string]any{"reason": reason}
var resp map[string]any
return c.post(ctx, "/tasks/"+taskID+"/fail", body, &resp)
}

// GetModelRoutes returns the default model routing table.
func (c *Client) GetModelRoutes(ctx context.Context) ([]ModelRoute, error) {
var resp struct {
Routes []ModelRoute `json:"routes"`
}
if err := c.get(ctx, "/model-routes", &resp); err != nil {
return nil, err
}
return resp.Routes, nil
}

// Health checks the server health.
func (c *Client) Health(ctx context.Context) error {
var resp map[string]any
return c.get(ctx, "/health", &resp)
}

// --- HTTP helpers ---

func (c *Client) post(ctx context.Context, path string, body any, dest any) error {
var buf bytes.Buffer
if body != nil {
if err := json.NewEncoder(&buf).Encode(body); err != nil {
return fmt.Errorf("encode request: %w", err)
}
}
req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, &buf)
if err != nil {
return err
}
req.Header.Set("Content-Type", "application/json")
req.Header.Set("Authorization", "Bearer "+c.apiKey)
return c.do(req, dest)
}

func (c *Client) get(ctx context.Context, path string, dest any) error {
req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
if err != nil {
return err
}
req.Header.Set("Authorization", "Bearer "+c.apiKey)
return c.do(req, dest)
}

func (c *Client) do(req *http.Request, dest any) error {
resp, err := c.httpClient.Do(req)
if err != nil {
return fmt.Errorf("request %s %s: %w", req.Method, req.URL.Path, err)
}
defer resp.Body.Close()
if resp.StatusCode >= 400 {
var e struct{ Error string `json:"error"` }
json.NewDecoder(resp.Body).Decode(&e)
return fmt.Errorf("ACP API %s %s: %d %s", req.Method, req.URL.Path, resp.StatusCode, e.Error)
}
if dest != nil {
return json.NewDecoder(resp.Body).Decode(dest)
}
return nil
}
