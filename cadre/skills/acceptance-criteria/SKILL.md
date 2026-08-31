---
name: acceptance-criteria
description: Load before eliciting or organizing acceptance criteria. Covers user-led example mapping, Given/When/Then scenarios, delivery-slice mapping, and scope-splitting patterns.
license: AGPL-3.0-or-later
---

# Acceptance Criteria

Use these techniques to ask better questions, organize confirmed examples, and propose delivery slices. They do not authorize inventing requirements, actors, expected outcomes, or scenarios.

______________________________________________________________________

## Example Mapping

Use four categories to explore one intended behaviour:

| Category  | Represents                                       |
| --------- | ------------------------------------------------ |
| Behaviour | The outcome the user wants to achieve            |
| Rules     | Constraints that govern the behaviour            |
| Examples  | Concrete cases supplied or confirmed by the user |
| Questions | Unresolved or contested points                   |

For each behaviour:

1. Ask the user for a concrete example.
1. Record the rules they state.
1. Ask for at least one example for each stated rule.
1. Record anything unresolved as a question; do not decide it for the user.

Ask only questions needed to turn the user's intended behaviour into an observable scenario. Useful prompts include:

- What is the simplest successful case?
- What should happen when [the user-mentioned boundary] occurs?
- Is there a case where this rule does not apply?
- What should not happen here?

Do not add examples because they are common in similar systems. A rule without an example, an example without a rule, or a contested outcome needs clarification from the user.

______________________________________________________________________

## INVEST Task Checks

Use the six INVEST concerns to assess any proposed task: a delivery slice, a single scenario, or another unit of work. Score each concern from 0 to 2 and show the score with brief evidence to the user. Use the result to identify the next user question, dependency, deferral, or grouping change. Do not invent requirements to make a check pass or reject a task without user input.

**Independent**

- `0`: hard dependency on another task
- `1`: soft dependency
- `2`: can proceed independently

For a score of `0` or `1`, surface the dependency and propose an order or a smaller task before the user approves it.

**Negotiable**

- `0`: implementation locked in
- `1`: some flexibility
- `2`: states outcomes, not a solution

For a score of `0` or `1`, ask whether the stated solution is required or whether the outcome is what matters before the user approves it.

**Valuable**

- `0`: no user-visible outcome
- `1`: indirect user value
- `2`: directly supports the confirmed goal

For a score of `0` or `1`, ask whether the task belongs in this delivery or should be deferred before the user approves it.

**Estimable**

- `0`: key facts are unknown
- `1`: some uncertainty remains
- `2`: examples, outcomes, constraints, and dependencies are understood

For a score of `0` or `1`, ask the missing question before the user approves it; do not infer an answer.

**Small**

- `0`: too broad for one implementation cycle
- `1`: several behaviours remain coupled
- `2`: coherent, bounded unit

For a score of `0` or `1`, propose a split using the patterns below before the user approves it.

**Testable**

- `0`: no observable acceptance criteria
- `1`: criteria exist but are unclear
- `2`: clear Given/When/Then scenarios

For a score of `0` or `1`, ask what the actor should observe before the user approves it.

Use these checks to guide the discussion, not as a pass/fail gate. Do not impose a score threshold. The user decides whether to answer a question, approve a split, defer a behaviour, accept a trade-off, or proceed with the task.

______________________________________________________________________

## Given/When/Then Scenarios

Write a scenario only from an example the user supplied or explicitly confirmed.

```text
Given [starting context]
When [one action]
Then [observable outcome]
```

Each scenario must be:

- **Observable**: describes what an actor can see, receive, or otherwise verify.
- **Focused**: has one action and one intended behaviour.
- **Implementation-independent**: does not name classes, tables, services, frameworks, or other solution details.
- **Decidable**: has an outcome that can be confirmed as true or false.

Ask the user to clarify instead of completing a scenario from inference. In particular, do not infer error handling, boundary behaviour, performance targets, or authorization rules.

Examples of weak wording and the question it requires:

| Weak wording         | Ask instead                                                                       |
| -------------------- | --------------------------------------------------------------------------------- |
| The API returns JSON | What does the actor need to receive or observe?                                   |
| The system is secure | Which actor is allowed to do what, and what should an unauthorized actor observe? |
| It works quickly     | What result or limit matters to the user?                                         |

______________________________________________________________________

## Delivery Slice Mapping

Group confirmed scenarios by user-visible behaviour, never by technical layer. A delivery slice is a small, coherent group that can be implemented and evaluated together.

Possible groups include:

- The core successful behaviour
- Input validation or a stated boundary
- A stated failure or recovery path
- A privileged or administrative action

Propose the grouping to the user. Do not treat a proposed grouping as a requirement until they approve it. The first slice should be the smallest confirmed end-to-end behaviour with an observable outcome; do not invent one merely to create a walking skeleton.

______________________________________________________________________

## Splitting Large Scope

When the user identifies a large or mixed set of behaviours, use these patterns to propose smaller delivery slices:

| Pattern                    | Split by                                                  |
| -------------------------- | --------------------------------------------------------- |
| Paths                      | Different user journeys or outcomes                       |
| Interfaces                 | Different user-visible input or output channels           |
| Data                       | Distinct data shapes or complexity the user identified    |
| Rules                      | Independently meaningful business rules                   |
| Workflow steps             | Discrete stages of a user-visible flow                    |
| Happy path then exceptions | Confirmed success behaviour before confirmed errors       |
| Performance                | Functional behaviour before a separately confirmed target |

Keep slices vertical: a slice should deliver a user-visible outcome, not only a database, API, frontend, model, or other technical layer. Splitting is a proposal for user approval, not a license to add scope.

______________________________________________________________________

## Common Traps

| Trap                    | Response                                                                         |
| ----------------------- | -------------------------------------------------------------------------------- |
| Premature design        | Separate the desired outcome from the proposed solution.                         |
| Missing unhappy paths   | Ask only about failures or boundaries the user identifies as relevant.           |
| Non-functional omission | Use the NFR catalogue in `problem-analysis` to ask focused questions.            |
| Scope drift             | Record an explicit exclusion and challenge additions against the confirmed goal. |
| Accepted vagueness      | Ask for the observable outcome before recording the scenario.                    |
| Gold-plating            | Do not add a behaviour the user did not request or confirm.                      |
