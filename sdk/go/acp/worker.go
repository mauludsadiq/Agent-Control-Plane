package acp

import (
"context"
"log"
"time"
)

// TaskHandler is called when a task is claimed.
// Return an error to fail the task; return nil to complete it with output.
type TaskHandler func(ctx context.Context, task *Task) (output string, err error)

// WorkerConfig configures a TaskWorker.
type WorkerConfig struct {
AgentID           string
PollInterval      time.Duration // default 2s
HeartbeatInterval time.Duration // default 30s
MaxConcurrent     int           // default 1
}

// TaskWorker polls for tasks and processes them.
type TaskWorker struct {
client  *Client
cfg     WorkerConfig
handler TaskHandler
}

// NewTaskWorker creates a new TaskWorker.
func NewTaskWorker(client *Client, cfg WorkerConfig, handler TaskHandler) *TaskWorker {
if cfg.PollInterval == 0 {
cfg.PollInterval = 2 * time.Second
}
if cfg.HeartbeatInterval == 0 {
cfg.HeartbeatInterval = 30 * time.Second
}
if cfg.MaxConcurrent == 0 {
cfg.MaxConcurrent = 1
}
return &TaskWorker{client: client, cfg: cfg, handler: handler}
}

// Run starts the worker loop. Blocks until ctx is cancelled.
func (w *TaskWorker) Run(ctx context.Context) {
sem := make(chan struct{}, w.cfg.MaxConcurrent)
ticker := time.NewTicker(w.cfg.PollInterval)
defer ticker.Stop()

log.Printf("acp worker: started agent=%s poll=%v heartbeat=%v concurrency=%d",
w.cfg.AgentID, w.cfg.PollInterval, w.cfg.HeartbeatInterval, w.cfg.MaxConcurrent)

for {
select {
case <-ctx.Done():
log.Printf("acp worker: stopped agent=%s", w.cfg.AgentID)
return
case <-ticker.C:
select {
case sem <- struct{}{}:
go func() {
defer func() { <-sem }()
w.processOne(ctx)
}()
default:
// All slots busy
}
}
}
}

func (w *TaskWorker) processOne(ctx context.Context) {
task, err := w.client.ClaimTask(ctx, w.cfg.AgentID)
if err != nil {
log.Printf("acp worker: claim error: %v", err)
return
}
if task == nil {
return // no task available
}

log.Printf("acp worker: claimed task=%s workflow=%s node=%s", task.TaskID, task.WorkflowID, task.NodeID)

// Start heartbeat goroutine
hbCtx, hbCancel := context.WithCancel(ctx)
defer hbCancel()
go func() {
ticker := time.NewTicker(w.cfg.HeartbeatInterval)
defer ticker.Stop()
for {
select {
case <-hbCtx.Done():
return
case <-ticker.C:
if err := w.client.Heartbeat(hbCtx, task.TaskID, w.cfg.AgentID); err != nil {
log.Printf("acp worker: heartbeat error task=%s: %v", task.TaskID, err)
}
}
}
}()

// Execute handler
output, err := w.handler(ctx, task)
hbCancel()

if err != nil {
log.Printf("acp worker: task failed task=%s: %v", task.TaskID, err)
if failErr := w.client.FailTask(ctx, task.TaskID, err.Error()); failErr != nil {
log.Printf("acp worker: fail error task=%s: %v", task.TaskID, failErr)
}
return
}

if completeErr := w.client.CompleteTask(ctx, task.TaskID, output, "pass"); completeErr != nil {
log.Printf("acp worker: complete error task=%s: %v", task.TaskID, completeErr)
return
}
log.Printf("acp worker: completed task=%s", task.TaskID)
}
