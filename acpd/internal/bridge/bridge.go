package bridge

import (
	"context"
"encoding/json"
"fmt"
"os"
"os/exec"
	"time"

	"github.com/mauludsadiq/agent-control-plane/acpd/internal/telemetry"
"path/filepath"
)

// Bridge executes FARD programs via fardrun and returns their JSON output.
// FARD remains the source of truth for all ACP semantics.
// Go never reimplements policy, receipt, or state transition logic.

type Bridge struct {
fardrunBin string // path to fardrun binary
fardDir    string // path to fard/bridge/ programs
outDir     string // temp dir for fardrun --out
}

func New(fardrunBin, fardDir, outDir string) *Bridge {
return &Bridge{
fardrunBin: fardrunBin,
fardDir:    fardDir,
outDir:     outDir,
}
}

// Run executes a FARD bridge program with the given input JSON and returns
// the result JSON. Each call writes input to a temp file, runs fardrun,
// and reads result.json from the output directory.
func (b *Bridge) Run(program string, input any) (json.RawMessage, error) {
   var result json.RawMessage
   err := telemetry.TraceBridge(context.Background(), program, func(ctx context.Context) error {
   var runErr error
   result, runErr = b.runInner(program, input)
   return runErr
   })
   return result, err
}

func (b *Bridge) runInner(program string, input any) (json.RawMessage, error) {
// Write input to temp file
inputJSON, err := json.Marshal(input)
if err != nil {
return nil, fmt.Errorf("marshal input: %w", err)
}
inputFile, err := os.CreateTemp("", "acp-bridge-input-*.json")
if err != nil {
return nil, fmt.Errorf("create input temp file: %w", err)
}
defer os.Remove(inputFile.Name())
if _, err := inputFile.Write(inputJSON); err != nil {
return nil, fmt.Errorf("write input: %w", err)
}
inputFile.Close()

// Create unique output dir for this run
outDir, err := os.MkdirTemp(b.outDir, "acp-bridge-out-*")
if err != nil {
return nil, fmt.Errorf("create out dir: %w", err)
}
defer os.RemoveAll(outDir)

programPath := filepath.Join(b.fardDir, program)
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
cmd := exec.CommandContext(ctx, b.fardrunBin, "run",
"--program", programPath,
"--out", outDir,
)
// Pass input as environment variable — FARD bridge programs read it
cmd.Env = append(os.Environ(), "ACP_INPUT_FILE="+inputFile.Name())

out, err := cmd.CombinedOutput()
if err != nil {
return nil, fmt.Errorf("fardrun %s: %w\noutput: %s", program, err, string(out))
}

resultPath := filepath.Join(outDir, "result.json")
resultRaw, err := os.ReadFile(resultPath)
if err != nil {
return nil, fmt.Errorf("read result: %w", err)
}

// fardrun wraps result in {"result": ...}
var wrapper struct {
Result json.RawMessage `json:"result"`
}
if err := json.Unmarshal(resultRaw, &wrapper); err != nil {
return nil, fmt.Errorf("unwrap result: %w", err)
}
return wrapper.Result, nil
}

// RunAndUnmarshal runs a FARD program and unmarshals the result into dest.
func (b *Bridge) RunAndUnmarshal(program string, input any, dest any) error {
raw, err := b.Run(program, input)
if err != nil {
return err
}
return json.Unmarshal(raw, dest)
}
