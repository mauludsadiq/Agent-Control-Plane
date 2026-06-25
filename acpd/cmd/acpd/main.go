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
"strconv"
"syscall"
"time"

"github.com/mauludsadiq/agent-control-plane/acpd/internal/api"
	"github.com/mauludsadiq/agent-control-plane/acpd/internal/telemetry"
"github.com/mauludsadiq/agent-control-plane/acpd/internal/auth"
"github.com/mauludsadiq/agent-control-plane/acpd/internal/bridge"
"github.com/mauludsadiq/agent-control-plane/acpd/internal/queue"
"github.com/mauludsadiq/agent-control-plane/acpd/internal/store"
)

func envOr(key, def string) string {
if v := os.Getenv(key); v != "" {
return v
}
return def
}

func envIntOr(key string, def int) int {
if v := os.Getenv(key); v != "" {
if n, err := strconv.Atoi(v); err == nil {
return n
}
}
return def
}

func main() {
var (
addr          = flag.String("addr", envOr("ACP_ADDR", ":8080"), "listen address")
dsn           = flag.String("db", envOr("ACP_DB", "acp.db"), "SQLite DSN")
migrationsDir = flag.String("migrations", envOr("ACP_MIGRATIONS", "migrations"), "path to migrations dir")
fardrunBin    = flag.String("fardrun", envOr("ACP_FARDRUN", "fardrun"), "path to fardrun binary")
fardDir       = flag.String("fard-dir", envOr("ACP_FARD_DIR", "fard/bridge"), "path to FARD bridge programs")
snapshotEvery = flag.Int("snapshot-every", envIntOr("ACP_SNAPSHOT_EVERY", 10), "snapshot interval")
seed          = flag.Bool("seed", false, "seed default admin actor and exit")
seedKey       = flag.String("seed-key", envOr("ACP_SEED_KEY", ""), "API key for seeded admin actor")
seedActorID   = flag.String("seed-actor", envOr("ACP_SEED_ACTOR", "admin"), "actor ID for seed")
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
log.Fatal("--seed-key or ACP_SEED_KEY required with --seed")
}
actor := &store.Actor{
ActorID: *seedActorID,
Roles:   []string{"operator", "manager", "admin"},
}
if err := db.CreateActor(actor, *seedKey); err != nil {
log.Printf("seed actor (may already exist): %v", err)
} else {
fmt.Printf("seeded actor %s\n", *seedActorID)
}
return
}

outDir, err := os.MkdirTemp("", "acpd-bridge-*")
if err != nil {
log.Fatalf("create bridge outdir: %v", err)
}
defer os.RemoveAll(outDir)

br := bridge.New(*fardrunBin, *fardDir, outDir)
q := queue.New(db)

ctx, cancel := context.WithCancel(context.Background())
defer cancel()
go q.RequeueLoop(ctx, 30*time.Second)

mux := http.NewServeMux()
authMW := auth.Middleware(db)

handlers := api.New(db, br, q, *snapshotEvery)
handlers.Register(mux)
	var handler http.Handler = telemetry.HTTPMiddleware(mux)

mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
w.Header().Set("Content-Type", "application/json")
_ = json.NewEncoder(w).Encode(map[string]any{
"ok":      true,
"version": "0.2.0",
"db":      "ok",
"fardrun": *fardrunBin,
})
})

_ = handler // used below
	srv := &http.Server{
Addr:         *addr,
Handler:      authMW(mux),
ReadTimeout:  30 * time.Second,
WriteTimeout: 60 * time.Second,
IdleTimeout:  120 * time.Second,
}

go func() {
log.Printf("acpd v0.2.0 listening on %s (db=%s fard=%s)", *addr, *dsn, *fardDir)
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
log.Println("stopped")
}
