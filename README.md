# Agent Control Plane

**Run AI workflows like governed business operations: visible, editable, auditable, replayable, and policy-safe.**

Agent Control Plane is not sold as agent infrastructure. It is sold as operational control over AI work.

> Companies do not need more chat windows. They need a cockpit where AI work can be inspected, steered, approved, replayed, and proven.

---

## Status

   v0.2.0 — production-ready service layer

   FARD core     19 modules   2,145 lines   152 tests   0 failures
   Go service    21 endpoints  21 int. tests  0 failures
   Total         173 tests     0 failures

---

## Quickstart

   # Run the selling demo (no server needed)
   go run ./acpd/cmd/acp demo vendor-selection

   # Start the server
   go run ./acpd/cmd/acpd --db acp.db --seed --seed-key acp_live_yourkey
   go run ./acpd/cmd/acpd --db acp.db

   # Or with Docker
   docker compose -f acpd/docker/docker-compose.yml up

   # Run all tests
   go test ./acpd/tests/... -timeout 120s
   fardrun test --program tests/test_policy.fard --json
   # ... (see full list below)

Demo output:

   Agent Control Plane — Vendor Selection Demo
   workflow: wf_vendor_8d2e227c

   ── Step 1  Create vendor selection workflow
      ✓  workflow created
      ✓  state hash              sha256:66e27fa2...

   ── Step 2  Operator advances workflow to research stage
      ✓  stage                   research
      ✓  policy                  PASS

   ── Step 3  Research agent commits vendor evidence artifact
      ✓  artifact committed      vendor_evidence.md
      ✓  confidence              91%
      ✓  digest                  sha256:a5366d70...

   ── Step 4  Finance agent requests $9,000 spend — policy gate triggered
      ⊠  policy gate opened      manager_spend
      ⊠  stage                   gated
      ⊠  spend requested         $9,000 (limit: $5,000)
         Workflow is GATED. Runtime released for other workflows.

   ── Step 5  Manager reviews operator inbox
      pending gates: 1

   ── Step 6  Manager approves $9,000 spend
      ✓  gate resolved           approved
      ✓  stage                   decision

   ── Step 7  Seal final vendor decision
      ✓  final decision          vendor:a — ISO-27001 certified
      ✓  stage                   complete

   ── Step 8  Replay verifies complete decision chain
      ▶  receipts verified       4 transitions
      ▶  chain integrity         ✓ VERIFIED — no tampering detected

   COMPLIANCE: PASS
   Every plan visible. Every artifact tracked. Every policy enforced.
   Every state editable. Every decision replayable.

---

## Architecture

   FARD core (src/acp/)        defines all semantics
     policy evaluation
     state transitions
     receipt generation
     delta reconstruction
     gate logic
     replay verification
     compliance reports

   Go service (acpd/)          owns the product layer
     HTTP API (15 endpoints)
     SQLite / Postgres storage
     API-key auth
     pull-based task queue
     atomic commit pipeline
     FARD execution bridge
     operator CLI
     Docker deployment

The Go service never reimplements FARD semantics. Every transition calls
fardrun as a subprocess. FARD is the source of truth.

---

## Repository contents

   src/acp/
     receipt.fard          Linked hash chain, verify_chain
     policy.fard           Rules, scoping, composition
     state.fard            Workflow state schema, patch paths
     artifact.fard         Artifact provenance, digest diffs
     plan.fard             Plan graph, blockers, risk hotspots
     timeline.fard         Digested multi-agent timeline
     runtime.fard          Policy-gated transition executor, dashboard
     replay.fard           Deterministic replay, chain verification
     control_plane.fard    Store-backed workflow lifecycle
     delta.fard            Incremental storage, reconstruction
     lineage.fard          Branching, counterfactual replay
     gate.fard             Async policy gates, token resumption
     autonomy.fard         Staged autonomy scoring, override
     anchor.fard           External chain anchoring, proof verification
     interop.fard          LangGraph/CrewAI/Temporal/BPMN adapters
     compliance.fard       9-section audit report, evidence digest
     merge.fard            Multi-framework workflow merging
     sample_workflows.fard Vendor selection demo

   tests/
     test_policy.fard        9 tests
     test_receipt.fard       2 tests
     test_plan.fard          2 tests
     test_runtime.fard       2 tests
     test_replay.fard        4 tests
     test_control_plane.fard 5 tests
     test_delta.fard         6 tests
     test_lineage.fard       6 tests
     test_gate.fard          7 tests
     test_autonomy.fard     13 tests
     test_anchor.fard       12 tests
     test_interop.fard      27 tests
     test_compliance.fard   22 tests
     test_merge.fard        15 tests
     test_bpmn.fard         20 tests

   acpd/
     migrations/001_initial.sql    11 tables, all indexes
     internal/store/               8 files — all CRUD + atomic commit pipeline
     internal/auth/                API key middleware, actor context
     internal/bridge/              fardrun subprocess bridge
     internal/queue/               claim/requeue loop, timeout handling
     internal/api/                 15 HTTP endpoints
     internal/demo/                vendor selection end-to-end scenario
     internal/testutil/            test server harness
     fard/bridge/                  8 FARD bridge programs
     tests/integration_test.go     21 integration tests
     cmd/acpd/main.go              HTTP server (env var config)
     cmd/acp/main.go               CLI entry point
     docker/Dockerfile             two-stage build, alpine runtime
     docker/docker-compose.yml     single-service compose with healthcheck

   sdk/openapi/acp.openapi.json    v2.0.0 — 15 endpoints, 30 typed schemas
   docs/ARCHITECTURE.md
   examples/vendor_selection.fard

---

## HTTP API

   POST   /workflows                         Create workflow
   GET    /workflows                         List workflows (?stage=)
   GET    /workflows/:id                     Get workflow
   POST   /workflows/:id/state/edit          Commit human state edit
   POST   /workflows/:id/artifacts           Commit artifact
   GET    /workflows/:id/dashboard           Dashboard projection
   GET    /workflows/:id/receipts            Receipt chain + verification
   POST   /workflows/:id/replay              Replay from initial state
   GET    /workflows/:id/tasks               List workflow tasks (?status=)
   POST   /workflows/:id/gates/:token/resume Resume or reject gate
   GET    /tasks/next                        Claim next task (?agent=)
   GET    /tasks/:id                         Get task
   POST   /tasks/:id/complete                Submit task output
   POST   /tasks/:id/fail                    Fail task
   GET    /operator/inbox                    Pending gates + tasks
   GET    /health                            Health check (unauthenticated)

Auth: Bearer token in Authorization header. Every endpoint except /health
requires a valid API key. Actor identity is resolved from the key —
callers never supply their own actor.

---

## Storage schema

   workflows             summary row per workflow, updated on every transition
   workflow_snapshots    full state JSON at every transition (baseline for deltas)
   deltas                patch set per transition, linked hash chain
   receipts              one per committed transition, cryptographically linked
   artifacts             artifact JSON + optional external content ref
   gates                 pending/resolved gate records with token
   tasks                 pull-based worker queue with claim/timeout/requeue
   audit_events          append-only API-level event log
   actors                API key hashes + roles + workflow/agent restrictions
   policy_versions       cached policy records by version
   schema_migrations     applied migration tracking

Every transition commits snapshot + delta + receipt atomically in one
SQLite transaction. If any write fails, nothing commits.

---

## Worker contract

Workers poll for tasks and submit results. The runtime decides whether
to commit — workers never write state directly.

   GET /tasks/next?agent=<agent_id>

   Response:
   {
     "task_id":               "task_123",
     "workflow_id":           "wf_vendor_001",
     "node_id":               "research",
     "input_json": {
       "task_id":             "task_123",
       "workflow_id":         "wf_vendor_001",
       "state_hash":          "sha256:...",
       "allowed_tools":       ["search", "write_artifact"],
       "expected_output_schema": "vendor_evidence"
     }
   }

   POST /tasks/:id/complete
   {
     "kind":         "summary",
     "name":         "vendor_evidence.md",
     "media_type":   "text/markdown",
     "content":      "...",
     "producer":     "agent:research",
     "input_state_hash": "sha256:...",
     "tool_trace":   [{ "tool": "search", "ok": true }],
     "confidence":   91
   }

Policy is evaluated at commit time against current state.
A submitted artifact that fails policy is rejected — the task fails,
the state does not change.

---

## Gate lifecycle

   1. Transition blocked by policy (e.g. spend > threshold)
   2. gate.open() transitions stage to "gated", issues token
   3. Runtime is released — other workflows continue
   4. GET /operator/inbox shows pending gates
   5. Manager calls POST /workflows/:id/gates/:token/resume
      with resolution: "approved" | "rejected"
   6. On approval: original transition re-evaluated with approval added
      If policy now passes: transition commits
      If policy still blocks: gate rejected, stage unchanged
   7. Token is seq-based — deterministic for replay

---

## Specs

### Workflow state schema

   {
     "schema":         "acp.workflow_state.v1",
     "workflow_id":    "wf_vendor_001",
     "goal":           "Select a vendor with auditable approval and replay",
     "owner":          "ops",
     "stage":          "research",
     "seq":            4,
     "plan":           { "nodes": [], "edges": [] },
     "artifacts":      [],
     "approvals":      [],
     "violations":     [],
     "timeline":       [],
     "variables":      {},
     "final_decision": null
   }

Patch paths:

   "stage"          string
   "goal"           string
   "owner"          string
   "plan"           full plan graph replacement
   "variables"      full variables replacement
   "final_decision" any value
   "approvals"      appends one approval record
   "var.<key>"      sets one key inside variables

Unknown paths surface as patch_error — never silently corrupt state.

### Receipt schema

   {
     "receipt_type":      "acp.state_transition.v1",
     "workflow_id":       "wf_vendor_001",
     "seq":               4,
     "kind":              "human_state_edit",
     "actor":             "manager:ops",
     "state_before_hash": "sha256:...",
     "input_hash":        "sha256:...",
     "policy_version":    "ACP-POLICY-1.0.0",
     "policy_digest":     "sha256:...",
     "tool_version":      "human",
     "tool_digest":       "sha256:...",
     "artifact_digests":  [],
     "state_after_hash":  "sha256:...",
     "digest":            "sha256:..."
   }

Chain root: linked hash from genesis — insertion or reordering breaks root.
Verification: receipt.verify_state_receipt checks all four hashes including body digest.

### Policy rule schema

   {
     "id":            "tool.allow.core",
     "kind":          "tool_allow",
     "severity":      "high",
     "allowed_tools": [...],
     "applies_to":    ["none"]
   }

Rule kinds: tool_allow, required_role, spend_limit, data_boundary.
Policy composition: merge_policies, scope_rule, extend_tool_allowlist.

### Delta schema

   {
     "delta_type":        "acp.delta.v1",
     "seq":               3,
     "kind":              "human_state_edit",
     "actor":             "operator:1",
     "patches":           [{ "path": "stage", "value": "review" }],
     "artifact_digests":  [],
     "state_before_hash": "sha256:...",
     "state_after_hash":  "sha256:...",
     "receipt_digest":    "sha256:..."
   }

### Gate schema

   {
     "gate_type":    "acp.gate.v1",
     "token":        "sha256:...",
     "kind":         "manager_spend",
     "input":        { ... },
     "actor_id":     "operator:1",
     "deadline_seq": 15,
     "context":      { "amount": 9000 },
     "status":       "pending",
     "digest":       "sha256:..."
   }

### Autonomy levels

   0  restricted    no history or violations     spend: $0
   1  supervised    3+ receipts, 90% pass rate   spend: $100
   2  standard      10+ receipts, 95% pass rate  spend: $5,000
   3  extended      25+ receipts, 97% pass rate  spend: $25,000
   4  autonomous    50+ receipts, 99% pass rate  spend: $100,000

Human override: var.autonomy.<actor_id> = { max_level: N } or { min_level: N }

### Anchor schema

   Payload: { anchor_type, workflow_id, seq_from, seq_to, chain_root, payload_digest }
   Proof:   { payload, external_ref: { kind, tx_hash, block, network }, proof_digest }

External ref kinds: ethereum, rfc3161, ct_log.
Any third party with the receipt list can verify without trusting the operator.

### Compliance report schema

   {
     "report_type":         "acp.compliance_report.v1",
     "verdict":             "compliant",
     "reasons":             [],
     "warnings":            [],
     "identity":            { workflow_id, goal, owner, stage, seq, state_hash },
     "policy_coverage":     { rule_count, violation_count, clean },
     "chain_integrity":     { receipt_count, chain_root, chain_verified },
     "anchor_status":       { proof_count, fully_anchored, gap_count },
     "storage":             { delta_count, total_patch_ops },
     "autonomy_roster":     { actor_count, restricted_count },
     "violations":          { total, by_severity },
     "artifact_provenance": { artifact_count, avg_confidence },
     "evidence_digest":     "sha256:..."
   }

Verdict derived from evidence, not asserted. Tampered reports fail verification.

### Interop trace formats

   LangGraph:  { run_id, nodes, edges, events: [{ type, node_id, output, tool }] }
   CrewAI:     { crew_id, tasks, events: [{ type, task_id, agent_id, output }] }
   Temporal:   { run_id, workflow, activities, events: [{ type, activity_id, output }] }
   BPMN:       { process_instance, activity_instances, sequence_flows, history_events }

BPMN gateways filtered to edges. Variable events extracted as var. patches.
Incidents map to policy_violation timeline events.
Output events from all frameworks materialized as ACP artifacts with provenance.

---

## Run all tests

   # FARD suite (152 tests)
   for f in tests/test_*.fard; do
     fardrun test --program "$f" --json 2>&1 | tail -1
   done

   # Go integration suite (21 tests)
   cd acpd && go test ./tests/... -timeout 120s

   # Demo (end-to-end, no server required)
   cd acpd && go run ./cmd/acp demo vendor-selection

---

## Why this is not another agent wrapper

A chat wrapper stores messages.
Agent Control Plane stores state transitions.

A chat wrapper asks the model to follow instructions.
Agent Control Plane blocks invalid transitions regardless of what the model says.

A chat wrapper loses artifacts in conversation history.
Agent Control Plane gives every artifact a digest, producer, confidence score, input state, and tool trace.

A chat wrapper can explain after the fact.
Agent Control Plane can replay from the fact.

A chat wrapper has no counterfactual.
Agent Control Plane can fork at any checkpoint and replay with different decisions.

A chat wrapper blocks on slow approvals.
Agent Control Plane gates the transition, releases the runtime, and resumes on callback.

A chat wrapper cannot prove its history to a regulator.
Agent Control Plane produces a tamper-evident compliance report with an evidence digest.

A chat wrapper treats every agent the same.
Agent Control Plane expands agent permissions based on verified historical performance.

A chat wrapper is opaque to existing process infrastructure.
Agent Control Plane ingests and exports LangGraph, CrewAI, Temporal, and BPMN traces.

---

## Deployment shape

   control-plane-web     dashboard projection
   workflow-runtime      POST /workflows/:id/state/edit, /artifacts
   policy-service        policy.evaluate (FARD bridge)
   state-service         snapshots, deltas, checkpoints
   artifact-service      artifact shelf, object storage
   receipt-service       receipt chain, verification
   gate-service          gate open, token validation, resume callbacks
   autonomy-service      autonomy scoring, override, policy augmentation
   anchor-service        payload generation, proof storage, gap reporting
   interop-service       framework trace ingestion and export
   compliance-service    report generation and verification
   tool-gateway          governed external tool invocation
   agent-worker-pool     workers polling GET /tasks/next

Backing services:

   Postgres/FoundationDB    workflow state and delta log
   S3/MinIO                 artifact bodies
   Kafka/NATS               event stream and gate callbacks
   OpenTelemetry            infrastructure traces
   KMS/Vault                keys, secrets, gate token signing
   Ethereum/RFC3161/CT log  external chain anchoring

---

## Selling line

> Agent Control Plane turns AI work from opaque chat into governed operations: every plan visible, every artifact tracked, every policy enforced, every state editable, every decision replayable, every branch auditable, every approval gateable, every agent calibrated, every chain anchored, every framework connected, every compliance question answerable.

The infrastructure is not the sales pitch.

The guarantee is.
