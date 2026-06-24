# Agent Control Plane

**Run AI workflows like governed business operations: visible, editable, auditable, replayable, and policy-safe.**

Agent Control Plane is not sold as agent infrastructure. It is sold as operational control over AI work.

The dominant signal:

> Companies do not need more chat windows. They need a cockpit where AI work can be inspected, steered, approved, replayed, and proven.

This repository implements that cockpit substrate in FARD. The user-facing product is simple: plan graph, artifact shelf, policy rails, timeline, state editor, replay. The machinery underneath is typed workflow state, policy-enforced transitions, artifact provenance, cryptographic receipts, and deterministic replay.

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

```text
state_before + input + actor + policy + tool_version
        -> policy evaluation
        -> state_after
        -> receipt digest
        -> timeline event
        -> replayable checkpoint
```

The buyer sees `Verified`, `Replay Available`, `Policy Compliant`, and `Audit Receipt Available`.

The system maintains the hard proof behind those labels.

---

## Repository contents

```text
src/acp/
  receipt.fard          canonical JSON, SHA-256 digests, state receipts, chain roots
  policy.fard           executable policy rails and violation cards
  state.fard            canonical workflow state and state patching
  artifact.fard         artifact shelf records, cards, diffs, provenance
  plan.fard             plan graph, blockers, ready nodes, risk hotspots
  timeline.fard         readable multi-agent execution timeline
  runtime.fard          policy-gated transition executor and dashboard projection
  replay.fard           deterministic replay and replay comparison
  sample_workflows.fard vendor-selection workflow demo

tests/
  test_policy.fard
  test_receipt.fard
  test_plan.fard
  test_runtime.fard
  test_replay.fard

examples/
  vendor_selection.fard

sdk/openapi/
  acp.openapi.json

docs/
  ARCHITECTURE.md
```

---

## Core model

### Workflow state

A workflow state is the canonical object of the system:

```json
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
```

Humans steer workflows by editing this state through patches. Agents do work by proposing artifacts and state patches. The runtime is the only component allowed to commit transitions.

### Policy rails

Policies are machine-readable rules. They are not prompts.

Implemented rule kinds:

```text
tool_allow       block unauthorized tools
required_role    require actor role for protected transitions
spend_limit      require approval above a threshold
data_boundary    block sensitive data to forbidden destinations
```

A failed policy check produces a violation card:

```json
{
  "ok": false,
  "code": "POLICY_DATA_BOUNDARY",
  "message": "Data boundary violation",
  "rule_id": "data.no_pii_public",
  "severity": "critical",
  "data": {
    "classification": "pii",
    "destination": "public_model"
  }
}
```

### Receipts

Every committed transition emits a state receipt:

```json
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
```

Receipts make audit and replay concrete. A dashboard can say `Verified` because state hashes and input hashes can be recomputed.

### Artifacts

Artifacts are first-class objects, not buried chat messages.

An artifact records:

```text
kind
name
media_type
content
producer
input_state_hash
tool_trace
confidence
digest
```

The UI projection is an artifact card with name, producer, confidence, digest, and tool count.

### Timeline

Timeline events are business-readable:

```text
handoff
 tool_call
 human_state_edit
 policy_violation
 state_transition
```

Each event has its own digest and can be displayed without exposing internal machinery.

### Replay

Replay takes:

```text
initial_state
policy
actor
events
tool_version
```

and returns:

```text
final_state
receipts
chain_root
```

Two replay runs are equal when their final state hash and receipt chain root match.

---

## Quickstart

Run the sample workflow:

```bash
fardrun run --program examples/vendor_selection.fard --out out/vendor_selection
cat out/vendor_selection/result.json
```

Run the tests:

```bash
fardrun run --program tests/test_policy.fard --out out/test_policy
fardrun run --program tests/test_receipt.fard --out out/test_receipt
fardrun run --program tests/test_plan.fard --out out/test_plan
fardrun run --program tests/test_runtime.fard --out out/test_runtime
fardrun run --program tests/test_replay.fard --out out/test_replay
```

Expected behavior:

```text
policy tests verify allowed transitions, denied tools, PII boundaries, and spend approval
receipt tests verify exact hash binding and tamper rejection
plan tests verify blockers and risk hotspots
runtime tests verify human state edits and policy violation preservation
replay tests verify deterministic chain equality
```

---

## Product surfaces

### Plan Graph

```fard
plan.graph(nodes, edges)
plan.ready_nodes(g)
plan.risk_hotspots(g)
```

### Artifact Shelf

```fard
artifact.make(kind, name, media_type, content, producer, input_state_hash, tool_trace, confidence)
artifact.shelf(artifacts)
artifact.diff_text(old_artifact, new_artifact)
```

### Policy Rails

```fard
policy.evaluate(policy, state, actor, input)
policy.default_enterprise_policy()
```

### State Editor

```fard
runtime.human_state_edit(actor, state, policy, patches)
state.apply_patches(state, patches)
```

### Timeline

```fard
timeline.visible(events)
```

### Replay

```fard
replay.replay(initial_state, policy, actor, events, tool_version)
replay.compare_replay(a, b)
```

### Dashboard Projection

```fard
runtime.dashboard(state, artifacts)
```

Returns the product-facing object:

```json
{
  "workflow_id": "wf_vendor_001",
  "goal": "Select a vendor with auditable approval and replay",
  "stage": "research",
  "seq": 4,
  "plan_graph": {},
  "artifact_shelf": [],
  "timeline": [],
  "policy_violations": [],
  "state_hash": "sha256:...",
  "replay_available": true
}
```

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

---

## Deployment shape

A production deployment maps directly onto these modules:

```text
control-plane-web       dashboard projection
workflow-runtime        runtime.transition / runtime.commit_artifact
policy-service          policy.evaluate
state-service           state patches and checkpoints
artifact-service        artifact shelf and object storage
receipt-service         receipt generation and verification
timeline-service        timeline projection
replay-service          replay and comparison
tool-gateway            governed external tool invocation
agent-worker-pool       specialized agents that propose patches and artifacts
```

Recommended backing services:

```text
Postgres/FoundationDB    canonical workflow state
S3/MinIO                 artifact bodies
Kafka/NATS               event stream
OpenTelemetry            infrastructure traces
OPA/Cedar/custom FARD    enterprise policy rules
KMS/Vault                keys and secrets
Anka/EOS                 claims, witnesses, challenges, mesh publication
```

---

## Selling line

> Agent Control Plane turns AI work from opaque chat into governed operations: every plan visible, every artifact tracked, every policy enforced, every state editable, every decision replayable.

The infrastructure is not the sales pitch.

The guarantee is.
