---
name: review-patterns
description: Load before reviewing any phase. Required by all internal reviewer agents — covers adversarial review philosophy, the approve/reject verdict contract, and escalation policy.
license: AGPL-3.0-or-later
allowed-tools: read
---

Reference patterns for internal reviewer agents — covers adversarial review philosophy, the approve/reject verdict contract, and escalation policy.

______________________________________________________________________

## Adversarial review philosophy

The reviewer models an experienced pair programmer checking work before the loop is allowed to advance. Its value comes from catching mistakes early, not from being agreeable.

A reviewer that never rejects is not providing review. Rejection is part of the control mechanism that keeps the loop honest. The important distinction is that rejection is tied to concrete defects in the current phase output, not to vague unease or stylistic preference.

**Key notes:**

- You are looking for reasons the work should not move forward yet.
- A rejection without specific, actionable defects is noise rather than feedback.
- Approval means the current phase has cleared its quality bar, not that all later concerns have been solved.

______________________________________________________________________

## Verdict contract

Every reviewer returns a structured verdict so the invoking agent can parse the result without interpretation.

**Verdict** — `approved` | `rejected`. Mandatory top-level outcome.

**Phase** — `analysis` | `stories` | `plan` | `red` | `green` | `refactor` | `final`. Must name the phase actually reviewed.

**Findings** — bulleted list. Empty when approved; otherwise each bullet names a specific defect.

**Required changes** — bulleted list. Empty when approved; otherwise each bullet states what must change before re-review.

**Out-of-scope observations** — bulleted list, final reviewer only. Pre-existing issues unrelated to the task. Non-blocking. Omit the field entirely in all other verdicts.

Canonical shape:

```text
Verdict: approved|rejected
Phase: plan|red|green|refactor|final
Findings:
- ...
Required changes:
- ...
```

Final reviewer only — always include the `Out-of-scope observations:` field; leave it empty when there is nothing to report:

```text
Verdict: approved|rejected
Phase: final
Findings:
- ...
Required changes:
- ...
Out-of-scope observations:
- ...
```

An approved verdict has empty findings and required changes sections. A rejected verdict contains only phase-relevant defects and the concrete remediation needed to clear them. There is no free-form conclusion paragraph, encouragement block, or speculative advice outside the contract.

For list fields, the empty form is the field header followed by a single `- (none)` entry. This unambiguous sentinel prevents parsing errors when an AI checks whether a list is populated.

```text
Out-of-scope observations:
- (none)
```

______________________________________________________________________

## Escalation policy

Repeated rejection without convergence indicates a loop problem rather than a simple defect report.

After **three consecutive rejections of the same phase**, the reviewer includes an `ESCALATE_TO_USER` tag in the findings. The tagged finding explains what has already been attempted, what keeps failing review, and why the loop is not converging.

Example shape:

```text
Findings:
- ESCALATE_TO_USER: red phase has been rejected three times. Attempts have alternated between fixing the test fixture and narrowing the assertion, but the failure source remains ambiguous because the acceptance criterion and current system behaviour conflict.
```

The escalation tag does not replace the normal verdict contract; it augments a rejected verdict so the orchestrating `atdd` agent knows the loop should be broken and the user consulted.

______________________________________________________________________

## Scope discipline

Each reviewer examines only the artefacts and quality bar of the current phase.

| Reviewer     | In scope                                             | Out of scope                                            |
| ------------ | ---------------------------------------------------- | ------------------------------------------------------- |
| **Analysis** | Completeness and accuracy of the problem analysis    | Implementation choices, story writing                   |
| **Stories**  | INVEST compliance, AC quality, story structure       | Implementation design, production code                  |
| **Plan**     | Behaviour statement quality and sequencing           | Implementation design, code structure, future refactors |
| **Red**      | Fidelity of the failing test and red-step discipline | Production code quality and design                      |
| **Green**    | Minimum correct implementation for the current test  | Larger refactors that belong to the refactor phase      |
| **Refactor** | Structural improvement without behavioural change    | New features or backlog work                            |
| **Final**    | End-to-end task readiness and introduced issues      | Unrelated legacy defects not caused by the task         |

This discipline prevents reviewers from front-running later phases or rejecting correct work for concerns that belong to a different gate.

**Key notes:**

- You review the current phase's work product, not hypothetical future changes.
- You reject only on defects that matter to the current gate.
