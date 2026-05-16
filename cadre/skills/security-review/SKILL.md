---
name: security-review
description: Load before any security review. Required by the security-reviewer agent — covers OWASP Top 10 (2021) with code-level signals, secrets hygiene, input validation, error message hygiene, auth patterns, and a four-tier severity model.
license: AGPL-3.0-or-later
allowed-tools: read
---

# Security Review Reference

Security findings are not a style preference — they represent exploitable risk. Apply this reference to every code change that touches authentication, authorisation, input handling, external calls, configuration loading, or data persistence.

______________________________________________________________________

## OWASP Top 10 (2021)

### A01 — Broken Access Control

Access control enforces that users can only perform actions and access data they are permitted to. Broken access control is the most common web application security failure.

Code-level signals:

- Route handlers or functions that perform operations without checking the caller's identity or role.
- Authorisation checks that are present on some endpoints but missing on others that access the same resource.
- Direct Object References: a resource ID taken from user input used directly in a query without verifying the caller owns or has access to that resource.
- Horizontal privilege escalation: user A can access user B's data by changing a parameter.
- `isAdmin` or equivalent flag that can be supplied by the client rather than derived from a trusted source.

______________________________________________________________________

### A02 — Cryptographic Failures

Sensitive data exposed in transit or at rest due to absent or weak cryptography.

Code-level signals:

- Passwords stored as plaintext or with a reversible encoding (base64, hex).
- Passwords hashed with MD5 or SHA-1 — these are not password hashing algorithms.
- Sensitive data (PII, tokens, credentials) transmitted over HTTP rather than HTTPS.
- TLS certificate validation disabled in HTTP client configuration.
- Symmetric encryption keys hardcoded in source or stored alongside the data they protect.
- Cryptographic nonces or IVs reused across encryptions.
- Use of deprecated or known-weak cipher suites.

______________________________________________________________________

### A03 — Injection

User-supplied data interpreted as code or commands.

Code-level signals:

- SQL queries constructed by string concatenation with user input rather than parameterised queries or prepared statements.
- Shell commands constructed with user input: `exec("cmd " + userInput)`.
- Template rendering with unsanitised user input where the template engine can evaluate code.
- XML/HTML constructed by string concatenation — XSS vectors in HTML; XXE in XML.
- LDAP queries or NoSQL queries built with unsanitised user input.
- Log injection: user-supplied strings written directly to log output without sanitisation.

______________________________________________________________________

### A04 — Insecure Design

Architectural and design choices that inherently cannot be made secure by implementation. No amount of correct implementation fixes a fundamentally insecure design.

Code-level signals:

- Business logic that can be bypassed by calling steps out of order (e.g., paying after receiving the goods by manipulating request sequence).
- Multi-tenancy implemented by filtering at the application layer with no database-level separation.
- Rate limiting absent on sensitive operations (login, password reset, MFA).
- Password reset flows that reveal whether an account exists or send tokens that do not expire.
- Security controls that rely on the client to report the user's own role or permissions.

______________________________________________________________________

### A05 — Security Misconfiguration

Default configurations, unnecessary features enabled, or missing hardening.

Code-level signals:

- Debug mode enabled or debug endpoints exposed in a production configuration.
- Default credentials unchanged in shipped configuration.
- Unnecessary services, ports, or features enabled.
- CORS configured with `*` (any origin) on endpoints that serve authenticated data.
- Security headers absent: `Content-Security-Policy`, `X-Frame-Options`, `Strict-Transport-Security`.
- Stack traces returned directly to API callers.
- Verbose error messages that expose internal paths, version strings, or framework identifiers.

______________________________________________________________________

### A06 — Vulnerable and Outdated Components

Dependencies with known vulnerabilities.

Code-level signals:

- Direct dependencies with published CVEs. Check with the language-appropriate audit tool.
- Dependencies pinned to versions that are many major releases behind.
- Dependencies that have been archived or abandoned by their maintainers.
- Vendored copies of libraries that are not kept up to date with upstream security patches.

See also the `/dependency-review` skill for a full treatment of dependency risk.

______________________________________________________________________

### A07 — Identification and Authentication Failures

Weaknesses in how identity is established and sessions are managed.

Code-level signals:

- Password policy not enforced (no minimum length, no complexity, no breach-list check).
- Login endpoint with no rate limiting or account lockout after repeated failures.
- Session tokens with no expiry or with expiry set to an unreasonably long value.
- Session token not invalidated on logout.
- JWT `alg` field accepted from the token itself rather than enforced server-side; `alg: none` accepted.
- JWT not verified (signature check skipped or exception swallowed).
- Credentials or session tokens transmitted in URL query parameters (visible in logs, referrer headers).

______________________________________________________________________

### A08 — Software and Data Integrity Failures

Assumptions about the integrity of software updates, data, or deserialised objects that are not verified.

Code-level signals:

- Deserialisation of untrusted data using formats that can instantiate arbitrary types (`pickle`, Java native serialisation, or equivalent runtime-native object formats).
- CI/CD pipeline that fetches and executes scripts from external URLs without integrity verification.
- Dependency lockfiles absent or not committed — builds can silently pull different versions.
- Cryptographic signatures on software updates or releases not verified before installation.
- Webhook payloads processed without verifying the signature supplied by the sender.

______________________________________________________________________

### A09 — Security Logging and Monitoring Failures

Insufficient logging of security-relevant events prevents detection of and response to attacks.

Code-level signals:

- Login attempts (success and failure) not logged.
- Access control failures not logged with enough context to identify the requesting identity.
- High-value transactions (payment, privilege change, data export) not logged with a correlation ID.
- Logs that contain credentials, tokens, or PII — these are a secondary vulnerability.
- Log storage that an attacker who has gained access can modify or delete.

See also the `/observability-review` skill for general logging quality.

______________________________________________________________________

### A10 — Server-Side Request Forgery (SSRF)

The server fetches a URL that is fully or partially controlled by the caller.

Code-level signals:

- A URL taken from user input passed directly to an HTTP client, file reader, or DNS resolver.
- URL validation that relies on blocklists (localhost, 169.254.x.x) rather than allowlists of known-good targets.
- Redirect following enabled on an HTTP client that fetches user-supplied URLs.
- Image, file, or webhook URL fields that are not restricted to external, pre-approved domains.

______________________________________________________________________

## SAST tooling

Run `semgrep --config=auto .` as a first-pass SAST step. Semgrep auto-selects rulesets for the project's detected languages using the community and security rule registry. It covers all seven canonical languages at generally-available quality, though ruleset depth is deepest for JavaScript, TypeScript, Python, Java, C#, and Go — Rust and C++ have full language support but fewer community rules.

Semgrep does not replace reading the code. It surfaces known-bad patterns quickly. Manual review of the diff for business-logic and context-dependent risks (broken access control, IDOR, privilege escalation) remains essential.

For **Rust**: also run `cargo-geiger` (`cargo geiger`) to count and audit `unsafe` blocks and their transitive exposure.

For **C++**: also run `clang-tidy -checks='cert-*,bugprone-*,cppcoreguidelines-*'` for additional language-specific signals.

______________________________________________________________________

## Language-specific risk hotspots

### JavaScript and TypeScript

- Prototype pollution via unsafe deep merge helpers or object assignment into trusted configuration objects.
- `eval`, `new Function`, or template engines that execute user-controlled strings.
- XSS sinks such as `innerHTML`, `dangerouslySetInnerHTML`, or unsafe DOM construction from untrusted input.

### Python

- `pickle` or similarly unsafe deserialisation of untrusted data.
- Server-side template injection through Jinja2 or equivalent template engines fed with untrusted expressions.
- Shell injection via `subprocess` when `shell=True` or string-built commands include user input.

### Java

- Deserialisation gadget chains through Java native serialisation or libraries with unsafe object input.
- XXE in XML parsers where external entities remain enabled.
- OGNL, SpEL, or other expression-language injection where user input reaches an evaluator.

### C\#

- Unsafe deserialisation through binary or XML serialisers that materialise attacker-controlled types or graphs.
- XXE via `XmlDocument`, `XmlReader`, or similar XML APIs when DTD and external entity processing is not disabled.
- Dynamic expression compilation or reflection over user-controlled type names without strict allowlisting.

### C++

- Buffer overflows from unchecked copies, length mismatches, or manual memory management.
- Use-after-free and double-free risks when ownership is unclear or lifetime rules are violated.
- Integer overflow or truncation that leads to undersized allocations or bypassed bounds checks.

### Go

- Command injection when user input reaches `exec.Command` arguments or shell wrappers incorrectly.
- SSRF where user-supplied URLs flow into `http.Get`, custom transport code, or cloud metadata lookups.
- Unsafe template execution or HTML generation that bypasses the standard escaping rules.

### Rust

- `unsafe` blocks that assume invariants not enforced at the boundary.
- FFI boundaries where C memory, length, or ownership rules are trusted without validation.
- Deserialisation or parsing code that panics on malformed attacker input instead of rejecting it safely.

### Other languages

If another language is present, identify the equivalent high-risk primitives: unsafe deserialisation, dynamic evaluation, shell execution, XML parsing, raw memory handling, and trust-boundary crossings.

______________________________________________________________________

## STRIDE threat model

For each identified entry point (HTTP handler, CLI command, message consumer, webhook), apply STRIDE categorisation:

| Threat                 | Description                                 | Key question                                                |
| ---------------------- | ------------------------------------------- | ----------------------------------------------------------- |
| Spoofing               | Impersonating another user or system        | Is identity verified at this entry point?                   |
| Tampering              | Modifying data in transit or at rest        | Is integrity protected? Are writes authorised?              |
| Repudiation            | Denying that an action was taken            | Is there an audit trail?                                    |
| Information disclosure | Exposing data to unauthorised parties       | What data does this endpoint return? Who can call it?       |
| Denial of service      | Making the service unavailable              | Is this endpoint rate-limited? Are inputs bounded?          |
| Elevation of privilege | Gaining permissions beyond what was granted | Can a low-privilege caller reach high-privilege operations? |

This is a starter threat model. Apply it to changed entry points to frame findings in terms of risk, not just defect.

______________________________________________________________________

## OWASP ASVS

OWASP ASVS v4 provides a more thorough assurance framework beyond the Top 10, with three assurance levels:

- L1: opportunistic — basic controls expected in any application.
- L2: standard — appropriate for most commercial applications.
- L3: advanced — high-value targets (financial, healthcare, critical infrastructure).

When a deep assurance review is requested (not a diff review), map findings to ASVS v4 categories (V1 Architecture through V14 Configuration). For diff reviews, the Top 10 is sufficient.

______________________________________________________________________

## Secrets hygiene

- **Hardcoded credentials:** API keys, passwords, tokens, or private keys present in source code — including test code and example files.
- **Secrets in logs:** credential values written to log output; token values included in error messages or debug output.
- **Secrets in error messages:** internal error detail that includes token fragments, connection strings, or key material returned to callers.
- **Secrets in environment without protection:** environment variable names that suggest they hold secrets (`*_KEY`, `*_SECRET`, `*_TOKEN`, `*_PASSWORD`) loaded and then logged or returned in responses.
- **Git history scanning:** hardcoded secrets removed in later commits still live in git history and are exploitable — scan git history with `trufflehog git file://.` or `gitleaks detect --source .` (both scan history by default); a secret found only in history is a Critical finding because it may have already been extracted.

### Tool note

- `trufflehog` — `trufflehog git file://.` scans the repository and git history for leaked secrets.
- `gitleaks` — `gitleaks detect --source .` scans the repository and git history for leaked secrets.

______________________________________________________________________

## Input validation

- **Unsanitised user input** passed to downstream systems (queries, shell commands, templates, XML processors) without sanitisation or parameterisation.
- **Missing length checks:** string or binary inputs accepted without an upper bound; can be used for denial-of-service or buffer overflows.
- **Missing type checks:** inputs expected to be integers or UUIDs accepted as arbitrary strings and parsed without validation.
- **Missing range checks:** numeric inputs used in business logic (quantities, prices, indices) accepted without bounds checking.
- **Trusting client-provided data for authorisation:** user ID, role, or permission level taken from the request body or query string rather than from the authenticated session.
- **File upload without type validation:** file uploads accepted based only on the client-supplied `Content-Type` or filename extension rather than inspecting the file content.

______________________________________________________________________

## Error message hygiene

- **Stack traces to callers:** full stack traces returned in API responses reveal internal structure and can point attackers at specific code paths.
- **Internal paths or names leaked:** file system paths, class names, function names, or database schema details exposed in error responses.
- **Version strings exposed:** framework or runtime version numbers in response headers or error bodies enable targeted exploitation of known CVEs.
- **Enumeration-enabling messages:** login errors that distinguish "user does not exist" from "wrong password" allow attackers to enumerate valid usernames. Use a generic message for both.
- **Timing oracles:** code paths that take measurably different time for valid vs invalid inputs can leak information about existence or validity.

______________________________________________________________________

## Auth patterns

- **Missing authentication checks:** an endpoint that performs a sensitive operation is reachable without authentication because the auth middleware was not applied to that route.
- **Privilege escalation paths:** a lower-privileged user can reach a higher-privileged operation by manipulating a role field, resource ID, or request parameter.
- **JWT/token misuse:** see A07 signals above. Additionally: tokens not scoped to the minimum necessary permissions; refresh tokens with the same or longer lifetime than access tokens with no revocation mechanism.
- **Inconsistent auth application:** authentication applied to some methods on a resource but not others (e.g., `GET /admin/users` authenticated, `DELETE /admin/users/:id` not).
- **Auth bypass for internal endpoints:** endpoints prefixed `/internal/` or `/admin/` that rely on network segmentation rather than explicit authentication — an SSRF or misconfigured proxy bypasses this.

______________________________________________________________________

## Severity model

### Critical

Exploitable now with low effort by an external attacker. Immediate action required. Do not merge.

Examples: SQL injection in a public endpoint; hardcoded production credential; authentication bypass.

### High

Likely exploitable by a motivated attacker; may require specific conditions or authenticated access.

Examples: SSRF with no URL validation; JWT signature not verified; IDOR allowing horizontal access to other users' data.

### Medium

Increases the attack surface or aids exploitation; not directly exploitable in isolation.

Examples: verbose error messages exposing stack traces; missing rate limiting on login; CORS misconfiguration.

### Low

Hardening improvement; does not represent an immediate exploitable risk.

Examples: security header absent; password complexity policy not enforced at the API level; log entry missing a correlation ID.

______________________________________________________________________

## Output format

Include a brief `Security posture strengths` paragraph noting what protective measures are already in place (HttpOnly/Secure/SameSite cookies, CSP/HSTS headers, CSRF protection, rate limiting, input validation libraries). A reviewer that only lists problems is less useful than one that says "here's what's in place, here's what's missing."

Group findings by severity (Critical first, then High, Medium, Low). For each finding:

```
**[SEVERITY] location** (file:line or endpoint)
Description: what the issue is.
Why it matters: the specific attack vector or consequence.
Direction: the general approach to resolution — not an implementation.
```

Do not write security patches. Do not rewrite the code under review. Provide direction, not implementation. If no findings exist in a severity tier, omit that tier.
