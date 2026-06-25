# Agent Control Plane

**Run AI workflows like governed business operations: visible, editable, auditable, replayable, and policy-safe.**

Agent Control Plane is not sold as agent infrastructure. It is sold as operational control over AI work.

> Companies do not need more chat windows. They need a cockpit where AI work can be inspected, steered, approved, replayed, and proven.

---

## Status

    v0.9.5 — external chain anchoring production

    FARD core     19 modules   2,145 lines   152 tests    0 failures
    Go service    26 endpoints  33 int. tests  0 failures
    Worker tests  6 tests        0 failures
    Postgres      6 tests        0 failures
    Load tests    2 tests        0 failures
    Security      10 tests       0 failures
    Branch tests  4 tests        0 failures
    Anchor tests  5 tests        0 failures
    SDK tests     10 tests       0 failures
    Total         233 tests      0 failures

---

## Versioned capacity claims

Each version must govern more AND handle more capacity than the last.

    v0.2.0  Proven correct + operable
            152 FARD tests, 21 integration tests, vendor selection demo passes

    v0.3.0  10,000 workflows
            Load test: 10,000 workflows, 30,000 transitions, 178 tx/sec, 0 chain breaks
            Postgres driver: dual-driver store, idempotent migrations, 6/6 Postgres tests

    v0.4.0  100 concurrent agents
            Worker tests: 100 workers, 500 tasks, 0 double-claims
            Atomic claim (SELECT + conditional UPDATE), heartbeat, dead letter queue,
            priority lanes (critical → normal → background)

    v0.5.0  Multi-model workflows
            DAG plan execution: ready_nodes computed from blocker graph
            Per-node model routing: research → claude-opus, finance → claude-sonnet
            finance + legal tasks blocked until research node marked done

    v0.6.0  Observable at scale
            OTel traces on every HTTP request, commit, bridge call, task claim
            9 instruments: counters + histograms across workflows, tasks, HTTP

    v0.7.0  Security hardened
            HMAC-keyed API key hashes (ACP_MASTER_KEY), signed gate tokens
            mTLS: CA generation, cert issuance, TLS 1.3, RequireAndVerifyClientCert
            10/10 security tests: forgery rejected, untrusted CA rejected

    v0.8.0  10,000 decision variants
            Fork any workflow at any seq: POST /workflows/:id/fork
            355 forks/sec on SQLite, all branch records intact
            Fork-of-fork chain integrity verified

    v0.9.0  SDK + framework adapters live
            Go SDK: typed client, WorkflowBuilder, TaskWorker with heartbeat
            Adapters: LangGraph, CrewAI, Temporal trace ingestion
            10/10 SDK tests against mock ACP server

    v0.9.5  External chain anchoring production
            Any third party can verify receipt chain without trusting the operator
            Backends: LocalBackend (dev), RFC3161/Ethereum/CTLog stubs (prod)
            POST /workflows/:id/anchor — submit chain root to external backend
            GET /workflows/:id/anchor/verify — independent verification
            Gap detection: finds unanchored seq ranges

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
    cd acpd && go test ./tests/... -timeout 120s
    fardrun test --program tests/test_policy.fard --json

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
      plan execution (ready_nodes, mark_node)
      compliance reports

    Go service (acpd/)          owns the product layer
      HTTP API (21 endpoints)
      SQLite / Postgres storage (dual-driver, dialect-safe)
      API-key auth
      pull-based task queue with heartbeat + dead letter
      atomic commit pipeline
      plan DAG execution engine
      model routing (per-node agent assignment)
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
      plan.fard             Plan graph, blockers, risk hotspots, ready_nodes
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
      migrations/
        001_initial.sql               11 tables, all indexes (SQLite)
        001_initial.postgres.sql      Postgres variant
        002_indexes.sql               v0.3.0 performance indexes
        003_worker_hardening.sql      v0.4.0 retry/heartbeat/dead-letter/priority
        004_branches.sql              v0.8.0 branch/fork lineage table
        005_anchors.sql               v0.9.5 external anchor proof table
      internal/store/                 11 files — CRUD + atomic commit + plan execution + fork/branch + anchors
      internal/auth/                  API key middleware, actor context
      internal/security/              KeyProvider (HMAC/KMS), mTLS CA + cert utilities
      internal/telemetry/             OTel tracer + meter, HTTP middleware, span helpers
     internal/security/              KeyProvider (HMAC/KMS), mTLS CA + cert utilities
     internal/telemetry/             OTel tracer + meter, HTTP middleware, span helpers
      internal/bridge/                fardrun subprocess bridge
      internal/queue/                 heartbeat-aware requeue loop, dead letter
      internal/api/                   21 HTTP endpoints
      internal/demo/                  vendor selection end-to-end scenario
      internal/testutil/              test server harness
      fard/bridge/                    16 FARD bridge programs (incl. fork, counterfactual, anchor_payload, anchor_proof, anchor_verify)
      tests/integration_test.go       33 integration tests (incl. multi-model, telemetry, security, branch, anchor)
      tests/workers/                  6 concurrent worker tests
      tests/postgres/                 6 Postgres testcontainer tests
      tests/load/                     2 load tests (100–10,000 workflows)
      cmd/acpd/main.go                HTTP server (env var config)
      cmd/acp/main.go                 CLI entry point
      docker/Dockerfile               two-stage build, alpine runtime
      docker/docker-compose.yml       single-service compose with healthcheck

    sdk/openapi/acp.openapi.json      v2.0.0 — 21 endpoints, 30 typed schemas
    docs/ARCHITECTURE.md
    examples/vendor_selection.fard

---

## HTTP API

    POST   /workflows                              Create workflow (with optional plan)
    GET    /workflows                              List workflows (?stage=)
    GET    /workflows/:id                          Get workflow
    POST   /workflows/:id/state/edit              Commit human state edit
    POST   /workflows/:id/artifacts               Commit artifact
    GET    /workflows/:id/dashboard               Dashboard projection
    GET    /workflows/:id/receipts                Receipt chain + verification
    POST   /workflows/:id/replay                  Replay from initial state
    GET    /workflows/:id/tasks                   List workflow tasks (?status=)
    POST   /workflows/:id/gates/:token/resume     Resume or reject gate
    POST   /workflows/:id/plan/execute            Enqueue ready plan nodes as tasks
    POST   /workflows/:id/plan/nodes/:id/done     Mark plan node complete, unblock DAG
    POST   /workflows/:id/fork                    Fork workflow at any seq, new independent workflow
    GET    /workflows/:id/branches                List all branches of a workflow
    POST   /workflows/:id/anchor                  Anchor receipt chain to external backend
    GET    /workflows/:id/anchor/log              Full anchor proof log + gap report
    GET    /workflows/:id/anchor/verify           Independent chain verification
    GET    /model-routes                          Default model routing table
    GET    /tasks/next                            Claim next task (?agent=)
    GET    /tasks/:id                             Get task
    POST   /tasks/:id/complete                    Submit task output
    POST   /tasks/:id/fail                        Fail task
    POST   /tasks/:id/heartbeat                   Heartbeat — reset expiry clock
    GET    /operator/inbox                        Pending gates + tasks
    GET    /operator/dead-letter                  Dead-lettered tasks (?workflow_id=)
    GET    /health                                Health check (unauthenticated)

Auth: Bearer token in Authorization header. Every endpoint except /health
requires a valid API key. Actor identity is resolved from the key —
callers never supply their own actor.

---

## Multi-model workflow execution

A workflow plan is a DAG of nodes, each with an assigned agent:

    POST /workflows
    {
      "goal": "Select compliant vendor",
      "owner": "ops",
      "plan": {
        "nodes": [
          { "id": "goal",     "agent": "manager",          "status": "done",    "risk": "medium"   },
          { "id": "research", "agent": "research_agent",   "status": "pending", "risk": "medium"   },
          { "id": "finance",  "agent": "finance_agent",    "status": "pending", "risk": "high"     },
          { "id": "legal",    "agent": "legal_agent",      "status": "pending", "risk": "critical" },
          { "id": "decision", "agent": "procurement_agent","status": "pending", "risk": "critical" }
        ],
        "edges": [
          { "from": "goal",     "to": "research" },
          { "from": "research", "to": "finance"  },
          { "from": "research", "to": "legal"    },
          { "from": "finance",  "to": "decision" },
          { "from": "legal",    "to": "decision" }
        ]
      }
    }

    POST /workflows/:id/plan/execute
    {
      "routes": [
        { "node_agent": "research_agent",    "agent_id": "agent:claude-opus",   "model": "claude-opus-4-6"   },
        { "node_agent": "finance_agent",     "agent_id": "agent:claude-sonnet", "model": "claude-sonnet-4-6" },
        { "node_agent": "legal_agent",       "agent_id": "agent:claude-opus",   "model": "claude-opus-4-6"   },
        { "node_agent": "procurement_agent", "agent_id": "agent:claude-sonnet", "model": "claude-sonnet-4-6" }
      ]
    }

    Response: { "enqueued": 1, "task_ids": ["task_wf_..._research"] }
    (finance + legal blocked — research not done yet)

    POST /workflows/:id/plan/nodes/research/done
    POST /workflows/:id/plan/execute
    Response: { "enqueued": 2, "task_ids": ["task_..._finance", "task_..._legal"] }

Risk-to-priority mapping: critical → priority lane critical,
high/medium → normal, low → background.

---

## Worker contract (v0.4.0+)

Workers poll for tasks and submit results. The runtime decides whether
to commit — workers never write state directly.

    GET /tasks/next?agent=<agent_id>

    Response:
    {
      "task_id":     "task_wf_vendor_001_research",
      "workflow_id": "wf_vendor_001",
      "node_id":     "research",
      "agent":       "agent:claude-opus",
      "input_json":  {
        "workflow_id":   "wf_vendor_001",
        "node_id":       "research",
        "node_title":    "Research vendors",
        "output_schema": "vendor_evidence",
        "model":         "claude-opus-4-6",
        "state_seq":     0
      }
    }

    POST /tasks/:id/heartbeat          (every 30s while working)
    POST /tasks/:id/complete           (submit output)
    POST /tasks/:id/fail               (report failure)

Heartbeat contract: workers must POST /tasks/:id/heartbeat within
timeout_sec seconds. Missing heartbeat causes the task to be requeued.
After max_attempts failures, the task is moved to dead_letter_tasks.

Priority lanes: tasks are claimed in priority order —
critical first, then normal, then background.

---

## Storage schema

    workflows             summary row per workflow, updated on every transition
    workflow_snapshots    full state JSON at every transition
    deltas                patch set per transition, linked hash chain
    receipts              one per committed transition, cryptographically linked
    artifacts             artifact JSON + optional external content ref
    gates                 pending/resolved gate records with token
    tasks                 pull-based worker queue with claim/timeout/requeue/heartbeat
    dead_letter_tasks     tasks exhausted max_attempts — never requeued
    workers               worker registry with heartbeat tracking
    branches              fork/counterfactual lineage — branch_id, parent_id, branch_point_seq
    anchors               external proof log — chain_root, payload_digest, proof_digest, external_ref
    audit_events          append-only API-level event log
    actors                API key hashes + roles + restrictions
    policy_versions       cached policy records by version
    schema_migrations     applied migration tracking

Every transition commits snapshot + delta + receipt atomically.
If any write fails, nothing commits.

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
      "plan":           { "nodes": [...], "edges": [...] },
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

### Plan node schema

    {
      "id":            "research",
      "title":         "Research vendors",
      "agent":         "research_agent",
      "status":        "pending",
      "risk":          "medium",
      "output_schema": "vendor_evidence"
    }

Node statuses: pending → (task claimed) → (task completed) → done
Node is ready when all predecessor nodes are done.

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
Verification: receipt.verify_state_receipt checks all four hashes.

### Autonomy levels

    0  restricted    no history or violations     spend: $0
    1  supervised    3+ receipts, 90% pass rate   spend: $100
    2  standard      10+ receipts, 95% pass rate  spend: $5,000
    3  extended      25+ receipts, 97% pass rate  spend: $25,000
    4  autonomous    50+ receipts, 99% pass rate  spend: $100,000

Human override: var.autonomy.<actor_id> = { max_level: N } or { min_level: N }

### Interop trace formats

    LangGraph:  { run_id, nodes, edges, events: [{ type, node_id, output, tool }] }
    CrewAI:     { crew_id, tasks, events: [{ type, task_id, agent_id, output }] }
    Temporal:   { run_id, workflow, activities, events: [{ type, activity_id, output }] }
    BPMN:       { process_instance, activity_instances, sequence_flows, history_events }

---

## Go SDK

   go get github.com/mauludsadiq/agent-control-plane/sdk/go

   import "github.com/mauludsadiq/agent-control-plane/sdk/go/acp"

   // Create a client
   client := acp.NewClient("https://your-acp-server", "acp_live_yourkey")

   // Build a workflow
   wf, _ := client.NewWorkflow("Select compliant vendor", "ops").
       WithPlan(&acp.Plan{...}).
       Create(ctx)

   // Run a worker
   worker := acp.NewTaskWorker(client, acp.WorkerConfig{
       AgentID:           "agent:claude-opus",
       PollInterval:      2 * time.Second,
       HeartbeatInterval: 30 * time.Second,
       MaxConcurrent:     5,
   }, func(ctx context.Context, task *acp.Task) (string, error) {
       // process task.InputJSON
       return outputJSON, nil
   })
   worker.Run(ctx)

   // Ingest a LangGraph trace
   import "github.com/mauludsadiq/agent-control-plane/sdk/go/adapters"

   result, _ := adapters.IngestLangGraph(ctx, client, "ops", &adapters.LangGraphRun{
       RunID:  "lg_run_001",
       Nodes:  []adapters.LangGraphNode{{ID: "retriever", Type: "retrieval"}},
       Events: []adapters.LangGraphEvent{{Type: "node_complete", NodeID: "retriever"}},
   })
   // result.WorkflowID — governed ACP workflow with full receipt chain

---

## Run all tests

    # FARD suite (152 tests)
    for f in tests/test_*.fard; do
      fardrun test --program "$f" --json 2>&1 | tail -1
    done

    # Go suite (233 tests across 4 packages)
    cd acpd && go test ./tests/... -timeout 120s

    # SDK tests
    cd sdk/go && go test ./... -timeout 30s

    # Full load test (10,000 workflows)
    cd acpd && ACP_LOAD_WORKFLOWS=10000 go test ./tests/load/... -timeout 600s

    # Postgres tests (requires Docker)
    cd acpd && go test ./tests/postgres/... -timeout 300s

    # Worker tests
    cd acpd && go test ./tests/workers/... -timeout 60s

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

A chat wrapper treats every model the same.
Agent Control Plane routes each plan node to the right model based on risk and role.

A chat wrapper is opaque to existing process infrastructure.
Agent Control Plane ingests and exports LangGraph, CrewAI, Temporal, and BPMN traces.

A chat wrapper cannot be independently verified by a regulator.
Agent Control Plane anchors its receipt chain to Ethereum, RFC3161, or CT logs — any third party can verify without trusting the operator.

---

## Roadmap

    v0.2.0  done  Proven correct + operable
    v0.3.0  done  10,000 workflows (Postgres + load tests)
    v0.4.0  done  100 concurrent agents (heartbeat, dead letter, priority lanes)
    v0.5.0  done  Multi-model workflows (DAG routing, per-node model assignment)
    v0.6.0  done  Observable at scale (OTel traces + metrics, 9 instruments)
    v0.7.0  done  Security hardened (mTLS, HMAC API keys, KMS stub)
    v0.8.0  done  10,000 decision variants (fork/counterfactual/lineage, 355 forks/sec)
    v0.9.0  done  SDK + framework adapters live (Go SDK, LangGraph/CrewAI/Temporal adapters)
    v0.9.5  done  External chain anchoring production (RFC3161/Ethereum/CTLog)
    v1.0.0  next  Production hardened + commercially viable

Principle: each version must govern more AND handle more capacity than the last.
Capacity is the pitch. Provability is the moat.

---

## Selling line

> Agent Control Plane turns AI work from opaque chat into governed operations: every plan visible, every artifact tracked, every policy enforced, every state editable, every decision replayable, every branch auditable, every approval gateable, every agent calibrated, every chain anchored, every framework connected, every model routed, every variant forkable, every chain anchored externally, every compliance question answerable.

The infrastructure is not the sales pitch.

The guarantee is.
