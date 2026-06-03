---
name: docs-review
description: Load before any documentation review. Required by the docs-reviewer agent — covers coverage thresholds, accuracy signals, completeness checklist, Godoc and JSDoc format, and a three-tier severity model.
license: AGPL-3.0-or-later
---

# Documentation Review Reference

Documentation is the first interface a developer encounters. Absent or inaccurate documentation forces readers into the code to answer questions the code should not have to answer. When reviewing a change, check that the documentation reflects the new state of the code — not the old state, and not nothing.

______________________________________________________________________

## Coverage thresholds

Not all code requires documentation at the same level of detail. Apply these thresholds to the code under review.

### Exported symbols

Every exported (public) function, type, method, constant, and variable must have a doc comment. This applies regardless of how "obvious" the symbol appears to the author. The author's context is not the reader's context.

Missing doc comments on exported symbols are a tooling failure as well as a human one: `godoc`, IDE hover documentation, and `go doc` all surface these comments.

### README

A project README must cover:

- What the project is and what problem it solves (one paragraph).
- How to install or acquire it.
- A minimal working example that can be copied and run.
- Where to get help or report issues.

A README that exists but covers none of these is equivalent to no README.

### DEVELOPMENT.md

A project with contributors (including future self) must document the development workflow:

- How to run the tests.
- How to build the project.
- How to run the application locally.
- Any non-obvious setup steps (environment variables required, services to start, code generation to run).

If DEVELOPMENT.md does not exist and the project has non-trivial build steps, its absence is a finding.

______________________________________________________________________

## Docstring usefulness rubric

| Score | Criterion                                                        |
| ----- | ---------------------------------------------------------------- |
| 0     | Absent — no doc comment                                          |
| 1     | Tautological — restates the name: `// GetUser returns the user`  |
| 2     | Describes what the function does, no params or return documented |
| 3     | What + params + return types documented                          |
| 4     | Adds rationale, invariants, or non-obvious behaviour             |
| 5     | Includes examples, edge cases, and failure modes                 |

Scores 0 and 1 both count as undocumented. A tautological comment is not documentation — it adds noise without value. When detecting missing documentation, score 0–1 as "missing" and 2–3 as "partial"; only 4–5 counts as well-documented.

______________________________________________________________________

## Onboarding simulation

A project's documentation should answer these five questions for a new engineer:

1. What does this system do? (README purpose section)
1. How do I run it locally? (README or DEVELOPMENT.md setup steps)
1. How do I run the tests? (README or DEVELOPMENT.md test command)
1. How do I add a new feature? (contributing guide or architecture overview)
1. Who do I contact when something goes wrong? (CODEOWNERS, support section, or incident runbook)

A "No" on questions 1–3 is a major onboarding barrier. Questions 4–5 are Recommendations. Check these explicitly when reviewing README and DEVELOPMENT.md changes.

______________________________________________________________________

## Accuracy signals

A doc comment that describes old behaviour is worse than no comment: it actively misleads. Every review of a code change must check whether existing documentation accurately reflects the changed code.

- **Doc comments that describe old behaviour:** a function that was modified but whose doc comment describes the pre-modification behaviour. Check: does the comment's description match what the code currently does?
- **Examples that no longer compile or run:** code examples in doc comments or README files that use APIs that have been renamed, removed, or changed. These fail silently (the reader's time is wasted) or loudly (the reader loses confidence in the documentation).
- **Parameter descriptions that don't match current signatures:** a doc comment that names or describes parameters that no longer exist, or omits parameters that have been added.
- **Return value descriptions that are stale:** a doc comment that says "returns nil on error" when the function now returns a typed error.
- **Behaviour documented that is now configuration-dependent:** a doc comment that states a fixed behaviour that has since been made configurable without updating the documentation.

______________________________________________________________________

## Documentation decay

- Documentation decays as code changes without accompanying doc updates.
- Use `git log --follow -n 1 -- <file>` to find when a file was last modified.
- Compare the last modification date of a doc comment or README section against the last modification of the code it documents.
- Signal: documentation that has not been updated across multiple code changes to the same symbol or module is likely stale.
- Flag: README sections that describe removed features, doc comments that reference deleted parameters, or architecture docs that predate major structural changes.

______________________________________________________________________

## Completeness checklist

For each changed exported symbol, verify the doc comment answers these questions:

- What does this function/type/method do?
- What are the preconditions (valid input, required state)?
- What are the postconditions (what is guaranteed about the return value or state change)?
- What errors can be returned, and under what conditions?
- Are edge cases or non-obvious behaviour documented?
- Are the parameters described where non-obvious?
- Is the return value described where non-obvious?

A doc comment that answers "what" without answering "when it fails" or "what the caller must do" is incomplete.

______________________________________________________________________

## Language-specific documentation conventions

### JavaScript and TypeScript

For JavaScript and TypeScript projects using JSDoc:

- **`@param` populated:** every parameter that is not self-explanatory from its name should have a `@param {type} name description` entry.
- **`@returns` populated:** the return value should be documented with `@returns {type} description` unless the function returns `void` or `undefined`.
- **First sentence is a complete sentence:** the summary line (before any tags) should be a complete sentence describing what the function does.
- **Types accurate:** `@param` and `@returns` annotations must match the current function signature. Stale types after a refactor are misleading.
- **`@throws` where meaningful:** if callers must handle a thrown error, document the condition explicitly.

### Python

- **Project docstring style recognised:** detect whether the project uses Google style, NumPy style, or reStructuredText, then apply that style consistently.
- **Parameters and returns documented:** document arguments, return values, and raised exceptions where the behaviour is not obvious from the signature.
- **Module and class docstrings present:** public modules, classes, and complex functions should have docstrings that explain purpose and invariants, not just mechanics.
- **Sphinx or pydoc friendliness:** ensure field names and directives render correctly in the project's documentation tooling.

### Java

- **Javadoc tags complete:** use `@param`, `@return`, and `@throws` where applicable.
- **Behaviour and contracts documented:** Javadoc should cover nullability, side effects, and exceptional conditions, not just restate the method name.
- **Generics and overloads explained:** overloaded methods and generic type parameters need enough context for callers to distinguish the right entry point.
- **Examples stay aligned:** if `@since`, `@deprecated`, or example snippets are present, they must match the current API.

### C\#

- **XML doc comments use `///`:** public types and members should use XML documentation comments recognised by the compiler and IDE tooling.
- **`<param>`, `<returns>`, and `<exception>` accurate:** keep parameter names, return descriptions, and exception conditions aligned with the current signature.
- **`<remarks>` for non-obvious behaviour:** record invariants, side effects, and usage constraints where a one-line summary is not enough.
- **Compiler warnings matter:** missing or broken XML documentation on public APIs should be treated as a documentation quality signal, not ignored noise.

### C++

- **Doxygen-compatible comments:** public APIs should use `///` or `/** */` consistently if the project relies on Doxygen or a similar extractor.
- **Parameters and ownership documented:** document pointer ownership, lifetime expectations, thread-safety guarantees, and error signalling conventions.
- **Template behaviour explained:** template parameters and specialisation constraints must be described where they affect use.
- **Examples compile conceptually:** code examples should match the current header signatures and namespace layout.

### Go

Go's documentation tool (`go doc`, `godoc`) has specific formatting conventions. Comments that violate these conventions are rendered incorrectly or not at all.

- **First sentence:** the doc comment for a function, type, or package must begin with a complete sentence that starts with the name of the symbol. `// UserService manages user lifecycle operations.` not `// This is the user service.`
- **Plain text:** Godoc does not render Markdown. Asterisks, backticks, and `#` headers are rendered literally. Use plain prose. Blank lines produce paragraph breaks.
- **Code examples:** use `Example` functions in `_test.go` files for executable examples. These are verified by `go test` and shown in `go doc` output. Do not embed code examples as plain text in doc comments.
- **Deprecated symbols:** use `// Deprecated:` as its own paragraph to mark a symbol as deprecated. This is recognised by tooling.

### Rust

- **Rustdoc comments use `///`:** public items should use rustdoc comments that render well in `cargo doc`.
- **Sections such as `# Examples`, `# Errors`, and `# Panics`:** use standard rustdoc section headings where they add value.
- **Examples should remain buildable:** doctests should compile or be marked appropriately if they are illustrative only.
- **Trait and lifetime constraints documented:** when a public API relies on lifetimes, ownership, or trait bounds, the docs should explain the practical implications for callers.

### Other languages

If the project uses another language, identify the documentation extractor or compiler convention already in place and apply it consistently. The minimum expectation is still the same: public APIs need accurate summaries, parameter details, return or error behaviour, and examples where the usage is non-obvious.

______________________________________________________________________

## Outdated references

- **Links to renamed or deleted files:** relative links in documentation that point to files that have been moved, renamed, or deleted. These produce 404s in rendered documentation and confusion when following cross-references in an IDE.
- **References to deprecated APIs:** documentation that describes or recommends an API that has been deprecated or removed in the current or a recent version.
- **Version numbers that are stale:** documentation that cites a specific version number (minimum Go version, minimum Node version, framework version) that is no longer accurate.
- **References to external documentation that has moved:** URLs in documentation that redirect or 404. Check key external links when reviewing documentation changes.
- **Architecture diagrams that don't match the current architecture:** diagrams in the repository that show components, services, or relationships that have since changed. Stale diagrams actively mislead.

______________________________________________________________________

## Architecture documentation

Code that embodies significant design decisions must have those decisions recorded. Undocumented decisions are rediscovered expensively or, worse, unknowingly reversed.

- **Significant design decisions recorded:** when a non-obvious choice was made (a specific algorithm, a particular data structure, an unusual pattern), the reason must be recorded in a doc comment, a README section, or an Architecture Decision Record (ADR). "We use X because Y" is the minimum content.
- **Non-obvious architectural choices explained:** code that deviates from the dominant pattern in the codebase requires a comment explaining why. The absence of explanation implies the deviation was accidental.
- **ADRs for reversals of previous decisions:** when the code changes in a way that reverses a previous documented decision, the ADR must be updated or superseded.
- **System integration points documented:** where the system integrates with external services, the integration contract (protocol, auth, data format, failure handling expectations) must be documented. This documentation is essential for operators and future maintainers.

______________________________________________________________________

## ADR lifecycle

Architecture Decision Records should have a lifecycle status tag:

- `Proposed` — under discussion, not yet accepted.
- `Accepted` — in force.
- `Deprecated` — superseded or no longer applicable; the replacement should be linked.
- `Superseded` — replaced by a specific newer ADR (link required).
- `Rejected` — considered but not adopted; reason should be documented.

Signals:

- ADR stuck in `Proposed` without recent activity suggests a decision is being deferred.
- `Deprecated` or `Superseded` ADRs without a link to the replacement leave readers without guidance.
- ADRs that describe an approach that is no longer present in the codebase should be updated to `Deprecated`.

______________________________________________________________________

## Severity model

### Blocking

The documentation is absent or incorrect in a way that will cause integration failures, compliance issues, or significant wasted time for the next person who works with this code.

Examples: a changed exported function with no doc comment in a public API; a README example that no longer works; a DEVELOPMENT.md that documents a build step that has been removed and replaced with something that requires different setup.

### Recommendation

The documentation is present but incomplete or stale in a way that will confuse readers.

Examples: a doc comment missing the error conditions; a stale parameter description; a link to a renamed file; a design decision made during this change with no explanation.

### Observation

Minor quality issue. Low priority.

Examples: a doc comment that could be more descriptive; an example that could demonstrate a more useful use case; a minor formatting correction for Godoc conventions.

______________________________________________________________________

## Output format

Group findings by severity (Blocking first). For each finding:

```
**[SEVERITY] location** (file:line or symbol name or document)
Description: what the issue is.
Why it matters: the consequence for readers or integrators.
Direction: the general approach to resolution — not an implementation.
```

Do not write documentation. Provide direction, not content. If no findings exist in a severity tier, omit that tier.
