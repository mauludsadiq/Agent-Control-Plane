package main

import (
"flag"
"fmt"
"log"
"os"

"github.com/mauludsadiq/agent-control-plane/acpd/internal/bridge"
"github.com/mauludsadiq/agent-control-plane/acpd/internal/demo"
"github.com/mauludsadiq/agent-control-plane/acpd/internal/store"
)

func usage() {
fmt.Fprintf(os.Stderr, `acp - Agent Control Plane CLI

Usage:
  acp demo vendor-selection   Run the vendor selection demo end-to-end
  acp demo vendor-selection --json  Output full JSON audit trail

Flags:
`)
flag.PrintDefaults()
os.Exit(1)
}

func main() {
var (
dsn           = flag.String("db", ":memory:", "SQLite DSN (:memory: for demo)")
migrationsDir = flag.String("migrations", "migrations", "path to migrations dir")
fardrunBin    = flag.String("fardrun", "fardrun", "path to fardrun binary")
fardDir       = flag.String("fard-dir", "fard/bridge", "path to FARD bridge programs")
jsonOutput    = flag.Bool("json", false, "output full JSON audit trail")
)
flag.Usage = usage
flag.Parse()

args := flag.Args()
if len(args) < 2 {
usage()
}

if args[0] == "demo" && args[1] == "vendor-selection" {
store.MigrationDir = *migrationsDir

db, err := store.Open(*dsn)
if err != nil {
log.Fatalf("open db: %v", err)
}
defer db.Close()

outDir, err := os.MkdirTemp("", "acp-demo-*")
if err != nil {
log.Fatalf("create bridge outdir: %v", err)
}
defer os.RemoveAll(outDir)

br := bridge.New(*fardrunBin, *fardDir, outDir)

if err := demo.RunVendorSelection(db, br, *jsonOutput); err != nil {
log.Fatalf("demo failed: %v", err)
}
return
}

usage()
}
