package main

import (
"context"
"encoding/json"
"flag"
"fmt"
"log/slog"
"net/http"
"os"
"os/signal"
"strconv"
"sync"
"syscall"
"time"

"github.com/mauludsadiq/agent-control-plane/acpd/internal/api"
"github.com/mauludsadiq/agent-control-plane/acpd/internal/auth"
"github.com/mauludsadiq/agent-control-plane/acpd/internal/bridge"
"github.com/mauludsadiq/agent-control-plane/acpd/internal/queue"
"github.com/mauludsadiq/agent-control-plane/acpd/internal/security"
"github.com/mauludsadiq/agent-control-plane/acpd/internal/store"
"github.com/mauludsadiq/agent-control-plane/acpd/internal/telemetry"
)

const Version = "1.0.0"

// maxRequestBodyBytes is the maximum size of any request body (4MB).
const maxRequestBodyBytes = 4 << 20

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
dsn           = flag.String("db", envOr("ACP_DB", "acp.db"), "SQLite or Postgres DSN")
migrationsDir = flag.String("migrations", envOr("ACP_MIGRATIONS", "migrations"), "path to migrations dir")
fardrunBin    = flag.String("fardrun", envOr("ACP_FARDRUN", "fardrun"), "path to fardrun binary")
fardDir       = flag.String("fard-dir", envOr("ACP_FARD_DIR", "fard/bridge"), "path to FARD bridge programs")
snapshotEvery = flag.Int("snapshot-every", envIntOr("ACP_SNAPSHOT_EVERY", 10), "snapshot interval")
seed          = flag.Bool("seed", false, "seed default admin actor and exit")
seedKey       = flag.String("seed-key", envOr("ACP_SEED_KEY", ""), "API key for seeded admin actor")
seedActorID   = flag.String("seed-actor", envOr("ACP_SEED_ACTOR", "admin"), "actor ID for seed")
rateLimit     = flag.Int("rate-limit", envIntOr("ACP_RATE_LIMIT", 100), "max requests/sec per actor (0=unlimited)")
logFormat     = flag.String("log-format", envOr("ACP_LOG_FORMAT", "json"), "log format: json|text")
telemetryOut  = flag.Bool("telemetry", envOr("ACP_TELEMETRY", "true") == "true", "emit OTel traces+metrics to stderr")
)
flag.Parse()

// Structured logging
var logHandler slog.Handler
if *logFormat == "json" {
logHandler = slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})
} else {
logHandler = slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})
}
logger := slog.New(logHandler)
slog.SetDefault(logger)

slog.Info("starting acpd", "version", Version, "addr", *addr, "db", *dsn)

store.MigrationDir = *migrationsDir

// Open DB
db, err := store.Open(*dsn)
if err != nil {
slog.Error("open db", "err", err)
os.Exit(1)
}
defer db.Close()
slog.Info("db connected", "dsn", *dsn)

// Init KeyProvider
kp, err := security.DefaultProvider()
if err != nil {
slog.Error("security key provider", "err", err)
os.Exit(1)
}
db.SetKeyProvider(kp)
slog.Info("security: key provider initialised")

// Seed mode
if *seed {
if *seedKey == "" {
slog.Error("--seed-key or ACP_SEED_KEY required with --seed")
os.Exit(1)
}
actor := &store.Actor{
ActorID: *seedActorID,
Roles:   []string{"operator", "manager", "admin"},
}
if err := db.CreateActor(actor, *seedKey); err != nil {
slog.Warn("seed actor (may already exist)", "err", err)
} else {
			fmt.Printf("seeded actor %s\n", *seedActorID)
}
return
}

// Init telemetry
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

var tel *telemetry.Provider
if *telemetryOut {
var telErr error
tel, telErr = telemetry.Init(ctx, os.Stderr, os.Stderr)
if telErr != nil {
slog.Warn("telemetry init failed", "err", telErr)
} else {
slog.Info("telemetry: OTel traces+metrics enabled")
}
} else {
telemetry.NoopInit()
}

// Bridge setup
outDir, err := os.MkdirTemp("", "acpd-bridge-*")
if err != nil {
slog.Error("create bridge outdir", "err", err)
os.Exit(1)
}
defer os.RemoveAll(outDir)

br := bridge.New(*fardrunBin, *fardDir, outDir)
q := queue.New(db)
go q.RequeueLoop(ctx, 30*time.Second)

// Build handler stack
mux := http.NewServeMux()
authMW := auth.Middleware(db)
handlers := api.New(db, br, q, *snapshotEvery)
handlers.Register(mux)

// /health — liveness (always responds)
mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(map[string]any{
"ok":      true,
"version": Version,
"db":      "ok",
"fardrun": *fardrunBin,
})
})

// /ready — readiness (checks DB is live)
mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
w.Header().Set("Content-Type", "application/json")
if err := db.RawDB().PingContext(r.Context()); err != nil {
w.WriteHeader(http.StatusServiceUnavailable)
json.NewEncoder(w).Encode(map[string]any{"ok": false, "db": err.Error()})
return
}
json.NewEncoder(w).Encode(map[string]any{"ok": true, "version": Version, "db": "ok"})
})

// Middleware stack: body limit → rate limit → auth → OTel → mux
var handler http.Handler = mux
handler = telemetry.HTTPMiddleware(handler)
handler = authMW(handler)
if *rateLimit > 0 {
handler = rateLimitMiddleware(handler, *rateLimit)
}
handler = bodySizeMiddleware(handler, maxRequestBodyBytes)

srv := &http.Server{
Addr:              *addr,
Handler:           handler,
ReadTimeout:       30 * time.Second,
WriteTimeout:      60 * time.Second,
IdleTimeout:       120 * time.Second,
ReadHeaderTimeout: 5 * time.Second,
MaxHeaderBytes:    1 << 20, // 1MB
}

go func() {
slog.Info("acpd listening", "version", Version, "addr", *addr, "db", *dsn, "fard", *fardDir)
if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
slog.Error("server error", "err", err)
os.Exit(1)
}
}()

// Graceful shutdown
sig := make(chan os.Signal, 1)
signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
<-sig
slog.Info("shutting down", "timeout", "10s")

shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
defer shutCancel()
_ = srv.Shutdown(shutCtx)

if tel != nil {
slog.Info("flushing telemetry")
tel.Shutdown(shutCtx)
}
slog.Info("stopped")
}

// bodySizeMiddleware rejects requests with bodies larger than maxBytes.
func bodySizeMiddleware(next http.Handler, maxBytes int64) http.Handler {
return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
if r.ContentLength > maxBytes {
http.Error(w, `{"error":"request body too large"}`, http.StatusRequestEntityTooLarge)
return
}
r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
next.ServeHTTP(w, r)
})
}

// rateLimitMiddleware is a simple token bucket rate limiter per remote addr.
// Production: use a per-actor limiter backed by Redis.
type rateLimiter struct {
mu      sync.Mutex
buckets map[string]*bucket
rps     int
}

type bucket struct {
tokens   int
lastFill time.Time
}

func rateLimitMiddleware(next http.Handler, rps int) http.Handler {
rl := &rateLimiter{buckets: make(map[string]*bucket), rps: rps}
// Cleanup goroutine
go func() {
for range time.Tick(time.Minute) {
rl.mu.Lock()
cutoff := time.Now().Add(-time.Minute)
for k, b := range rl.buckets {
if b.lastFill.Before(cutoff) {
delete(rl.buckets, k)
}
}
rl.mu.Unlock()
}
}()
return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// Skip rate limiting for health/ready
if r.URL.Path == "/health" || r.URL.Path == "/ready" {
next.ServeHTTP(w, r)
return
}
key := r.RemoteAddr
rl.mu.Lock()
b, ok := rl.buckets[key]
if !ok {
b = &bucket{tokens: rps, lastFill: time.Now()}
rl.buckets[key] = b
}
// Refill tokens since last request
now := time.Now()
elapsed := now.Sub(b.lastFill).Seconds()
b.tokens += int(elapsed * float64(rps))
if b.tokens > rps {
b.tokens = rps
}
b.lastFill = now
allow := b.tokens > 0
if allow {
b.tokens--
}
rl.mu.Unlock()
if !allow {
w.Header().Set("Retry-After", "1")
http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
return
}
next.ServeHTTP(w, r)
})
}
