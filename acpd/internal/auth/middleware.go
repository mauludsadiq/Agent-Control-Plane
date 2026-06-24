package auth

import (
"encoding/json"
"net/http"
"strings"

"github.com/mauludsadiq/agent-control-plane/acpd/internal/store"
)

type ActorKey struct{}

type ActorRecord struct {
ActorID          string   `json:"actor_id"`
Roles            []string `json:"roles"`
AllowedAgents    []string `json:"allowed_agents,omitempty"`
AllowedWorkflows []string `json:"allowed_workflows,omitempty"`
}

func Middleware(db *store.DB) func(http.Handler) http.Handler {
return func(next http.Handler) http.Handler {
return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// Health endpoint is unauthenticated
if r.URL.Path == "/health" {
next.ServeHTTP(w, r)
return
}
key := extractKey(r)
if key == "" {
writeErr(w, http.StatusUnauthorized, "missing Authorization header")
return
}
actor, err := db.ResolveAPIKey(key)
if err != nil {
writeErr(w, http.StatusInternalServerError, "auth lookup failed")
return
}
if actor == nil {
writeErr(w, http.StatusUnauthorized, "invalid API key")
return
}
ar := &ActorRecord{
ActorID:          actor.ActorID,
Roles:            actor.Roles,
AllowedAgents:    actor.AllowedAgents,
AllowedWorkflows: actor.AllowedWorkflows,
}
next.ServeHTTP(w, r.WithContext(
contextWithActor(r.Context(), ar),
))
})
}
}

func extractKey(r *http.Request) string {
h := r.Header.Get("Authorization")
if strings.HasPrefix(h, "Bearer ") {
return strings.TrimPrefix(h, "Bearer ")
}
return ""
}

func writeErr(w http.ResponseWriter, code int, msg string) {
w.Header().Set("Content-Type", "application/json")
w.WriteHeader(code)
_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": msg})
}
