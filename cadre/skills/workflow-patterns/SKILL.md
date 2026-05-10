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

## Requirement elicitation

**When:** The request is vague, the problem isn't fully understood, or the user hasn't described acceptance criteria. Run this before any execution workflow when requirements are unclear.

**Chain:**

```
[problem-analyser] → [user-story-writer]
```

| Step | Agent               | Hand it                                  | Success                                                                                   |
| ---- | ------------------- | ---------------------------------------- | ----------------------------------------------------------------------------------------- |
| 1    | `problem-analyser`  | The raw request as the user expressed it | An approved problem analysis: goal, subproblem decomposition, contradictions, NFRs, risks |
| 2    | `user-story-writer` | The approved problem analysis            | An approved story set: INVEST-scored stories with AC, dependency diagram, risk/value/size |

**Notes:**

- Run both steps in sequence. The `user-story-writer` input is the `problem-analyser` output — do not skip step 1.
- Do not run either agent when requirements are already clear and testable — it adds no value and creates friction.
- If the user is technical and has expressed the requirement as a clear story with acceptance criteria, skip both and go directly to feature delivery.

**Failure handling:** If the user cannot answer the Impact Mapping questions (especially "Why?"), the work should not start. Surface the missing goal as a blocker and stop.

______________________________________________________________________

## Feature delivery

**When:** The user wants to add new behaviour — a feature, user story, or acceptance criterion.

**Precondition check:** Before starting, confirm the development environment is ready. If the test runner is not configured or baseline tests are not passing, run the `devex` step first. If requirements are vague, run the `analyst` step first.

**Chain:**

```
[problem-analyser?] → [user-story-writer?] → [devex?] → [atdd] → [refactor?]
```

| Step         | Agent               | Hand it                                                               | Success                                                                 |
| ------------ | ------------------- | --------------------------------------------------------------------- | ----------------------------------------------------------------------- |
| 0 (optional) | `problem-analyser`  | The raw request; project README                                       | Approved problem analysis: goal, subproblems, contradictions, NFRs      |
| 1 (optional) | `user-story-writer` | Approved problem analysis                                             | Approved stories with AC, INVEST scores, risk/value/size                |
| 2 (optional) | `devex`             | Project language/framework; what tooling is needed                    | Test runner works; `make test` or equivalent passes cleanly             |
| 3            | `atdd`              | User story + acceptance criteria; relevant source files; test command | Acceptance test passes; new behaviour works end-to-end; committed       |
| 4 (optional) | `refactor`          | Files changed in step 3; passing test suite; complexity baseline      | No method over CC 10; no new SRP violations; metrics stable or improved |

**Notes:**

- The `atdd` agent's internal Refactor phase covers local cleanup of the code written in the Green phase — making the new code readable and principle-compliant. The optional post-feature `refactor` step (step 3) is for broader structural review: god classes introduced, coupling increased, metrics degraded. Only run step 3 if cyclomatic complexity or coupling metrics degraded measurably during step 2.
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

## Documentation

**When:** The user wants to write or update user-facing documentation — README, CLI reference, tutorial, how-to guide, or explanation.

**Chain:**

```
[technical-author]
```

| Step | Agent              | Hand it                                                                                  | Success                                             |
| ---- | ------------------ | ---------------------------------------------------------------------------------------- | --------------------------------------------------- |
| 1    | `technical-author` | The brief (what to document), relevant source files or diff, any existing docs to update | Documentation written, examples verified, committed |

**Notes:**

- `technical-author` writes documentation. `docs-reviewer` finds gaps in existing documentation. Do not conflate them.
- If the brief is vague ("write docs for the project"), ask the user which document type is needed (tutorial, how-to, reference, explanation) before delegating.
- If a code change is in flight and documentation must accompany it, sequence `atdd` first, then `technical-author` with the changed source files as context.
- `technical-author` does not modify source code. If it discovers that `--help` output or source behaviour needs to change, it surfaces that as a separate task.

**Failure handling:** If `technical-author` cannot verify an example because the tool is not installed or the environment is not set up, delegate to `devex` first, then retry.

______________________________________________________________________

## Review and audit

**When:** The user wants to review code, assess quality, check a PR, or audit for a specific concern — no changes required immediately.

**Chain options:**

```
[reviewer]                    — quick general review
[full-reviewer]               — all specialist reviewers in parallel
[test-reviewer]               — test quality only
[security-reviewer]           — security vulnerabilities only
[api-reviewer]                — API/CLI interface design only
[error-handling-reviewer]     — error handling only
[observability-reviewer]      — logging, metrics, tracing only
[dependency-reviewer]         — dependency changes only
[docs-reviewer]               — documentation coverage and accuracy only
[concurrency-reviewer]        — concurrency correctness only
[dead-code-detector]          — unreachable code, stale flags, unused exports, orphaned files
```

| User signal                                                 | Route to                                                |
| ----------------------------------------------------------- | ------------------------------------------------------- |
| "review this", "check this PR", "look at this code"         | `reviewer` — general review with adaptive skill loading |
| "full review", "comprehensive review", "review everything"  | `full-reviewer` — all specialists in parallel           |
| "check the tests", "are these tests good"                   | `test-reviewer`                                         |
| "security review", "any vulnerabilities", "check for vulns" | `security-reviewer`                                     |
| "review the API", "check the CLI flags"                     | `api-reviewer`                                          |
| "error handling", "are errors handled correctly"            | `error-handling-reviewer`                               |
| "logging", "observability", "are we logging enough"         | `observability-reviewer`                                |
| "dependency audit", "check the deps"                        | `dependency-reviewer`                                   |
| "check the docs", "documentation coverage"                  | `docs-reviewer`                                         |
| "race conditions", "concurrency", "goroutine leaks"         | `concurrency-reviewer`                                  |
| "dead code", "unused code", "stale flags", "orphaned files" | `dead-code-detector`                                    |

**Notes:**

- Review agents are read-only. They never modify code, write files, or make commits.
- Each agent defaults to `git diff HEAD~1` as the review scope when no scope is specified.
- Pass the git ref, file path, or diff explicitly when reviewing something other than the most recent commit.
- `full-reviewer` launches all specialists simultaneously and synthesises findings into one report. It runs a pre-flight step to install missing analysis tools before launch.
- If the development environment is not yet set up (language runtime missing, core tools absent), run `devex` first — then re-run the review with all tooling available.

**Failure handling:** If a review agent reports Blocking findings, surface them immediately and ask the user whether they want to address the findings before proceeding with any planned work.

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

1. **Clarity before planning.** A vague brief produces a vague plan. If the requirement is unclear, run `problem-analyser` then `user-story-writer` before building a plan. Do not plan against ambiguity.
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
