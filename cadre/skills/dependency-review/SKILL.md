---
name: dependency-review
description: Dependency review reference covering vulnerability signals, abandonment, licence compatibility, transitive risk, version hygiene, and supply chain signals. Includes language-specific audit commands. Used by the dependency-reviewer agent.
license: AGPL-3.0-or-later
allowed-tools: read
---

# Dependency Review Reference

Dependencies are code you did not write, do not control, and must trust. Each dependency brings its own bugs, security vulnerabilities, licence obligations, and maintenance trajectory. Reviewing dependency changes is as important as reviewing application code changes.

______________________________________________________________________

## Known vulnerability signals

A dependency with a published CVE is an immediate risk. The severity of the risk depends on whether the vulnerable code path is reachable from your application.

### How to check

Run `osv-scanner .` — it auto-detects all lockfiles in the project (`uv.lock`, `package-lock.json`, `pnpm-lock.yaml`, `yarn.lock`, `go.sum`, `Cargo.lock`, `pom.xml`, `packages.lock.json`, etc.) and queries the OSV database in a single pass. Install via `/tool-install` if not present.

### What a CVE finding means

- **CVSS Score 9.0–10.0 (Critical):** exploitable remotely with no authentication required; update immediately.
- **CVSS Score 7.0–8.9 (High):** exploitable under common conditions; update before next release.
- **CVSS Score 4.0–6.9 (Medium):** exploitable under specific conditions; plan an update.
- **CVSS Score 0.1–3.9 (Low):** limited impact; include in next routine update cycle.

Reachability matters: a vulnerability in a code path your application never exercises is lower priority than one on a hot path. Use reachability-aware tools where available, such as `govulncheck` for Go.

______________________________________________________________________

## Abandonment signals

A dependency that is no longer maintained will accumulate unfixed vulnerabilities and will eventually become incompatible with the rest of your dependency graph.

- **Last commit date:** a repository with no commits in 12 months is stale; no commits in 24 months is likely abandoned. Distinguish between mature-and-stable (few changes expected) and abandoned (issues open with no response).
- **Open issues with no response:** a repository with many open bug reports and security issues and no maintainer responses indicates an abandoned project even if recent commits exist.
- **Unmaintained forks still in use:** the application depends on a fork of a project rather than the original; the fork has not tracked upstream changes. Forks used for one-off fixes that were never upstreamed accumulate drift.
- **Archived repositories:** GitHub/GitLab archived status means the maintainer has formally declared the project inactive. Dependencies on archived repositories must be replaced.
- **Single maintainer with no succession plan:** a project maintained by one person with no co-maintainers and no documented succession plan is a bus-factor-1 risk.

______________________________________________________________________

## Licence compatibility

Licence obligations must be understood before a dependency is added. Licence problems are discovered at the worst time — during legal review before a major release or acquisition.

### Automated licence scan

Run `trivy fs --scanners license .` — it scans npm (`package-lock.json`, `yarn.lock`, `pnpm-lock.yaml`), Maven/Gradle (`pom.xml`, `build.gradle`), NuGet (`*.csproj`, `packages.lock.json`), and Go (`go.sum`) in one pass and reports each dependency's detected licence. Install via the container's nix channel (`nixpkgs#trivy`) or use `/tool-install`.

Trivy does not support licence detection for Python (`uv.lock`) or Rust (`Cargo.lock`) — inspect those manually via the package registry pages for each new dependency. C++ (vcpkg, Conan) is also manual.

### Permissive licences (generally safe for proprietary projects)

- **MIT:** use freely; retain copyright notice and licence text.
- **Apache 2.0:** use freely; retain notice; provide patent grant to users; note changes.
- **BSD 2-Clause / BSD 3-Clause:** use freely; retain copyright notice; BSD 3-Clause adds a no-endorsement clause.
- **ISC:** functionally equivalent to BSD 2-Clause.

### Copyleft licences (require analysis before use)

- **GPL-2.0 / GPL-3.0:** if your application links against a GPL library (statically or dynamically in some interpretations), your application must also be distributed under GPL. This is typically incompatible with proprietary projects.
- **LGPL-2.1 / LGPL-3.0:** weaker copyleft; designed for libraries. Dynamic linking to an LGPL library generally does not require your code to be LGPL. Static linking or modification of the LGPL code does.
- **AGPL-3.0:** GPL-equivalent for network-served applications. If you run AGPL software as a service, you must offer the source of your modifications to users. This is typically incompatible with proprietary SaaS products.
- **MPL-2.0 (Mozilla):** file-level copyleft; modifications to MPL files must be distributed under MPL; your proprietary code in separate files is unaffected.

### Implications

- **Open-source project releasing under MIT/Apache:** GPL and AGPL dependencies may be incompatible; consult your licence policy.
- **Proprietary project:** GPL and AGPL are almost always incompatible. LGPL requires analysis. Permissive licences are generally safe.
- **Licence not declared:** a package with no licence is not open source. Using it without explicit permission is copyright infringement.

______________________________________________________________________

## Transitive dependency risk

Your declared dependencies bring their own dependencies. The full transitive graph is what you are actually running.

- **Large transitive graphs:** a single direct dependency that pulls in 50 transitive dependencies is a maintenance and security surface 50 times larger than it appears. Audit the full graph, not just the direct dependencies.
- **Heavyweight dependency for a trivial function:** importing a large framework to use one utility function. Evaluate whether the function can be implemented in a few lines or whether a smaller, focused package exists.
- **Transitive dependencies with mismatched licences:** the direct dependency may be MIT but its transitive dependencies may include GPL code. Audit the full graph for licence compliance.
- **Conflicting transitive versions:** two direct dependencies requiring incompatible versions of the same transitive dependency. This produces behaviour that depends on which version was resolved, which varies across package managers and lockfile states.

______________________________________________________________________

## Unused dependencies

Declared-but-unused dependencies increase attack surface for no benefit. Check for unused dependencies using:

- **JavaScript and TypeScript:** `depcheck` or `knip`.
- **Python:** manual inspection against imports, or compare the manifest with the project's actual import graph.
- **Java:** dependency analysis via `mvn dependency:analyze`.
- **C#:** compare `PackageReference` entries against actual project references and remove packages that are only transitively required.
- **C++:** compare `find_package`, `target_link_libraries`, vcpkg manifests, or Conan requirements against the targets that are actually built.
- **Go:** `go mod tidy` — if it removes packages, they were unused.
- **Rust:** `cargo-udeps` (`cargo +nightly udeps`).

Signal: a dependency that appears in the manifest but is not imported in any source file is an Observation finding (not Blocking — it may be a transitive peer dep or optional).

______________________________________________________________________

## Version hygiene

How versions are specified determines how reproducible and safe your builds are.

- **Floating versions (`*`, `latest`, `^major`, `~minor`):** the resolved version can change between two builds with no change to the manifest. A security fix in your CI today may not be present in the production build tomorrow. Pin to exact versions for reproducibility.
- **Lockfile absent or not committed:** a lockfile (`go.sum`, `package-lock.json`, `uv.lock`, `Cargo.lock`) records the exact resolved versions. Without a committed lockfile, builds are not reproducible.
- **Dependency staleness — major versions behind:** being many major versions behind the current release is a risk because security patches are typically applied to the current major release only. Check whether the old major version is still receiving security backports.
- **Known breaking changes in pending upgrades:** before updating a major version, check the changelog for breaking changes. Flag any that affect code paths used by this application.
- **Pre-release versions (`alpha`, `beta`, `rc`) in production:** pre-release versions have no stability guarantees. Pin to stable releases for production code.

______________________________________________________________________

## Supply chain integrity

The supply chain attack surface includes the packages themselves and the infrastructure through which they are published.

- **Packages with unexpected publish authors:** a package that has historically been maintained by one person and suddenly shows a new publisher may have been taken over. Check the package registry's ownership history.
- **Recent ownership transfers:** a package whose npm, PyPI, Maven Central, or NuGet ownership transferred recently may have been acquired by a malicious actor. This is a known attack pattern.
- **Package with unusually broad scope for its stated purpose:** a package that requests filesystem access, network access, or environment variable access that is not needed for its stated function.

### Typosquatting

- Check added package names against common top-downloaded packages for Levenshtein distance ≤ 2 — e.g. `reqests` vs `requests`, `lodsh` vs `lodash`.
- A newly added package with a similar name to a well-known package is High severity until confirmed intentional.

### Dependency confusion

- If the project uses private/internal packages, verify they are not accidentally resolvable from a public registry.
- Dependency confusion: an attacker publishes a public package with the same name as an internal package at a higher version number — the package manager resolves the public one.

### Install-time script abuse

- `postinstall`/`preinstall` scripts in package.json that make network requests or access the filesystem are a supply chain risk.
- Check all added dependencies for `scripts` in their package.json that run on install.

### Abandoned-then-reclaimed packages

- A package that was abandoned (no commits for years) and then suddenly shows new activity under a new maintainer should be treated as potentially compromised.

______________________________________________________________________

## Container base images

When a Dockerfile is present in the changed files:

- Base image pinned by digest (`FROM node:20@sha256:...`), not by floating tag (`FROM node:latest` or `FROM node:20`) — a tag can point to a different image after a pull.
- Multi-stage build: build tools and source code should not ship in the final image; the final stage should contain only what is needed to run.
- Vulnerability scan: run `trivy image <base-image>` or `grype <base-image>` on the base image; Critical and High findings in the base image are findings in the review.
- Non-root user: the final image should run as a non-root user.

______________________________________________________________________

## Upgrade prioritisation

Order upgrade recommendations by `(severity × surface area) / upgrade risk`, not by CVSS score alone:

- A Critical CVE in a library used in one optional CLI command is lower priority than a High CVE in a library on every hot request path.
- Surface area factors: is the library used in a network-facing path? Is it used at startup? Is it transitive-only?
- Upgrade risk factors: does the upgrade cross a major version? Does it have a migration guide? Are there known breaking changes?

______________________________________________________________________

## Severity model

### Blocking

The dependency introduces an immediate, concrete risk that must be resolved before merge.

Examples: a direct dependency with a Critical CVE on a reachable code path; an AGPL dependency in a proprietary project; a typosquatted package name; a package with a malicious install script.

### Recommendation

The dependency introduces risk that should be addressed before the next release or within the current sprint.

Examples: a High CVE on a direct dependency; an archived or abandoned dependency with no replacement identified; an LGPL dependency that requires licence analysis; a major version many releases behind with active security backports only for the current version.

### Observation

The dependency practice could be improved but poses no immediate risk.

Examples: a floating version that could be pinned; a transitive graph that is larger than necessary; a lockfile entry that is stale relative to a newer patch release with no security relevance.

______________________________________________________________________

## Output format

Include the raw output of the audit tool execution before the findings section.

Group findings by severity (Blocking first). For each finding:

```
**[SEVERITY] dependency** (package name@version)
Description: what the issue is.
Why it matters: the concrete consequence (exploitable vulnerability, licence conflict, supply chain risk).
Direction: the general approach to resolution — not an implementation.
```

Do not write dependency manifest changes. Provide direction, not implementation. If no findings exist in a severity tier, omit that tier.
