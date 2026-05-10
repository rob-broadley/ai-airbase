# Cadre — Agent Reference

The cadre is the group of AI agents bundled inside every revetment container. Each agent has a specific area of expertise and the tools to match. `mission-control` loads automatically when you attach to a container — all other agents are invoked by `mission-control` or by asking for them directly.

## Entry point

### `mission-control`

**Description:** Default agent for all software tasks. Plans and coordinates work, selects the right specialist agents, confirms the approach, then orchestrates delivery. Handles feature work, refactoring, bug fixes, environment setup, and reviews.

`mission-control` is the co-pilot. It reads your project, classifies your request against a routing table, builds a numbered plan scoped to individual agents, and presents it for approval before executing. It does not write code itself — it delegates to specialists and passes context between them.

**Auto-loads:** GitHub Copilot CLI inside the revetment starts with `mission-control` as the active agent. Describe what you want; `mission-control` takes it from there.

**When to use:** Start here for any task — especially complex or multi-step ones, or when you are unsure which specialist to invoke.

## Specialist agents

### Feature delivery

**`problem-analyser`** — Use when a requirement is vague, incomplete, or not yet ready to build. Decomposes the problem into subproblems, surfaces contradictions and edge cases, probes non-functional requirements, and produces a confidence-scored problem analysis. Does not write user stories — hand off to `user-story-writer` once the problem is understood.

**`user-story-writer`** — Use for decomposing a clear problem brief into granular, independent, valuable, testable user stories. Applies INVEST scoring, Elephant Carpaccio, Story Mapping, and SPIDR splitting. Outputs stories as structured markdown. Requires a clear problem brief — run `problem-analyser` first if requirements are still vague.

**`atdd`** — Use when implementing a user story via Acceptance Test Driven Development (ATDD). Drives the Red-Green-Refactor-Commit cycle with explicit permission gates between phases. Not for exploratory refactoring or writing tests after the fact.

**`legacy-code`** — Use when you need to safely modify legacy code that lacks tests. Specialises in Michael Feathers' dependency-breaking techniques. Introduces seams for testing, breaks dependencies to enable unit tests, and creates safe pathways for refactoring untested code. Use before `refactor` or `atdd` when no safety net exists.

**`refactor`** — Use when refactoring code to improve readability, maintainability, and structure without changing behaviour. Applies SOLID, GRASP, and clean code principles. Invoked automatically by `atdd` during the Refactor phase of TDD. Never mixes structural and behaviour changes in the same commit.

### Review

**`reviewer`** — General-purpose code reviewer. Accepts a git ref, file path, or diff. Loads `code-review`, `design-principles`, and `test-review` skills. Routes to specialist reviewers for targeted concerns. Use for a focused review of a specific change; use `full-reviewer` for a comprehensive multi-dimensional review.

**`full-reviewer`** — Fleet orchestrator. Launches all nine specialist review agents in parallel against the same scope and synthesises their findings into one unified, deduplicated report. Use when a comprehensive review across all dimensions is needed.

**`security-reviewer`** — Reviews code for security vulnerabilities using the OWASP Top 10 (2021) and security hygiene patterns. Actively searches for auth, input parsing, and external call patterns regardless of what changed. Use for any security audit or when new routes, authentication logic, or external calls are introduced.

**`test-reviewer`** — Reviews test code quality against Dave Farley's 8 properties of good tests. Covers test double misuse, fragility signals, and coverage gaps. Reviews test files only — skips non-test files entirely. Use when test quality is in question or after a batch of new tests is written.

**`api-reviewer`** — Reviews API and CLI interface design for consistency, correctness, and evolution safety. Covers REST naming, HTTP semantics, error consistency, breaking changes, CLI conventions, pagination, and auth. Use when adding or changing API endpoints or CLI flags.

**`error-handling-reviewer`** — Reviews error handling for correctness, completeness, and resilience. Actively searches the codebase for swallowed errors and empty error handlers beyond the current diff. Use when reviewing code that performs I/O, calls external services, or coordinates multi-step operations.

**`observability-reviewer`** — Reviews logging, metrics, tracing, and alerting quality. Establishes the existing observability baseline before reviewing new code. Applies USE and RED method naming conventions. Use when new log statements, metrics, or trace instrumentation are added.

**`concurrency-reviewer`** — Reviews code for race conditions, deadlocks, resource leaks, shared state misuse, synchronisation errors, and distributed concurrency issues across languages. Runs available static analysis tools when applicable. Use when goroutines, threads, async tasks, or shared mutable state are involved.

**`dependency-reviewer`** — Reviews changes to dependency manifest files. Runs language-appropriate audit tools and checks for vulnerabilities, abandonment, licence issues, and supply chain signals. Not applicable when no manifest files changed.

**`docs-reviewer`** — Reviews documentation coverage and accuracy for changed exported symbols and project-level docs. Produces findings only — does not write documentation. Use to check whether a code change broke, missed, or outdated documentation. To write or update documentation, use `technical-author`.

**`dead-code-detector`** — Reviews code for unreachable paths, unused exports, orphaned files, stale feature flags, zombie dependencies, and dead routes. Supports both diff-scoped reviews and full-codebase scans. Use for periodic clean-up audits or after refactoring that may have left unused code behind.

### Documentation

**`technical-author`** — Use when writing or updating user-facing documentation — README, CLI reference, how-to guides, tutorials, or explanations. Applies the Diataxis framework to produce clear, accurate, maintainable documentation. Do not use for finding documentation gaps — use `docs-reviewer` for that.

### Environment

**`devex`** — Use when setting up, auditing, or improving the development environment inside the container — installing runtimes, linters, formatters, test runners, and build tools. Also use for greenfield projects to choose and install a toolset. Does not modify application source code.

## Skills

Agents load skills on demand — domain-knowledge reference cards that ground every decision in documented patterns. You do not invoke skills directly; agents load them as needed using the `skill` tool.

**`api-review`** — REST and CLI interface review reference — naming, HTTP semantics, error consistency, breaking changes, and three-tier severity model.

**`code-review`** — Code review reference — smell categories, complexity thresholds, dead code signals, and three-tier severity model.

**`concurrency-review`** — Concurrency review reference — race conditions, deadlocks, resource leaks, synchronisation, and cancellation propagation.

**`dead-code-review`** — Patterns for detecting unreachable code, unused exports, orphaned files, stale feature flags, and zombie dependencies.

**`dependency-review`** — Dependency review reference — vulnerability signals, licence compatibility, transitive risk, version hygiene, and audit commands.

**`design-principles`** — SOLID, GRASP, DRY, KISS, YAGNI, and Law of Demeter principles with a principle-to-refactoring map.

**`docs-review`** — Documentation review reference — coverage thresholds, accuracy signals, doc comment format, and three-tier severity model.

**`error-handling-review`** — Error handling review reference — swallowed errors, error specificity, resilience, partial failure, and user-facing errors.

**`observability-review`** — Observability review reference — log levels, structured logging, trace context propagation, metric naming, and alert quality.

**`problem-analysis`** — Requirements elicitation reference — Impact Mapping, contradiction taxonomy, NFR catalogue, premortem, and confidence scoring.

**`security-review`** — Security review reference — OWASP Top 10 (2021), secrets hygiene, input validation, auth patterns, and four-tier severity model.

**`story-craft`** — User story reference — INVEST scoring, SPIDR splitting, Elephant Carpaccio, Story Mapping, and acceptance criteria patterns.

**`tdd-patterns`** — ATDD/TDD reference — walking skeleton, London vs Chicago school, test doubles, contract testing, and characterisation testing.

**`technical-writing`** — Technical authoring reference — Diataxis framework, writing principles, README structure, and coverage standards.

**`test-review`** — Test quality reference — Dave Farley's 8 properties of good tests, test double misuse, fragility signals, and coverage gaps.

**`tool-install`** — Installation recipes for language runtimes and analysis tools using uv and Nix.

**`workflow-patterns`** — Agent-chain templates and routing logic for `mission-control` — maps task archetypes to ordered sequences of specialist agents.

## Extending the cadre

Agent files live in `cadre/agents/`; skill files live in `cadre/skills/`. Both are included in the revetment container image at build time. See `DEVELOPMENT.md` for project structure, build commands, and contribution conventions.
