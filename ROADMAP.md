# Agent Control Plane — Roadmap to 1.0

**Current:** v0.2.0 — verified core + operable service + selling demo
**Target:** v1.0.0 — production-grade, enterprise-deployable, commercially viable

---

## Guiding principle

Each version must be demonstrably more trustworthy than the last.
Not more featured. More provable.

The progression is:

   v0.2.0   proven correct (FARD invariants) + operable (Go service + demo)
   v0.3.0   proven persistent (Postgres + migrations + durability)
   v0.4.0   proven secure (mTLS, KMS, signed receipts, threat model)
   v0.5.0   proven observable (OTel traces, metrics, structured logs)
   v0.6.0   proven scalable (horizontal workers, distributed gates)
   v0.7.0   proven interoperable (SDK releases, framework adapters live)
   v0.8.0   proven compliant (EU AI Act, SOC 2, HIPAA control mapping)
   v0.9.0   proven auditable (external chain anchoring, third-party verify)
   v1.0.0   proven shippable (hardened, documented, supported, priced)

---

## v0.3.0 — Durable Storage

**Goal:** replace SQLite with Postgres-compatible production storage.
**Definition of done:** `acpd serve --db postgres://...` works under load.

   Postgres driver swap (pgx)
   Connection pooling (pgxpool, max 20 conns)
   002_indexes.sql — composite indexes for query patterns
   003_partitioning.sql — receipts and audit_events by workflow_id range
   Migration runner hardening — idempotent, transactional per file
   Snapshot compaction job — prune intermediate snapshots older than N days,
     keep every Kth for reconstruction
   Delta GC — archive deltas older than retention window to S3/cold store
   Workflow archival — completed workflows moved to archive table after TTL
   Read replicas — dashboard and receipt queries routed to replica
   Backup/restore CLI — acp db backup / acp db restore
   Store layer integration tests against real Postgres (testcontainers)

**Why this version exists:** SQLite is correct but not durable under crashes
or concurrent writers. Postgres is the minimum for production deployment.

---

## v0.4.0 — Security Hardening

**Goal:** the system can be handed to a security team and pass review.
**Definition of done:** threat model documented, penetration test checklist green.

   mTLS between acpd and agent workers
   API key rotation — keys have expiry, rotation does not break in-flight tasks
   KMS integration — gate tokens signed by KMS, not self-signed digests
   Receipt signing — optional Ed25519 signature on every receipt digest,
     verifiable by third parties without trusting operator
   Actor permission model hardening:
     allowed_workflows enforced at middleware, not just stored
     allowed_agents enforced at task claim time
     role hierarchy (admin > manager > operator > agent > viewer)
   Rate limiting — per actor, per endpoint, configurable
   Audit log completeness — every 401, 403, 500 recorded in audit_events
   Secret scanning in CI — no keys in repo, no keys in logs
   CORS and CSP headers for dashboard
   Input validation layer — all request bodies validated before bridge call
   Threat model document — STRIDE analysis of every trust boundary

**Why this version exists:** the system handles sensitive workflow data and
approval decisions. Security cannot be retrofitted at v0.9.

---

## v0.5.0 — Observability

**Goal:** operators can diagnose any production issue from telemetry alone.
**Definition of done:** zero print-debugging required for any on-call incident.

   OpenTelemetry traces — every HTTP request, every bridge call, every tx
   Structured logging — zerolog, JSON output, correlation IDs
   Metrics — Prometheus exposition:
     acp_transitions_total (by workflow, kind, policy_ok)
     acp_bridge_duration_seconds (by program)
     acp_task_queue_depth (by agent)
     acp_gate_pending_total
     acp_receipt_chain_verify_total (pass/fail)
   Distributed trace context propagated to FARD bridge via env
   Health endpoint expanded:
     /health/live   liveness (process up)
     /health/ready  readiness (db connected, bridge available)
     /health/deep   deep check (run verify_chain on latest workflow)
   Dashboard SSE endpoint — GET /workflows/:id/events streams timeline
   Alerting runbook — what each metric alert means and how to respond

**Why this version exists:** you cannot operate what you cannot observe.
On-call engineers need traces, not log grep.

---

## v0.6.0 — Scalable Worker Infrastructure

**Goal:** 100+ concurrent agent workers, distributed gate callbacks.
**Definition of done:** vendor selection demo runs with 10 parallel agents, no corruption.

   Distributed task queue hardening:
     advisory locks on claim (no double-claim under concurrent workers)
     exponential backoff on requeue
     dead letter queue for repeatedly failing tasks
     task priority lanes (critical / normal / background)
   Horizontal acpd scaling:
     leader election for background jobs (requeue loop, snapshot compaction)
     stateless HTTP handlers — all state in DB
     session affinity not required
   Gate callback webhook model:
     external systems POST to /webhooks/gate/:token/resolve
     webhook signature verification (HMAC-SHA256)
     retry with exponential backoff on webhook delivery failure
   Worker SDK (Go):
     acpworker.Poll(agent, handler) — typed pull loop
     acpworker.Submit(output) — validated artifact submission
     acpworker.Heartbeat() — extend claim timeout
   Workflow concurrency controls:
     max_concurrent_tasks per workflow
     task dependency graph (task B cannot start until task A completes)
   Load test suite — k6 scripts, target: 1000 transitions/min sustained

**Why this version exists:** real agent deployments run dozens of specialized
workers. The queue model must hold under concurrent load.

---

## v0.7.0 — SDK and Framework Adapters Live

**Goal:** a developer can integrate ACP into an existing LangGraph or Temporal
deployment in under one day.
**Definition of done:** three reference integrations with public repos.

   Python SDK (acpd-py):
     acpd.WorkflowClient — typed wrapper for HTTP API
     acpd.Worker — pull loop with artifact submission
     acpd.InteropClient — LangGraph/CrewAI trace ingestion
     pip install acpd
   TypeScript SDK (acpd-ts):
     same surface as Python SDK
     npm install @acpd/client
   LangGraph adapter (live, not just ingest):
     LangGraph node that emits ACP-compatible trace events
     Receipt hook — after_step callback seals receipt via ACP
     Gate integration — node that blocks on ACP gate resolution
   CrewAI adapter (live):
     CrewAI task completion hooks -> ACP artifact commit
     Crew policy check before tool invocation
   Temporal adapter (live):
     Temporal workflow interceptor — wraps activities with ACP receipts
     Temporal signal handler for gate resume
   BPMN/Camunda adapter (live):
     Camunda 8 connector — writes history events to ACP ingest endpoint
     Process variable sync — Camunda variables -> ACP var. patches
   Adapter certification suite:
     Each adapter must pass a standard compliance test:
       create workflow, commit artifact, trigger gate, approve, replay, verify chain
   OpenAPI client generation — acpd clients generated from acp.openapi.json

**Why this version exists:** the interop module proves the semantics.
The SDK makes adoption frictionless.

---

## v0.8.0 — Compliance Mapping

**Goal:** compliance reports map directly to regulatory control requirements.
**Definition of done:** a compliance officer can hand the report to an auditor
without translation.

   EU AI Act mapping:
     Article 9  — risk management system -> policy rails evidence
     Article 12 — record-keeping -> receipt chain + delta log
     Article 13 — transparency -> dashboard + timeline
     Article 14 — human oversight -> gate approval records
     Article 17 — quality management -> artifact confidence + provenance
   SOC 2 Type II mapping:
     CC6.1  — logical access -> actor table, API key auth, role enforcement
     CC7.2  — system monitoring -> audit_events, OTel traces
     CC8.1  — change management -> receipt chain proves every state change
     CC9.2  — vendor risk -> artifact provenance, tool trace
   HIPAA mapping (for healthcare agent deployments):
     data_boundary policy rule -> PHI destination enforcement
     audit_events -> access log for PHI-touching workflows
   GDPR mapping:
     right to explanation -> compliance report + timeline
     data minimization -> artifact content caps, configurable retention
   Compliance report enhancements:
     regulatory_mapping section in compliance report JSON
     one-click PDF export (compliance.fard -> PDF via pdf_v0)
     evidence pack export — zip of receipts + deltas + report
     scheduled compliance snapshots — daily report stored in audit_events
   Policy library:
     pre-built policies for HIPAA, GDPR, EU-AI-Act-High-Risk
     policy.load_regulatory_preset(standard) -> Policy

**Why this version exists:** the system targets regulated industries.
Compliance is not a feature — it is the product.

---

## v0.9.0 — External Chain Anchoring Production

**Goal:** any third party can verify the receipt chain without trusting ACP.
**Definition of done:** auditor with only the receipt list and chain root can
independently verify the full workflow history.

   Ethereum anchoring backend:
     acpd anchor --workflow <id> --backend ethereum --network mainnet
     Writes chain_root to Ethereum via EIP-712 typed data
     Stores tx_hash in anchor_proofs table
   RFC 3161 timestamping backend:
     acpd anchor --backend rfc3161 --tsa https://timestamp.digicert.com
     TSA token stored in anchor_proofs
   Scheduled anchoring:
     configurable interval (e.g. every 100 transitions or every hour)
     gap_report surfaced in compliance report and operator inbox
   Verification CLI:
     acp verify --workflow <id> --chain-root <hash>
     acp verify --receipts receipts.json --anchor-proof proof.json
     Exit 0 = verified, Exit 1 = tampered
   Third-party verifier package (Go):
     standalone binary, no acpd dependency
     input: receipts JSON + anchor proof JSON
     output: verified / not verified + reason
   Hardware attestation (TPM/TEE) for bridge:
     fardrun execution attested via TPM quote
     attestation stored alongside receipt
     verifiable without trusting the operator's machine

**Why this version exists:** self-attested receipts are only as trustworthy
as the operator. External anchoring removes that trust dependency.

---

## v1.0.0 — Production Hardened

**Goal:** the system can be deployed by an enterprise, supported by a team,
and priced for commercial sale.
**Definition of done:** five paying customers in production, zero data loss incidents.

   Hardening:
     Chaos engineering suite — kill DB mid-transaction, kill bridge mid-call,
       restart acpd mid-gate, verify no corruption in all cases
     Fuzz testing — fuzz HTTP endpoints, fuzz FARD bridge inputs
     Long-running soak test — 72h continuous load, measure receipt chain integrity
     Dependency audit — all Go and FARD dependencies pinned and audited
   Operations:
     acp admin — admin CLI for key rotation, workflow archival, manual replay
     acp doctor — diagnoses common misconfigurations
     Runbook library — documented procedure for every alert
     SLA definition — 99.9% uptime, <100ms p99 for state transitions
   Documentation:
     Operator guide — deploy, configure, scale, monitor, recover
     Developer guide — integrate, write workers, write policies, extend
     Security guide — threat model, key management, network topology
     Compliance guide — EU AI Act, SOC 2, HIPAA mapping and evidence
     API reference — generated from OpenAPI, with examples
   Packaging:
     Helm chart — acpd + Postgres + optional Kafka
     Terraform module — AWS / GCP / Azure deployment
     Docker images on GHCR — acpd:latest, acpd:1.0.0
     ARM64 + AMD64 builds
   Licensing and pricing:
     Open core — src/acp/ FARD modules, acpd service, basic SDK: MIT
     Enterprise — advanced analytics, hosted gates, SLA support,
       compliance report PDF, regulatory presets, SSO: commercial
   Support:
     GitHub Discussions for community
     Private Slack for enterprise customers
     90-day onboarding for first five customers
     Quarterly security review commitment

---

## Version summary

   v0.2.0  ✓  Proven correct + operable           (current)
   v0.3.0     Durable storage (Postgres)
   v0.4.0     Security hardening (mTLS, KMS, signing)
   v0.5.0     Observability (OTel, metrics, SSE)
   v0.6.0     Scalable workers (distributed queue, webhooks)
   v0.7.0     SDK + framework adapters live
   v0.8.0     Compliance mapping (EU AI Act, SOC 2, HIPAA)
   v0.9.0     External chain anchoring production
   v1.0.0     Production hardened + commercially viable

Estimated cadence: one version per 6-8 weeks with a focused team of 3-4.
Total runway to v1.0: 12-16 months.

---

## What does not change between versions

The FARD invariant layer is frozen after v0.2.0 except for additive changes.
The receipt schema, state schema, and policy evaluation logic are stable.
No version breaks the audit trail of a prior version.
A workflow started at v0.3.0 must replay correctly at v1.0.0.

That backward compatibility guarantee is not a constraint.
It is the product.
