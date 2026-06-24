# Agent Control Plane

**Run AI workflows like governed business operations: visible, editable, auditable, replayable, and policy-safe.**

Agent Control Plane is not sold as agent infrastructure. It is sold as operational control over AI work.

The dominant signal:

> Companies do not need more chat windows. They need a cockpit where AI work can be inspected, steered, approved, replayed, and proven.

This repository implements that cockpit substrate in FARD. The user-facing product is simple: plan graph, artifact shelf, policy rails, timeline, state editor, replay. The machinery underneath is typed workflow state, policy-enforced transitions, artifact provenance, cryptographic receipts, deterministic replay, incremental state storage, workflow branching, and async policy gates.

---

## What is being sold

### The buyer-facing promise

Agent Control Plane lets an organization answer six operational questions:

1. **What is the AI team doing?**
   The Plan Graph shows task decomposition, dependencies, assigned agents, and risk hotspots.

2. **What did it produce?**
   The Artifact Shelf turns outputs into first-class, versioned objects with provenance and confidence.

3. **What is it allowed to do?**
   Policy Rails enforce rules as executable constraints, not prompt instructions.

4. **How did it get here?**
   The Timeline records agent handoffs, tool calls, human edits, policy violations, and state commits.

5. **How can a human steer it without prompt gymnastics?**
   The State Editor applies validated state patches and commits them as receipted transitions.

6. **Can the decision be replayed and audited?**
   Replay re-executes from checkpoints and compares state hashes and receipt chains.

### The infrastructure promise

Every material action is a typed, policy-checked, receipted state transition:

    state_before + input + actor + policy + tool_version
            -> policy evaluation
            -> state_after
            -> receipt digest
            -> timeline event
            -> replayable checkpoint

The buyer sees Verified, Replay Available, Policy Compliant, and Audit Receipt Available.

The system maintains the hard proof behind those labels.

---

## Repository contents

    src/acp/
      receipt.fard          canonical JSON, SHA-256 digests, linked receipt chain, chain verification
      policy.fard           executable policy rails, violation cards, rule scoping, policy composition
      state.fard            canonical workflow state, patch paths, var. prefix convention
      artifact.fard         artifact shelf records, cards, digest-based diffs, provenance
      plan.fard             plan graph, blockers, ready nodes, risk hotspots
      timeline.fard         readable multi-agent execution timeline
      runtime.fard          policy-gated transition executor and dashboard projection
      replay.fard           deterministic replay, chain verification, replay comparison
      control_plane.fard    store-backed workflow lifecycle, artifact commit, dashboard, replay
      delta.fard            incremental state storage, delta chain verification, reconstruction
      lineage.fard          workflow branching, counterfactual replay, branch comparison
      gate.fard             async policy gates, token-based resumption, expiry
      sample_workflows.fard vendor-selection workflow demo

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

    examples/
      vendor_selection.fard

    sdk/openapi/
      acp.openapi.json

    docs/
      ARCHITECTURE.md

Total: 1,552 lines across 23 files. 43 tests, zero failures.

---

## Core model

### Workflow state

A workflow state is the canonical object of the system:

    {
      "schema": "acp.workflow_state.v1",
      "workflow_id": "wf_vendor_001",
      "goal": "Select a vendor with auditable approval and replay",
      "stage": "research",
      "seq": 3,
      "plan": { "nodes": [], "edges": [] },
      "artifacts": [],
      "approvals": [],
      "violations": [],
      "timeline": [],
      "variables": {},
      "final_decision": null
    }

Humans steer workflows by editing this state through patches. Agents do work by proposing artifacts and state patches. The runtime is the only component allowed to commit transitions.

### Patch paths

State patches use explicit paths. Unknown paths surface as patch_error — no silent corruption.

    "stage"              workflow stage string
    "goal"               workflow goal string
    "owner"              workflow owner string
    "plan"               full plan graph replacement
    "variables"          full variables record replacement
    "final_decision"     final decision record
    "approvals"          appends one approval record (governed audit trail)
    "var.<key>"          sets a single key inside variables

### Policy rails

Policies are machine-readable rules. They are not prompts.

Implemented rule kinds:

    tool_allow       block unauthorized tools
    required_role    require actor role — scopeable to specific tools via applies_to
    spend_limit      require approval above a threshold
    data_boundary    block sensitive data to forbidden destinations

A failed policy check produces a violation card:

    {
      "ok": false,
      "code": "POLICY_DATA_BOUNDARY",
      "message": "Data boundary violation",
      "rule_id": "data.no_pii_public",
      "severity": "critical",
      "data": { "classification": "pii", "destination": "public_model" }
    }

Policy composition:

    policy.merge_policies(base, overlay)          combine two policies, rules appended
    policy.scope_rule(rule, applies_to)           limit a rule to specific tools
    policy.extend_tool_allowlist(policy, id, tools)  add tools without replacing base rule

### Receipts

Every committed transition emits a state receipt:

    {
      "receipt_type": "acp.state_transition.v1",
      "workflow_id": "wf_vendor_001",
      "seq": 4,
      "kind": "human_state_edit",
      "actor": "manager:ops",
      "state_before_hash": "sha256:...",
      "input_hash": "sha256:...",
      "policy_version": "ACP-POLICY-1.0.0",
      "policy_digest": "sha256:...",
      "tool_version": "human",
      "tool_digest": "sha256:...",
      "artifact_digests": [],
      "state_after_hash": "sha256:...",
      "digest": "sha256:..."
    }

Receipt verification checks state_before_hash, input_hash, state_after_hash, and body digest.
The receipt chain is a linked hash chain from genesis — insertion or reordering of any receipt
breaks the root. Use receipt.verify_chain(receipts, chain_root) to confirm integrity.

### Artifacts

Artifacts are first-class objects, not buried chat messages.

An artifact records kind, name, media_type, content, producer, input_state_hash, tool_trace,
confidence, and digest. Content comparison uses digest equality — type-safe for any content type.

### Timeline

Timeline events are business-readable:

    handoff
    tool_call
    human_state_edit
    policy_violation
    state_transition

Each event has its own digest. timeline.visible() strips internal data for UI projection
without affecting the stored events used for verification.

### Replay

Replay takes initial_state, policy, actor, events, and tool_version and returns final_state,
receipts, and chain_root. On policy violation, the fold halts cleanly — the violation receipt
is not appended to the chain.

replay.compare_replay(a, b) returns:

    {
      "equal": bool,         both ok, same state hash, same chain root
      "both_ok": bool,
      "same_state": bool,
      "same_chain": bool,
      "a_ok": bool,
      "b_ok": bool,
      "a_hash": "sha256:...",
      "b_hash": "sha256:...",
      "a_chain_root": "sha256:...",
      "b_chain_root": "sha256:..."
    }

### Delta storage

Deltas record patch sets instead of full state blobs. Storage cost is O(patches) not O(state_size).
The baseline (seq: 0) is stored as a full state; all subsequent seqs are stored as deltas.

    delta.from_transition(result, patches, kind, actor_id)
    delta.apply_deltas(baseline, deltas, max_seq)     pass -1 for all
    delta.reconstruct_at(baseline, deltas, target_seq)
    delta.verify_delta_chain(baseline_hash, deltas)
    delta.summarize(deltas)                           compact audit log, no patch payloads
    delta.storage_report(deltas)

Reconstructed states reflect logical fields (stage, variables, plan, artifacts, approvals).
Timeline events are runtime-only and are not reconstructed from deltas.

### Workflow branching

Lineage supports counterfactual replay without overwriting the original audit trail.

    lineage.fork(parent_state, new_workflow_id, branch_point_seq, reason)
    lineage.fork_from_replay(initial_state, policy, actor, events, tool_version, branch_point_seq, new_id, reason)
    lineage.counterfactual(forked_state, policy, actor, events, tool_version)
    lineage.compare_branches(original, counterfactual_result)
    lineage.ancestry(state)

fork resets timeline and violations. Logical state (stage, variables, plan, artifacts, approvals)
is inherited. compare_branches uses logical_converged for outcome comparison and chains_independent
to confirm cryptographic separation.

### Async policy gates

Gates suspend a workflow pending external resolution without blocking the runtime.

    gate.open(actor, state, policy, input, gate_kind, deadline_seq, context)
    gate.resume(actor, gated_state, policy, tool_version, token, resolution)
    gate.pending_gate(state)
    gate.is_expired(gate, current_seq)

open transitions stage to "gated" and stores the gate in var.gate with a deterministic token.
resume validates the token, adds the approval to state, re-evaluates policy (no bypass),
and commits the original transition if policy now passes. resolution is "approved" or "rejected".
Expiry is seq-based for deterministic replay.

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

Expected: 43 passed, 0 failed.

---

## Product surfaces

### Plan Graph

    plan.graph(nodes, edges)
    plan.ready_nodes(g)
    plan.risk_hotspots(g)
    plan.mark_node(g, id, status)

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

### Dashboard Projection

    runtime.dashboard(state, artifacts)

Returns:

    {
      "workflow_id": "wf_vendor_001",
      "goal": "...",
      "stage": "research",
      "seq": 4,
      "plan_graph": {},
      "artifact_shelf": [],
      "timeline": [],
      "policy_violations": [],
      "state_hash": "sha256:...",
      "replay_available": true
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
    tool-gateway            governed external tool invocation
    agent-worker-pool       specialized agents that propose patches and artifacts

Recommended backing services:

    Postgres/FoundationDB    canonical workflow state and delta log
    S3/MinIO                 artifact bodies
    Kafka/NATS               event stream and gate callback queue
    OpenTelemetry            infrastructure traces
    OPA/Cedar/custom FARD    enterprise policy rules
    KMS/Vault                keys, secrets, and gate token signing
    Anka/EOS                 claims, witnesses, challenges, mesh publication

---

## Selling line

> Agent Control Plane turns AI work from opaque chat into governed operations: every plan visible, every artifact tracked, every policy enforced, every state editable, every decision replayable, every branch auditable, every approval gateable.

The infrastructure is not the sales pitch.

The guarantee is.
