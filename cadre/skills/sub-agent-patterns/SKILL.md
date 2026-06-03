---
name: sub-agent-patterns
description: Load when operating as a sub-agent invoked via the agent tool — covers handoff mode detection, autonomous execution, clarification protocol, and the minimum completion report contract.
license: AGPL-3.0-or-later
---

Reference patterns for agents invoked by another agent via the `agent` tool. These patterns apply only in handoff mode; interactive sessions still use normal user-facing behaviour.

______________________________________________________________________

## Detecting handoff mode

**What it is:** Handoff mode describes an agent-to-agent invocation made through the `agent` tool rather than a human starting an interactive session.

**How it is recognised:**

- The invocation is structured as Task / Context / Constraints / Success criteria
- The prompt explicitly names an orchestrating agent such as `mission-control`
- The caller is delegating a scoped step rather than asking for open-ended help

**Key notes:**

- Handoff mode is inferred from the shape and source of the invocation, not from the task domain alone.
- When the signals are unclear, the safer interpretation is interactive mode.

______________________________________________________________________

## Autonomous execution

In handoff mode, the invocation serves as approval to execute the full assigned task.

**Key notes:**

- Progress reporting does not create extra interactive gates.
- Execution continues until the task is complete or a genuine blocker is reached.
- Genuine blockers are limited to ambiguity that prevents safe progress, environment or tooling limits, or decisions that genuinely require human judgement.
- A pause simply to ask whether to continue is outside the normal handoff pattern.

______________________________________________________________________

## Clarification protocol

Handoff mode keeps clarification inside the agent-to-agent exchange rather than turning it into a direct user question.

**Key notes:**

- Ambiguity is recorded in the structured completion report's **Blockers** field.
- The `CLARIFICATION_NEEDED` tag identifies issues the caller must resolve.
- The exact unresolved question is included so the orchestrating agent can decide whether to answer from existing context or surface it to the user.

Example:
`CLARIFICATION_NEEDED: acceptance criterion 3 is contradictory — does "immediate" mean synchronous or within one second?`

______________________________________________________________________

## Minimum completion report

Every handoff-mode execution ends with a structured report that gives the caller enough information to continue orchestration.

**Required fields:**

- **Status:** `completed` or `blocked`
- **Summary:** one sentence describing what was done or why execution stopped
- **Blockers:** `none`, or each blocker with a `CLARIFICATION_NEEDED` tag or exact error

**Key notes:**

- Agent-specific report formats may add fields.
- These three fields remain the minimum contract across handoff-mode executions.
