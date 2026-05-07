---
name: dead-code-review
description: Patterns for detecting unreachable code, unused exports, orphaned files, stale feature flags, and zombie dependencies. Use to identify code that exists but is never executed or accessed.
license: AGPL-3.0-or-later
allowed-tools: read
---

Dead code increases maintenance burden, inflates cognitive load, introduces false confidence in test coverage, and can conceal security-relevant code paths. Every category below is a distinct detection target.

______________________________________________________________________

## Categories

### Unreachable code

Code that can never be reached by any execution path:

- Statements after an unconditional `return`, `throw`, `panic`, or `exit`
- Branches of a conditional where the condition is always true or always false (constant folding)
- `default` cases in exhaustive switches that the type system guarantees cannot be reached
- Catch/rescue blocks for exception types that the enclosed code can never throw
- Loop bodies that are never entered because the loop condition is always false on entry

### Unused exports

Symbols that are exported (public) but never imported or called outside the defining package:

- Exported functions, methods, types, constants, and variables with no external callers
- Public interface methods that exist solely to satisfy an interface no external code uses
- Re-exported names (type aliases, re-exports) from removed APIs

Note: exported symbols in a library may have legitimate external consumers that are not visible in the current codebase. Flag with Observation severity and note that external consumers cannot be ruled out without broader analysis.

### Orphaned files

Source files that are not reachable from any entry point:

- Source files not imported by any other file and not an entry point themselves
- Test files that import a package that no longer exists
- Configuration files referenced by no build tool, CI workflow, or application code
- Documentation files (`docs/`, `spec/`) that describe features removed from the codebase

### Stale feature flags

Feature flags that are permanently enabled or permanently disabled and whose conditional branches are therefore dead:

- A flag that is hardcoded to `true` — the `false` branch is dead code
- A flag that is hardcoded to `false` — the `true` branch is dead code
- A flag whose value is read from configuration but the configuration key no longer exists
- A flag that was introduced for a rollout that completed — both branches coexist but one will never be activated again

Signals: search for flag-read patterns (`getFlag(`, `isEnabled(`, `featureEnabled`, `flags.Get`, environment variable reads for feature names) and trace whether the flag value can still vary at runtime.

### Zombie dependencies

Dependencies declared in the manifest but never imported in source code:

- A package listed in `dependencies` (or equivalent) with no `import` / `require` / `use` in any source file
- A dependency that was a transitive requirement of a removed feature and is now only transitively required — check whether the manifest entry is still necessary or whether the transitive pull is sufficient
- Development tools listed in `dependencies` instead of `devDependencies` (not dead code, but misclassification)

### Commented-out code blocks

Code that was disabled by commenting rather than deleted:

- Blocks of commented-out source code (as opposed to explanatory comments)
- `TODO: re-enable when X` comments attached to commented-out code where X has long since happened or is no longer relevant
- Conditional compilation guards (`#if DEBUG`, `//go:build ignore`) that permanently exclude a block

Note: small commented-out lines may be intentional (e.g., a temporarily disabled assertion during debugging). Flag multi-line blocks or blocks with no explanatory note.

### Dead routes and dead endpoints

HTTP routes, CLI subcommands, or message consumer subscriptions that are registered but never reached:

- A route that is defined but overshadowed by a more specific route registered earlier
- A CLI subcommand registered with a router but with no handler attached (panics or no-ops on invocation)
- A message topic or queue that is subscribed to but the consumer has no handler for any message type on that topic
- An RPC method in a `.proto` service definition with no server-side implementation

______________________________________________________________________

## Severity model

| Severity       | Criteria                                                                                                                                                                   |
| -------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Blocking       | Unreachable code after a mandatory return/throw; a dead route that shadows a real route; a permanently-disabled feature flag whose code has diverged from the enabled path |
| Recommendation | Unused exported symbols with no plausible external consumer; zombie dependencies; orphaned files with no external references; large commented-out blocks                   |
| Observation    | Stale feature flags that are still wired correctly; dead routes in non-public interfaces; unused exports in libraries (external consumers possible)                        |

______________________________________________________________________

## Detection tooling

| Language                  | Tool                                           | Command                                     |
| ------------------------- | ---------------------------------------------- | ------------------------------------------- |
| JavaScript and TypeScript | `knip`                                         | `npx knip`                                  |
| JavaScript and TypeScript | `depcheck`                                     | `npx depcheck`                              |
| JavaScript and TypeScript | `ts-prune`                                     | `npx ts-prune`                              |
| Python                    | `vulture`                                      | `uvx vulture .`                             |
| Java                      | PMD UnusedCode                                 | `mvn pmd:check`                             |
| Java                      | SpotBugs or IDE unused detection               | `mvn spotbugs:check` or IntelliJ inspection |
| C#                        | Roslyn analysers                               | `dotnet build`                              |
| C#                        | `dotnet-unused`                                | `dotnet tool run dotnet-unused`             |
| C++                       | `clang-tidy`                                   | `clang-tidy <files>`                        |
| C++                       | `cppcheck`                                     | `cppcheck --enable=unusedFunction,style .`  |
| Go                        | `deadcode` (`golang.org/x/tools/cmd/deadcode`) | `deadcode -test ./...`                      |
| Go                        | `go mod tidy`                                  | removes unused module dependencies          |
| Rust                      | `cargo-udeps`                                  | `cargo +nightly udeps`                      |
| Any                       | `ripgrep`                                      | Manual cross-reference for symbol names     |

Tools are a starting point, not a complete answer. Static tools cannot resolve all dynamic dispatch, reflection, or plugin patterns — findings from tooling must be confirmed by reading the code.

______________________________________________________________________

## Scope of a dead-code review

Dead code review is most valuable as a periodic full-codebase scan, not a diff review. On a diff, flag:

1. Code in the diff that introduces new dead paths (new unreachable branches, new unused exports)
1. Code in the diff that makes previously-live code dead (removing a caller of a public function, deleting the only import of a file)

For a full-codebase scan, run the detection tooling and apply the category patterns across the entire tree.
