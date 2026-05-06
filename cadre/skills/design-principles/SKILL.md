---
name: design-principles
description: SOLID, GRASP, DRY, KISS, YAGNI, and Law of Demeter design principles. Used by the refactor and atdd agents during code review and refactoring phases.
license: AGPL-3.0-or-later
allowed-tools: read
---

# Design Principles Reference

Software design principles are heuristics, not laws. They express forces that, when in tension, require judgment. Use them to notice resistance, not to justify premature complexity.

Correctness always comes first.

______________________________________________________________________

## SOLID

### S — Single Responsibility Principle (SRP)

> A class should have only one reason to change.

- One class = one responsibility = one actor/stakeholder that could request change.
- Violation signals: a class has both business logic and I/O; methods reference unrelated subsystems; test setup creates unrelated collaborators.
- Common fixes: Extract Class, Extract Method, Move Method.

______________________________________________________________________

### O — Open/Closed Principle (OCP)

> Software entities should be open for extension, closed for modification.

- New behaviour should be addable without touching existing code.
- Violation signals: adding a new type requires modifying a switch or chain of if-else; feature flags scattered through a class.
- Common fixes: Replace Conditional with Polymorphism, Strategy Pattern, Visitor Pattern.

______________________________________________________________________

### L — Liskov Substitution Principle (LSP)

> Subtypes must be substitutable for their base types.

- Callers should not need to know the concrete type. Contracts must be honoured in all subclasses.
- Violation signals: subclass throws an exception the base type doesn't; `instanceof` checks to dispatch behaviour; overridden methods that narrow preconditions or widen postconditions.
- Common fixes: Replace Inheritance with Delegation, extract a new interface that the subtype can honour cleanly.

______________________________________________________________________

### I — Interface Segregation Principle (ISP)

> Clients should not depend on interfaces they don't use.

- Split fat interfaces into narrower, cohesive ones.
- Violation signals: implementing class leaves methods as `throw new NotImplementedException()`; test doubles implement many no-op methods.
- Common fixes: Extract Interface, Decompose Interface.

______________________________________________________________________

### D — Dependency Inversion Principle (DIP)

> Depend on abstractions, not concretions. High-level modules should not depend on low-level modules.

- Depend toward stability. Stable abstractions, not volatile concretions.
- Violation signals: `new ConcreteService()` inside a business-logic class; hard dependency on infrastructure (DB, file system, HTTP client) without an interface.
- Common fixes: Extract Interface, Inject Dependency, Introduce Service Locator.

______________________________________________________________________

## GRASP

| Pattern                  | When to apply                                                                                                            |
| ------------------------ | ------------------------------------------------------------------------------------------------------------------------ |
| **Information Expert**   | A method needs data that lives in another class — move the method there instead of exposing the data                     |
| **Creator**              | Deciding who should call `new X()` — assign it to the class that aggregates, contains, or closely uses X                 |
| **Controller**           | Use-case logic is leaking into domain objects or the UI — introduce a dedicated coordinator that owns the workflow       |
| **Low Coupling**         | A change in one class ripples into many others — reduce direct dependencies via injection or an intermediary             |
| **High Cohesion**        | A class is hard to name in one sentence, or its test setup touches unrelated concerns — split along responsibility lines |
| **Polymorphism**         | You're branching on type (`instanceof`, `switch` on a type field) to vary behaviour — replace with a type hierarchy      |
| **Pure Fabrication**     | Low Coupling or SRP demands a class that has no counterpart in the domain — introduce it deliberately, name it clearly   |
| **Indirection**          | Two components need to evolve independently but currently depend directly on each other — insert a mediating layer       |
| **Protected Variations** | A dependency is likely to change (third-party API, storage format, algorithm) — wrap it behind a stable interface now    |

______________________________________________________________________

## DRY — Don't Repeat Yourself

> Every piece of knowledge must have a single, unambiguous, authoritative representation.

DRY is about knowledge, not syntax. Two pieces of code that look alike for different reasons are NOT duplicates. Forcing DRY on coincidental duplication creates false coupling.

Apply DRY when: the same business rule appears in two places; the same algorithm is copy-pasted.

Don't apply DRY when: two pieces of code are structurally similar but represent different concepts.

______________________________________________________________________

## KISS — Keep It Simple

> Prefer the simplest design that could possibly work.

- Complexity is a cost. Every level of indirection, every abstraction, every design pattern has a carrying cost.
- Add complexity only when the problem demands it — not in anticipation.
- If you can't explain it in a sentence, it may be too complex.

______________________________________________________________________

## YAGNI — You Aren't Gonna Need It

> Don't build what you might need. Build what you need now.

- Speculative generality is the most common source of accidental complexity.
- Every unused extension point or premature abstraction is code that must be maintained without delivering value.
- Refactor to accommodate the requirement when it actually arrives. You will understand the domain better then.

YAGNI and KISS reinforce each other. Together they push back hardest against OCP misapplied: don't make everything extensible — make the things that change extensible, when they change.

______________________________________________________________________

## Law of Demeter (LoD) / Tell Don't Ask

> A unit should only talk to its immediate friends. Don't talk to strangers.

Only call methods on:

- `self`
- Parameters passed in
- Objects created locally
- Direct fields / components

Chaining like `order.customer().address().city()` is a violation. It creates structural coupling: the caller knows the entire navigation chain.

**Tell Don't Ask** is the positive form: instead of querying an object's state and acting on it yourself, tell the object what to do.

______________________________________________________________________

### Command Query Separation (CQS)

> A method should either return a value (query) or change state (command), not both.

- Queries are safe to call freely; commands are not.
- Violations make behaviour unpredictable and testing harder.
- Exceptions: builder pattern (conventional), iterators in some languages.

______________________________________________________________________

## Composition over Inheritance

Prefer assembling behaviour from small, single-purpose objects over deep inheritance hierarchies.

Use inheritance when:

- A true IS-A relationship exists.
- The subtype fully satisfies LSP.
- The relationship is stable (unlikely to change).

Prefer composition when:

- Behaviour needs to vary at runtime.
- The relationship is HAS-A or USES-A.
- The hierarchy would exceed 2–3 levels.
- You need multiple axes of variation (the Cartesian explosion problem).

______________________________________________________________________

## Encapsulation

- Objects own their data. Expose behaviour, not state.
- Getters/setters are not encapsulation. Mutable public fields with thin wrappers leak representation.
- Reveal intent through method names, not data fields.

______________________________________________________________________

## Immutability

- Prefer immutable value objects. Immutable objects are inherently thread-safe and simpler to reason about.
- Make things mutable only when there is a compelling reason.
- Value Objects (DDD) should always be immutable.

______________________________________________________________________

## Pure Functions

> Given the same input, always return the same output, with no observable side effects.

- Pure functions are trivially testable, composable, and parallelisable.
- Push impure logic (I/O, state mutation, randomness, time) to the boundaries of the system.
- The functional core / imperative shell pattern isolates purity: pure domain logic in the core, side effects in the shell.

______________________________________________________________________

## Principle → Refactoring quick map

| Principle violated        | Indicator                                 | Refactoring to apply                          |
| ------------------------- | ----------------------------------------- | --------------------------------------------- |
| SRP                       | God Class, divergent change               | Extract Class, Extract Method, Move Method    |
| OCP                       | Conditional on type                       | Replace Conditional with Polymorphism         |
| LSP                       | Refused bequest, type check               | Replace Inheritance with Delegation           |
| ISP                       | Fat interface, empty implementations      | Extract Interface, Decompose Interface        |
| DIP                       | Direct construction in business logic     | Extract Interface, Inject Dependency          |
| DRY                       | Copy-paste business rules                 | Extract Method, Extract Class, Pull Up Method |
| LoD / TDA                 | Message chain, data clump                 | Move Method, Hide Delegate, Extract Class     |
| Composition > inheritance | Deep hierarchy with variation axes        | Replace Inheritance with Delegation           |
| Immutability              | Mutable shared state with races/surprises | Extract Value Object, parameterise state      |
| YAGNI / KISS              | Speculative hook nobody calls             | Inline Class, Collapse Hierarchy              |

______________________________________________________________________

## Principle tensions and resolution order

When principles conflict, use this priority order as a starting point. Context always wins.

1. **Correctness** — broken > smelly. Never sacrifice correctness.
1. **Simplicity (KISS/YAGNI)** over premature abstraction.
1. **DRY** — but only for knowledge, not structure.
1. **SOLID** — applied proportionally to the change frequency and complexity.
1. **Pattern application** — patterns emerge; don't force them.

Most principles conflict only when misapplied at the wrong scale or the wrong time. When in doubt, step back and ask: what is the simplest design that correctly reflects the current requirements and remains easy to change?

______________________________________________________________________

## Tidy First

Kent Beck's Tidy First principle bridges design principles and the act of changing code:

> *"For each hard change, make the change easy (warning, this may be hard), then make the easy change."*

Before making a behaviour change, ask whether the existing structure makes that change awkward. If it does, tidy the structure first — in a separate commit — so the behavioural change becomes straightforward. Then make the behavioural change.

This has a practical consequence for commit discipline: **structure changes and behaviour changes are never mixed in the same commit.** A tidy commit is verifiably behaviour-preserving (tests stay green, no new logic). A behaviour commit is verifiably intentional (the structure is already clean). Mixing them makes both impossible to review and hard to revert.

Tidy First is not an excuse to over-engineer. Tidy only as much as the next change requires. If the change is already easy, make it directly.
