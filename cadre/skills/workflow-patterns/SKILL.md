---
name: workflow-patterns
description: Agent-chain templates and routing logic for mission-control. Maps task archetypes to ordered sequences of specialist agents, with handoff context and success criteria for each step. Load this skill when planning and delegating work.
license: AGPL-3.0-or-later
allowed-tools: read
---

# Workflow Patterns

Each pattern describes a task archetype, the ordered agent chain to execute it, what to hand each agent, and what success looks like at each step.

These are templates, not rules. Adapt based on what you find in the codebase. The right workflow is the simplest one that delivers the required outcome safely.

______________________________________________________________________

## Feature delivery

**When:** The user wants to add new behaviour — a feature, user story, or acceptance criterion.

**Precondition check:** Before starting, confirm the development environment is ready. If the test runner is not configured or baseline tests are not passing, run the `devex` step first.

**Chain:**

```
[devex?] → [atdd] → [refactor?]
```

| Step         | Agent      | Hand it                                                               | Success                                                                 |
| ------------ | ---------- | --------------------------------------------------------------------- | ----------------------------------------------------------------------- |
| 0 (optional) | `devex`    | Project language/framework; what tooling is needed                    | Test runner works; `make test` or equivalent passes cleanly             |
| 1            | `atdd`     | User story + acceptance criteria; relevant source files; test command | Acceptance test passes; new behaviour works end-to-end; committed       |
| 2 (optional) | `refactor` | Files changed in step 1; passing test suite; complexity baseline      | No method over CC 10; no new SRP violations; metrics stable or improved |

**Notes:**

- The `atdd` agent's internal Refactor phase covers local cleanup of the code written in the Green phase — making the new code readable and principle-compliant. The optional post-feature `refactor` step (step 2) is for broader structural review: god classes introduced, coupling increased, metrics degraded. Only run step 2 if cyclomatic complexity or coupling metrics degraded measurably during step 1.
- If the story touches untested legacy code, insert a `legacy-code` step before `atdd`.

**Failure handling:** If `atdd` cannot make the acceptance test pass, STOP. Do not run the optional refactor step. Report the failing test and the implementation state to the user.

______________________________________________________________________

## Structural improvement

**When:** The user wants to improve code quality without changing behaviour — refactoring, tidying, cleaning up.

**Precondition check:** Tests must pass before any structural work begins. If coverage is thin, the `legacy-code` step is mandatory, not optional.

**Chain:**

```
[legacy-code?] → [refactor]
```

| Step         | Agent         | Hand it                                                                     | Success                                                   |
| ------------ | ------------- | --------------------------------------------------------------------------- | --------------------------------------------------------- |
| 0 (optional) | `legacy-code` | Files to be refactored; change intent                                       | Characterisation tests passing; seams identified          |
| 1            | `refactor`    | Files to improve; passing test suite; complexity baseline; any smell report | Metrics improved; no behaviour change; committed per step |

**Notes:**

- If the code under improvement has reasonable test coverage, skip step 0 entirely.
- The `refactor` agent applies Kent Beck's Tidy First: structure commits separate from any behaviour commits.

**Failure handling:** If `refactor` finds no passing tests and `legacy-code` is not in the chain, STOP and add `legacy-code` as step 0 before proceeding.

______________________________________________________________________

## Legacy rescue

**When:** The user needs to modify code that lacks tests, or the code is so tightly coupled that safe modification requires dependency-breaking first.

**Chain:**

```
[legacy-code] → [atdd] → [refactor]
```

| Step | Agent         | Hand it                                                                                  | Success                                                                       |
| ---- | ------------- | ---------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------- |
| 1    | `legacy-code` | Files to be changed; what the change needs to achieve; language/framework                | Seams identified; characterisation tests passing; dependency-breaking applied |
| 2    | `atdd`        | User story or bug description; seams from step 1; characterisation tests as the baseline | New behaviour tested and passing; legacy code modified safely                 |
| 3    | `refactor`    | Changed files; full test suite passing                                                   | Structure improved; no regressions; metrics stable or better                  |

**Notes:**

- This is the most conservative chain. Use it whenever test coverage is absent or thin and the change is non-trivial.
- Do not compress steps 1 and 2 — the seam work must precede the behavioural change.

**Failure handling:** If `legacy-code` cannot identify seams, STOP. Do not proceed to `atdd`. Report which dependencies are blocking and ask the user whether a larger rewrite is appropriate.

______________________________________________________________________

## Environment setup

**When:** The user needs to install tools, configure the development environment, set up linters, formatters, test runners, or CI.

**Chain:**

```
[devex]
```

| Step | Agent   | Hand it                                                                            | Success                                                                                  |
| ---- | ------- | ---------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------- |
| 1    | `devex` | Language/framework; what needs to be installed or configured; any existing tooling | Tools installed and working; configuration committed; `make test` (or equivalent) passes |

**Notes:**

- `devex` operates entirely within the development environment — it does not modify application source code.
- If environment setup is a prerequisite for feature delivery, run this chain first and then proceed with the feature delivery chain.

**Failure handling:** If `devex` cannot install a required tool, STOP and report the exact error. Do not attempt workarounds that would change application code.

______________________________________________________________________

## Review and audit

**When:** The user wants to understand, assess, or review existing code — no changes required immediately.

**Handle inline** (mission-control does not need to delegate):

1. Use `read` and `search` to explore the relevant files.
1. Produce a structured assessment: what the code does, how it's structured, what risks or quality issues exist, what the recommended next action is.
1. If the review reveals work that warrants a full workflow, propose the appropriate chain and ask for approval to proceed.

**When to delegate instead:**

- Scope is large (multiple modules, whole codebase) → propose a plan with appropriate agents
- User asks for a specific type of review (security, accessibility, observability) → handle inline with focused investigation

______________________________________________________________________

## Investigation

**When:** The user wants to understand existing code, debug a problem, or answer a question.

**Handle inline** — no delegation needed. Use `read` and `search` to explore, `execute` to run diagnostics.

**Produce:** A clear explanation of what the code does, why the problem occurs, or the answer to the question. If investigation reveals work that warrants a workflow, propose the appropriate chain.

______________________________________________________________________

## Debug

**When:** A test is failing and the user needs it fixed.

**Chain:** Investigate inline first.

- If the failure is in tested code → fix inline and verify with `execute`.
- If the failure is in untested code → `legacy-code` → fix → verify.
- If the failure is environmental → `devex`.

______________________________________________________________________

## Sequencing rules

Apply these when composing or adapting chains:

1. **Environment before feature.** A broken test suite poisons every subsequent step. Always confirm the environment is working before writing new code.
1. **Safety net before structure.** Never run `refactor` on code with no passing tests. Insert `legacy-code` first.
1. **Behaviour before cleanup.** In legacy rescue, establish the seams and characterisation tests before writing new behaviour. Don't refactor and add features simultaneously.
1. **One archetype per step.** Each agent step has one concern. If a step is doing two things, split it.
1. **Confirm before executing.** Show the full chain to the user before invoking the first agent. A plan the user can see is a plan they can correct.
1. **A failed step poisons its successors.** Never run step N+1 if step N has not completed successfully.

______________________________________________________________________

## Handoff context template

When invoking an agent, always provide:

```
Task: [what this agent needs to accomplish]
Context: [relevant files, recent test output, previous agent's summary]
Constraints: [what must not change, performance budgets, API contracts]
Success criteria: [what done looks like for this step]
```

The more precise the handoff, the better the specialist agent performs. Vague handoffs produce vague results.
