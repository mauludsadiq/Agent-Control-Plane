package acp

import (
"context"
"fmt"
)

// WorkflowBuilder provides a fluent API for creating and advancing workflows.
type WorkflowBuilder struct {
client     *Client
workflowID string
goal       string
owner      string
plan       *Plan
}

// NewWorkflow starts building a new workflow.
func (c *Client) NewWorkflow(goal, owner string) *WorkflowBuilder {
return &WorkflowBuilder{client: c, goal: goal, owner: owner}
}

// WithPlan adds a DAG plan to the workflow.
func (b *WorkflowBuilder) WithPlan(plan *Plan) *WorkflowBuilder {
b.plan = plan
return b
}

// Create creates the workflow and returns the builder for chaining.
func (b *WorkflowBuilder) Create(ctx context.Context) (*WorkflowBuilder, error) {
wf, err := b.client.CreateWorkflow(ctx, b.goal, b.owner, b.plan)
if err != nil {
return nil, fmt.Errorf("create workflow: %w", err)
}
b.workflowID = wf.WorkflowID
return b, nil
}

// WorkflowID returns the created workflow ID.
func (b *WorkflowBuilder) WorkflowID() string { return b.workflowID }

// SetStage advances the workflow to a new stage.
func (b *WorkflowBuilder) SetStage(ctx context.Context, stage string) (*WorkflowBuilder, error) {
if err := b.client.EditState(ctx, b.workflowID, []map[string]any{
{"path": "stage", "value": stage},
}); err != nil {
return nil, fmt.Errorf("set stage %s: %w", stage, err)
}
return b, nil
}

// SetVariable sets a workflow variable.
func (b *WorkflowBuilder) SetVariable(ctx context.Context, key string, value any) (*WorkflowBuilder, error) {
if err := b.client.EditState(ctx, b.workflowID, []map[string]any{
{"path": "var." + key, "value": value},
}); err != nil {
return nil, fmt.Errorf("set var %s: %w", key, err)
}
return b, nil
}

// Fork forks the workflow at the given seq.
func (b *WorkflowBuilder) Fork(ctx context.Context, seq int, reason string) (*WorkflowBuilder, error) {
branch, err := b.client.Fork(ctx, b.workflowID, seq, reason)
if err != nil {
return nil, fmt.Errorf("fork at seq %d: %w", seq, err)
}
return &WorkflowBuilder{
client:     b.client,
workflowID: branch.BranchID,
goal:       b.goal + " [fork]",
owner:      b.owner,
}, nil
}

// Receipts returns the receipt chain for this workflow.
func (b *WorkflowBuilder) Receipts(ctx context.Context) ([]*Receipt, error) {
return b.client.GetReceipts(ctx, b.workflowID)
}
