package testutil

import (
   "bytes"
   "encoding/json"
   "fmt"
   "net/http"
   "net/http/httptest"
   "os"
   "testing"

   "github.com/mauludsadiq/agent-control-plane/acpd/internal/api"
   "github.com/mauludsadiq/agent-control-plane/acpd/internal/auth"
   "github.com/mauludsadiq/agent-control-plane/acpd/internal/bridge"
   "github.com/mauludsadiq/agent-control-plane/acpd/internal/queue"
   "github.com/mauludsadiq/agent-control-plane/acpd/internal/store"
)

const TestAPIKey = "acp_test_key_operator"
const TestActorID = "operator:test"

// NewTestServer creates an in-memory test server wired to a real SQLite DB,
// real FARD bridge, and real auth middleware. Tests run against the actual
// stack — no mocks.
func NewTestServer(t *testing.T) (*httptest.Server, *store.DB) {
   t.Helper()

   // Find migrations dir relative to test file location
   migrationsDir := findMigrationsDir(t)
   fardDir := findFardDir(t)
   store.MigrationDir = migrationsDir

   db, err := store.Open(":memory:")
   if err != nil {
   t.Fatalf("open db: %v", err)
   }
   t.Cleanup(func() { db.Close() })

   // Seed test actor
   if err := db.CreateActor(&store.Actor{
   ActorID: TestActorID,
   Roles:   []string{"operator", "manager"},
   }, TestAPIKey); err != nil {
   t.Fatalf("seed actor: %v", err)
   }

   outDir, err := os.MkdirTemp("", "acp-test-bridge-*")
   if err != nil {
   t.Fatalf("create bridge outdir: %v", err)
   }
   t.Cleanup(func() { os.RemoveAll(outDir) })

   br := bridge.New("fardrun", fardDir, outDir)
   q := queue.New(db)

   mux := http.NewServeMux()
   authMW := auth.Middleware(db)
   handlers := api.New(db, br, q, 10)
   handlers.Register(mux)
   mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
   w.Header().Set("Content-Type", "application/json")
   fmt.Fprint(w, `{"ok":true,"version":"0.2.0"}`)
   })

   srv := httptest.NewServer(authMW(mux))
   t.Cleanup(srv.Close)
   return srv, db
}

// Do makes an authenticated request and returns the parsed JSON body.
func Do(t *testing.T, srv *httptest.Server, method, path string, body any) map[string]any {
   t.Helper()
   var reqBody *bytes.Buffer
   if body != nil {
   b, err := json.Marshal(body)
   if err != nil {
   t.Fatalf("marshal body: %v", err)
   }
   reqBody = bytes.NewBuffer(b)
   } else {
   reqBody = bytes.NewBuffer(nil)
   }

   req, err := http.NewRequest(method, srv.URL+path, reqBody)
   if err != nil {
   t.Fatalf("new request: %v", err)
   }
   req.Header.Set("Authorization", "Bearer "+TestAPIKey)
   req.Header.Set("Content-Type", "application/json")

   resp, err := http.DefaultClient.Do(req)
   if err != nil {
   t.Fatalf("do request: %v", err)
   }
   defer resp.Body.Close()

   var result map[string]any
   if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
   t.Fatalf("decode response: %v", err)
   }
   result["_status"] = float64(resp.StatusCode)
   return result
}

// DoUnauth makes an unauthenticated request.
func DoUnauth(t *testing.T, srv *httptest.Server, method, path string) map[string]any {
   t.Helper()
   req, _ := http.NewRequest(method, srv.URL+path, nil)
   resp, err := http.DefaultClient.Do(req)
   if err != nil {
   t.Fatalf("do request: %v", err)
   }
   defer resp.Body.Close()
   var result map[string]any
   _ = json.NewDecoder(resp.Body).Decode(&result)
   result["_status"] = float64(resp.StatusCode)
   return result
}

func findMigrationsDir(t *testing.T) string {
   t.Helper()
   candidates := []string{
   "migrations",
   "../migrations",
   "../../migrations",
   "../../../migrations",
   }
   for _, c := range candidates {
   if _, err := os.Stat(c); err == nil {
   return c
   }
   }
   t.Fatal("could not find migrations dir")
   return ""
}

func findFardDir(t *testing.T) string {
   t.Helper()
   candidates := []string{
   "fard/bridge",
   "../fard/bridge",
   "../../fard/bridge",
   "../../../fard/bridge",
   }
   for _, c := range candidates {
   if _, err := os.Stat(c); err == nil {
   return c
   }
   }
   t.Fatal("could not find fard/bridge dir")
   return ""
}
