package store

import (
"crypto/sha256"
"database/sql"
"encoding/json"
"fmt"
"time"
)

type Actor struct {
ActorID             string
APIKeyHash          string
Roles               []string
AllowedAgents       []string // nil = no restriction
AllowedWorkflows    []string // nil = no restriction
CreatedAt           time.Time
UpdatedAt           time.Time
}

func HashAPIKey(key string) string {
h := sha256.Sum256([]byte(key))
return fmt.Sprintf("sha256:%x", h)
}

// hashAPIKey uses KeyProvider if set, falls back to plain SHA256.
func (d *DB) hashAPIKey(key string) string {
   if d.keys != nil {
   return d.keys.HashAPIKey(key)
   }
   return HashAPIKey(key)
}

func (d *DB) CreateActor(a *Actor, apiKey string) error {
rolesJSON, _ := json.Marshal(a.Roles)
agentsJSON, _ := json.Marshal(a.AllowedAgents)
workflowsJSON, _ := json.Marshal(a.AllowedWorkflows)
_, err := d.exec(`
INSERT INTO actors (actor_id, api_key_hash, roles_json, allowed_agents_json, allowed_workflows_json, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
a.ActorID, d.hashAPIKey(apiKey),
string(rolesJSON), nullBytes(agentsJSON), nullBytes(workflowsJSON),
now(), now(),
)
return err
}

func (d *DB) ResolveAPIKey(apiKey string) (*Actor, error) {
hash := d.hashAPIKey(apiKey)
row := d.queryRow(`
SELECT actor_id, api_key_hash, roles_json, allowed_agents_json, allowed_workflows_json, created_at, updated_at
FROM actors WHERE api_key_hash = ?`, hash)
var a Actor
var rolesJSON string
var agentsJSON, workflowsJSON sql.NullString
var createdAt, updatedAt string
err := row.Scan(&a.ActorID, &a.APIKeyHash, &rolesJSON, &agentsJSON, &workflowsJSON, &createdAt, &updatedAt)
if err == sql.ErrNoRows {
return nil, nil
}
if err != nil {
return nil, err
}
_ = json.Unmarshal([]byte(rolesJSON), &a.Roles)
if agentsJSON.Valid {
_ = json.Unmarshal([]byte(agentsJSON.String), &a.AllowedAgents)
}
if workflowsJSON.Valid {
_ = json.Unmarshal([]byte(workflowsJSON.String), &a.AllowedWorkflows)
}
a.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
a.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
return &a, nil
}

func nullString(s string) sql.NullString {
if s == "" {
return sql.NullString{}
}
return sql.NullString{String: s, Valid: true}
}

func nullBytes(b []byte) sql.NullString {
if b == nil || string(b) == "null" {
return sql.NullString{}
}
return sql.NullString{String: string(b), Valid: true}
}
