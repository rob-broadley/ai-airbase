---
name: observability-review
description: Load before any observability review. Required by the observability-reviewer agent — covers log levels, structured logging, missing log statements, trace context, metric naming, alert quality, and a three-tier severity model.
license: AGPL-3.0-or-later
---

# Observability Review Reference

Observability is the ability to understand the internal state of a system by examining its outputs. A system that cannot be observed cannot be operated. Code that produces poor logs, missing metrics, or misleading alerts increases mean time to detection and mean time to recovery when things go wrong.

______________________________________________________________________

## Log level appropriateness

Log levels are a contract between the application and the operator. Misuse makes logs useless: too verbose and operators ignore them; too sparse and failures go undetected.

### DEBUG

For development detail that is not useful in a running production system. Internal state, intermediate values, loop iterations.

Misuse signals:

- DEBUG statements left in hot paths in production code — produces excessive log volume, degrades performance.
- DEBUG used for significant state transitions or external call outcomes — this information will be invisible in production where DEBUG is typically disabled.

### INFO

For significant, discrete state transitions that are meaningful to an operator: service started, job completed, user authenticated, payment processed.

Misuse signals:

- INFO used for every iteration of a loop or every record processed — produces noise that buries real events.
- INFO absent from key state transitions — the operator has no record of what the system did.
- INFO used for recoverable problems (use WARN for those).

### WARN

For recoverable problems: a retry attempt, a degraded fallback in use, a configuration value outside its expected range.

Misuse signals:

- WARN used for normal conditions (e.g., "cache miss" when misses are expected) — operators learn to ignore WARN.
- WARN used for unrecoverable failures (use ERROR for those).

### ERROR

For failures that require attention: an operation failed and cannot be retried; a dependency is unreachable; a data consistency invariant was violated.

Misuse signals:

- ERROR used for user-input validation failures (these are expected; use WARN or INFO).
- ERROR logged and then the same failure is propagated as a returned error — the failure will be logged again by the caller, producing duplicate error entries.
- ERROR with no context about what was being attempted or what the inputs were.

______________________________________________________________________

## Structured logging

Structured logs are queryable; interpolated string logs are not.

- **Key-value pairs, not interpolated strings:** `log.Info("request completed", "method", "GET", "path", "/users", "status", 200, "duration_ms", 42)` not `log.Info(fmt.Sprintf("GET /users returned 200 in 42ms"))`. The former can be filtered, aggregated, and alerted on; the latter can only be text-searched.
- **Consistent field names:** if the field for request duration is `duration_ms` in one handler, it must be `duration_ms` in every handler — not `elapsed`, `latency_ms`, or `time`. Inconsistent field names fragment queries across log sources.
- **No PII in logs:** names, email addresses, phone numbers, national ID numbers, card numbers, and similar personal data must not appear in log output. Once in a log, PII is hard to purge and creates regulatory exposure.
- **No credentials or tokens in logs:** API keys, session tokens, passwords, and similar secrets in log output are a secondary credential exposure. Truncate or redact before logging.
- **Request/correlation ID included:** structured log entries for request-handling code must include a correlation or trace ID so entries can be grouped across services.

______________________________________________________________________

## Missing log statements

The absence of a log statement is often invisible until an incident.

- **Key decision points with no logging:** branching logic that determines which code path executes (payment gateway selected, feature flag evaluated, role check result) should be logged at an appropriate level.
- **Errors swallowed without logging:** see `/error-handling-review` for the error dimension. From an observability perspective: every swallowed or ignored error is also a missing log entry.
- **External calls with no outcome logged:** every call to an external service, database, or message queue should log the outcome (success or failure) and the duration. Without this, diagnosing slow dependencies requires adding log statements during an incident.
- **Startup and shutdown not logged:** service startup (with configuration values that affect behaviour logged at INFO), graceful shutdown initiated, and shutdown complete should all be logged. Missing these makes it impossible to determine from logs alone when a service restarted.
- **Background jobs and scheduled tasks:** job started, job completed (with record count or relevant metrics), and job failed should all produce log entries.

______________________________________________________________________

## Trace context

Distributed systems require trace context to correlate events across service boundaries.

- **Correlation ID propagated:** a request that enters the system at service A and calls service B must carry a trace or correlation ID that appears in the logs of both services. If A generates an ID but does not pass it to B, the request cannot be traced end-to-end.
- **Trace ID in outbound calls:** HTTP requests, gRPC calls, and message queue publishes to downstream services must include the trace context in headers (`traceparent` for W3C Trace Context, `X-Request-ID`, or the equivalent for the platform in use).
- **Trace ID extracted from inbound calls:** the first handler to receive an inbound request must extract the trace context from the request headers and attach it to the context passed through the call chain.
- **Missing trace propagation across async boundaries:** when an event is published to a message queue and consumed asynchronously, the trace context must be serialised into the message metadata and restored by the consumer.

______________________________________________________________________

## OpenTelemetry semantic conventions

Use OpenTelemetry semantic convention names for spans, metrics, and resources so standard dashboards and alerting rules continue to work. Custom attribute names in place of the standard ones break interoperability.

### Resource attributes

- `service.name`
- `service.version`
- `deployment.environment`

### HTTP spans

- `http.request.method`
- `http.response.status_code`
- `url.full`
- `server.address`
- `server.port`

### Database spans

- `db.system`
- `db.operation.name`
- `db.namespace`
- `db.query.text` (sanitised)

### Error events on spans

- `exception.type`
- `exception.message`
- `exception.stacktrace` (on the span event, not as span attributes)

______________________________________________________________________

## Language-specific instrumentation patterns

### JavaScript and TypeScript

- Prefer the OpenTelemetry JS SDK for tracing and metrics so HTTP servers, queues, and database clients emit standard context.
- Use structured loggers such as `pino` or `winston`; avoid free-form `console.log` in production paths.
- In Node.js services, ensure async context propagation survives `await`, background jobs, and queue consumers.

### Python

- Use `opentelemetry-sdk` for tracing and metrics where distributed context matters.
- Prefer structured logging via `structlog` or a disciplined `logging` configuration; `loguru` can be acceptable if it is already the project standard.
- Verify request, job, and task context is attached to logs consistently across async and threaded code.

### Java

- Micrometer is the usual metrics facade in Spring and other JVM services; check that counters, timers, and gauges follow existing naming conventions.
- Use SLF4J with Logback or the project-standard backend, and rely on MDC for correlation data such as request or trace IDs.
- Confirm OpenTelemetry or equivalent tracing bridges do not fight with framework defaults and duplicate spans.

### C\#

- Use OpenTelemetry for traces and metrics where the service boundary matters.
- Prefer structured logging with Serilog or the built-in structured logging abstractions; ensure correlation identifiers are enriched consistently.
- Background services, hosted workers, and C# request pipelines should all emit compatible fields and trace context.

### C++

- The OpenTelemetry C++ SDK can provide standard trace context when distributed tracing is required, but manual wiring is common and easy to get wrong.
- Prefer structured logging libraries such as `spdlog` over ad-hoc stream concatenation in operational code.
- Review lifetime and flush behaviour carefully: logs and spans buffered in long-lived objects are often lost on abnormal shutdown.

### Go

- Prefer `slog` or the project's structured logger for consistent key-value logs.
- Use the OpenTelemetry Go SDK where traces or metrics cross service boundaries.
- Ensure `context.Context` carries correlation and trace data through goroutines, HTTP clients, and message consumers.

### Rust

- Prefer the `tracing` crate for structured events and spans.
- Use `opentelemetry-rust` integration where traces need to leave the process boundary.
- Confirm async executors preserve span context across spawned tasks and background workers.

### Other languages

If another language is present, identify the project's standard logging, metrics, and tracing stack before reviewing the diff. The essential checks remain the same: structured logs, low-cardinality metrics, trace propagation, and alerts tied to operator action.

______________________________________________________________________

## Signal coverage frameworks

Signal coverage should be evaluated with three standard frameworks. For a service, apply RED as the primary frame and Four Golden Signals as a cross-check. For infrastructure components (queues, databases, caches), apply USE.

### RED (services — request-oriented)

- **Rate:** requests per second (or events per second).
- **Errors:** error rate (count or percentage).
- **Duration:** latency distribution (p50, p95, p99) — not mean.

### USE (resources — infrastructure-oriented)

- **Utilisation:** percentage of time the resource is busy.
- **Saturation:** amount of queued work (backlog).
- **Errors:** error events for the resource.

### Four Golden Signals (Google SRE)

- **Latency:** time to serve a request (distinguish successful vs error latency).
- **Traffic:** demand on the system.
- **Errors:** rate of failing requests.
- **Saturation:** how full the service is — leading indicator of degradation.

______________________________________________________________________

## Metric naming conventions

Metrics that are hard to name or inconsistently named are hard to query and hard to alert on.

### Naming signals

- Units in the metric name: `_seconds`, `_bytes`, `_total` (for counters), `_ratio` (for fractions 0–1).
- Counters named with `_total` suffix (Prometheus convention).
- Inconsistent label names for the same concept across metrics: if request method is labelled `method` on one metric and `http_method` on another, queries must handle both.

______________________________________________________________________

## High-cardinality label warning

- Metric labels (Prometheus) or metric dimensions (Datadog, CloudWatch) must be low-cardinality.
- High-cardinality labels that destroy monitoring systems: `user_id`, `request_id`, `session_id`, `url` with query params, full stack trace fragments.
- These create unbounded time series and cause Prometheus TSDB to OOM or exceed scrape limits.
- The label cardinality of a metric should be expressible as a small finite set — status codes, endpoint names, result types.

______________________________________________________________________

## Alert quality signals

An alert exists to prompt human action. Alerts that fire on the wrong things, too often, or with insufficient context produce alert fatigue and missed real incidents.

- **Alerts on causes not symptoms:** alert on the symptom the user experiences (high error rate, high latency, service unavailable) rather than the internal cause (CPU high, memory high). Internal resource alerts are useful for capacity planning but not for incident response.
- **Missing alerts for key failure modes:** a service that can fail in a way that has no alert will fail silently in production. For each dependency and each critical operation, ask: "If this fails completely, how long before an alert fires?"
- **Alert storms from cascading failures:** a single failure that causes many downstream alerts simultaneously creates noise that obscures the root cause. Alerts should be designed to identify the source, not every downstream effect.
- **Alerts with no runbook:** an alert without a runbook leaves the on-call engineer to improvise during an incident. Every alert should link to a runbook that describes: what the alert means, how to investigate, and what the remediation options are.
- **Alert thresholds that never trigger in normal conditions but also never trigger during real failures:** thresholds set too conservatively (so nothing ever fires) or too aggressively (so everything fires) are both useless. Alert thresholds should be validated against historical incident data.

______________________________________________________________________

## Severity model

### Blocking

The observability gap will prevent detection or diagnosis of a failure in a production environment.

Examples: an external call with no error logging; a background job with no failure alert; trace context not propagated across a service boundary; PII written to logs.

### Recommendation

The observability is present but degraded; incidents will take longer to diagnose than necessary.

Examples: interpolated log strings instead of structured key-value pairs; inconsistent field names across log statements; missing duration logging on external calls; an alert with no runbook.

### Observation

Minor quality issue with low impact in normal operation.

Examples: a log level that could be more appropriate; a metric that could have a more descriptive label; an opportunity to add a log entry at a low-traffic, low-risk decision point.

______________________________________________________________________

## Output format

Group findings by severity (Blocking first). For each finding:

```
**[SEVERITY] location** (file:line or file:function or metric/alert name)
Description: what the issue is.
Why it matters: the consequence during an incident or investigation.
Direction: the general approach to resolution — not an implementation.
```

Do not write log statements, metrics, or alert configurations. Provide direction, not implementation. If no findings exist in a severity tier, omit that tier.
