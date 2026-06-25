package acp_test

import (
"encoding/json"
"net/http"
"strings"
"sync"
"testing"

"github.com/google/uuid"
)

// newMockACPMux returns a minimal ACP-compatible HTTP mux for SDK tests.
// It implements enough of the API to test the SDK client without needing acpd.
func newMockACPMux(t *testing.T) *http.ServeMux {
t.Helper()
mux := http.NewServeMux()
store := &mockStore{workflows: map[string]*mockWorkflow{}}

writeJSON := func(w http.ResponseWriter, status int, v any) {
w.Header().Set("Content-Type", "application/json")
w.WriteHeader(status)
json.NewEncoder(w).Encode(v)
}

auth := func(r *http.Request) bool {
return strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ")
}

mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
writeJSON(w, 200, map[string]any{"ok": true})
})

mux.HandleFunc("/workflows", func(w http.ResponseWriter, r *http.Request) {
if !auth(r) { writeJSON(w, 401, map[string]any{"error": "unauthorized"}); return }
if r.Method == http.MethodPost {
var req struct {
Goal  string         `json:"goal"`
Owner string         `json:"owner"`
Plan  map[string]any `json:"plan"`
}
json.NewDecoder(r.Body).Decode(&req)
wfID := "wf_" + uuid.New().String()[:8]
store.mu.Lock()
store.workflows[wfID] = &mockWorkflow{
id: wfID, goal: req.Goal, owner: req.Owner,
stage: "created", seq: 0, plan: req.Plan,
}
store.mu.Unlock()
writeJSON(w, 200, map[string]any{
"ok": true, "workflow_id": wfID,
"stage": "created", "state_hash": "sha256:mock",
})
} else {
store.mu.Lock()
var wfs []map[string]any
for _, wf := range store.workflows {
wfs = append(wfs, map[string]any{"workflow_id": wf.id, "goal": wf.goal, "stage": wf.stage})
}
store.mu.Unlock()
writeJSON(w, 200, map[string]any{"ok": true, "workflows": wfs})
}
})

mux.HandleFunc("/workflows/", func(w http.ResponseWriter, r *http.Request) {
if !auth(r) { writeJSON(w, 401, map[string]any{"error": "unauthorized"}); return }
path := strings.TrimPrefix(r.URL.Path, "/workflows/")
parts := strings.SplitN(path, "/", 2)
wfID := parts[0]
sub := ""
if len(parts) > 1 { sub = parts[1] }

store.mu.Lock()
wf := store.workflows[wfID]
store.mu.Unlock()

switch {
case sub == "" && r.Method == http.MethodGet:
if wf == nil { writeJSON(w, 404, map[string]any{"error": "not found"}); return }
writeJSON(w, 200, map[string]any{"workflow_id": wf.id, "goal": wf.goal, "stage": wf.stage, "owner": wf.owner})

case sub == "state/edit":
if wf == nil { writeJSON(w, 404, map[string]any{"error": "not found"}); return }
var req struct { Patches []map[string]any `json:"patches"` }
json.NewDecoder(r.Body).Decode(&req)
store.mu.Lock()
wf.seq++
for _, p := range req.Patches {
if p["path"] == "stage" { wf.stage = p["value"].(string) }
}
wf.receipts = append(wf.receipts, map[string]any{"seq": wf.seq, "receipt_digest": "sha256:mock-" + string(rune('a'+wf.seq))})
store.mu.Unlock()
writeJSON(w, 200, map[string]any{"ok": true, "seq": wf.seq})

case sub == "receipts":
if wf == nil { writeJSON(w, 404, map[string]any{"error": "not found"}); return }
store.mu.Lock()
receipts := wf.receipts
store.mu.Unlock()
writeJSON(w, 200, map[string]any{"ok": true, "receipts": receipts, "chain_verified": true})

case sub == "fork":
if wf == nil { writeJSON(w, 404, map[string]any{"error": "not found"}); return }
var req struct {
BranchPointSeq int    `json:"branch_point_seq"`
Reason         string `json:"reason"`
}
json.NewDecoder(r.Body).Decode(&req)
branchID := "wf_fork_" + uuid.New().String()[:8]
store.mu.Lock()
store.workflows[branchID] = &mockWorkflow{
id: branchID, goal: wf.goal + " [fork]",
owner: wf.owner, stage: wf.stage, seq: 0,
}
wf.branches = append(wf.branches, branchID)
store.mu.Unlock()
writeJSON(w, 200, map[string]any{
"ok": true, "branch_id": branchID,
"new_workflow_id": branchID, "parent_id": wfID,
"branch_point_seq": req.BranchPointSeq,
"branch_point_hash": "sha256:mock",
})

case sub == "branches":
if wf == nil { writeJSON(w, 404, map[string]any{"error": "not found"}); return }
store.mu.Lock()
var branches []map[string]any
for _, bid := range wf.branches {
branches = append(branches, map[string]any{"branch_id": bid, "parent_id": wfID})
}
store.mu.Unlock()
writeJSON(w, 200, map[string]any{"ok": true, "branches": branches, "count": len(wf.branches)})

case sub == "plan/execute":
if wf == nil { writeJSON(w, 404, map[string]any{"error": "not found"}); return }
// Return 1 task for first ready node
taskID := "task_" + wfID + "_node0"
writeJSON(w, 200, map[string]any{"ok": true, "enqueued": 1, "task_ids": []string{taskID}})

default:
if strings.HasSuffix(sub, "/done") {
writeJSON(w, 200, map[string]any{"ok": true, "status": "done"})
} else {
writeJSON(w, 404, map[string]any{"error": "not found"})
}
}
})

mux.HandleFunc("/tasks/next", func(w http.ResponseWriter, r *http.Request) {
if !auth(r) { writeJSON(w, 401, map[string]any{"error": "unauthorized"}); return }
writeJSON(w, 200, map[string]any{"ok": true, "task": nil})
})

mux.HandleFunc("/tasks/", func(w http.ResponseWriter, r *http.Request) {
if !auth(r) { writeJSON(w, 401, map[string]any{"error": "unauthorized"}); return }
writeJSON(w, 200, map[string]any{"ok": true})
})

mux.HandleFunc("/model-routes", func(w http.ResponseWriter, r *http.Request) {
if !auth(r) { writeJSON(w, 401, map[string]any{"error": "unauthorized"}); return }
writeJSON(w, 200, map[string]any{
"ok": true,
"routes": []map[string]any{
{"node_agent": "research_agent", "agent_id": "agent:claude-opus", "model": "claude-opus-4-6"},
{"node_agent": "finance_agent",  "agent_id": "agent:claude-sonnet", "model": "claude-sonnet-4-6"},
},
})
})

return mux
}

type mockStore struct {
mu        sync.Mutex
workflows map[string]*mockWorkflow
}

type mockWorkflow struct {
id       string
goal     string
owner    string
stage    string
seq      int
plan     map[string]any
receipts []map[string]any
branches []string
}
