package api

import (
"encoding/json"
"net/http"

"github.com/mauludsadiq/agent-control-plane/acpd/internal/auth"
"github.com/mauludsadiq/agent-control-plane/acpd/internal/store"
)

func (h *Handlers) claimNextTask(w http.ResponseWriter, r *http.Request) {
agent := r.URL.Query().Get("agent")
if agent == "" {
writeErr(w, http.StatusBadRequest, "agent query param required")
return
}
task, err := h.db.ClaimNextTask(agent)
if err != nil {
writeErr(w, http.StatusInternalServerError, err.Error())
return
}
if task == nil {
writeJSON(w, http.StatusOK, map[string]any{"ok": true, "task": nil})
return
}
writeJSON(w, http.StatusOK, map[string]any{"ok": true, "task": task})
}

func (h *Handlers) getTask(w http.ResponseWriter, r *http.Request, taskID string) {
task, err := h.db.GetTask(taskID)
if err != nil || task == nil {
writeErr(w, http.StatusNotFound, "task not found")
return
}
writeJSON(w, http.StatusOK, map[string]any{"ok": true, "task": task})
}

func (h *Handlers) listWorkflowTasks(w http.ResponseWriter, r *http.Request, workflowID string) {
status := r.URL.Query().Get("status")
tasks, err := h.db.ListTasks(workflowID, status)
if err != nil {
writeErr(w, http.StatusInternalServerError, err.Error())
return
}
writeJSON(w, http.StatusOK, map[string]any{"ok": true, "tasks": tasks})
}

func (h *Handlers) completeTask(w http.ResponseWriter, r *http.Request, taskID string) {
if r.Method != http.MethodPost {
writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
return
}
actor := auth.ActorFromContext(r.Context())
_ = actor

var output json.RawMessage
if err := json.NewDecoder(r.Body).Decode(&output); err != nil {
writeErr(w, http.StatusBadRequest, "invalid body")
return
}

task, err := h.db.GetTask(taskID)
if err != nil || task == nil {
writeErr(w, http.StatusNotFound, "task not found")
return
}
if task.Status != "claimed" {
writeErr(w, http.StatusConflict, "task not in claimed state")
return
}

// Validate output and commit through policy via FARD bridge
snap, _ := h.db.GetLatestSnapshot(task.WorkflowID)
if snap == nil {
writeErr(w, http.StatusInternalServerError, "workflow state not found")
return
}

var result struct {
OK            bool   `json:"ok"`
StateJSON     string `json:"state_json"`
StateHash     string `json:"state_hash"`
ReceiptJSON   string `json:"receipt_json"`
ReceiptDigest string `json:"receipt_digest"`
PolicyOK      bool   `json:"policy_ok"`
Violations    any    `json:"violations"`
Seq           int    `json:"seq"`
}
err = h.br.RunAndUnmarshal("commit_artifact.fard", map[string]any{
"state_json":    snap.StateJSON,
"actor":         actor,
"artifact_json": string(output),
"tool_version":  "tool-gateway-1.0.0",
}, &result)
if err != nil {
writeErr(w, http.StatusInternalServerError, err.Error())
return
}

outputStr, _ := json.Marshal(output)
policyStr, _ := json.Marshal(map[string]any{"ok": result.PolicyOK, "violations": result.Violations})

// TODO: persist state update in transaction
_ = store.Task{}

writeJSON(w, http.StatusOK, map[string]any{
"ok":          result.OK,
"policy_ok":   result.PolicyOK,
"state_hash":  result.StateHash,
"seq":         result.Seq,
"violations":  result.Violations,
"output_size": len(outputStr),
"policy_size": len(policyStr),
})
}

func (h *Handlers) heartbeatTask(w http.ResponseWriter, r *http.Request, taskID string) {
if r.Method != http.MethodPost {
writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
return
}
actor := auth.ActorFromContext(r.Context())
if actor == nil {
writeErr(w, http.StatusUnauthorized, "unauthorized")
return
}
if err := h.db.HeartbeatTask(taskID, actor.ActorID); err != nil {
writeErr(w, http.StatusInternalServerError, err.Error())
return
}
writeJSON(w, http.StatusOK, map[string]any{"ok": true, "task_id": taskID})
}

func (h *Handlers) failTask(w http.ResponseWriter, r *http.Request, taskID string) {
if r.Method != http.MethodPost {
writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
return
}
var req struct {
Reason string `json:"reason"`
}
_ = json.NewDecoder(r.Body).Decode(&req)
writeJSON(w, http.StatusOK, map[string]any{"ok": true, "task_id": taskID, "status": "failed"})
}
