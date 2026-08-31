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
- A pause simply to ask whether to continue is outside the normal handoff pattern unless the agent definition explicitly declares a user decision gate for that point.

______________________________________________________________________

## Explicit user decision gates

User gates are opt-in. Declare each gate at the point of use in the owning agent or skill; do not add generic gates around delegated steps.

The agent that owns a transition owns its gate and must have `question: allow`. A gate declaration must state:

- The question and decision it resolves
- The summary, changed files, verification, and open risks to present
- The next step authorized by approval
- Any common context-specific alternative
- How custom text is handled

Use the built-in `question` tool. Declare one approval option unless a common alternative, such as skipping an optional phase, warrants a second. Its built-in custom-answer path collects feedback and other instructions; do not declare a generic feedback option.

Keep prompt context brief. Do not reproduce the artefact, diff, or review report. Include the result summary, changed files, relevant verification, and open risks or questions.

On custom text, apply the instruction to the current decision and show the gate again unless it explicitly chooses an alternative or cancellation. Do not advance on feedback alone. If it is ambiguous, ask a focused clarification question.

If the user asks to stop or pause through custom text, do not start the next mutating action. Leave existing changes intact, do not commit without commit approval, and report the current phase, completed work, verification, changed files, and unstarted next step.

User gates replace mandatory internal review loops in delivery workflows. Report work status and risks, but do not dispatch review agents or perform review-only checks unless the user explicitly requests a review or the current task requires a specific verification command.

______________________________________________________________________

## Clarification protocol

When an orchestrating agent needs information to make a safe decision, it asks the user directly with the built-in `question` tool. Ask one focused question, state why the answer matters, and offer a proposed default where appropriate.

Use a `CLARIFICATION_NEEDED` blocker only when the agent cannot ask the user, or the user has stopped or paused the workflow. Include the exact unresolved question and why it blocks progress.

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
