---
name: tdd-patterns
description: Load before any TDD implementation or legacy code work. Required by the atdd and legacy-code agents — covers walking skeleton, London vs Chicago school, double-loop TDD, test double selection, contract testing, property-based tests, approval tests, and characterisation testing.
license: AGPL-3.0-or-later
allowed-tools: read
---

Reference patterns for ATDD and TDD. These complement the Red-Green-Refactor cycle; they are not replacements for it.

______________________________________________________________________

## Walking Skeleton

**What it is:** The thinnest possible slice of the system that exercises the full stack end-to-end — from the outermost entry point (e.g. HTTP request) to the outermost exit point (e.g. database write) — and can be deployed.

**When to use:** At the very start of a project or a major new capability. The skeleton proves that the pipeline works before any real logic is added. All subsequent stories add flesh to the skeleton.

**Key notes:**

- The walking skeleton is not a spike — it is production code, committed, deployed.
- It should do almost nothing useful (e.g. "return 200 OK with an empty list"), but do it correctly through the real infrastructure.
- Write an end-to-end acceptance test for it first (it will be red until the skeleton is wired up).

______________________________________________________________________

## Outside-In vs Inside-Out

Two schools of TDD that differ in where you start and how you discover design.

### Outside-In — London School

Start at the outermost layer (acceptance test / UI / API boundary) and work inward, using mocks to stand in for collaborators that do not yet exist. Design emerges from the consumer's perspective.

**When to use:**

- You have a clear picture of the external interface but not the internal structure.
- You want the design of lower layers to be driven by how higher layers use them.
- Working in a layered architecture (controller → service → repository).

**Key notes:**

- Mocks are used as a design tool, not just a test isolation tool.
- Danger zone: over-mocking leads to tests that pass even when the real collaborators would fail. Supplement with integration tests at the seams.

### Inside-Out — Chicago / Detroit School

Start with the core domain model (pure functions, value objects, entities) and build outward. No mocks required until you reach infrastructure boundaries.

**When to use:**

- The domain logic is complex and you want to nail it before thinking about delivery mechanisms.
- You don't yet know the external interface.
- The core can be tested in complete isolation (no I/O).

**Key notes:**

- Risk of building the wrong thing — you may discover the external interface doesn't fit the domain model you built.
- Combine with a walking skeleton to keep the pipeline honest.

______________________________________________________________________

## Double-Loop TDD

Two concurrently running test loops at different granularities:

```
Outer loop (acceptance):   Red ──────────────────────────────────► Green
                                  ↓                          ↑
Inner loop (unit):              Red → Green → Refactor → Red → Green → Refactor
```

1. Write a **failing acceptance test** (outer loop goes red).
1. Drop into the inner loop — write a failing **unit test** for the first unit of implementation needed.
1. Make the unit test green, refactor, then write the next unit test.
1. Repeat until the acceptance test goes green (outer loop green).
1. Refactor at the acceptance level if needed, then commit.

**When to use:** Always, on any non-trivial story. The outer loop keeps you honest about user value; the inner loop drives the design of individual units.

**Key note:** Never make the acceptance test pass by hacking — every step through the inner loop should be a genuine, clean implementation.

______________________________________________________________________

## Test Double Selection

Choose the simplest double that makes the test work. Complexity order: Dummy < Stub < Spy < Mock < Fake.

| Double    | What it does                                 | Use when                                                                       |
| --------- | -------------------------------------------- | ------------------------------------------------------------------------------ |
| **Dummy** | Passed but never used                        | A parameter is required but irrelevant to this test                            |
| **Stub**  | Returns a canned value; no assertions        | You need the SUT to receive a predictable input from a dependency              |
| **Spy**   | Records calls; assertions made after         | You want to verify a side-effect occurred, but the double is still passive     |
| **Mock**  | Expectations set upfront; auto-verifies      | You want to verify an interaction as part of the test (London School)          |
| **Fake**  | Working implementation, not production-ready | You need real behaviour (e.g. in-memory database) but can't use the real thing |

**Rules of thumb:**

- Mock roles, not objects — mock interfaces/abstractions, not concrete classes.
- Don't mock what you don't own — if you need to isolate a third-party library, wrap it in an adapter you own, then mock the adapter.
- Prefer fakes for persistence layers in integration tests — they're faster and less brittle than mocking repository methods individually.

______________________________________________________________________

## Contract Testing

**What it is:** Tests that verify the contract between a consumer and a provider — typically between two services — without requiring both to be running at the same time.

**When to use:**

- Microservices or separate deployable components that communicate over HTTP, messaging, or RPC.
- You want to catch breaking API changes before integration.

**How it works (consumer-driven, e.g. Pact):**

1. The **consumer** writes tests that express what it expects from the provider; these generate a contract artefact.
1. The **provider** runs the contract against its own codebase to verify it honours the expectations.
1. Contracts are stored in a broker (Pact Broker, Spectral, etc.) and verified in CI on both sides.

**Key notes:**

- Consumer-driven contracts catch breaking changes earlier than provider-driven ones.
- Not a replacement for end-to-end tests — they test the contract, not the full system behaviour.
- Keep contracts narrow; only assert on fields the consumer actually uses.

______________________________________________________________________

## Property-Based Tests

**What it is:** Instead of example-based tests with fixed inputs, you define _properties_ (invariants) that must hold for _all_ inputs from a generated domain. The framework finds counter-examples.

**When to use:**

- Pure functions with a wide input space (parsers, encoders, sorting, arithmetic, validation).
- Business rules that must hold universally ("the total is always non-negative", "round-trip serialisation is lossless").
- Anywhere you suspect your example-based tests are only covering the happy path.

**Key notes:**

- Tools: Hypothesis (Python), fast-check (JS/TS), QuickCheck (Haskell, ported to many languages), jqwik (Java).
- When a counter-example is found, the framework _shrinks_ it to the minimal failing case — inspect that, not the raw random input.
- Property tests complement, not replace, example-based tests. Use both.

**Common properties to test:**

- Round-trip: `decode(encode(x)) == x`
- Idempotence: `f(f(x)) == f(x)`
- Commutativity / associativity where applicable
- Invariants: output is always within a valid range

______________________________________________________________________

## Approval / Golden-Master Tests

**What it is:** The system produces a text or binary output; you _approve_ the first output as correct, commit it as a snapshot, and future runs fail if the output changes unexpectedly.

**When to use:**

- Complex or verbose output where specifying every assertion by hand is impractical (reports, rendered HTML, serialised data, legacy system output).
- Characterisation tests when working with legacy code — capture what the system _does_ before refactoring.
- Anywhere output stability is the goal and the exact content is the spec.

**Key notes:**

- Tools: ApprovalTests (Java, C#, C++, Python, Ruby), snapshot testing in Jest (JS/TS), `insta` (Rust).
- Approved files must be committed to source control — they are part of the spec.
- Review diffs carefully on approval updates; accidental approvals silently bless regressions.
- Not suitable for output that is intentionally non-deterministic (timestamps, UUIDs) without normalisation.

______________________________________________________________________

## Characterisation Tests

**What it is:** Tests written to document what a piece of code *currently does*, not what it *should* do. The goal is to pin existing behaviour before modifying or refactoring untested code.

**When to use:**

- Before touching legacy code that has no tests.
- When you need to understand a poorly documented module.
- As the safety net that enables dependency-breaking and refactoring.

**Key notes:**

- Characterisation tests capture behaviour including bugs — do not fix bugs while writing them. Fix bugs separately, after the safety net is in place.
- They are often temporary: once the code is refactored and properly unit-tested, some characterisation tests can be retired.
- Name them clearly as characterisation tests (e.g. `// characterisation: current behaviour of legacy billing calculation`) so future readers know their intent.

**Choosing the right style:**

| Code characteristic                   | Recommended style                                    |
| ------------------------------------- | ---------------------------------------------------- |
| Pure function, wide input space       | Property-based                                       |
| Stateful workflow or side effects     | Approval / golden-master                             |
| Complex or verbose output             | Golden-master                                        |
| Mathematical or structural invariants | Invariant / property-based                           |
| Output is non-deterministic           | Invariant tests (check properties, not exact output) |

**Invariant tests:** When output is non-deterministic (e.g. ordering varies, timestamps differ), test invariants instead of exact output — for example, "the total always equals the sum of the line items", or "every item in the input appears exactly once in the output".

**Worked sequence:**

1. Run the code with representative inputs; capture the output (manually or via a test harness).
1. Commit the captured output as an approved snapshot or fixture.
1. Write the test that asserts the current output matches the snapshot.
1. Confirm the test passes on the current code.
1. Proceed with your dependency-breaking or refactoring steps, using the characterisation test as the safety net.
