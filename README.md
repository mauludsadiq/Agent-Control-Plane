# Agent Control Plane

**Run AI workflows like governed business operations: visible, editable, auditable, replayable, and policy-safe.**

Agent Control Plane is not sold as agent infrastructure. It is sold as operational control over AI work.

> Companies do not need more chat windows. They need a cockpit where AI work can be inspected, steered, approved, replayed, and proven.

This repository implements that cockpit in FARD. The machinery is typed workflow state, policy-enforced transitions, artifact provenance, cryptographic receipts, deterministic replay, delta storage, workflow branching, async policy gates, staged autonomy, external chain anchoring, framework interoperability, multi-framework merging, and compliance reporting.

---

## Repository at a glance

    src/acp/          19 modules   2,145 lines
    tests/            15 files      152 tests    0 failures
    sdk/openapi/      acp.openapi.json v2.0.0   15 endpoints   30 schemas

    fardrun test --program tests/test_<module>.fard --json

All 152 tests pass against fardrun v1.7.1.

---

## Module index

    receipt.fard          Canonical JSON, SHA-256 digests, linked receipt chain
    policy.fard           Executable policy rules, scoping, composition
    state.fard            Workflow state schema, patch paths
    artifact.fard         Artifact provenance, digest-based diffs
    plan.fard             Plan graph, blockers, ready nodes, risk hotspots
    timeline.fard         Digested multi-agent execution timeline
    runtime.fard          Policy-gated transition executor, dashboard projection
    replay.fard           Deterministic replay, chain verification, comparison
    control_plane.fard    Store-backed workflow lifecycle
    delta.fard            Incremental state storage, reconstruction, chain verify
    lineage.fard          Workflow branching, counterfactual replay
    gate.fard             Async policy gates, token-based resumption
    autonomy.fard         Staged autonomy scoring, tool expansion, human override
    anchor.fard           External chain anchoring, proof verification, gap report
    interop.fard          LangGraph / CrewAI / Temporal / BPMN ingest and export
    compliance.fard       9-section audit report, tamper-evident evidence digest
    merge.fard            Multi-framework workflow merging, conflict detection
    sample_workflows.fard Vendor selection end-to-end demo

---

## What is being sold

Agent Control Plane lets an organization answer six operational questions:

1. **What is the AI team doing?**
   The Plan Graph shows task decomposition, dependencies, assigned agents, and risk hotspots.

2. **What did it produce?**
   The Artifact Shelf turns outputs into first-class objects with provenance, confidence, and tool trace.

3. **What is it allowed to do?**
   Policy Rails enforce rules as executable constraints, not prompt instructions.

4. **How did it get here?**
   The Timeline records agent handoffs, tool calls, human edits, policy violations, and state commits.

5. **How can a human steer it without prompt gymnastics?**
   The State Editor applies validated patches committed as receipted transitions.

6. **Can the decision be replayed and audited?**
   Replay re-executes from checkpoints and compares state hashes and receipt chains.

---

## Specs

### Workflow state schema

Every workflow is a single canonical record. Schema version is pinned in every receipt.

    {
      "schema":         "acp.workflow_state.v1",
      "workflow_id":    "wf_vendor_001",
      "goal":           "Select a vendor with auditable approval and replay",
      "owner":          "ops",
      "stage":          "research",
      "seq":            4,
      "plan":           { "nodes": [], "edges": [] },
      "artifacts":      [],        // sha256: digest refs
      "approvals":      [],
      "violations":     [],
      "timeline":       [],
      "variables":      {},
      "final_decision": null
    }

Patch paths (used in state.apply_patches):

    "stage"              string
    "goal"               string
    "owner"              string
    "plan"               full plan graph replacement
    "variables"          full variables replacement
    "final_decision"     any value
    "approvals"          appends one approval record
    "var.<key>"          sets one key inside variables

Unknown paths surface as patch_error — never silently corrupt state.

### Receipt schema

Every committed transition emits a receipt. The receipt chain is a linked hash from genesis —
insertion or reordering of any receipt produces a different chain root.

    {
      "receipt_type":       "acp.state_transition.v1",
      "workflow_id":        "wf_vendor_001",
      "seq":                4,
      "kind":               "human_state_edit",
      "actor":              "manager:ops",
      "state_before_hash":  "sha256:...",
      "input_hash":         "sha256:...",
      "policy_version":     "ACP-POLICY-1.0.0",
      "policy_digest":      "sha256:...",
      "tool_version":       "human",
      "tool_digest":        "sha256:...",
      "artifact_digests":   [],
      "state_after_hash":   "sha256:...",
      "digest":             "sha256:..."
    }

Verification: receipt.verify_state_receipt(receipt, state_before, input, state_after)
checks state_before_hash, input_hash, state_after_hash, and body digest.

Chain root: receipt.chain_root(receipts) — linked hash, not a flat hash.
Chain verify: receipt.verify_chain(receipts, expected_root)

### Artifact schema

    {
      "artifact_type":      "acp.artifact.v1",
      "kind":               "summary",
      "name":               "vendor_evidence.md",
      "media_type":         "text/markdown",
      "content":            "...",
      "producer":           "research_agent",
      "input_state_hash":   "sha256:...",
      "tool_trace":         [{ "tool": "search", "ok": true }],
      "confidence":         91,
      "digest":             "sha256:..."
    }

Content comparison uses digest equality — type-safe for text, records, lists, or bytes.

### Policy rule schema

    {
      "id":            "tool.allow.core",
      "kind":          "tool_allow",      // tool_allow | required_role | spend_limit | data_boundary
      "severity":      "high",            // low | medium | high | critical
      "allowed_tools": [...],             // tool_allow
      "role":          "operator",        // required_role
      "applies_to":    ["none"],          // required_role — scope to specific tools
      "max_amount":    5000,              // spend_limit
      "approval_kind": "manager_spend",   // spend_limit
      "block":         [...]              // data_boundary
    }

Violation card:

    {
      "ok":        false,
      "code":      "POLICY_TOOL_DENIED",
      "message":   "Tool not allowed: external_email",
      "rule_id":   "tool.allow.core",
      "severity":  "high",
      "data":      { "tool": "external_email", "allowed_tools": [...] }
    }

Policy composition:

    policy.merge_policies(base, overlay)
    policy.scope_rule(rule, applies_to)
    policy.extend_tool_allowlist(policy, rule_id, extra_tools)

### Delta schema

Deltas store patch sets instead of full state blobs. Storage cost is O(patches) not O(state).

    {
      "delta_type":         "acp.delta.v1",
      "seq":                3,
      "kind":               "human_state_edit",
      "actor":              "operator:1",
      "patches":            [{ "path": "stage", "value": "review" }],
      "artifact_digests":   [],
      "state_before_hash":  "sha256:...",
      "state_after_hash":   "sha256:...",
      "receipt_digest":     "sha256:..."
    }

Baseline (seq 0) stored as full state. All subsequent seqs stored as deltas.
Reconstruction: delta.reconstruct_at(baseline, deltas, target_seq)
Chain verify: delta.verify_delta_chain(baseline_hash, deltas)

### Gate schema

    {
      "gate_type":     "acp.gate.v1",
      "token":         "sha256:...",       // deterministic — present to resume
      "kind":          "manager_spend",
      "input":         { ... },            // original transition, re-evaluated on resume
      "actor_id":      "operator:1",
      "deadline_seq":  15,                 // seq-based expiry for deterministic replay
      "context":       { "amount": 9000 },
      "status":        "pending",
      "digest":        "sha256:..."
    }

Gate lifecycle:
    gate.open(actor, state, policy, input, gate_kind, deadline_seq, context)
      -> stage: "gated", token issued, original transition NOT committed

    gate.resume(actor, gated_state, policy, tool_version, token, "approved"|"rejected")
      -> policy re-evaluated with approval added — no bypass
      -> on approval: original patches committed
      -> on rejection: stage: "gated_rejected"

### Autonomy levels

Agents earn broader permissions through verified receipt chain evidence.

    Level 0  restricted    no history or violations     tools: [none]
    Level 1  supervised    3+ receipts, 90% pass rate   tools: [none, read_file, search]
    Level 2  standard      10+ receipts, 95% pass rate  tools: + write_artifact, score_vendor, diff_contract
    Level 3  extended      25+ receipts, 97% pass rate  tools: + run_tests, run_query, write_file
    Level 4  autonomous    50+ receipts, 99% pass rate  tools: + external_api, send_notification

Spend limits: 0 / 100 / 5,000 / 25,000 / 100,000

Evidence inputs: policy_pass_rate, avg_confidence, intervention_rate, violation_count.
Cold-start protection: minimum receipt counts enforced before any level above restricted.
Human override: var.autonomy.<actor_id> = { max_level: N } or { min_level: N }

### Anchor schema

    Payload:
    {
      "anchor_type":    "acp.anchor.v1",
      "workflow_id":    "wf_vendor_001",
      "seq_from":       1,
      "seq_to":         8,
      "chain_root":     "sha256:...",
      "policy_version": "ACP-POLICY-1.0.0",
      "payload_digest": "sha256:..."
    }

    Proof:
    {
      "proof_type":   "acp.anchor_proof.v1",
      "payload":      { ... },
      "external_ref": {
        "kind":       "ethereum",        // ethereum | rfc3161 | ct_log
        "tx_hash":    "0x...",
        "block":      12345,
        "network":    "mainnet"
      },
      "anchored_by":  "anchor-service:1",
      "proof_digest": "sha256:..."
    }

Any third party with the receipt list can recompute chain_root and verify the payload
without trusting the ACP operator or the anchoring backend.

### Compliance report schema

    {
      "report_type":        "acp.compliance_report.v1",
      "verdict":            "compliant",    // compliant | warnings | non_compliant
      "reasons":            [],             // non-empty if non_compliant
      "warnings":           [],             // non-empty if warnings
      "identity":           { workflow_id, goal, owner, stage, seq, policy_version, state_hash },
      "policy_coverage":    { rule_count, violation_count, critical_count, high_count, clean },
      "chain_integrity":    { receipt_count, chain_root, chain_verified, tamper_evident },
      "anchor_status":      { proof_count, fully_anchored, gap_count, gaps },
      "storage":            { delta_count, total_patch_ops, baseline_plus_deltas },
      "autonomy_roster":    { actor_count, restricted_count, actors },
      "violations":         { total, by_severity: { critical, high, medium, low } },
      "artifact_provenance":{ artifact_count, avg_confidence, low_confidence_count },
      "evidence_digest":    "sha256:..."
    }

Verdict is derived from evidence — not asserted. Tampered reports fail verification.
compliance.verify(report) recomputes evidence_digest and compares.

### Interop trace format

Frameworks emit traces. ACP ingests them into plan graph + timeline + artifact shelf.

    LangGraph run:
    {
      "run_id":  "lg_run_001",
      "nodes":   [{ "id", "name", "agent", "status", "metadata": { "risk", "output_schema" } }],
      "edges":   [{ "source", "target", "condition" }],
      "events":  [{ "type": "tool_call"|"node_complete"|"handoff", "node_id", "output", "tool" }]
    }

    CrewAI run:
    {
      "crew_id": "crew_001",
      "tasks":   [{ "id", "description", "agent_id", "status", "expected_output" }],
      "events":  [{ "type": "task_complete"|"tool_use"|"agent_handoff", "agent_id", "output" }]
    }

    Temporal run:
    {
      "run_id":      "temporal_run_001",
      "workflow":    { "workflow_id", "workflow_type", "status" },
      "activities":  [{ "activity_id", "activity_type", "worker_id", "status" }],
      "events":      [{ "type": "ActivityTaskCompleted"|"ActivityTaskScheduled"|"ActivityTaskFailed", "worker_id", "output" }]
    }

    BPMN / Camunda run:
    {
      "process_instance":   { "id", "process_definition_key", "business_key", "state" },
      "activity_instances": [{ "activity_id", "activity_name", "activity_type", "assignee", "state" }],
      "sequence_flows":     [{ "source_ref", "target_ref", "name" }],
      "history_events":     [{ "type": "activityStarted"|"activityCompleted"|"variableCreated"|"incidentCreated", "activity_id", "assignee", "variables", "output" }]
    }

BPMN gateway types (ExclusiveGateway, ParallelGateway, InclusiveGateway) are filtered
to edges — not materialized as plan nodes.
BPMN variable events are extracted as ACP var. patches.
BPMN incidents map to policy_violation timeline events.

Output events from all frameworks are materialized as ACP artifacts with provenance.

### Multi-framework merge

    merge.merge(ingested_results, cross_edges, workflow_id)

Produces one plan graph, one timeline, one artifact shelf, one provenance chain root
across any combination of LangGraph, CrewAI, Temporal, and BPMN runs.

Conflict detection: duplicate node ids across frameworks surface as conflicts.
Resolution: conflicting node ids are prefixed with framework name (langgraph.research,
crewai.research) — never silently overwritten.

Cross-framework edges:
    merge.cross_edge("langgraph.research", "crewai.legal_review", "evidence feeds legal")

Framework subgraph extraction:
    merge.framework_subgraph(merged, "langgraph")

### Agent worker contract

Input (from ACP to agent):

    {
      "task_id":               "task_123",
      "workflow_id":           "wf_vendor_001",
      "state_hash":            "sha256:...",
      "allowed_tools":         ["search", "write_artifact"],
      "policy_context":        {},
      "artifact_inputs":       [],
      "expected_output_schema":"scorecard"
    }

Output (from agent to ACP):

    {
      "task_id":    "task_123",
      "workflow_id":"wf_vendor_001",
      "artifact":   { ... },
      "claims":     [],
      "confidence": 91,
      "tool_trace": [{ "tool": "search", "ok": true }],
      "state_patch":[{ "path": "var.score", "value": 87 }]
    }

The runtime, not the agent, decides whether this return value may be committed.
Policy is evaluated against current state at commit time — not at submission time.

---

## Quickstart

Run the sample workflow:

    fardrun run --program examples/vendor_selection.fard --out out/vendor_selection
    cat out/vendor_selection/result.json

Run the full test suite:

    fardrun test --program tests/test_policy.fard --json
    fardrun test --program tests/test_receipt.fard --json
    fardrun test --program tests/test_plan.fard --json
    fardrun test --program tests/test_runtime.fard --json
    fardrun test --program tests/test_replay.fard --json
    fardrun test --program tests/test_control_plane.fard --json
    fardrun test --program tests/test_delta.fard --json
    fardrun test --program tests/test_lineage.fard --json
    fardrun test --program tests/test_gate.fard --json
    fardrun test --program tests/test_autonomy.fard --json
    fardrun test --program tests/test_anchor.fard --json
    fardrun test --program tests/test_interop.fard --json
    fardrun test --program tests/test_compliance.fard --json
    fardrun test --program tests/test_merge.fard --json
    fardrun test --program tests/test_bpmn.fard --json

Expected: 152 passed, 0 failed.

---

## Product surfaces

### Plan Graph

    plan.graph(nodes, edges)
    plan.ready_nodes(g)
    plan.risk_hotspots(g)
    plan.mark_node(g, id, status)
    plan.children(g, id)
    plan.blockers(g, id)

### Artifact Shelf

    artifact.make(kind, name, media_type, content, producer, input_state_hash, tool_trace, confidence)
    artifact.shelf(artifacts)
    artifact.diff_text(old_artifact, new_artifact)
    artifact.by_digest(artifacts, digest)

### Policy Rails

    policy.evaluate(policy, state, actor, input)
    policy.default_enterprise_policy()
    policy.merge_policies(base, overlay)
    policy.scope_rule(rule, applies_to)
    policy.extend_tool_allowlist(base_policy, rule_id, extra_tools)

### State Editor

    runtime.human_state_edit(actor, state, policy, patches)
    state.apply_patches(state, patches)
    state.checkpoint(state)

### Timeline

    timeline.visible(events)

### Replay

    replay.replay(initial_state, policy, actor, events, tool_version)
    replay.replay_from_checkpoint(checkpoint_state, policy, actor, events, tool_version)
    replay.compare_replay(a, b)

### Control Plane Store

    cp.empty_store()
    cp.create_workflow(store, workflow_id, goal, owner)
    cp.get_workflow(store, workflow_id)
    cp.edit_state(store, workflow_id, actor, patches, policy)
    cp.commit_artifact(store, workflow_id, actor, policy, tool_version, artifact)
    cp.dashboard(store, workflow_id)
    cp.replay_workflow(store, workflow_id, policy, actor, events, tool_version)
    cp.compare_workflows(store, wf_a, wf_b, policy, actor, events, tool_version)

### Delta Storage

    delta.from_transition(result, patches, kind, actor_id)
    delta.from_artifact_commit(result, artifact_digest, actor_id)
    delta.apply_deltas(baseline, deltas, max_seq)
    delta.reconstruct_at(baseline, deltas, target_seq)
    delta.verify_delta_chain(baseline_hash, deltas)
    delta.summarize(deltas)
    delta.storage_report(deltas)

### Workflow Branching

    lineage.fork(parent_state, new_workflow_id, branch_point_seq, reason)
    lineage.fork_from_replay(initial_state, policy, actor, events, tool_version, seq, new_id, reason)
    lineage.counterfactual(forked_state, policy, actor, events, tool_version)
    lineage.compare_branches(original, counterfactual_result)
    lineage.ancestry(state)
    lineage.logical_snapshot(state)

### Async Policy Gates

    gate.open(actor, state, policy, input, gate_kind, deadline_seq, context)
    gate.resume(actor, gated_state, policy, tool_version, token, resolution)
    gate.pending_gate(state)
    gate.is_expired(gate, current_seq)

### Staged Autonomy

    autonomy.score(actor_id, receipts, artifacts, human_edits)
    autonomy.override_level(autonomy_record, state)
    autonomy.evaluate_with_autonomy(policy, state, actor, input, autonomy_record)
    autonomy.summary(autonomy_record)
    autonomy.allowed_tools_for_level(level)
    autonomy.spend_limit_for_level(level)

### External Chain Anchoring

    anchor.make_payload(workflow_id, seq_from, seq_to, chain_root, policy_version)
    anchor.make_proof(payload, external_ref, anchored_by)
    anchor.verify_payload(payload, receipts)
    anchor.verify_proof(proof, receipts)
    anchor.new_log()
    anchor.append_proof(log, proof)
    anchor.gap_report(log, max_seq)
    anchor.summary(log, max_seq)
    anchor.is_anchored(log, seq)

### Framework Interop

    interop.ingest(framework, run)           // "langgraph" | "crewai" | "temporal" | "bpmn"
    interop.export_to(framework, state)
    interop.langgraph_ingest(run)
    interop.crewai_ingest(run)
    interop.temporal_ingest(run)
    interop.bpmn_ingest(run)
    interop.extract_artifacts(framework, run_id, events, input_state_hash)
    interop.bpmn_extract_variable_patches(history_events)

### Multi-Framework Merge

    merge.merge(ingested_results, cross_edges, workflow_id)
    merge.cross_edge(from_node, to_node, reason)
    merge.framework_subgraph(merged, framework)
    merge.find_node_conflicts(ingested_results)
    merge.summary(merged)

### Compliance Report

    compliance.generate(workflow_state, receipts, artifacts, deltas, anchor_log, autonomy_records, policy)
    compliance.verify(report)
    compliance.section_identity(state, policy)
    compliance.section_policy_coverage(state, receipts, policy)
    compliance.section_chain_integrity(receipts)
    compliance.section_anchor_status(anchor_log, max_seq)
    compliance.section_storage(deltas)
    compliance.section_autonomy_roster(autonomy_records)
    compliance.section_violations(state)
    compliance.section_artifact_provenance(artifacts)
    compliance.derive_verdict(policy_sec, chain_sec, anchor_sec, violation_sec, artifact_sec)

### Dashboard Projection

    runtime.dashboard(state, artifacts)

Returns:

    {
      "workflow_id":       "wf_vendor_001",
      "goal":              "...",
      "stage":             "research",
      "seq":               4,
      "plan_graph":        {},
      "artifact_shelf":    [],
      "timeline":          [],
      "policy_violations": [],
      "state_hash":        "sha256:...",
      "replay_available":  true
    }

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

A production deployment maps directly onto these modules:

    control-plane-web       dashboard projection
    workflow-runtime        runtime.transition / runtime.commit_artifact
    policy-service          policy.evaluate
    state-service           state patches, checkpoints, delta storage
    artifact-service        artifact shelf and object storage
    receipt-service         receipt generation, verification, chain integrity
    timeline-service        timeline projection
    replay-service          replay, comparison, branching, counterfactuals
    gate-service            async gate open, token validation, resumption callbacks
    autonomy-service        autonomy scoring, override, policy augmentation
    anchor-service          payload generation, proof storage, gap reporting
    interop-service         framework trace ingestion and export
    merge-service           multi-framework workflow assembly
    compliance-service      report generation and verification
    tool-gateway            governed external tool invocation
    agent-worker-pool       specialized agents that propose patches and artifacts

Recommended backing services:

    Postgres/FoundationDB    canonical workflow state and delta log
    S3/MinIO                 artifact bodies
    Kafka/NATS               event stream and gate callback queue
    OpenTelemetry            infrastructure traces
    OPA/Cedar/custom FARD    enterprise policy rules
    KMS/Vault                keys, secrets, and gate token signing
    Ethereum/RFC3161/CT log  external chain anchoring backends
    Anka/EOS                 claims, witnesses, challenges, mesh publication

---

## Selling line

> Agent Control Plane turns AI work from opaque chat into governed operations: every plan visible, every artifact tracked, every policy enforced, every state editable, every decision replayable, every branch auditable, every approval gateable, every agent calibrated, every chain anchored, every framework connected, every compliance question answerable.

The infrastructure is not the sales pitch.

The guarantee is.
