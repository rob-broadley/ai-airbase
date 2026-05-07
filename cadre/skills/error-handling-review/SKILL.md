---
name: error-handling-review
description: Error handling review reference covering swallowed errors, error specificity, language-specific patterns, resilience, partial failure, user-facing errors, and a three-tier severity model. Used by the error-handling-reviewer agent.
license: AGPL-3.0-or-later
allowed-tools: read
---

# Error Handling Review Reference

Error handling is the part of the code that runs when things go wrong — which in production is the part that matters most. Poor error handling produces silent data corruption, hard-to-diagnose outages, and users who cannot understand what failed or how to recover.

______________________________________________________________________

## Error taxonomy

The error taxonomy is the primary frame for the review. Every finding should cite the category being violated.

| Category              | Semantics                                                      | Correct handling                                                 |
| --------------------- | -------------------------------------------------------------- | ---------------------------------------------------------------- |
| Domain errors         | Business-rule violations                                       | Return as values; never raise exceptions for expected conditions |
| Validation errors     | Malformed or invalid input from callers                        | Return to caller (HTTP 4xx equivalent); do not log as errors     |
| Infrastructure errors | External dependency failure (DB, network, queue)               | Retry with backoff, fallback, or circuit break; log with context |
| Programming errors    | Bugs — nil dereference, assertion failure, index out of bounds | Fail fast; do NOT catch; let the runtime surface them            |
| Cancellation          | Client gone or deadline exceeded                               | Propagate cleanly; do not swallow; do not retry                  |

Catching bare `Exception`, `error`, or `Throwable` and handling all categories uniformly conflates these semantics. Each finding should cite the category being violated.

______________________________________________________________________

## Swallowed errors

An error is swallowed when it is received and then discarded without any action — no logging, no propagation, no recovery. This produces silent failures that are nearly impossible to diagnose.

Signals:

- A `catch` or `recover` block that is empty or contains only a comment.
- `_ = someFunc()` where `someFunc` returns an error and the caller has no documented reason to ignore it.
- An error caught and logged at DEBUG level when the operation has observable side effects (data not written, cache not populated, downstream call not made).
- An error caught, a fallback value used, and the caller's output is therefore silently wrong.
- `defer` calls whose return values are not checked (common with `rows.Close()`, `file.Close()`, `tx.Rollback()`).

______________________________________________________________________

## Error specificity

An error that loses context makes debugging slower and root-cause analysis harder. Errors should carry enough information to answer: what failed, where it failed, and what was being attempted.

Signals:

- Catching an overly broad exception type (`Exception`, `error`, `interface{}`) when a narrower type is available and meaningful.
- Re-wrapping an error with `fmt.Errorf("error: %v", err)` — adding no information to the original message; `%v` instead of `%w` loses the wrappable chain.
- Returning a generic sentinel error (`ErrNotFound`, `ErrInvalid`) from a function where the caller needs more context to act correctly.
- Logging `err.Error()` without any context about which operation was being performed or what inputs were involved.
- Converting a typed error into a string for transmission (e.g., across an RPC boundary) without preserving the type or a machine-readable code.

______________________________________________________________________

## Language-specific patterns

Each language has established idioms for error handling. Deviations from them make code harder to read, tool-analyse, and maintain.

### JavaScript and TypeScript

- **Unhandled Promise rejection:** a `Promise` with no `.catch()` and no `await` inside a `try/catch` block can terminate the process or surface as an unhandled rejection.
- **Empty or logging-only `.catch()`:** `.catch(console.log)` or `.catch(() => {})` swallows the error without recovery.
- **Throwing non-`Error` values:** `throw "something went wrong"` or `throw { message: "..." }` loses stack trace information. Throw `Error` instances or subclasses.
- **Error cause preserved:** use `new Error("context", { cause: originalError })` (or an equivalent typed error wrapper) so the original chain survives.
- **Async boundaries checked:** every `await` boundary that can fail needs either local handling or deliberate propagation. Promise-returning helpers that are not awaited often drop failures on the floor.

### Python

- **Bare `except:` clause:** catches `SystemExit`, `KeyboardInterrupt`, and `GeneratorExit` — almost always unintentional. Use `except Exception:` at minimum, or a specific type.
- **`except Exception: pass`:** swallows all exceptions silently. Even if the intent is to continue, log or transform the exception deliberately.
- **Exception chaining:** `raise NewException("context") from original_exception` preserves the original failure. Omitting `from` loses causality.
- **Re-raising after logging:** `logger.exception(...)` followed by bare `raise` preserves the traceback; `raise e` resets the traceback origin.
- **Context managers and cleanup:** `with` blocks should guard resources that must be closed; manual open/close pairs often lose the failure from cleanup.

### Java

- **Checked vs unchecked exceptions:** checked exceptions such as `IOException` or `SQLException` must be caught or declared. Swallowing them with an empty catch block is a common mistake.
- **Swallowing `InterruptedException`:** catching `InterruptedException` and doing nothing discards the thread's interrupted status. Either re-interrupt the thread (`Thread.currentThread().interrupt()`) or propagate the exception.
- **Over-broad catch:** `catch (Exception e)` or `catch (Throwable e)` catches everything, including serious runtime failures. Catch the narrowest meaningful type.
- **Exception chaining:** use `new SomeException("context", cause)` or `initCause` so the original exception is preserved.
- **Checked resources:** prefer try-with-resources so cleanup failures are attached as suppressed exceptions instead of disappearing.

### C\#

- **Exception hierarchy respected:** avoid `catch (Exception)` unless you are at a true boundary and can translate the error meaningfully. Catch narrower exception types when behaviour differs.
- **Aggregate failures surfaced:** when awaiting or joining multiple tasks, inspect `AggregateException` or the awaited exception set carefully rather than discarding inner failures.
- **`using` and disposal:** disposable resources should use `using`, `await using`, or a `try/finally` equivalent so cleanup runs reliably, even on failure.
- **Async methods return `Task`:** `async void` should be reserved for event handlers; elsewhere it hides failures from callers.
- **Error wrapping keeps context:** when translating an exception, preserve the original via `InnerException` and add actionable context.

### C++

- **Exceptions vs error codes:** projects must be consistent about whether failures are signalled through exceptions, `std::error_code`, `std::expected`, or return codes. Mixed conventions without clear boundaries lose information.
- **RAII for cleanup:** resource release should follow RAII so failure paths do not leak files, locks, or memory. Manual cleanup code is easy to skip in early returns.
- **`noexcept` honesty:** functions marked `noexcept` must not let exceptions escape. A violated `noexcept` contract calls `std::terminate`, which is often worse than the original failure.
- **Error context preserved:** when translating library errors, keep the original status code or exception data alongside the higher-level message.
- **Ignored return values:** APIs that signal failure via return codes must have those values checked on every path.

### Go

- **Error return as last return value:** convention is `(value, error)`. Reversing this breaks consistency.
- **`fmt.Errorf` with `%w` for wrapping:** this keeps the error chain intact for `errors.Is` and `errors.As`. Using `%v` loses the wrappable chain.
- **Sentinel errors for known conditions:** package-level `var ErrNotFound = errors.New("not found")` is appropriate when callers must distinguish a condition with `errors.Is`.
- **Typed errors for structured data:** when callers need structured data from the error, use a typed error and `errors.As` to extract it.
- **Panic vs error return:** panic is for programming errors or violated invariants; recoverable runtime conditions should be returned as errors.
- **Check all branches:** `if err != nil` after every call that returns an error. Ignoring an error needs a documented, deliberate reason.

### Rust

- **`Result` and `Option` propagated deliberately:** use `?` to preserve propagation paths rather than unwrapping errors in normal control flow.
- **`unwrap` and `expect` kept for invariants only:** using them on user input, I/O, or network results turns recoverable failures into panics.
- **Typed errors where boundaries matter:** `thiserror` is appropriate for library-facing typed errors; `anyhow` or similar wrappers suit application boundaries where rich context matters more than matching.
- **Context added without discarding the source:** use combinators such as `context` or explicit enum variants so the original error remains inspectable.
- **`Option` vs `Result` distinguished:** returning `None` for an actual failure hides the reason and makes remediation impossible.

______________________________________________________________________

## Resilience patterns

A system that does not handle transient failures gracefully will produce user-visible errors for conditions that are routine in distributed environments.

- **Missing retry logic for transient failures:** network calls, database operations, and external API calls will occasionally fail transiently. Without retry logic, every transient failure is a user-visible error.
- **No timeout on external calls:** an HTTP client, database query, or RPC call with no timeout will block indefinitely if the remote system is unresponsive. This exhausts connection pools and goroutines.
- **No circuit breaker for repeated failures:** a dependency that is failing for all requests should be short-circuited so that callers fail fast rather than accumulating blocked threads waiting for timeouts.
- **No backoff strategy:** retrying at full speed against a failing dependency exacerbates the problem for the dependency and produces a thundering herd when the dependency recovers. Use exponential backoff with jitter.
- **Cancellation not respected in retry loops:** a retry loop that does not check for cancellation (context cancellation, thread interrupt, AbortSignal) will continue retrying even after the caller has cancelled the request.

______________________________________________________________________

## Resilience pattern quality rubric

### Retry quality signals

- **Poor:** fixed interval, unbounded retries, retries 4xx responses.
- **Good:** exponential backoff + jitter, bounded attempt count, only idempotent operations, respects outer deadline.

### Circuit breaker quality signals

- **Poor:** trips on single failure, all-at-once recovery, no fallback.
- **Good:** failure-rate threshold over time window, gradual half-open probing, meaningful fallback behaviour.

### Timeout quality signals

- **Poor:** ambiguous constants, inner timeout ≥ outer timeout, hard-coded values.
- **Good:** outer timeout < sum of inner timeouts, explicit units, propagated via cancellation context.

______________________________________________________________________

## Message consumer error handling

- Malformed messages that cannot be processed should be routed to a dead-letter queue (DLQ) rather than crashing or looping.
- Retry policy (with bounded attempts) should run before DLQ routing.
- DLQ should have alerting so poison messages are noticed.
- Replay tooling should exist for when the processing bug is fixed.

______________________________________________________________________

## Partial failure handling

Batch operations and multi-step workflows create partial failure scenarios that single-item operations do not. Partial failure must be an explicit, designed state — not something that accidentally produces inconsistent data.

- **Undefined batch failure semantics:** when processing a batch of items, what happens when one fails? Does the whole batch abort? Does processing continue? Are partial results returned? This must be explicit in the code and in any documentation.
- **Partial success not clearly communicated:** a function that processed 80 of 100 items successfully must not return a success result. It must signal the partial state and identify which items failed.
- **State consistency after failure:** if a multi-step operation fails partway through, is the state consistent? Are partial writes rolled back? Is the caller able to safely retry from the beginning or must they resume from a checkpoint?
- **Compensation logic absent:** operations that cannot be made fully atomic need compensating transactions or cleanup paths for the case where a later step fails. Absent compensation logic produces lingering partial state.

______________________________________________________________________

## User-facing errors

Errors that reach users — whether human users via a UI or machine callers via an API — must be appropriate for that audience.

- **Internal implementation detail exposed:** stack traces, SQL error text, filesystem paths, internal service names, or database schema details returned to callers. These reveal internal structure and aid attackers. See `/security-review` for the security dimension.
- **Messages that don't help users take action:** "An error occurred" provides no information. "Payment failed: the card was declined. Check the card number and expiry date, or try a different card." provides actionable information.
- **Missing context about what failed:** a generic "not found" error that does not specify what was not found forces users to guess. "Order #12345 not found" is useful; "not found" is not.
- **Error messages that change between environments:** a message that differs between development and production makes test-based verification of error messages unreliable.
- **Missing error codes for machine callers:** API error responses should include a stable machine-readable code alongside the human-readable message so that callers can handle specific errors without parsing text. See `/api-review` for the full treatment.

______________________________________________________________________

## Severity model

### Blocking

Error handling is absent or incorrect in a way that will cause data loss, corruption, or incorrect behaviour under normal operating conditions.

Examples: a swallowed error on a database write; a missing timeout on an external call in a high-traffic path; a panic where an error return was appropriate; partial failure with no consistency guarantee on shared state.

### Recommendation

Error handling is present but produces poor diagnostic information or lacks resilience for a production environment.

Examples: `%v` used instead of `%w` in error wrapping; retry logic absent on a known-transient operation; user-facing error message that exposes internal detail; generic error returned where a typed error would let callers distinguish cases.

### Observation

Minor quality issue with low immediate impact.

Examples: a defer's return error unchecked where failure is truly inconsequential; a log message that could include more context; a sentinel error that could be replaced with a typed error for better ergonomics.

______________________________________________________________________

## Output format

Group findings by severity (Blocking first). For each finding:

```
**[SEVERITY] location** (file:line or file:function)
Description: what the issue is.
Why it matters: the consequence if left unaddressed.
Direction: the general approach to resolution — not an implementation.
```

Do not write error handling code. Provide direction, not implementation. If no findings exist in a severity tier, omit that tier.
