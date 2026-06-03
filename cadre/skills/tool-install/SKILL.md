---
name: tool-install
description: Recipe reference for installing tools in the container environment. Used by bootstrap and specialist reviewer agents to look up the correct install command for any tool.
license: AGPL-3.0-or-later
---

> **Opencode environment only.** These instructions apply to the Opencode container environment. They are not guidance for configuring developer machines or CI pipelines — use the `devex` agent for toolchain design and configuration, or the `bootstrap` agent for tool installation.
>
> **No root access.** `sudo`, `apt`, `dnf`, and all system package managers are unavailable. Use `nix` and `uv` to install language runtimes, compilers, and core development tools. Once a runtime is available, use its own toolchain for ecosystem-specific packages (e.g. `go install`, `cargo install`, `npm install -g`).

Use **Nix** to bootstrap ecosystems — language runtimes, compilers, and tools not available on PyPI. Use **uv** for tools distributed via PyPI — Python or otherwise (e.g. `pre-commit`). Once a language runtime is installed, use its own toolchain for the rest of that ecosystem.

| Use when                                                               | Tool                                  |
| ---------------------------------------------------------------------- | ------------------------------------- |
| Installing a language runtime, compiler, or tool not available on PyPI | `nix`                                 |
| Installing a tool distributed via PyPI (Python or otherwise)           | `uv`                                  |
| Node.js is installed (via nix) — installing a JS/TS package globally   | `npm install -g`                      |
| Python runtime is installed — installing a tool                        | `uv tool install`                     |
| Java runtime is installed — installing project tools                   | `mvn`, `gradle`, or `nix profile add` |
| C# runtime is installed — installing a tool                            | `dotnet tool install --global`        |
| C++ toolchain is installed — installing or resolving packages          | `cmake`, `conan`, or `vcpkg`          |
| Go runtime is installed — installing a Go tool                         | `go install`                          |
| Rust and Cargo are installed — installing a Rust tool                  | `cargo install`                       |

## Language runtime quick reference

**JavaScript and TypeScript** — Install Node.js with Nix (`nixpkgs#nodejs_22` or the pinned version). `npm` is included with Node.js and used for ecosystem tools.

**Python** — A Python version is pre-installed in the container image. Use `nix profile add nixpkgs#python312` (or similar versioned attribute) to add specific or additional versions the project requires. Use `uv tool install` for tools distributed via PyPI.

**Java** — Install a JDK with Nix, then use Maven or Gradle according to the project.

**C#** — Install the matching `dotnet-sdk` with Nix, then use `dotnet` CLI tooling and global tools.

**C++** — Install a compiler and CMake with Nix, then follow the project's `conan`, `vcpkg`, or plain CMake workflow.

**Go** — Install the Go toolchain with Nix, then use `go install` for Go-native tools.

**Rust** — Install `rustup` with Nix, select the required toolchain, then use `cargo install` for Cargo-native tools.

## Nix

[Nix](https://nixos.org/) is the primary bootstrap tool. Use it for language runtimes, compilers, and any tool not available on PyPI. The `nixpkgs` flake is pinned to a fixed revision, so package versions are reproducible and served from the binary cache without compiling from source.

### Finding a package

```sh
nix search nixpkgs <name>
```

The `name` attribute in the results is what you pass to other `nix` commands.

### Installing a tool persistently

```sh
nix profile add nixpkgs#<package>
```

The tool is on `PATH` immediately and persists across sessions.

### Installing multiple tools at once

```sh
nix profile add nixpkgs#delta nixpkgs#fd nixpkgs#bat
```

### Running a tool once (no permanent install)

```sh
nix run nixpkgs#<package> -- [args]
```

### Checking what is installed

```sh
nix profile list
nix profile list --json | jq -r '.elements[] | .originalUrl'
```

The `--json` form lists the original flake URLs for installed packages.

### Checking the latest available version

```sh
nix search nixpkgs <name>
```

### Constraints

- Only `nixpkgs` is allowed — do not use other flake URLs.
- The binary cache (`cache.nixos.org`) is used automatically.
- The Nix store at `/nix/store` is persistent across session restarts.

### Versioned packages

Many tools have versioned nixpkgs attributes for installing a specific release:

```sh
nix profile add nixpkgs#go_1_22        # Go 1.22
nix profile add nixpkgs#jdk21          # OpenJDK 21
nix profile add nixpkgs#jdk17          # OpenJDK 17
nix profile add nixpkgs#nodejs_22      # Node.js 22
nix profile add nixpkgs#nodejs_20      # Node.js 20
nix profile add nixpkgs#python312      # Python 3.12
nix profile add nixpkgs#python313      # Python 3.13
```

Use `nix search nixpkgs <name>` to discover available versioned attributes.

## uv

Use `uv` for tools distributed via PyPI — Python or otherwise (e.g. `pre-commit`, `ruff`, `mypy`). Installed tools are placed in `$XDG_DATA_HOME/uv/bin` and are available on `PATH` immediately.

### Installing a tool

```sh
uv tool install <package>
```

### Installing a specific version

```sh
uv tool install <package>==<version>
```

### Installing with extras

```sh
uv tool install <package>[extra]
```

### Upgrading a tool

```sh
uv tool upgrade <package>
```

### Checking what is installed

```sh
uv tool list
```

### Uninstalling a tool

```sh
uv tool uninstall <package>
```

### Running a tool once (no permanent install)

`uvx` downloads and runs the tool in a temporary environment without permanently
installing it.

```sh
uvx <package> [args]
```

### Installing Python versions

A Python version is pre-installed in the container image. Use nix to add specific or additional versions the project requires:

```sh
nix profile add nixpkgs#python312   # Python 3.12
nix profile add nixpkgs#python313   # Python 3.13
nix search nixpkgs python3          # find available versions
```

Installed Pythons are used automatically by `uv run`, `uv venv`, and similar commands.

## JavaScript and TypeScript tools

`npm` is not pre-installed in the container. Install Node.js with Nix first — `npm` is bundled with it:

```sh
nix profile add nixpkgs#nodejs_22
```

The npm global prefix is configured to `$XDG_DATA_HOME/npm`, so `npm install -g` works without root once Node.js is installed.

```sh
npm install -g <package>          # install latest
npm install -g <package>@1.2.3    # install specific version
npm list -g --depth=0             # check what is installed
npm update -g <package>           # upgrade
```

## Python tools

Use nix for Python interpreters and `uv` for Python-based tools:

```sh
nix profile add nixpkgs#python312   # install a specific Python version
uv tool install <package>           # install a Python-based tool
uv tool upgrade <package>           # upgrade a tool
```

## Java tools

Install the required JDK and the project's preferred build tool with Nix, then use the build tool for project-scoped work:

```sh
nix profile add nixpkgs#jdk21 nixpkgs#maven
nix profile add nixpkgs#jdk21 nixpkgs#gradle
```

Use Maven or Gradle according to the project's existing build files and wrapper scripts.

## C# tools

Install the matching SDK first, then use the `dotnet` CLI for project work or global tools:

```sh
nix profile add nixpkgs#dotnet-sdk
dotnet --info
dotnet tool install --global <tool-name>
dotnet tool list --global
```

Prefer the SDK version pinned by `global.json` when that file exists.

## C++ tools

Install the compiler, CMake, and any package manager the project already uses:

```sh
nix profile add nixpkgs#clang nixpkgs#cmake
nix profile add nixpkgs#gcc nixpkgs#cmake
nix profile add nixpkgs#conan
```

If the project uses vcpkg, follow the repository's documented bootstrap and manifest workflow rather than inventing a parallel package path.

## Go tools

Once `go` is installed, use it to install Go ecosystem tools:

```sh
go install <module-path>@vX.Y.Z   # install specific version (preferred — reproducible)
go install <module-path>@latest   # exploratory or one-off installs only; not reproducible
```

Installed binaries land in `$GOPATH/bin` (`$XDG_DATA_HOME/go/bin`), which is on `PATH`.

## Rust tools

`rustup` must be installed first via Nix, then a toolchain selected:

```sh
nix profile add nixpkgs#rustup
rustup toolchain install stable   # or a specific version, e.g. 1.78
rustup default stable
```

Once `cargo` is available, use it for Rust ecosystem tools:

```sh
cargo install <crate>                  # install latest
cargo install <crate> --version 1.2.3  # install specific version
cargo install --list                   # check what is installed
```

Installed binaries land in `$CARGO_HOME/bin` (`$XDG_DATA_HOME/cargo/bin`), which is on `PATH`.

______________________________________________________________________

## Installing review and analysis tools

When a review agent needs an analysis tool that is not yet installed, apply these checks before installing.

### Trustworthiness signals

A tool is safe to install directly if it meets most of these signals:

- **Origin:** hosted under the official organisation for the ecosystem or language (for example, a PyPA-maintained package, an official npm organisation, or the project's canonical source repository)
- **Adoption:** widely used in the ecosystem — high download counts, referenced in official documentation or well-known guides
- **Maintenance:** recent commits, responsive to issues, not archived or abandoned
- **Transparency:** open source with a visible licence compatible with the project
- **No install-time side effects:** the package does not run network requests or filesystem writes outside its install path during `postinstall`/`preinstall` hooks

If a tool clearly meets these signals, install it. If any signal raises doubt — unusual origin, very low adoption, recent maintainer change, install scripts that access the network — report the concern and ask the user to confirm before proceeding.

### Missing runtimes

If the tool requires a language runtime that is not installed (for example, Java is absent but SpotBugs is needed):

1. Note the gap: which tools could not be run and why.
1. Inform the caller that the required runtime is absent and the task cannot be completed until it is installed.

The caller is responsible for deciding whether to install the runtime (using the package manager guidance above) or to defer the work.

______________________________________________________________________

## Cache hygiene

Never write tool caches inside the repository working directory. If a cache or data environment variable is unset or points inside the workspace, redirect it to a directory under `$HOME`. Always check `.github/opencode-instructions.md` for project-specific overrides.
