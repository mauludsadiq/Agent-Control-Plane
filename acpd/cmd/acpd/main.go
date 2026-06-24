package main

import (
"context"
"encoding/json"
"flag"
"fmt"
"log"
"net/http"
"os"
"os/signal"
"syscall"
"time"

"github.com/mauludsadiq/agent-control-plane/acpd/internal/api"
"github.com/mauludsadiq/agent-control-plane/acpd/internal/auth"
"github.com/mauludsadiq/agent-control-plane/acpd/internal/bridge"
"github.com/mauludsadiq/agent-control-plane/acpd/internal/queue"
"github.com/mauludsadiq/agent-control-plane/acpd/internal/store"
)

func main() {
var (
addr          = flag.String("addr", ":8080", "listen address")
dsn           = flag.String("db", "acp.db", "SQLite DSN or postgres://... URL")
migrationsDir = flag.String("migrations", "migrations", "path to migrations dir")
fardrunBin    = flag.String("fardrun", "fardrun", "path to fardrun binary")
fardDir       = flag.String("fard-dir", "fard/bridge", "path to FARD bridge programs")
snapshotEvery = flag.Int("snapshot-every", 10, "snapshot interval in transitions")
seed          = flag.Bool("seed", false, "seed default admin actor and exit")
seedKey       = flag.String("seed-key", "", "API key for seeded admin actor")
)
flag.Parse()

store.MigrationDir = *migrationsDir

db, err := store.Open(*dsn)
if err != nil {
log.Fatalf("open db: %v", err)
}
defer db.Close()

if *seed {
if *seedKey == "" {
log.Fatal("--seed-key required with --seed")
}
actor := &store.Actor{
ActorID: "admin",
Roles:   []string{"operator", "manager", "admin"},
}
if err := db.CreateActor(actor, *seedKey); err != nil {
log.Fatalf("seed actor: %v", err)
}
fmt.Printf("seeded actor admin with key %s\n", *seedKey)
return
}

outDir, err := os.MkdirTemp("", "acpd-bridge-*")
if err != nil {
log.Fatalf("create bridge outdir: %v", err)
}
defer os.RemoveAll(outDir)

br := bridge.New(*fardrunBin, *fardDir, outDir)
q := queue.New(db)

// Start background goroutines
ctx, cancel := context.WithCancel(context.Background())
defer cancel()
go q.RequeueLoop(ctx, 30*time.Second)

// Build router
mux := http.NewServeMux()
authMW := auth.Middleware(db)

handlers := api.New(db, br, q, *snapshotEvery)
handlers.Register(mux)

// Health endpoint — unauthenticated
mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
w.Header().Set("Content-Type", "application/json")
_ = json.NewEncoder(w).Encode(map[string]any{
"ok":      true,
"version": "0.2.0",
"db":      "ok",
})
})

srv := &http.Server{
Addr:         *addr,
Handler:      authMW(mux),
ReadTimeout:  30 * time.Second,
WriteTimeout: 60 * time.Second,
IdleTimeout:  120 * time.Second,
}

go func() {
log.Printf("acpd listening on %s", *addr)
if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
log.Fatalf("server: %v", err)
}
}()

sig := make(chan os.Signal, 1)
signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
<-sig
log.Println("shutting down...")
shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
defer shutCancel()
_ = srv.Shutdown(shutCtx)
}
