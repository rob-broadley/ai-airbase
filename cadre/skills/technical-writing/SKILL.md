---
name: technical-writing
description: Load before writing or updating any documentation. Required by the technical-author agent — covers the Diataxis document-type framework, writing principles, interface-element reference template, README structure, working examples, voice and tone, and coverage standards.
license: AGPL-3.0-or-later
allowed-tools: read
---

# Technical Writing Reference

Good documentation is a product, not an afterthought. It has an intended reader, a clear purpose, and a structure that serves that purpose. This skill covers how to plan, write, and maintain user-facing documentation for software projects.

______________________________________________________________________

## Document types — the Diataxis framework

Every piece of documentation belongs to exactly one of four types. Mixing types in a single document is the most common cause of documentation that is long but unhelpful.

| Type         | Oriented towards | Answers                               | Analogy |
| ------------ | ---------------- | ------------------------------------- | ------- |
| Tutorial     | Learning         | "How do I get started?"               | Lesson  |
| How-to guide | Goals            | "How do I do this specific thing?"    | Recipe  |
| Reference    | Information      | "What does this option/flag/type do?" | Map     |
| Explanation  | Understanding    | "Why does it work this way?"          | Essay   |

**Tutorials** lead the reader through a complete, meaningful task with no assumed knowledge. Every step must work. The reader should finish with something running, not just read about something that could run.

**How-to guides** assume the reader knows the system and wants to accomplish a specific goal. They are shorter than tutorials, skip background, and go straight to the steps. Each guide solves exactly one problem.

**Reference** documents describe the system exhaustively and accurately. CLI flag reference, API docs, configuration schema — these are reference. They do not teach or explain; they record. Completeness and accuracy are the only metrics.

**Explanation** documents discuss why — design decisions, tradeoffs, background concepts. They do not instruct. They are for readers who want to understand, not just use.

Before writing any documentation, decide which type it is. If you cannot decide, split it.

______________________________________________________________________

## Coverage standards

### What must exist

A project is not adequately documented until a new user can answer all of these without asking anyone:

1. What does this project do? (one sentence, in the README)
1. How do I install, deploy, or access it?
1. What does a minimal working interaction look like?
1. What is the primary interface and how do I use it?
1. How do I contribute or develop locally? (DEVELOPMENT.md)

These are not optional. Missing any one of them is a documentation gap, not a style preference.

### What every interface element needs

Regardless of project type — CLI subcommand, HTTP endpoint, UI screen, exported function, configuration option — every interface element must be documentable by answering the same five questions:

1. **What is it?** Name, signature, or path. One sentence saying what it does.
1. **What does it accept?** Inputs: parameters, flags, fields, user actions. For each: type, constraints, default, required or optional.
1. **What does it produce?** Outputs: return value, response, state change, side-effect.
1. **When does it fail?** Error conditions, and what the caller or user should do about each.
1. **What does it look like in use?** At least one concrete, verified example.

Apply this template to every interface element in the project. The form of each answer varies by project type; the questions do not.

______________________________________________________________________

## Writing principles

### Audience first

Before writing a word, answer: who will read this? A new user who has never seen the tool? An experienced user looking up a flag? A developer adding a feature? Each audience needs different depth, vocabulary, and assumed knowledge. Write for one audience per document.

### Active voice

Write what the software does, not what happens to the user.

| Avoid                                   | Prefer                          |
| --------------------------------------- | ------------------------------- |
| "The config file will be read by myapp" | "myapp reads the config file"   |
| "Errors should be reported to stderr"   | "myapp writes errors to stderr" |
| "The flag can be set to override…"      | "Set the flag to override…"     |

### Imperative for instructions

Use the imperative mood for all instructional steps: "Run", "Set", "Open", "Create". Not "You should run" or "You will need to set".

### Present tense for descriptions

Describe what the software does in the present tense: "myapp creates a record", not "myapp will create a record".

### Progressive disclosure

Lead with the essential. Detail comes after. Structure every document so a reader who stops after the first paragraph has still learned the most important thing. Bury prerequisites, advanced options, and edge cases towards the end.

### Concrete over abstract

Show, then tell — or show instead of telling. A working example communicates more precisely than a paragraph of description, and it is checkable.

### Banned words and phrases

These words add length without adding information. Remove them on sight:

- simply, just, easy, easily, straightforward, obvious, trivially
- "Note that…", "Please note…", "It should be noted…"
- "As you can see…", "As mentioned above…"
- "In order to" → "To"
- "Make sure to" → imperative verb

### One idea per paragraph

If a paragraph contains two ideas, split it. A paragraph is not a list of loosely related sentences — it is a unit of thought. If you cannot summarise a paragraph in a single sentence, it needs to be split or restructured.

______________________________________________________________________

## Voice and tone

Technical documentation is not formal prose, but it is not casual chat either.

- **Direct.** Get to the point. Readers are looking for answers, not engaging with the text for pleasure.
- **Neutral.** Avoid enthusiasm markers ("This is a great way to…") and hedging ("might perhaps be useful for…").
- **Consistent.** Use the same term for the same concept throughout. If a concept is called "workspace", do not call it "workspace", "project", and "environment" interchangeably.
- **Second person for instructions.** Address the reader as "you": "You can override the default by…"
- **Avoid first person plural.** "We recommend" implies an organisation, not a tool. "Use X to…" is clearer.

______________________________________________________________________

## Reference documentation patterns

Reference documentation describes the system exhaustively and accurately. Every interface element — CLI subcommand, HTTP endpoint, UI screen, exported function, configuration key — follows the same underlying structure. Adapt the terminology to the project type; the structure stays the same.

**For each interface element, document:**

1. **Identity** — name, path, or signature
1. **Purpose** — one sentence describing what it does and when to use it
1. **Inputs** — every parameter, flag, field, or user action, with type, constraints, default, and required/optional status
1. **Outputs** — return value, HTTP response, state change, or visible effect
1. **Failure modes** — what can go wrong, the signal the user or caller receives, and what to do about it
1. **Example** — a minimal, verified, realistic demonstration (see Working examples)

Do not omit sections because they seem obvious. "No parameters" and "always succeeds" are valid answers and should be stated.

### Examples (all project types)

Good examples:

- Use realistic values, not placeholders like `<foo>` unless the user must substitute something
- Show the complete interaction, not just the new part
- Show output or response where non-obvious
- Progress from the common case to edge cases

```bash
# Create a project using the default image
mytool create my-project

# Create using a specific registry image
mytool create --image registry.example.com/team/devenv:latest my-project
```

### Failure modes in documentation

When documenting interfaces that can fail, describe the common failure modes and what to do. "If X fails with Y, check Z" is more useful than omitting error handling entirely.

______________________________________________________________________

## README structure

A README is the front door. It must work for a reader who has never heard of the project and a reader who used it last month and needs a quick reminder.

Required sections, in order:

1. **Project name and one-sentence description** — what it is and what problem it solves.
1. **Prerequisites** — what must already be installed or configured.
1. **How to get it** — installation, deployment, or access instructions. The heading and content depend on the project type: "Installation" for CLIs and libraries, "Deployment" for server applications, "Download" for desktop/mobile apps, "Access" for hosted services.
1. **Quick start** — the shortest working example. This is the most-read section after the description. Keep it short.
1. **Usage** — command, endpoint, or API reference, or a link to it. Cover the common cases; link to full reference for the rest.
1. **Development** — or a link to `DEVELOPMENT.md`.
1. **Licence** — one line.

Optional but valuable:

- **Status badges** (CI, version) — only if they are kept current
- **Architecture overview** — a paragraph or diagram for complex projects
- **FAQ or troubleshooting** — for known common problems

Sections to avoid:

- Promotional language ("the best", "blazing fast", "revolutionary")
- Roadmaps in the README — they go stale immediately; use issues or a separate document
- Changelogs in the README — use `CHANGELOG.md`

______________________________________________________________________

## Working examples

An example that does not work is worse than no example. It wastes the reader's time and destroys trust.

Rules for examples:

- **Test every example before publishing it.** Run it. See the output. Paste the real output.
- **Show realistic values.** `my-project` is better than `<project-name>`. Use values that look like what a user would actually type.
- **Show output where it matters.** If the command produces output, show it. If it produces no output on success, say so.
- **Keep examples minimal.** The example should demonstrate exactly one thing. If it needs five flags to demonstrate one flag, restructure the example.
- **Update examples when the interface changes.** A stale example is a broken example.

### Minimal working example

The minimal working example (MWE) is the single most important piece of documentation after the project description. It must:

- Require the fewest possible prerequisites
- Complete in under a minute
- Produce visible, meaningful output
- Be copyable and runnable without modification (or with clearly marked substitutions)

______________________________________________________________________

## Markdown conventions

### Headings

- Use ATX headings (`#`, `##`, `###`) not Setext (`===`, `---`)
- Headings describe content — not "Step 3" but "Configure the registry"
- Do not skip levels (h1 → h3 without h2)
- One h1 per document

### Code blocks

- Always specify the language for syntax highlighting: ```` ```bash ````, ```` ```go ````, ```` ```yaml ````
- Use inline code (backticks) for: command names, flag names, file paths, type names, literal values
- Use code blocks for: multi-line commands, output, configuration files, examples

### Lists

- Use unordered lists for items with no natural order
- Use ordered lists for sequential steps
- Do not nest lists more than two levels deep
- Each list item is either a sentence fragment (no period) or a full sentence (with period) — pick one per list

### Links

- Use descriptive link text: `[myapp documentation](url)` not `[click here](url)`
- Prefer relative links for files within the same repository
- Verify links work before publishing

### Tables

Use tables for comparative reference information (parameter descriptions, option comparisons, endpoint summaries). Do not use tables for sequential steps or narrative content.

______________________________________________________________________

## Maintenance discipline

Documentation that is not maintained becomes a liability. Every change to the interface, behaviour, or architecture of a project must trigger a documentation review.

**Checklist for documentation changes accompanying a code change:**

- [ ] Does any README example use a changed interface? Update it.
- [ ] Does any how-to guide describe a changed behaviour? Update it.
- [ ] Does any reference entry (flag, endpoint, exported symbol) have a changed default, signature, or effect? Update it.
- [ ] Was an interface element (subcommand, endpoint, exported function) added, renamed, or removed? Update the reference.
- [ ] Was a breaking change made? Add a note to DEVELOPMENT.md or a migration guide.

**When to create new documentation vs. update existing:**

- New interface element (subcommand, endpoint, exported function) → add to the reference section and, if non-trivial, a how-to guide
- New parameter, flag, or field → add to the relevant reference entry
- New workflow → how-to guide
- New concept required to understand the project → explanation document
- Changed behaviour → update existing reference; note breaking changes prominently
