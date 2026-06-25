package api

import (
"encoding/json"
"fmt"
"net/http"

"github.com/mauludsadiq/agent-control-plane/acpd/internal/auth"
"github.com/mauludsadiq/agent-control-plane/acpd/internal/store"
"github.com/google/uuid"
)

// forkWorkflow handles POST /workflows/:id/fork
// Forks a workflow at a given seq, creating a new independent workflow.
func (h *Handlers) forkWorkflow(w http.ResponseWriter, r *http.Request, workflowID string) {
if r.Method != http.MethodPost {
writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
return
}
actor := auth.ActorFromContext(r.Context())
if actor == nil {
writeErr(w, http.StatusUnauthorized, "unauthorized")
return
}

var req struct {
BranchPointSeq int    `json:"branch_point_seq"`
Reason         string `json:"reason"`
NewWorkflowID  string `json:"new_workflow_id"`
}
if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
writeErr(w, http.StatusBadRequest, "invalid request body")
return
}
if req.Reason == "" {
req.Reason = "fork"
}
if req.NewWorkflowID == "" {
req.NewWorkflowID = "wf_fork_" + uuid.New().String()[:8]
}

// Get parent snapshot to pass to FARD
snap, err := h.db.GetLatestSnapshot(workflowID)
if err != nil || snap == nil {
writeErr(w, http.StatusNotFound, "parent workflow not found")
return
}
var parentState map[string]any
if err := json.Unmarshal([]byte(snap.StateJSON), &parentState); err != nil {
writeErr(w, http.StatusInternalServerError, "parse parent state: "+err.Error())
return
}

// If branch_point_seq not specified, use current seq
if req.BranchPointSeq == 0 {
if seq, ok := parentState["seq"].(float64); ok {
req.BranchPointSeq = int(seq)
}
}

// Call FARD fork bridge
var forkResult struct {
OK        bool           `json:"ok"`
StateJSON string         `json:"state_json"`
StateHash string         `json:"state_hash"`
Branch    map[string]any `json:"branch"`
}
	if err := h.br.RunAndUnmarshal("fork.fard", map[string]any{
"parent_state":     parentState,
"new_workflow_id":  req.NewWorkflowID,
"branch_point_seq": req.BranchPointSeq,
"reason":           req.Reason,
}, &forkResult); err != nil {
writeErr(w, http.StatusInternalServerError, "fork bridge: "+err.Error())
return
}
	if !forkResult.OK {
writeErr(w, http.StatusInternalServerError, "fork failed")
return
}

forkedStateJSON := forkResult.StateJSON
if len(forkedStateJSON) < 5 {
writeErr(w, http.StatusInternalServerError, "fork bridge returned empty state")
return
}
forkedStateHash := forkResult.StateHash
if forkedStateHash == "" {
forkedStateHash = fmt.Sprintf("sha256:fork-%s-seq%d", req.NewWorkflowID, req.BranchPointSeq)
}

// Commit fork
	result, err := h.db.CommitFork(&store.ForkParams{
ParentWorkflowID: workflowID,
NewWorkflowID:    req.NewWorkflowID,
BranchPointSeq:   req.BranchPointSeq,
Reason:           req.Reason,
Kind:             "fork",
}, string(forkedStateJSON), forkedStateHash)
if err != nil {
writeErr(w, http.StatusInternalServerError, "commit fork: "+err.Error())
return
}

	writeJSON(w, http.StatusOK, map[string]any{
"ok":                true,
"branch_id":         result.BranchID,
"new_workflow_id":   result.NewWorkflowID,
"parent_id":         workflowID,
"branch_point_seq":  result.BranchPointSeq,
"branch_point_hash": result.BranchPointHash,
})
}

// listBranches handles GET /workflows/:id/branches
func (h *Handlers) listBranches(w http.ResponseWriter, r *http.Request, workflowID string) {
if r.Method != http.MethodGet {
writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
return
}
branches, err := h.db.ListBranches(workflowID)
if err != nil {
writeErr(w, http.StatusInternalServerError, err.Error())
return
}
	writeJSON(w, http.StatusOK, map[string]any{
"ok":       true,
"branches": branches,
"count":    len(branches),
})
}

// sanitizeForFARD removes null values and fields that cause FARD spread errors.
// FARD v1.7.x cannot spread records containing null JSON values.
func sanitizeForFARD(state map[string]any) {
// Remove lineage — fork creates its own
delete(state, "lineage")
// Replace null values with FARD-safe defaults
for k, v := range state {
if v == nil {
switch k {
case "final_decision":
delete(state, k) // omit — FARD state.fard uses null check
default:
delete(state, k)
}
}
}
}
