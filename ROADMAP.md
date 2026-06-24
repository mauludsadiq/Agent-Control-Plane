# Agent Control Plane — Roadmap to 1.0

**Current:** v0.2.0
**Target:** v1.0.0

---

## The claim

ACP is the system that can run more agents, more models, more workflows,
and more decisions than competing platforms, while remaining auditable,
replayable, policy-safe, and provable.

Capacity is the pitch.
Provability is the moat.

Competing systems will eventually acquire governance properties.
ACP must be at scale before they do.

---

## The principle

   Each version must govern more than the last
   without weakening any guarantee from any prior version.

   More agents, more models, more workflows, more decisions —
   all provable, all replayable, all policy-enforced.

This is not a compliance product with scale as a future feature.
This is a scale product with provability as a structural property.

---

## Competitive scoreboard

   Dimension        Typical Agent Platform    ACP
   ─────────────────────────────────────────────────────────
   Agents           N agents                  More agents
   Models           N models                  More models, one chain
   Workflows        Isolated runs             Portfolio, all auditable
   Decisions        One path                  10,000 variants, all proven
   Auditability     Weak or absent            Native, cryptographic
   Replay           Rare                      Native, deterministic
   Policy           Optional, prompt-based    Native, runtime-enforced
   Counterfactual   None                      Fork at any checkpoint
   Compliance       Manual                    Generated, tamper-evident

The win condition is not "better governance than Temporal."
The win condition is "more governed capacity than anyone."

---

## v0.3.0 — 10,000 Workflows

**Claim:** ACP can manage 10,000 concurrent workflows with receipts intact
and chains verifiable on every one.

**Definition of done:**
- Load test: 10,000 concurrent workflows, 100 transitions each, zero chain breaks
- Postgres driver (pgx), connection pooling, read replicas for dashboard queries
- 002_indexes.sql — composite indexes tuned for the 10k workflow query patterns
- 003_partitioning.sql — receipts and audit_events partitioned by workflow_id
- Migration runner: idempotent, transactional, version-locked
- Snapshot compaction job: prune intermediate snapshots, keep interval boundaries
- Delta GC: archive deltas beyond retention window to cold store
- Workflow archival: completed workflows to archive table after TTL
- Backup/restore CLI: acp db backup / acp db restore
- Store integration tests against real Postgres via testcontainers
- Benchmark report published: p50/p95/p99 transition latency at 10k workflows

**Why capacity and trust advance together here:**
You cannot prove 10,000 chains are intact without Postgres. You cannot run
10,000 workflows without the schema optimizations. The scale target forces
the storage work that makes the guarantees durable.

---

## v0.4.0 — 100 Concurrent Agents

**Claim:** ACP can run 100 concurrent agent workers pulling tasks, committing
artifacts, and hitting policy gates — with zero double-commits and zero
lost receipts under failure.

**Definition of done:**
- Load test: 100 concurrent workers, mixed task types, 1 hour sustained, zero corruption
- Advisory locks on task claim — no double-claim under concurrent workers
- Exponential backoff on requeue, dead letter queue for repeatedly failing tasks
- Task priority lanes: critical / normal / background
- Worker heartbeat: extend claim timeout without re-claiming
- Leader election for background jobs (requeue loop, compaction)
- Webhook model for gate callbacks:
   external systems POST /webhooks/gate/:token/resolve
   HMAC-SHA256 signature verification
   retry with exponential backoff on delivery failure
- Worker SDK (Go): acpworker.Poll / Submit / Heartbeat
- Workflow concurrency controls: max_concurrent_tasks, task dependency graph
- Chaos suite: kill DB mid-transaction, kill bridge mid-call, restart acpd
 mid-gate — verify no corruption in all cases

**Why this matters:**
A governance system that serializes agent work is not a scale system.
100 concurrent agents is the minimum enterprise deployment.
The chaos suite proves the guarantees hold under failure, not just success.

---

## v0.5.0 — Multi-Model Workflows

**Claim:** ACP can coordinate Claude, GPT-4o, Gemini, Llama, and any other
model in one workflow, with every artifact bound to its model version in the
receipt chain, and the full workflow replayable with any model swapped out.

**Definition of done:**
- Model provider field on every artifact: producer includes model_id + model_version
- Tool version encodes model provider: "claude-sonnet-4-6", "gpt-4o-2024-11-20", etc.
- Receipt chain binds model_version in tool_digest — model swap breaks the hash
- Counterfactual replay with model substitution:
   replay.replay_with_substitution(events, substitutions)
   substitutions: { "agent:research": "gpt-4o", "agent:legal": "gemini-1.5-pro" }
- Model performance comparison:
   compare_models(workflow_id, model_a, model_b, events)
   returns: { confidence_delta, latency_delta, policy_violations_delta }
- Multi-model compliance report section:
   which models participated, which artifacts they produced,
   confidence distribution per model, violation rate per model
- Live model routing in agent worker contract:
   task input includes allowed_models list
   worker selects model, records selection in tool_trace
- Integration tests: vendor selection demo with 3 different models,
 same receipt chain, all artifacts verifiable

**Why this is the right claim:**
"More models in one governed workflow" is not a feature any other system
can easily copy. Temporal can add more models but cannot replay with model
substitution while preserving the receipt chain. That's the moat.

---

## v0.6.0 — Observable at Scale

**Goal:** operators can diagnose any issue in a 10,000-workflow, 100-agent
deployment from telemetry alone.

**Definition of done:**
- OpenTelemetry traces: every HTTP request, bridge call, DB transaction
- Structured logging: zerolog, JSON, correlation IDs propagated to FARD bridge
- Prometheus metrics:
   acp_transitions_total (workflow, kind, model, policy_ok)
   acp_bridge_duration_seconds (program, p50/p95/p99)
   acp_task_queue_depth (agent, priority)
   acp_gate_pending_total (kind)
   acp_chain_verify_total (pass/fail)
   acp_concurrent_workflows_active
   acp_concurrent_agents_active
- SSE endpoint: GET /workflows/:id/events — live timeline stream
- Health endpoints:
   /health/live    liveness
   /health/ready   DB + bridge available
   /health/deep    verify_chain on latest 10 workflows
- Grafana dashboard JSON — published in repo
- Alerting runbook — documented response for every alert
- SLA target locked: 99.9% uptime, p99 < 100ms for state transitions at 10k load

**Why observability comes after scale targets:**
You cannot write the correct metrics until you know what breaks at scale.
v0.3 and v0.4 reveal the failure modes. v0.6 instruments them.

---

## v0.7.0 — Security Hardened

**Goal:** the system passes a security review by an external team.

**Definition of done:**
- mTLS between acpd and agent workers
- API key rotation: keys have expiry, rotation does not break in-flight tasks
- KMS integration: gate tokens signed by KMS, not self-signed digests
- Receipt signing: Ed25519 signature on every receipt digest,
 verifiable by third parties without trusting the operator
- Actor permission model hardening:
   allowed_workflows enforced at middleware
   allowed_agents enforced at task claim
   role hierarchy: admin > manager > operator > agent > viewer
- Rate limiting: per actor, per endpoint, configurable
- Input validation: all request bodies validated before bridge call
- Audit log completeness: every 401, 403, 500 in audit_events
- Fuzz testing: HTTP endpoints, FARD bridge inputs
- Threat model document: STRIDE analysis of every trust boundary
- Penetration test checklist: OWASP Top 10 verified clean

**Why security comes after scale:**
Observability reveals what to protect. Scale reveals the attack surface.
Security is not retrofitted at v0.9 but it cannot be designed before v0.5.

---

## v0.8.0 — 10,000 Prompt Variants

**Claim:** ACP can explore 10,000 decision variants in one workflow portfolio,
with provenance preserved for every branch, and the best-performing variant
identified and proven.

**Definition of done:**
- Batch lineage: lineage.fork_batch(parent_state, variants) — creates N forks
 in one transaction
- Parallel counterfactual replay: replay N branches concurrently
- Variant comparison: compare_variants(results) — ranks by outcome quality,
 confidence, policy compliance, cost
- Prompt version tracking: every artifact records the prompt template version
 that produced it (prompt_id + prompt_version in tool_trace)
- Prompt portfolio manager:
   acp prompts list                     all prompt versions in use
   acp prompts compare v1 v2            artifact quality delta
   acp prompts rollback v2 --to v1      governed rollback with receipt
- Branch pruning: auto-archive variants below confidence threshold
- Scale test: 10,000 forks of one baseline, all receipts intact,
 chain roots independently verifiable
- Result: a compliance officer can prove which prompt version produced
 the best outcome and why, with full audit trail

**Why "more decisions" not "more prompts":**
Prompt throughput is a model provider problem. ACP's claim is decision
provenance at scale — not how many tokens are generated but how many
distinct governed decisions can be explored and proven. That's unreachable
by any current system.

---

## v0.9.0 — SDK and Framework Adapters Live

**Goal:** a developer integrates ACP into an existing LangGraph or Temporal
deployment in under one day. The adapter is certified, not just documented.

**Definition of done:**
- Python SDK (acpd-py): WorkflowClient, Worker, InteropClient — pip install acpd
- TypeScript SDK (acpd-ts): same surface — npm install @acpd/client
- LangGraph adapter (live):
   after_step callback seals receipt via ACP
   gate integration blocks node on ACP gate resolution
   tested at 10,000 node executions, receipts intact
- CrewAI adapter (live):
   task completion hooks -> ACP artifact commit
   crew policy check before tool invocation
- Temporal adapter (live):
   workflow interceptor wraps activities with ACP receipts
   signal handler for gate resume
- BPMN/Camunda adapter (live):
   Camunda 8 connector writes history events to ACP ingest
   process variable sync -> ACP var. patches
- Adapter certification suite: every adapter must pass:
   create workflow, commit artifact, trigger gate, approve,
   replay, verify chain, generate compliance report
- OpenAPI client generation from acp.openapi.json
- Public adapter registry: certified adapters listed at docs site

**Why SDKs come after scale and security:**
The SDK surface must be stable. That requires knowing the worker contract
under load (v0.4), the security model (v0.7), and the variant scale
requirements (v0.8). Shipping SDKs before that creates API debt.

---

## v0.9.5 — External Chain Anchoring Production

**Claim:** any third party can verify any ACP workflow history without
trusting the operator, the infrastructure, or ACP itself.

**Definition of done:**
- Ethereum anchoring: chain_root written via EIP-712, tx_hash stored
- RFC 3161 timestamping: TSA token stored in anchor_proofs
- Scheduled anchoring: configurable interval, gap_report in operator inbox
- Verification CLI:
   acp verify --workflow <id> --chain-root <hash>
   acp verify --receipts receipts.json --anchor-proof proof.json
   exit 0 = verified, exit 1 = tampered + reason
- Standalone verifier binary: no acpd dependency, auditor-portable
- Hardware attestation: fardrun execution attested via TPM quote,
 stored alongside receipt
- Compliance report anchor section: proof_count, gap_count, backend kinds,
 last anchored seq, next scheduled anchor

**Why this is v0.9.5 not v1.0:**
Anchoring is the final proof of the moat. It must be built on top of
a stable receipt chain (v0.3), signed receipts (v0.7), and a compliance
report that maps to regulatory requirements. Anchoring before those
foundations exist proves nothing.

---

## v1.0.0 — Production Hardened and Commercially Viable

**Goal:** five paying customers in production, zero data loss incidents,
full documentation, commercial license available.

**Definition of done:**

   Hardening:
   Soak test: 72h continuous load, 10k workflows, 100 agents, zero corruption
   Fuzz corpus: 10,000 malformed inputs, zero panics, zero silent failures
   Dependency audit: all deps pinned, CVE-free at release
   Backward compatibility: workflow from v0.3.0 replays correctly at v1.0.0

   Compliance mapping (built on v0.8 report + v0.9.5 anchoring):
   EU AI Act articles 9, 12, 13, 14, 17 — evidence mapped to report sections
   SOC 2 Type II controls CC6.1, CC7.2, CC8.1, CC9.2
   HIPAA audit log controls for PHI-touching workflows
   One-click compliance pack: PDF report + receipt export + anchor proof

   Operations:
   acp admin: key rotation, workflow archival, manual replay
   acp doctor: diagnoses common misconfigurations
   Runbook library: documented procedure for every alert
   SLA: 99.9% uptime, p99 < 100ms transitions, p99 < 5s replay at 10k

   Documentation:
   Operator guide: deploy, configure, scale, monitor, recover
   Developer guide: integrate, write workers, write policies, extend
   Security guide: threat model, key management, network topology
   Compliance guide: EU AI Act, SOC 2, HIPAA mapping and evidence
   API reference: generated from OpenAPI with examples

   Packaging:
   Helm chart: acpd + Postgres + optional Kafka
   Terraform module: AWS / GCP / Azure
   Docker images on GHCR: acpd:latest, acpd:1.0.0
   ARM64 + AMD64 builds

   Licensing:
   Open core: src/acp/ FARD modules, acpd service, basic SDK — MIT
   Enterprise: advanced analytics, hosted gates, SLA support,
     compliance PDF, regulatory presets, SSO, anchor backends — commercial

---

## Version summary

   v0.2.0   ✓  Proven correct + operable           (current)
   v0.3.0      10,000 workflows (Postgres + load proof)
   v0.4.0      100 concurrent agents (distributed queue + chaos)
   v0.5.0      Multi-model workflows (model binding + substitution replay)
   v0.6.0      Observable at scale (OTel + metrics + SSE)
   v0.7.0      Security hardened (mTLS + KMS + signing + pentest)
   v0.8.0      10,000 decision variants (batch lineage + prompt portfolio)
   v0.9.0      SDK + framework adapters live (certified, not just documented)
   v0.9.5      External chain anchoring production (third-party verifiable)
   v1.0.0      Production hardened + commercially viable

Estimated cadence: one version per 6-8 weeks with a focused team of 3-4.
Total runway to v1.0: 14-18 months.

---

## What does not change between versions

The FARD invariant layer is frozen after v0.2.0 except for additive changes.
The receipt schema, state schema, and policy evaluation logic are stable.
No version breaks the audit trail of a prior version.
A workflow started at v0.3.0 must replay correctly at v1.0.0.
A receipt chain anchored at v0.9.5 must verify at v1.0.0.

The backward compatibility guarantee is not a constraint on ambition.
It is what makes the scale claim credible.

You can run more agents, more models, more workflows, more decisions —
because every one of them is provable, and always has been.
