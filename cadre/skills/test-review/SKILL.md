---
name: test-review
description: Load before any test quality review. Required by the test-reviewer agent — covers Farley's 8 properties of good tests, test double misuse, fragility signals, coverage gap patterns, and a three-tier severity model.
license: AGPL-3.0-or-later
---

# Test Review Reference

A test suite is only as useful as its ability to catch regressions quickly and reliably. Poor test quality produces slow, brittle, or misleading signal — which is often worse than no tests at all because it breeds distrust and causes teams to stop running or believing the suite.

Use Dave Farley's 8 properties as the primary evaluation framework.

______________________________________________________________________

## The 8 properties of good tests

### 1. Fast

Unit tests must complete in milliseconds. A test suite that takes minutes to run will be run infrequently, and infrequent runs mean late feedback.

Signals of a speed problem:

- Unit tests that hit a real database, file system, or network.
- Tests that `time.Sleep` or `Thread.sleep` to wait for asynchronous behaviour.
- Tests that spin up a full application context to test a single function.
- Test suites where the majority of runtime is in setup, not in assertions.

______________________________________________________________________

### 2. Isolated

Each test sets up its own context and tears it down. Tests must be order-independent: running them in any sequence must produce the same result.

Signals of an isolation problem:

- Shared mutable state between tests (static variables, singleton services, module-level caches).
- Tests that depend on the side effects of a previously run test.
- Tests that pass in isolation but fail in a full suite run.
- `beforeAll` / `TestMain` that mutates shared state without cleanup.

______________________________________________________________________

### 3. Repeatable

The same test in the same state must produce the same result every time. A test that flickers between pass and fail is not a test — it is noise.

Signals of a repeatability problem:

- Assertions on `time.Now()` or equivalent without injecting a clock.
- Tests that use random data without a seeded, deterministic random source.
- Tests that call real external services (network calls, external APIs, real cloud storage).
- Tests that rely on filesystem paths that differ between environments.
- Tests that pass locally and fail in CI due to environment differences.

______________________________________________________________________

### 4. Self-validating

The test must produce a clear pass or fail without manual inspection. Printing output and expecting a human to check it is not a test.

Signals of a self-validation problem:

- `fmt.Println` or `console.log` output as the sole signal of correctness.
- Tests that always pass because the assertion is missing or commented out.
- Assertions on the wrong value (e.g., asserting the input rather than the output).
- Tests where the assertion message is misleading about what was actually checked.

______________________________________________________________________

### 5. Timely

Tests written long after the code under test has been shipped provide no design benefit. Tests written before or alongside the code catch design problems early.

Signals of a timeliness problem:

- Test files that were added months after the implementation files they cover (check git history).
- Tests retrofitted to legacy code without characterisation — they describe what the code does, not what it should do.
- An entire subsystem with no test coverage while development has moved on.

______________________________________________________________________

### 6. Readable

A test is documentation. The test name should state the intent; the body should show the Arrange/Act/Assert structure; there should be no magic numbers or unexplained constants.

A test that runs slowly but clearly documents expected behaviour is preferable to one that runs fast but is incomprehensible. Speed is the easiest property to improve; clarity is the hardest.

Signals of a readability problem:

- Test names that describe the implementation (`testMethod1`, `test_case_3`) rather than the behaviour (`returns_empty_list_when_no_items_match`).
- Arrange/Act/Assert phases not visually separated or implied.
- Magic numbers: `assert result == 42` with no explanation of why 42 is the expected value.
- Test bodies longer than ~30 lines with no extracted helpers for setup.
- Deeply nested helper calls that obscure what is actually being tested.

______________________________________________________________________

### 7. Specific

One logical assertion per test. When a test fails, the name and the assertion must unambiguously identify the broken behaviour. A test that asserts twenty things makes failures hard to diagnose.

Signals of a specificity problem:

- A single test method that asserts on multiple unrelated behaviours.
- Assertions inside loops without a clear per-iteration failure message.
- Asserting on a serialised blob (JSON string, formatted output) when the relevant fields could be asserted individually.
- A test named `test_everything` or `integration_test` that covers an unbounded scope.

______________________________________________________________________

### 8. Comprehensive

The test suite must cover the behaviours that matter: the happy path, the key error paths, and the boundary conditions. A suite that only tests the sunny day is a liability disguised as an asset.

Signals of a comprehensiveness gap:

- Only the success case is tested; no test for the case where a dependency fails or returns an error.
- No test for boundary inputs: empty collection, single-element collection, maximum length, zero, negative.
- No test for concurrent access when the code under test accesses shared state.
- Coverage metrics that look healthy but cluster on trivial paths while complex branches go untested.

______________________________________________________________________

## Language-specific test conventions

Use the framework idioms of the language in scope when evaluating whether a test is readable, isolated, and maintainable.

| Language                      | Common frameworks and conventions                                                                                                                       |
| ----------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **JavaScript and TypeScript** | Jest, Vitest, and Mocha are common. Prefer explicit async assertions over timer sleeps, and keep mocks scoped to the test.                              |
| **Python**                    | `pytest` and `unittest` dominate. Prefer fixtures for setup, parametrisation for boundary cases, and explicit exception assertions.                     |
| **Java**                      | JUnit 5 with Mockito is common. Keep Spring or framework-heavy tests out of the unit tier unless the integration boundary is the actual subject.        |
| **C#**                        | xUnit and NUnit are common; Moq is a frequent mocking choice. Prefer `async Task` tests and expressive assertion libraries over manual thread blocking. |
| **C++**                       | Google Test and Catch2 are common. Fixtures should make ownership and lifetime explicit, especially when native resources are involved.                 |
| **Go**                        | The standard `testing` package and `testify` are common. Table-driven tests are idiomatic when they improve coverage rather than obscure intent.        |
| **Rust**                      | Built-in `#[test]` functions and crates such as `rstest` are common. Prefer direct assertions on `Result` and pattern matching over stringly checks.    |

### Other languages

If another language is present, judge the tests against that ecosystem's standard runner and assertion style. The same quality bar still applies: fast feedback, isolation, repeatability, and clear intent.

______________________________________________________________________

## Test double misuse

Test doubles (mocks, stubs, fakes, spies) are tools with precise purposes. Misuse produces brittle tests that give false confidence.

- **Mocking what you don't own:** mocking a third-party library's concrete type rather than an interface you define. When the library changes its internals, the mock stops reflecting reality.
- **Verifying interactions instead of outcomes:** asserting that a method was called with specific arguments when the meaningful question is whether the outcome is correct. Interaction-based tests tie tests to implementation.
- **Over-specifying call counts:** asserting `Times(1)` or `Times(3)` when the count is an implementation detail. The important question is whether the right thing happened, not how many times an internal step was called.
- **Stubs returning success unconditionally:** a stub that always returns a success response never exercises the error-handling paths of the code under test.

______________________________________________________________________

## Tautology theatre

- **Mock tautology:** mock return value configured, then asserted on the same mock with no real production code in between — the test cannot fail because you are asserting what you just configured.
- **Mock-only test:** all objects in the test are mocks with no real class instantiated — tests nothing about production behaviour.
- **Trivial tautology:** assertions like `assertTrue(true)`, `assertEquals(1, 1)` that always pass regardless of code.
- **Framework test:** verifies language or framework behaviour rather than application logic — adds maintenance burden with no application value.

______________________________________________________________________

## Fragility signals

Fragile tests break when unrelated code changes. They are a maintenance tax.

- Tests that import and directly reference private/internal symbols by name.
- Tests that assert on the exact string representation of an error message that is subject to change.
- Tests that are coupled to the order of elements in an unordered collection.
- Tests that replicate the implementation logic rather than asserting on its output — if the implementation changes, the test must change too, even if the behaviour is unchanged.
- Tests that break when a field is added to a struct or object that is not related to the test's concern.

______________________________________________________________________

## Coverage gap patterns

- **No error path tests:** every function that can fail has at least one test for the failure case. If a function returns an error or throws, there must be a test that exercises the error path.
- **No boundary tests:** functions with numeric inputs or collection inputs must be tested at the boundaries: zero, empty, maximum, minimum.
- **Only happy-path coverage:** a suite where every test uses valid, well-formed, in-range inputs only.
- **Untested integration points:** the code that calls an external service, a database, or a message queue has no test at any level that exercises the integration.
- **Coverage holes in changed code:** the diff adds new branches or conditions with no corresponding new tests.

______________________________________________________________________

## Test pyramid shape

- Unit tests should be the largest tier (fast, isolated, no I/O) — rough timing signal: under 10ms each.
- Integration tests are the middle tier — under 1 second each.
- End-to-end tests should be the smallest tier — they are slow, brittle, and expensive.
- Ice-cream cone anti-pattern: more end-to-end tests than unit tests — signals missing unit test coverage compensated by expensive E2E tests.
- Check: count test files or test functions per tier to identify the distribution.

______________________________________________________________________

## Mutation testing

- Static structural scoring can show "well-written" tests that catch no bugs — mutation testing provides a verification layer.
- Per-language tools where available: stryker-mutator (JavaScript and TypeScript), mutmut (Python), PIT or pitest (Java), Stryker for C# (C#), Mull (C++), go-mutesting (Go), and cargo-mutants (Rust).
- When mutation tool output is available, it overrides static scoring for the Necessary and Granular properties — a test that survives mutations is not providing meaningful coverage regardless of its structural quality.
- Note: run mutation testing on changed test files, not the whole suite — full-suite mutation runs are slow.

______________________________________________________________________

## Severity model

### Blocking

The test suite cannot be trusted for the feature or path in question. The change must not be merged until these are resolved.

Examples: a test that always passes regardless of the implementation (assertion absent or trivially wrong); a race condition in test setup; shared mutable state that makes the test order-dependent.

### Recommendation

The test suite works but has quality problems that will cost time later. Address before or shortly after merge.

Examples: missing error path coverage for a changed function; a test that mocks what it does not own; a magic number with no explanation; a test name that does not describe its intent.

### Observation

Minor quality issue. Low priority. Address in a subsequent tidy pass.

Examples: a test helper that could be extracted for reuse; an Assert message that could be more descriptive; a minor readability improvement.

______________________________________________________________________

## Output format

Group findings by severity (Blocking first). For each finding:

```
**[SEVERITY] location** (file:line or file:function)
Description: what the issue is.
Why it matters: the consequence if left unaddressed.
Direction: the general approach to resolution — not an implementation.
```

Do not write test code. Provide direction, not implementation. If no findings exist in a severity tier, omit that tier.
