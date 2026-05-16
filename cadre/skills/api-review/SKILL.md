---
name: api-review
description: Load before any API or CLI interface review. Required by the api-reviewer agent — covers REST naming, HTTP semantics, error consistency, breaking changes, CLI conventions, pagination, auth, and a three-tier severity model.
license: AGPL-3.0-or-later
allowed-tools: read
---

# API Review Reference

An API is a contract. Once callers depend on it, changes are costly. Review API changes with that permanence in mind. Apply this skill to REST HTTP APIs and CLI interfaces. The standards differ in form but share the same underlying goal: a predictable, consistent contract that callers can rely on.

______________________________________________________________________

## Naming consistency

### REST APIs

- Resources are nouns, not verbs. `/users`, `/orders/:id`, not `/getUser`, `/createOrder`.
- Collection endpoints are plural: `/users`, not `/user`.
- Hierarchical relationships expressed in the path: `/users/:id/orders`, not `/userOrders?userId=`.
- Consistent casing: pick one convention (`camelCase`, `snake_case`, `kebab-case`) and apply it uniformly across all endpoints in the API.
- Action endpoints where a resource model is insufficient: use a sub-resource noun or a deliberate POST to a verb-like resource (`/orders/:id/cancellations`, not `/cancelOrder`).

### CLI interfaces

- Long flags use `--kebab-case`: `--output-format`, not `--outputFormat` or `--output_format`.
- Short flags are single characters: `-o`, `-v`. Reserve them for the most commonly used flags.
- Flag names are consistent across subcommands: if `--output` sets the output file in one command, it should mean the same thing in all commands. Do not reuse flag names for different purposes.
- Subcommand names follow a `<noun> <verb>` or `<verb> <noun>` convention applied consistently: either `app deploy` everywhere or `deploy app` everywhere, not mixed.
- Boolean flags are named as affirmatives: `--verbose`, `--force`, `--dry-run`. Avoid negated flag names like `--no-verify` as the only option — provide both when ambiguity exists.

______________________________________________________________________

## HTTP method semantics

Using the wrong HTTP method breaks caching, idempotency guarantees, and client expectations.

| Method | Safe? | Idempotent? | Correct use                                                       |
| ------ | ----- | ----------- | ----------------------------------------------------------------- |
| GET    | Yes   | Yes         | Retrieve a resource or collection; must not mutate state          |
| HEAD   | Yes   | Yes         | Same as GET but response body omitted; for metadata checks        |
| POST   | No    | No          | Create a new resource; submit data for processing                 |
| PUT    | No    | Yes         | Replace a resource in full at a known URI                         |
| PATCH  | No    | No\*        | Apply a partial update to a resource; only changed fields         |
| DELETE | No    | Yes         | Remove a resource; repeated calls must return 204 or 404, not 500 |

\*PATCH is not inherently idempotent but should be designed to be so when possible.

Common violations:

- A GET endpoint that creates or mutates data.
- Using POST for both create and full-replace instead of PUT.
- A DELETE that returns 500 on a second call for the same resource instead of 404 or 204.
- A PATCH body that must contain all fields (making it effectively PUT).

______________________________________________________________________

## Error response consistency

Callers integrate against your error responses exactly as they do your success responses. Inconsistency forces callers to write special-case handling.

- **Uniform error shape:** every error response from every endpoint must have the same structure. Example: `{ "error": { "code": "...", "message": "...", "details": [...] } }`. A mix of `{ "error": "..." }` and `{ "message": "..." }` breaks callers.
- **Appropriate HTTP status codes:**
  - 400 Bad Request — invalid input, validation failure
  - 401 Unauthorised — not authenticated
  - 403 Forbidden — authenticated but not authorised
  - 404 Not Found — resource does not exist
  - 409 Conflict — state conflict (duplicate, version mismatch)
  - 422 Unprocessable Entity — semantically invalid input (business rule violation)
  - 429 Too Many Requests — rate limited
  - 500 Internal Server Error — unexpected server failure
  - Do not use 200 with an error body. Do not use 400 for authorisation failures (use 403).
- **Error messages useful to callers:** the message should tell the caller what went wrong and, where possible, what to do. "Invalid request" is not useful. "Field 'email' must be a valid email address" is useful.
- **Machine-readable error codes:** include a stable string code (e.g., `"code": "INVALID_EMAIL"`) alongside the human-readable message. This allows callers to handle specific errors programmatically without parsing message text.

______________________________________________________________________

## Breaking vs non-breaking changes

A breaking change is any change that causes an existing, correctly-written caller to fail or behave incorrectly without modification.

### Breaking changes

- Removing an endpoint.
- Removing a required or optional field from a response body.
- Changing the type of a field (string → integer, object → array).
- Changing the semantics of an existing field without a name change.
- Making a previously optional request field required.
- Changing a success status code to a different success code (e.g., 200 → 201).
- Changing authentication requirements (adding auth to a previously open endpoint).

### Non-breaking changes

- Adding a new optional field to a response body (callers ignore unknown fields).
- Adding a new optional field to a request body with a sensible default.
- Adding a new endpoint.
- Decreasing a rate limit (technically observable but not a correctness break for most callers).

### Versioning strategies

- URL path versioning (`/v1/`, `/v2/`): explicit, cacheable, easy to route; deprecated versions visible.
- Header versioning (`Accept: application/vnd.api+json;version=2`): clean URLs; harder to test in a browser.
- Query parameter versioning (`?version=2`): simple but pollutes query strings.

Whichever strategy is used must be applied consistently across the entire API.

______________________________________________________________________

## CLI specifics

### Exit codes

- `0`: success.
- Non-zero: failure. The specific non-zero value must be consistent and documented.
- Distinct failure modes must have distinct exit codes if callers need to distinguish them programmatically.
- Do not exit with `0` on a failure. Do not exit with a non-zero code on success.

### stderr and stdout

- `stdout`: the output of a successful operation. This is what gets piped, redirected, or processed by callers.
- `stderr`: error messages, warnings, progress indicators, diagnostics. These must not go to stdout.
- A command that mixes structured output with human-readable diagnostics on stdout breaks composability.

### Help text quality

- Every command and subcommand has a one-line description and a longer usage section.
- Every flag has a description.
- Defaults are shown in the help text: `--timeout duration   request timeout (default: 30s)`.
- At least one example is provided for non-trivial commands.
- The help text is reachable with `--help` and `-h` on every subcommand.

______________________________________________________________________

## Pagination

- Consistent pattern: all collection endpoints use the same pagination mechanism.
- **Cursor-based (recommended for large or frequently-updated collections):** a `cursor` or `next_token` returned in the response body and passed back as a parameter in the next request. Stable under concurrent writes; does not skip or duplicate items.
- **Offset-based:** `page` and `per_page` or `offset` and `limit`. Simple to implement; can skip or duplicate items when the collection is modified between pages.
- Maximum page size enforced server-side; requests above the maximum are capped, not rejected.
- A `total_count` field is useful but must not be required for callers to determine whether more pages exist — use a `has_more` boolean or a `next_cursor` that is absent when the final page is reached.
- Pagination parameters absent from collection endpoints that could grow unboundedly is a latent performance and reliability issue.

______________________________________________________________________

## Auth

- Consistent mechanism across all endpoints: do not mix Bearer tokens on some endpoints with API keys on others without a clear documented reason.
- No auth bypasses for "convenience" endpoints: an endpoint labelled `/internal/` or `/debug/` that skips the auth middleware is an access control vulnerability. See `/security-review` for the security dimension.
- Service-to-service auth is distinct from user auth; both must be present where required; neither substitutes for the other.
- Auth errors return 401 for missing or invalid credentials; 403 for authenticated but unauthorised. Using 401 for both obscures whether the caller is unauthenticated or lacks permission.

______________________________________________________________________

## Language-specific API surface conventions

Apply the general contract checks above first, then use the dominant framework conventions for the language in use.

### JavaScript and TypeScript

- Express and Fastify handlers should keep route registration, validation, and response shaping consistent across modules. A mix of ad-hoc middleware order or untyped `req.body` access is a contract risk.
- Prefer generated or schema-derived OpenAPI types for request and response payloads so the wire contract and the code stay aligned.
- Async handlers must return or await the response path consistently; hidden Promise rejections often surface as inconsistent 500 responses.

### Python

- FastAPI and Flask endpoints should validate input at the boundary and return one consistent error shape across blueprints or routers.
- In FastAPI, keep Pydantic models and OpenAPI metadata aligned; missing response models or ad-hoc dict responses erode the contract.
- Flask projects need extra care around manual status-code handling because nothing enforces a consistent response envelope by default.

### Java

- Spring Boot controllers should use consistent request mapping, validation annotations, and exception-to-response mapping across controllers.
- DTOs exposed on the wire should evolve deliberately; leaking JPA entities or framework-internal types couples the API to implementation details.
- Keep OpenAPI annotations, Jackson field names, and actual serialised output aligned.

### C\#

- C# web API controllers and minimal APIs should apply model binding, validation, and `ProblemDetails` consistently across endpoints.
- Route templates, API versioning attributes, and OpenAPI metadata should agree; mismatches confuse callers and client generation.
- Avoid returning anonymous or shape-shifting payloads from adjacent endpoints that conceptually represent the same resource.

### C++

- C++ HTTP and gRPC APIs are often thin wrappers over lower-level services, so contract drift hides in manual serialisation code. Check field presence, status mapping, and lifetime ownership carefully.
- For gRPC services, `.proto` files are the primary contract; for REST wrappers, confirm that route naming and error payloads are documented because framework conventions are weaker than in higher-level stacks.

### Go

- `net/http`, chi, gin, and echo handlers should keep route patterns, status codes, and JSON field naming consistent across packages.
- Validate request payloads before business logic and keep error envelope shapes stable; handwritten JSON responses drift easily across handlers.
- If OpenAPI is generated, confirm the generated contract still matches the actual handler behaviour and middleware.

### Rust

- axum and actix handlers should make extractor validation and rejection handling explicit so callers get predictable 4xx responses rather than framework-default 500s.
- Keep serde field names, enum tagging, and OpenAPI schema generation aligned with the published contract.
- Prefer one error-to-response mapping strategy per service; mixed `IntoResponse` implementations often create inconsistent payloads.

### Other languages

Use the framework's documented contract conventions where they exist, and fall back to the core checks in this skill: stable naming, predictable status codes, explicit versioning, and consistent error envelopes.

______________________________________________________________________

## GraphQL APIs

- Relay Cursor Connection spec: paginated fields should follow the Relay spec (`edges`, `node`, `cursor`, `pageInfo`) for client compatibility.
- Nullability discipline: every field that can legitimately be absent should be nullable; over-using non-null (`!`) makes schema evolution breaking.
- Deprecation via directive: use `@deprecated(reason: "...")` instead of removing fields — removal is a breaking change.
- N+1 risk in resolvers: a resolver that fetches one record per parent triggers N+1 queries — DataLoader (or equivalent batching) must be used for list resolvers that call a data store.
- Error handling: prefer union types or Result types for expected errors rather than the `errors` array, which conflates expected failures with server errors.

______________________________________________________________________

## gRPC / Protobuf APIs

- No field tag renumbering: changing a field's tag number in a `.proto` file is a breaking wire change — existing serialised messages will misread the field.
- `reserved` for removed fields: when removing a field, mark the tag and name as `reserved` to prevent future reuse.
- Scalar widening: `int32` → `int64` is a breaking wire change in some language implementations despite appearing safe — use `google.protobuf.Int64Value` wrapper for optional integers.
- Well-Known Types: prefer `google.protobuf.Timestamp` over custom date representations; `FieldMask` for partial updates; `Duration` for time spans.
- Service evolution: new RPCs are always safe; changing request/response message types follows the proto3 field addition rules.

______________________________________________________________________

## Async / Event APIs

- AsyncAPI spec: if the project exposes event-driven interfaces, an AsyncAPI document should describe topics, schemas, and bindings — the same way OpenAPI describes HTTP APIs.
- Topic and queue naming: names should follow a consistent convention (e.g. `domain.entity.event`) — inconsistent naming makes consumer discovery and filtering unreliable.
- Idempotency: every event consumer must handle duplicate delivery; events should carry a stable `message_id` for consumer-side deduplication.
- Dead-letter queues: every consumer should have a DLQ for messages that fail processing after the retry limit — a consumer without a DLQ silently drops poison messages.
- Webhook signing: outbound webhooks must carry an HMAC signature (`X-Hub-Signature-256` or equivalent) and a replay-protection timestamp — consumers must verify both before processing.

______________________________________________________________________

## HTTP contract headers

- Rate limiting: endpoints that enforce rate limits should return `X-RateLimit-Limit`, `X-RateLimit-Remaining`, `X-RateLimit-Reset` and `Retry-After` on 429 responses.
- Idempotency: non-idempotent write operations (payment, order submission) should accept an `Idempotency-Key` header and return the same response for duplicate requests within a window.
- Conditional requests: mutable resources should support `ETag` + `If-Match` / `If-None-Match` for optimistic concurrency and `304 Not Modified` for caching.
- Cache-Control: responses must include explicit `Cache-Control` directives — an absent header means the response is potentially cached indefinitely by intermediaries.

______________________________________________________________________

## Richardson Maturity Model

- **Level 0 — HTTP as a transport:** a single endpoint, all operations sent as POST, semantics in the payload. Flagging: any API that posts all operations to one path.
- **Level 1 — Resources:** separate endpoints per resource, but using POST for everything. Flagging: endpoints exist but ignore HTTP verbs.
- **Level 2 — HTTP verbs:** correct use of GET/POST/PUT/PATCH/DELETE with appropriate status codes. This is the expected baseline.
- **Level 3 — Hypermedia (HATEOAS):** responses include links to related resources and next actions. Optional; only flag regression from a previously HATEOAS API.

Signal: a new endpoint that introduces Level 0 or Level 1 behaviour in an otherwise Level 2 API is a Recommendation finding.

______________________________________________________________________

## Severity model

### Blocking

The API contract is incorrect or will cause integration failures. Must not be merged.

Examples: a breaking change with no version bump; an endpoint that returns 200 with an error body; missing auth on a sensitive endpoint.

### Recommendation

The API works but has consistency or usability problems that will affect callers and accumulate as technical debt.

Examples: inconsistent error response shape; wrong HTTP verb; pagination missing from an unbounded collection endpoint; flag naming inconsistency in CLI.

### Observation

Minor or informational. Low priority.

Examples: a response field that could be named more clearly; a help text entry missing a default value; an opportunity to add a machine-readable error code.

______________________________________________________________________

## Output format

Group findings by severity (Blocking first). For each finding:

```
**[SEVERITY] location** (file:line or endpoint or command)
Description: what the issue is.
Why it matters: the consequence for callers or integrators.
Direction: the general approach to resolution — not an implementation.
```

Do not write implementation code. Provide direction, not implementation. If no findings exist in a severity tier, omit that tier.
