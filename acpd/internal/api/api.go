package api

import (
"encoding/json"
"net/http"
"strings"

"github.com/mauludsadiq/agent-control-plane/acpd/internal/bridge"
"github.com/mauludsadiq/agent-control-plane/acpd/internal/queue"
"github.com/mauludsadiq/agent-control-plane/acpd/internal/store"
)

type Handlers struct {
db            *store.DB
br            *bridge.Bridge
q             *queue.Queue
snapshotEvery int
}

func New(db *store.DB, br *bridge.Bridge, q *queue.Queue, snapshotEvery int) *Handlers {
return &Handlers{db: db, br: br, q: q, snapshotEvery: snapshotEvery}
}

func (h *Handlers) Register(mux *http.ServeMux) {
mux.HandleFunc("/workflows", h.handleWorkflows)
mux.HandleFunc("/workflows/", h.handleWorkflow)
mux.HandleFunc("/tasks/next", h.handleTaskNext)
mux.HandleFunc("/tasks/", h.handleTask)
mux.HandleFunc("/operator/inbox", h.handleInbox)
mux.HandleFunc("/operator/dead-letter", h.handleDeadLetter)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
w.Header().Set("Content-Type", "application/json")
w.WriteHeader(code)
_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
writeJSON(w, code, map[string]any{"ok": false, "error": msg})
}

func pathSegment(r *http.Request, after string) (string, string) {
path := strings.TrimPrefix(r.URL.Path, after)
parts := strings.SplitN(strings.Trim(path, "/"), "/", 2)
if len(parts) == 0 {
return "", ""
}
if len(parts) == 1 {
return parts[0], ""
}
return parts[0], parts[1]
}

func (h *Handlers) handleWorkflows(w http.ResponseWriter, r *http.Request) {
switch r.Method {
case http.MethodGet:
h.listWorkflows(w, r)
case http.MethodPost:
h.createWorkflow(w, r)
default:
writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
}
}

func (h *Handlers) handleWorkflow(w http.ResponseWriter, r *http.Request) {
id, sub := pathSegment(r, "/workflows/")
if id == "" {
writeErr(w, http.StatusBadRequest, "missing workflow id")
return
}
switch sub {
case "":
if r.Method == http.MethodGet {
h.getWorkflow(w, r, id)
} else {
writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
}
case "state/edit":
h.editState(w, r, id)
case "artifacts":
h.commitArtifact(w, r, id)
case "dashboard":
h.getDashboard(w, r, id)
case "receipts":
h.getReceipts(w, r, id)
case "replay":
h.replayWorkflow(w, r, id)
case "tasks":
h.listWorkflowTasks(w, r, id)
default:
// Check for gates/:token/resume
if strings.HasPrefix(sub, "gates/") {
parts := strings.Split(sub, "/")
if len(parts) == 3 && parts[2] == "resume" {
h.resumeGate(w, r, id, parts[1])
return
}
}
writeErr(w, http.StatusNotFound, "not found")
}
}

func (h *Handlers) handleTaskNext(w http.ResponseWriter, r *http.Request) {
if r.Method != http.MethodGet {
writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
return
}
h.claimNextTask(w, r)
}

func (h *Handlers) handleTask(w http.ResponseWriter, r *http.Request) {
taskID, sub := pathSegment(r, "/tasks/")
switch sub {
case "complete":
h.completeTask(w, r, taskID)
case "fail":
h.failTask(w, r, taskID)
case "heartbeat":
h.heartbeatTask(w, r, taskID)
default:
if r.Method == http.MethodGet {
h.getTask(w, r, taskID)
} else {
writeErr(w, http.StatusNotFound, "not found")
}
}
}

func (h *Handlers) handleDeadLetter(w http.ResponseWriter, r *http.Request) {
if r.Method != http.MethodGet {
writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
return
}
workflowID := r.URL.Query().Get("workflow_id")
tasks, err := h.db.ListDeadLetterTasks(workflowID)
if err != nil {
writeErr(w, http.StatusInternalServerError, err.Error())
return
}
writeJSON(w, http.StatusOK, map[string]any{"ok": true, "dead_letter_tasks": tasks})
}

func (h *Handlers) handleInbox(w http.ResponseWriter, r *http.Request) {
if r.Method != http.MethodGet {
writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
return
}
h.getInbox(w, r)
}
