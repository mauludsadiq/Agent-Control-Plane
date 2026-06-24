package api

import (
"encoding/json"
"net/http"

"github.com/mauludsadiq/agent-control-plane/acpd/internal/auth"
"github.com/mauludsadiq/agent-control-plane/acpd/internal/store"
)

func (h *Handlers) resumeGate(w http.ResponseWriter, r *http.Request, workflowID, token string) {
if r.Method != http.MethodPost {
writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
return
}
actor := auth.ActorFromContext(r.Context())

var req struct {
Resolution string `json:"resolution"` // "approved" | "rejected"
}
if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
writeErr(w, http.StatusBadRequest, "invalid body")
return
}
if req.Resolution != "approved" && req.Resolution != "rejected" {
writeErr(w, http.StatusBadRequest, "resolution must be approved or rejected")
return
}

gate, err := h.db.GetGate(token)
if err != nil || gate == nil {
writeErr(w, http.StatusNotFound, "gate not found")
return
}
if gate.WorkflowID != workflowID {
writeErr(w, http.StatusForbidden, "gate does not belong to this workflow")
return
}
if gate.Status != "pending" {
writeErr(w, http.StatusConflict, "gate already resolved")
return
}

snap, _ := h.db.GetLatestSnapshot(workflowID)
if snap == nil {
writeErr(w, http.StatusNotFound, "workflow state not found")
return
}

var result struct {
OK            bool   `json:"ok"`
StateJSON     string `json:"state_json"`
StateHash     string `json:"state_hash"`
ReceiptJSON   string `json:"receipt_json"`
ReceiptDigest string `json:"receipt_digest"`
Error         string `json:"error"`
Seq           int    `json:"seq"`
}
err = h.br.RunAndUnmarshal("gate_resume.fard", map[string]any{
"state_json":   snap.StateJSON,
"actor":        actor,
"token":        token,
"resolution":   req.Resolution,
"tool_version": "human",
}, &result)
if err != nil {
writeErr(w, http.StatusInternalServerError, err.Error())
return
}

if commitErr := h.db.CommitGateResolution(
workflowID, token, req.Resolution, actor.ActorID,
&store.TransitionResult{
OK:            result.OK,
StateJSON:     result.StateJSON,
StateHash:     result.StateHash,
ReceiptJSON:   result.ReceiptJSON,
ReceiptDigest: result.ReceiptDigest,
Seq:           result.Seq,
},
h.snapshotEvery,
); commitErr != nil {
writeErr(w, http.StatusInternalServerError, "commit gate failed: "+commitErr.Error())
return
}

writeJSON(w, http.StatusOK, map[string]any{
"ok":         result.OK,
"state_hash": result.StateHash,
"seq":        result.Seq,
"error":      result.Error,
})
}

func (h *Handlers) getInbox(w http.ResponseWriter, r *http.Request) {
gates, _ := h.db.ListPendingGates()
tasks, _ := h.db.ListTasks("", "pending")
writeJSON(w, http.StatusOK, map[string]any{
"ok":            true,
"pending_gates": gates,
"pending_tasks": tasks,
})
}
