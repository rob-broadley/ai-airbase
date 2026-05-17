---
name: full-review
description: Load before running a comprehensive review. Covers scope resolution, pre-flight tool checks, parallel specialist dispatch, synthesis, and output format.
license: AGPL-3.0-or-later
allowed-tools: agent, execute, read, search
---

## Pre-flight

Before launching, check whether key analysis tools are present for the detected project language (use the `tool-install` skill for guidance). If any are missing, invoke `bootstrap` first.

If bootstrap fails, stop immediately. Surface the exact error to the user and do not proceed to scope resolution or specialist dispatch.

After completing pre-flight (whether or not bootstrap was needed), emit a one-line status to the user: either "Pre-flight: all required tools present" or "Pre-flight: bootstrap completed — [tools installed]" before proceeding to scope resolution.

## Scope resolution

- **Git ref provided:** use it as the scope for all specialists.
- **No ref given:** run `execute git diff HEAD~1 --stat` and use `HEAD~1`.
- **Full-codebase review:** run `execute git ls-files` to build a file manifest; pass it to each specialist and tell them to read files directly rather than use `git diff`.

Before emitting any `agent` calls, state the resolved scope to the user — the git ref, commit range, or file count — and ask the user to confirm this is the intended scope. Wait for explicit user acknowledgement before proceeding to parallel dispatch. Do not emit any `agent` calls until that confirmation is received. This gate survives handoff mode — do not skip it when invoked by another agent unless the invoking context explicitly specifies the git ref as the intended scope.

## Parallel dispatch

Do not begin dispatch until pre-flight is complete. If bootstrap was required, it must have succeeded before any specialist agent is launched.

Before making any `agent` tool calls, prepare the full handoff context for all specialists listed above. Then emit all calls **in a single response turn** — parallel execution is the entire point. Pass each agent the review scope, project context, and instruction to produce findings in the standard severity format.

1. `test-reviewer` — test quality and coverage
1. `security-reviewer` — security vulnerabilities and hygiene
1. `api-reviewer` — API and CLI interface quality
1. `error-handling-reviewer` — error handling completeness
1. `observability-reviewer` — logging, metrics, and trace quality
1. `dependency-reviewer` — dependency changes and vulnerability scan
1. `docs-reviewer` — documentation coverage and accuracy
1. `concurrency-reviewer` — race conditions and concurrency correctness
1. `dead-code-detector` — unreachable code, unused exports, stale flags, orphaned files

## Synthesis

In your dispatch response, write out the expected agent names as a tracking list. As each completion notification arrives, immediately **record** that agent's findings: update the tracking list, buffer the raw findings, and mark the agent complete. Do not deduplicate or merge at this stage — recording is distinct from synthesis. Do not wait passively for all agents. Do not use a sub-agent to collect or aggregate results — sub-agents have isolated contexts and cannot read sibling agents' results.

If an agent's response matches the error pattern — a single sentence stating the review could not be completed due to a tool failure, skill-loading error, or access error — treat that agent as having failed rather than completed. Label it "did not complete" in the tracking list (not as a successful completion), include it in the user-facing warning naming every agent that did not complete and which review dimension is consequently absent, and exclude it from the deduplication and merge steps. Do not surface its one-sentence error as a finding.

Wait for all completion notifications before beginning **synthesis**. Do not cut off a slow agent just because the others have finished first. However, apply the following exit condition: if 3 consecutive turns pass with no new completion notification and at least one agent has not yet notified, treat the wait as timed out. Label each non-notifying agent "did not complete" in your tracking list, then emit a proactive user-facing warning that names every agent that did not complete, describes which review dimensions are consequently absent from this report, and offers to re-run those agents before finalising the report. This exit condition ensures a consistent policy regardless of which orchestrator invokes the skill.

Once all notifications have been received, or the exit condition has fired, proceed to **synthesis** — the gated final step of deduplication and merging. Synthesis must not begin until the all-complete or exit-condition check is satisfied.

1. **Deduplicate** — a finding appearing in multiple reports appears once, attributed to the agent whose domain most directly owns the finding type (e.g., a security finding is attributed to `security-reviewer` regardless of arrival order); note corroborators.
1. **Merge by severity** using this unified scale:

| Source agent severity             | Unified tier   |
| --------------------------------- | -------------- |
| security-reviewer: Critical, High | Blocking       |
| security-reviewer: Medium         | Recommendation |
| security-reviewer: Low            | Observation    |
| All other agents: Blocking        | Blocking       |
| All other agents: Recommendation  | Recommendation |
| All other agents: Observation     | Observation    |

3. **Verify** every finding that cites a specific file or location — confirm the file exists, the location is correct, and the described issue is visible. Drop unconfirmed findings and note the count.

## Output format

Produce a single structured report as markdown in the conversation:

```
## Full Review — [ref or description]

### Summary table

| Agent                   | Blocking | Recommendations | Observations |
| ----------------------- | -------- | --------------- | ------------ |
| test-reviewer           | N        | N               | N            |
| security-reviewer       | N        | N               | N            |
| api-reviewer            | N        | N               | N            |
| error-handling-reviewer | N        | N               | N            |
| observability-reviewer  | N        | N               | N            |
| dependency-reviewer     | N        | N               | N            |
| docs-reviewer           | N        | N               | N            |
| concurrency-reviewer    | N        | N               | N            |
| dead-code-detector      | N        | N               | N            |
| **Total**               | **N**    | **N**           | **N**        |

> Agents that did not complete: replace their numeric cells with `—` and exclude them from the Total row.

> N findings dropped after verification (unconfirmed against source). Omit this note if zero were dropped.

### Blocking
### Recommendations
### Observations
```

Each finding:

```
**FR-NN [SEVERITY] location** (file:line) — raised by: [agent-name]
Description: what the issue is.
Why it matters: the consequence if left unaddressed.
Direction: the general approach to resolution — not an implementation.
Notes: [optional — domain-specific metadata from the source agent, e.g. property violated, taxonomy category]
```

Where `NN` is a zero-padded sequential number assigned during synthesis (FR-01, FR-02, … FR-NN), ordered Blocking → Recommendations → Observations after deduplication. Also include a "Ref" column in the summary table showing the FR-NN range for each agent's findings:

| Agent         | Refs        | Blocking | Recommendations | Observations |
| ------------- | ----------- | -------- | --------------- | ------------ |
| test-reviewer | FR-NN–FR-NN | N        | N               | N            |
