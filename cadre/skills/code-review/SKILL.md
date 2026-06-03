---
name: code-review
description: Load before any code review. Required by the reviewer agent — covers what to examine, smell categories, complexity thresholds, dead code signals, and a three-tier severity model.
license: AGPL-3.0-or-later
---

# Code Review Reference

A code review examines the change at hand for correctness, maintainability, and fitness for purpose. Findings are graded by severity so the most important issues are addressed first. This skill does not cover SOLID or GRASP violations — those are defined in `/design-principles`, which you should load alongside this skill.

______________________________________________________________________

## What to examine

### Structure

- Package and module organisation: does the directory layout reflect the domain or is it a flat dump of files?
- Layer violations: does a data access object import from the HTTP handler layer, or vice versa?
- Circular dependencies: module A imports B imports A. These prevent isolated testing and incremental builds.
- File and class size: see thresholds below.

### Naming

- Names should state intent, not mechanism. `processData` is a mechanism name; `applyDiscountRules` is an intent name.
- Boolean variables and parameters named as questions: `isExpired`, `hasPermission`, not `expired`, `permission`.
- Acronyms and abbreviations: expand them unless they are universal within the team's domain vocabulary.
- Inconsistent casing conventions within the same codebase.

### Complexity

See the Complexity Thresholds section below.

### Coupling

- A change in one module forces changes in many others (shotgun surgery).
- A module reaches into the internals of another to read or mutate its state directly.
- High fan-out: a single function calls many unrelated collaborators.
- See `/design-principles` for SOLID and GRASP violation signals that produce coupling.

### Test coverage gaps

- Changed or added behaviour with no corresponding test.
- Tests that only cover the happy path (no error paths, no boundaries).
- Tests that test implementation rather than behaviour (coupled to private method names, internal data shapes).

### Documentation gaps

- Exported or public symbols with no doc comment.
- Existing doc comments that describe old behaviour (stale docs are worse than no docs).
- README or DEVELOPMENT.md not updated when behaviour visible to external users changed.

### Dead code

See the Dead Code Signals section below.

______________________________________________________________________

## Code smell categories

Apply these to the diff under review. For each smell category, the signal tells you what to look for in the code.

### Bloaters

Code constructs that have grown too large to work with comfortably.

- **Long Method:** method body exceeds the line threshold; requires scrolling to understand; conditionals and loops nested more than three levels deep.
- **Large Class:** class has too many fields and methods; test setup requires many unrelated stubs; the class cannot be named in one sentence without "and".
- **Long Parameter List:** more than four parameters on a function or constructor; callers pass literal values whose meaning is invisible at the call site.
- **Data Clumps:** the same group of fields or parameters appears together in multiple places; a group that is always passed together should become its own type.
- **Primitive Obsession:** domain concepts encoded as raw strings, integers, or maps instead of named types; coordinates as `float64, float64` instead of a `Point` type.

### Change Preventers

Smells that make changes expensive by coupling unrelated code.

- **Divergent Change:** adding one new feature requires modifying the same class in multiple unrelated ways.
- **Shotgun Surgery:** a single conceptual change requires small edits scattered across many classes or files.
- **Parallel Inheritance Hierarchies:** every time you add a subclass in one hierarchy, you must add a corresponding subclass in another.

### Couplers

Smells that express unhealthy dependencies between classes.

- **Feature Envy:** a method uses data or methods from another class more than from its own; it probably belongs in that other class.
- **Inappropriate Intimacy:** two classes access each other's private fields or implementation details directly.
- **Message Chains:** `a.getB().getC().doSomething()` — the caller is tightly coupled to the entire object graph.
- **Middle Man:** a class does nothing except delegate every method to another class; remove it or give it real responsibility.

### Dispensables

Things that add noise without adding value.

- **Duplicate Code:** the same or nearly identical logic in multiple places; a change to the logic requires finding and updating all copies.
- **Speculative Generality:** abstract hooks, parameters, and base classes added "in case they're needed later" but currently unused.
- **Data Class:** a class with only fields and getters/setters and no behaviour; data classes invite Feature Envy in their consumers.
- **Lazy Class:** a class that does so little it doesn't justify its own existence; collapse it into its single caller.

### Object-Orientation Abusers

Incorrect or incomplete application of OO principles.

- **Switch Statements on Type:** dispatching on a type field or class identifier; signals a missing polymorphic type hierarchy. See also OCP and Polymorphism in `/design-principles`.
- **Temporary Field:** fields that are only set and used in certain code paths; nil/zero the rest of the time; these belong in a parameter object or a separate type.
- **Refused Bequest:** a subclass inherits but ignores most of what the parent provides; the inheritance relationship is wrong.
- **Alternative Classes with Different Interfaces:** two classes do the same thing but have different method names; they should share an interface.

______________________________________________________________________

## Complexity thresholds

These are split candidates, not absolute limits. Context matters, but a function that exceeds these warrants examination.

| Measure                    | Threshold         | Action                                             |
| -------------------------- | ----------------- | -------------------------------------------------- |
| Cyclomatic complexity (CC) | > 10 per function | Split into smaller functions; extract conditions   |
| Function length            | > 20 lines        | Extract behaviour into named helper functions      |
| Class / file length        | > 200 lines       | Extract Class or split file                        |
| Parameter count            | > 4               | Introduce Parameter Object or use named options    |
| Nesting depth              | > 3 levels        | Extract condition into named boolean; early-return |

______________________________________________________________________

## Dead code signals

- **Unreachable branches:** a condition that is always true or always false; an else branch after a return or panic; code after a loop that never terminates.
- **Unused exports:** exported functions, types, or constants that are not referenced anywhere in the codebase or its declared consumers. Use the language's tooling to verify (`knip` or `ts-prune`, `vulture`, `PMD`, Roslyn analysers, `clang-tidy`, `deadcode`, `cargo-udeps`).
- **Commented-out code blocks:** code in comments is dead by definition; it adds noise and accumulates stale context. Delete it; version control preserves history.
- **Feature flags that are always true or always false:** flags that can never flip at runtime are dead branches pretending to be live code. Remove the flag and collapse the branch.
- **Orphaned test helpers:** test utility functions or fixtures that are no longer referenced by any test.

______________________________________________________________________

## Severity model

Every finding must be assigned exactly one severity level.

### Blocking

Correctness, security, or data loss risk. The change must not be merged until these are resolved.

Examples: incorrect error handling that causes data corruption; an authentication bypass; a race condition; a nil dereference that will panic under normal use; a dependency with a known critical CVE.

### Recommendation

Quality or maintainability issue. The code works today but becomes a problem tomorrow. These should be addressed before or shortly after merge.

Examples: a method that exceeds the complexity threshold; duplicate logic that will diverge; missing test coverage for an error path; a stale doc comment.

### Observation

Minor or informational. Low-priority improvements worth capturing. Address in a subsequent tidy pass.

Examples: a variable name that could be more descriptive; a trivial redundancy; a micro-optimisation opportunity; a code style inconsistency.

______________________________________________________________________

## Output format

Group findings by severity (Blocking first). Within each severity group, order by file, then by line number.

For each finding:

```
**[SEVERITY] location** (file:line or file:function)
Description: what the issue is.
Why it matters: the consequence if left unaddressed.
Direction: the general approach to resolution — not an implementation.
```

Do not write code patches. Do not rewrite the code under review. Provide direction, not implementation.

If no findings exist in a severity tier, omit that tier from the output rather than writing "No Blocking findings."
