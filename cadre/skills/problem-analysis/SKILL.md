---
name: problem-analysis
description: Load before any problem decomposition or requirements analysis. Required by the problem-analyser agent — covers Impact Mapping, Jobs To Be Done, contradiction taxonomy, NFR catalogue, constraint taxonomy, premortem, and confidence scoring.
license: AGPL-3.0-or-later
---

# Problem Analysis

Reference techniques for surfacing the real problem, understanding its boundaries, and identifying what is unknown or at risk. Use these before writing acceptance criteria or planning begins.

______________________________________________________________________

## Impact Mapping

**Origin:** Gojko Adzic, *Impact Mapping* (2012).

Connect every deliverable to the goal it serves. Start from the goal and work outward — never from the feature.

```
Why?   → Goal         — what business outcome are we trying to achieve?
Who?   → Actors       — who can produce the impact, or who is affected?
How?   → Impacts      — how should actors behave differently?
What?  → Deliverables — what can we build to support that behaviour change?
```

Ask "Why?" at least three times:

- "We need a dashboard." → Why? → "So users can monitor jobs." → Why? → "So they don't email support for status." → Why? → "Support spends 30% of time on status requests."

Now you have a goal: *reduce the cost of status enquiries to support*. That goal changes every design decision.

A deliverable that cannot be connected to a goal through Actor and Impact should not be built.

______________________________________________________________________

## Jobs To Be Done (JTBD)

**Origin:** Clayton Christensen, *The Innovator's Dilemma*; Bob Moesta, Alan Klement.

Users hire software to get a job done. Understanding the job reveals whether two asks are the same need in disguise — and whether the stated solution is the right one.

**Job statement format:**

```
When [situation], I want to [motivation], so I can [outcome].
```

**Example:**

- Feature request: "Add export to CSV."
- JTBD: "When I need to share data with my finance team, I want a format they can open without special software, so I can avoid manual re-entry and errors."

CSV is one solution, not the requirement. A shared link or a direct integration may solve the same job better.

**Use JTBD when:** A user has specified a solution without explaining why, or when the stated requirement doesn't connect to a clear user goal.

______________________________________________________________________

## Contradiction taxonomy

Review every decomposition for hidden tensions before writing requirements.

| Type                       | What it looks like                                                                                 |
| -------------------------- | -------------------------------------------------------------------------------------------------- |
| **Direct contradiction**   | "instant response" AND "run expensive computation"                                                 |
| **Implicit trade-off**     | "maximum privacy" AND "rich personalisation"                                                       |
| **Scope drift**            | Headline ask is X, but examples imply Y                                                            |
| **Hidden dependency**      | A assumes B exists, but B is not in scope                                                          |
| **Definitional ambiguity** | A word ("real-time", "admin") used to mean different things in different parts of the conversation |

For each contradiction, state both sides explicitly and mark it as needing resolution before proceeding.

______________________________________________________________________

## Non-functional requirements catalogue

Ask about each dimension even when the user hasn't raised it. Do not invent answers — flag each uncovered item as an open question with a proposed default.

| Dimension                | Example questions                                      |
| ------------------------ | ------------------------------------------------------ |
| **Performance**          | Response time target? Throughput? Concurrent users?    |
| **Scalability**          | Growth forecast? Peak vs sustained load?               |
| **Availability**         | SLA target? Acceptable downtime windows?               |
| **Security / privacy**   | Data sensitivity? Regulatory scope?                    |
| **Observability**        | What must be monitored? What events need alerting?     |
| **Compliance**           | Which regulatory frameworks apply?                     |
| **Accessibility**        | Target conformance level (WCAG 2.1 AA)? Jurisdictions? |
| **Internationalisation** | Languages? Locales? Date/currency formats?             |
| **Operability**          | Who operates it? Who is the runbook audience?          |
| **Cost**                 | Budget ceiling? Per-request or per-user target?        |
| **Maintainability**      | Team size? Language constraints?                       |
| **Usability**            | Key personas? Known pain points?                       |

______________________________________________________________________

## Constraint taxonomy

Tag every constraint you discover. Each type is handled differently.

| Tag                | Meaning                                           | Implication                                                |
| ------------------ | ------------------------------------------------- | ---------------------------------------------------------- |
| **Regulatory**     | Legal or compliance requirement                   | Not negotiable without legal review                        |
| **Technical**      | Existing system, protocol, or platform boundary   | Softenable with effort or architecture change              |
| **Organisational** | Skills, budget, timeline                          | Negotiable at cost                                         |
| **Temporal**       | Deadline — hard vs target date                    | Distinguish hard (external event) from target (preference) |
| **Political**      | Unstated preference of an influential stakeholder | Name it explicitly so it can be surfaced and negotiated    |

______________________________________________________________________

## Premortem

**Origin:** Gary Klein, *The Power of Intuition* (2003).

> "Imagine it is 18 months from now and this has failed. What happened?"

Apply before eliciting acceptance criteria. Ask the user: *"Imagine it is 18 months from now and this has failed. What happened?"* Collect failure modes across:

- **Adoption** — users didn't use it, or used it wrongly
- **Complexity** — it was harder to build than expected; the scope exploded
- **Regulatory** — a compliance requirement blocked delivery or forced rework
- **Organisational** — team priorities shifted; key knowledge left
- **Market** — the problem was solved a different way before this shipped

Record each distinct failure mode as a risk for the acceptance-criteria discussion.

______________________________________________________________________

## Confidence self-assessment

Rate confidence 0–10 per dimension before handing off. A low score names what to explore next. Do not hand off with a score below 6 in any dimension — ask more questions first.

| Dimension                             | Score | What low looks like                                    |
| ------------------------------------- | ----- | ------------------------------------------------------ |
| Stakeholders and users                |       | Don't know who is affected or who is the primary actor |
| Current state and context             |       | Haven't read the codebase or existing docs             |
| Desired outcomes and success criteria |       | Can't state what done looks like                       |
| Constraints                           |       | No idea of time, budget, or technical bounds           |
| Risks and unknowns                    |       | Haven't run premortem; many open questions remain      |

______________________________________________________________________

## Edge case categories

For each subproblem and rule, probe systematically:

| Category                | What to ask                                                          |
| ----------------------- | -------------------------------------------------------------------- |
| **Boundary conditions** | Empty input, maximum value, zero, negative, duplicate, concurrent    |
| **Exceptional flows**   | Failure, timeout, partial success, retry, rollback                   |
| **Unusual actors**      | First-time user, returning user, privileged user, external system    |
| **Data variants**       | Missing fields, malformed input, multi-locale, multi-tenant          |
| **Timing**              | Clock skew, late-arriving data, long-lived sessions, batch vs stream |
