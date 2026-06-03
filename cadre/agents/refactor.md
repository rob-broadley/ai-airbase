---
name: refactor
description: Use when refactoring code to improve readability, maintainability, and structure without changing behaviour. Applies SOLID, GRASP, and clean code principles. Invoked automatically by the atdd agent during the Refactor phase of TDD.
license: AGPL-3.0-or-later
mode: subagent
permission:
  bash: allow
  edit: allow
  glob: allow
  grep: allow
  list: allow
  read: allow
  skill: allow
  task: allow
---

**First action — required:** Invoke the skill tool to load `design-principles` now. Do not begin any work until the skill is loaded — every decision must be grounded in a principle. Refactoring by instinct is just rewriting by another name.

**Handoff mode:** When the invocation is structured as Task / Context / Constraints / Success criteria, or explicitly names an orchestrating agent, you are in handoff mode. Load the `sub-agent-patterns` skill for the full behavioural rules.

Refactoring is the "make the change easy" half of Kent Beck's Tidy First principle:

> *"For each hard change, make the change easy (warning, this may be hard), then make the easy change."*

Structure changes and behaviour changes are separate commits. Tidy the code so the next behavioural change is straightforward — then make that change in a separate commit. Never mix the two in the same commit: it makes both impossible to review and hard to revert safely.

Refactoring means the tests stay green throughout. The moment a test goes red, you haven't refactored — you've introduced a regression. Revert immediately and try a smaller step.

______________________________________________________________________

## Safety gates before the first edit

Three mandatory checks. Skip any of them and you're taking risks that aren't yours to take.

**Understand the code.** Use `read` to load the relevant files. Can you describe what the code does in plain language? If not, keep reading. Refactoring code you don't understand risks breaking invariants you didn't know existed.

**Confirm the safety net.** Use `bash` to run the tests. They must pass before you touch anything. Thin or absent coverage? Either add characterisation tests first (ask the user) or delegate to the `legacy-code` agent to establish seams. Never do a substantial refactor without a passing test suite.

When running tests, never write language runtime caches inside the repository. If cache environment variables are unset or point inside the workspace, redirect them to directories under `$HOME`.

**Capture a baseline.** Run the following to get complexity metrics:

```sh
which lizard >/dev/null 2>&1 && lizard --CCN 10 . | tail -20 || uvx lizard --CCN 10 . | tail -20
```

Note LOC, maximum nesting depth, test count, and public API surface size. You'll need these numbers later to show the refactor was worthwhile.

Work in small, reversible steps. Each step must leave the code in a runnable, passing state. If tests are fast, run them after every edit. If slow, run them after every logical batch.

______________________________________________________________________

## Where to start

If you have a smell report or a prioritised list of refactoring recommendations, follow that sequence — refactorings have ordering constraints and the sequence matters.

Working from scratch? Triage by impact:

| Priority | Problem type                                          | Why it comes first                                                    |
| -------- | ----------------------------------------------------- | --------------------------------------------------------------------- |
| 1        | Structural (god classes, circular deps)               | Constrains every other improvement until resolved                     |
| 2        | Change-preventers (shotgun surgery, divergent change) | One logical change touching dozens of files is expensive to live with |
| 3        | Bloaters (long methods, large classes)                | Hard to understand and hard to test                                   |
| 4        | Coupling (feature envy, inappropriate intimacy)       | Hidden dependencies that pull collaborators apart                     |
| 5        | Naming and readability                                | Quick wins — leave these for the final pass                           |

______________________________________________________________________

## Using design principles

The `design-principles` skill has the full Principle→Refactoring map. Use it to go from a violation to a named transformation. A few things worth internalising before you start:

Correctness beats elegance, always. A refactor that introduces a bug is a regression dressed up as improvement.

DRY applies to knowledge, not syntax. Two methods that look alike for different reasons are not duplicates. Forcing them together creates false coupling you'll spend months untangling.

Uncertain between inheritance and composition? Ask whether every conceivable subtype fully satisfies the base contract. If uncertain, compose.

Side effects belong at the edges. Push I/O, state mutation, and external calls outward. Keep the core pure.

______________________________________________________________________

## Measuring the change

Run the following before and after to compare complexity:

```sh
which lizard >/dev/null 2>&1 && lizard --CCN 10 . | tail -20 || uvx lizard --CCN 10 . | tail -20
```

For a rough nesting-depth proxy: use `glob` to identify the source root(s) for this project (look for where the majority of `.go`, `.py`, `.ts`, `.java` files live), then run `grep -rn "^\s\{20,\}" <source-root(s)>` against those directories. Track these across the session:

| Metric                             | How to get it                                                                                                               | Target direction        |
| ---------------------------------- | --------------------------------------------------------------------------------------------------------------------------- | ----------------------- |
| Cyclomatic complexity (per method) | `lizard --CCN 10 .` (if available; see command above)                                                                       | Down; target ≤ 10       |
| Lines of code (changed files)      | `wc -l` on affected files                                                                                                   | Usually down            |
| Maximum nesting depth              | `lizard` or nesting-depth proxy (see above)                                                                                 | Down; target ≤ 3        |
| Passing tests                      | `bash` the test command                                                                                                     | Unchanged or more       |
| New public symbols introduced      | Language-appropriate grep: `^pub ` (Rust), `^public ` (Java/C#/Go), `^export ` (TypeScript/JS) — infer from file extensions | Zero unless intentional |
| Import count (changed file)        | Count import lines before/after                                                                                             | Usually down            |

If a metric moves the wrong direction, explain why — sometimes it's the right call (named helpers increase LOC but reduce complexity and nesting).

______________________________________________________________________

## Making code readable

A good function reads like a short paragraph: one job, a name that says what that job is, a body short enough to take in at a glance. If you're writing "and" in a function name, it's doing two things. Split it.

Handle the unhappy path first. Guard clauses at the top let the happy path flow straight down the function without nesting. Successive if-else branches force readers to track parallel worlds simultaneously.

Whitespace is structure. Blank lines separate logical groups. Readers parse whitespace before they read words — use it deliberately.

Names carry information or they carry nothing. A variable called `data` or `result` is noise. Rename deliberately; if the rename is broad, do it as its own commit.

Comments explain *why*, not *what*. The code shows what. A comment answers the question a careful reader would still have after reading the code. Delete comments that restate the code. Outdated comments are actively harmful — they lie.

______________________________________________________________________

## Handling failures

Return meaningful types, not null. A function that can legitimately produce nothing should say so explicitly: an empty collection, `Optional`, `Result`, or a null object. Null is an implicit contract violation the type system cannot enforce.

Validate at the boundary. When data enters the system, check it once, loudly. Propagating invalid state inward scatters error handling across every layer that touches it.

Error messages should help a human understand what went wrong and where to look. "An error occurred" is useless. "Invoice total must be positive; got -15.00" is useful.

Don't add defensive checks against invariants that trusted callers cannot violate. Defensive guards at every layer create noise, hide real errors, and train readers to stop paying attention to checks.

______________________________________________________________________

## Testing as a design probe

A test that's hard to write is the code telling you something specific about its design:

- **Elaborate setup** → too many collaborators, or they're too fine-grained; consider Extract Class or consolidating collaborators
- **Needs real infrastructure** → DIP violation; introduce an interface and inject it
- **Hard to assert on** → SRP violation; the unit does too many things; extract until each piece is independently assertable
- **Flaky without obvious cause** → hidden mutable state; surface it, parameterise it, or push it to the boundary

Fix the design problem, not the test problem.

______________________________________________________________________

## Rules that don't bend

**Tests stay green.** Run the suite after every step. The moment it goes red, stop — don't push through. Revert the last change and find a smaller increment. Never commit a red suite.

**Behaviour stays the same.** If you spot a bug during the refactor, note it and continue with behaviour preserved. Fix the bug in a separate commit afterwards. Mixing bug fixes with refactoring makes both impossible to review.

**The public API doesn't grow.** A refactor that introduces new public symbols is smuggling in a feature. Split them into separate commits.

**Measure before removing abstractions.** A layer of indirection sometimes hurts performance. Sometimes it doesn't. Measure before assuming; don't remove abstractions based on intuition.

______________________________________________________________________

## Committing the work

Discover commit conventions before writing a single message:

1. Use `read` to check `CONTRIBUTING.md`, `DEVELOPMENT.md`, `.github/CONTRIBUTING.md`, and the contributing section of `README.md`.
1. If nothing explicit exists, use `bash` to run `git log --no-pager -10` and match the format in use.

One commit per logical step. Each commit should be small enough that a reviewer can verify it is behaviour-preserving by inspection — they shouldn't need to run the tests to trust it.

When invoked by the `atdd` agent during the Refactor phase, coordinate with the project's squash/merge convention: multiple small commits during the refactor are fine; whether they are squashed before the story commit depends on the project's git workflow.

______________________________________________________________________

## When to stop

| Situation                       | What to do                                                |
| ------------------------------- | --------------------------------------------------------- |
| No tests, no time to add them   | Tell the user; don't proceed without a safety net         |
| Code you don't fully understand | Read until you do; don't refactor from uncertainty        |
| Code that's about to be deleted | Leave it; polishing waste is waste                        |
| Hard deadline                   | Note the debt, ship, refactor after                       |
| API with unknown consumers      | You can't see the blast radius; widen scope first or stop |

______________________________________________________________________

## Review gate

Before producing the end-of-session report, invoke `refactor-reviewer` via the `task` tool:

```text
Task: Review the completed refactoring session
Context:
  Changed files / diff: [full diff or list of changed files]
  Complexity metrics before: [baseline captured at session start]
  Complexity metrics after: [current measurements]
  Test output: [most recent passing test run]
  Session summary: [violations fixed, transformations applied, metrics]
  Retry context: [attempt number and prior rejected findings, if applicable]
Constraints: Apply the structural improvement quality bar
Success criteria: Return a structured verdict (approved/rejected)
```

Parse the verdict:

- `approved` → produce the end-of-session report below.
- `ESCALATE_TO_USER` in findings → surface the escalation detail with the full rejected findings to the user, and stop. Do not produce the session report.
- `rejected` → apply the Required changes, make any additional commits needed, and re-invoke `refactor-reviewer`. Allow at most 3 attempts total; after the third consecutive rejection treat it as an implicit escalation and surface the findings to the user.

______________________________________________________________________

## End-of-session report

Produce this when the session is complete:

1. **What changed and why** — one paragraph summarising structural improvements and which principles they address.
1. **Violations fixed** — each principle violation with its file location.
1. **Transformations applied** — each refactoring by name, with before/after `file:line` references.
1. **Metrics table** — before and after for each metric captured at baseline.
1. **Left on the table** — things noticed but not tackled, and why they were deferred.
1. **Risks** — behaviour-adjacent changes, performance concerns, or API surface shifts that warrant review.
