package api

import (
"encoding/json"
"net/http"
"time"

"github.com/google/uuid"
"github.com/mauludsadiq/agent-control-plane/acpd/internal/auth"
"github.com/mauludsadiq/agent-control-plane/acpd/internal/store"
)

type createWorkflowReq struct {
WorkflowID string `json:"workflow_id"`
Goal       string `json:"goal"`
Owner      string `json:"owner"`
}

type transitionInput struct {
StateJSON   string          `json:"state_json"`
Actor       auth.ActorRecord `json:"actor"`
Kind        string          `json:"kind"`
Patches     []patch         `json:"patches"`
Input       json.RawMessage `json:"input,omitempty"`
ToolVersion string          `json:"tool_version"`
}

type patch struct {
Path  string `json:"path"`
Value any    `json:"value"`
}

func (h *Handlers) createWorkflow(w http.ResponseWriter, r *http.Request) {
actor := auth.ActorFromContext(r.Context())
if actor == nil {
writeErr(w, http.StatusUnauthorized, "unauthorized")
return
}

var req createWorkflowReq
if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
writeErr(w, http.StatusBadRequest, "invalid request body")
return
}
if req.WorkflowID == "" {
req.WorkflowID = "wf_" + uuid.New().String()[:8]
}
if req.Goal == "" || req.Owner == "" {
writeErr(w, http.StatusBadRequest, "goal and owner required")
return
}

// Create initial state via FARD bridge
var result struct {
OK        bool            `json:"ok"`
StateJSON string          `json:"state_json"`
StateHash string          `json:"state_hash"`
ReceiptJSON string        `json:"receipt_json"`
ReceiptDigest string      `json:"receipt_digest"`
Seq       int             `json:"seq"`
}
err := h.br.RunAndUnmarshal("create_workflow.fard", map[string]any{
"workflow_id": req.WorkflowID,
"goal":        req.Goal,
"owner":       req.Owner,
}, &result)
if err != nil {
writeErr(w, http.StatusInternalServerError, "fard bridge error: "+err.Error())
return
}

wf := &store.Workflow{
WorkflowID:  req.WorkflowID,
Goal:        req.Goal,
Owner:       req.Owner,
Stage:       "created",
StateHash:   result.StateHash,
CurrentSeq:  0,
SnapshotSeq: 0,
CreatedAt:   time.Now(),
UpdatedAt:   time.Now(),
}

if err := h.db.CreateWorkflow(wf); err != nil {
writeErr(w, http.StatusConflict, "workflow already exists or db error: "+err.Error())
return
}

writeJSON(w, http.StatusOK, map[string]any{
"ok":          true,
"workflow_id": req.WorkflowID,
"state_hash":  result.StateHash,
"seq":         0,
})
}

func (h *Handlers) getWorkflow(w http.ResponseWriter, r *http.Request, id string) {
wf, err := h.db.GetWorkflow(id)
if err != nil {
writeErr(w, http.StatusInternalServerError, err.Error())
return
}
if wf == nil {
writeErr(w, http.StatusNotFound, "workflow not found")
return
}
writeJSON(w, http.StatusOK, map[string]any{
"ok":          true,
"workflow_id": wf.WorkflowID,
"goal":        wf.Goal,
"owner":       wf.Owner,
"stage":       wf.Stage,
"state_hash":  wf.StateHash,
"seq":         wf.CurrentSeq,
"created_at":  wf.CreatedAt,
"updated_at":  wf.UpdatedAt,
})
}

func (h *Handlers) listWorkflows(w http.ResponseWriter, r *http.Request) {
stage := r.URL.Query().Get("stage")
wfs, err := h.db.ListWorkflows(stage, 100)
if err != nil {
writeErr(w, http.StatusInternalServerError, err.Error())
return
}
writeJSON(w, http.StatusOK, map[string]any{"ok": true, "workflows": wfs})
}

func (h *Handlers) editState(w http.ResponseWriter, r *http.Request, id string) {
if r.Method != http.MethodPost {
writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
return
}
actor := auth.ActorFromContext(r.Context())

var req struct {
Patches []patch `json:"patches"`
}
if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
writeErr(w, http.StatusBadRequest, "invalid body")
return
}

snap, err := h.db.GetLatestSnapshot(id)
if err != nil || snap == nil {
writeErr(w, http.StatusNotFound, "workflow state not found")
return
}

var result struct {
OK            bool   `json:"ok"`
StateJSON     string `json:"state_json"`
StateHash     string `json:"state_hash"`
ReceiptJSON   string `json:"receipt_json"`
ReceiptDigest string `json:"receipt_digest"`
Seq           int    `json:"seq"`
PolicyOK      bool   `json:"policy_ok"`
Violations    any    `json:"violations"`
}
err = h.br.RunAndUnmarshal("transition.fard", map[string]any{
"state_json":   snap.StateJSON,
"actor":        actor,
"kind":         "human_state_edit",
"patches":      req.Patches,
"tool_version": "human",
}, &result)
if err != nil {
writeErr(w, http.StatusInternalServerError, err.Error())
return
}

if result.OK {
patches_any := make([]map[string]any, len(req.Patches))
for i, p := range req.Patches {
patches_any[i] = map[string]any{"path": p.Path, "value": p.Value}
}
if commitErr := h.db.CommitTransition(&store.CommitParams{
WorkflowID:    id,
Result:        &store.TransitionResult{
OK:            result.OK,
StateJSON:     result.StateJSON,
StateHash:     result.StateHash,
ReceiptJSON:   result.ReceiptJSON,
ReceiptDigest: result.ReceiptDigest,
PolicyOK:      result.PolicyOK,
Seq:           result.Seq,
},
Patches:       patches_any,
Kind:          "human_state_edit",
ActorID:       actor.ActorID,
SnapshotEvery: h.snapshotEvery,
}); commitErr != nil {
writeErr(w, http.StatusInternalServerError, "commit failed: "+commitErr.Error())
return
}
}

writeJSON(w, http.StatusOK, map[string]any{
"ok":         result.OK,
"state_hash": result.StateHash,
"seq":        result.Seq,
"policy_ok":  result.PolicyOK,
"violations": result.Violations,
})
}

func (h *Handlers) getDashboard(w http.ResponseWriter, r *http.Request, id string) {
wf, err := h.db.GetWorkflow(id)
if err != nil || wf == nil {
writeErr(w, http.StatusNotFound, "workflow not found")
return
}
snap, err := h.db.GetLatestSnapshot(id)
if err != nil || snap == nil {
writeErr(w, http.StatusNotFound, "workflow state not found")
return
}
arts, _ := h.db.ListArtifacts(id)
artJSONs := make([]string, 0, len(arts))
for _, a := range arts {
artJSONs = append(artJSONs, a.ArtifactJSON)
}
var dash any
_ = h.br.RunAndUnmarshal("dashboard.fard", map[string]any{
"state_json":     snap.StateJSON,
"artifact_jsons": artJSONs,
}, &dash)
writeJSON(w, http.StatusOK, map[string]any{"ok": true, "dashboard": dash})
}

func (h *Handlers) getReceipts(w http.ResponseWriter, r *http.Request, id string) {
receipts, err := h.db.GetReceipts(id)
if err != nil {
writeErr(w, http.StatusInternalServerError, err.Error())
return
}
var result struct {
ChainRoot     string `json:"chain_root"`
ChainVerified bool   `json:"chain_verified"`
}
_ = h.br.RunAndUnmarshal("verify_chain.fard", map[string]any{
"receipts": receipts,
}, &result)
writeJSON(w, http.StatusOK, map[string]any{
"ok":             true,
"receipts":       receipts,
"chain_root":     result.ChainRoot,
"chain_verified": result.ChainVerified,
})
}

func (h *Handlers) replayWorkflow(w http.ResponseWriter, r *http.Request, id string) {
if r.Method != http.MethodPost {
writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
return
}
var req struct {
Events      []any  `json:"events"`
ToolVersion string `json:"tool_version"`
}
if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
writeErr(w, http.StatusBadRequest, "invalid body")
return
}
actor := auth.ActorFromContext(r.Context())
snap, err := h.db.GetSnapshotAtSeq(id, 0)
if err != nil || snap == nil {
writeErr(w, http.StatusNotFound, "initial state not found")
return
}
var result any
_ = h.br.RunAndUnmarshal("replay.fard", map[string]any{
"initial_state_json": snap.StateJSON,
"actor":              actor,
"events":             req.Events,
"tool_version":       req.ToolVersion,
}, &result)
writeJSON(w, http.StatusOK, map[string]any{"ok": true, "replay": result})
}

func (h *Handlers) commitArtifact(w http.ResponseWriter, r *http.Request, id string) {
if r.Method != http.MethodPost {
writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
return
}
var artifactReq json.RawMessage
if err := json.NewDecoder(r.Body).Decode(&artifactReq); err != nil {
writeErr(w, http.StatusBadRequest, "invalid body")
return
}
actor := auth.ActorFromContext(r.Context())
snap, _ := h.db.GetLatestSnapshot(id)
if snap == nil {
writeErr(w, http.StatusNotFound, "workflow not found")
return
}
var result struct {
OK            bool   `json:"ok"`
Digest        string `json:"digest"`
StateJSON     string `json:"state_json"`
StateHash     string `json:"state_hash"`
ReceiptJSON   string `json:"receipt_json"`
ReceiptDigest string `json:"receipt_digest"`
Seq           int    `json:"seq"`
}
_ = h.br.RunAndUnmarshal("commit_artifact.fard", map[string]any{
"state_json":    snap.StateJSON,
"actor":         actor,
"artifact_json": string(artifactReq),
"tool_version":  "tool-gateway-1.0.0",
}, &result)

if result.OK {
if commitErr := h.db.CommitArtifact(&store.ArtifactCommitParams{
WorkflowID:     id,
ArtifactJSON:   string(artifactReq),
ArtifactDigest: result.Digest,
Result: &store.TransitionResult{
OK:            result.OK,
StateJSON:     result.StateJSON,
StateHash:     result.StateHash,
ReceiptJSON:   result.ReceiptJSON,
ReceiptDigest: result.ReceiptDigest,
Seq:           result.Seq,
},
ActorID:       actor.ActorID,
SnapshotEvery: h.snapshotEvery,
}); commitErr != nil {
writeErr(w, http.StatusInternalServerError, "commit artifact failed: "+commitErr.Error())
return
}
}

writeJSON(w, http.StatusOK, map[string]any{
"ok":         result.OK,
"digest":     result.Digest,
"state_hash": result.StateHash,
"seq":        result.Seq,
})
}
