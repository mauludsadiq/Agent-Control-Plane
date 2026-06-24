# Architecture

Agent Control Plane is divided into product surfaces and invariant services.

## Product surfaces

The UI surfaces are plan graph, artifact shelf, timeline, policy rails, approval queue, state editor, and replay controls. These are what operators buy and use.

## Invariant services

The invariant services are state, policy, artifact, receipt, runtime, and replay.

No agent directly mutates state. Agents submit proposed inputs. The runtime evaluates policy, applies patches, emits timeline events, and creates receipts.

## Transition invariant

For any committed transition:

```text
receipt.state_before_hash == digest(state_before)
receipt.input_hash        == digest(input)
receipt.state_after_hash  == digest(state_after)
receipt.digest            == digest(receipt body without digest)
```

This makes workflow history auditable.

## Policy invariant

For any transition with `ok: true`, `policy.evaluate(...).ok == true` at commit time.

For any policy violation, the violation is added to state and timeline without applying the forbidden state patch.

## Replay invariant

Given the same initial state, policy, actor, event list, and tool version, replay returns the same final state hash and chain root.

## Agent contract

An agent worker receives:

```json
{
  "task_id": "task_123",
  "state_hash": "sha256:...",
  "allowed_tools": [],
  "policy_context": {},
  "artifact_inputs": [],
  "expected_output_schema": "scorecard"
}
```

An agent worker returns:

```json
{
  "artifact": {},
  "claims": [],
  "confidence": 91,
  "tool_trace": [],
  "state_patch": []
}
```

The runtime, not the agent, decides whether this return value may be committed.
