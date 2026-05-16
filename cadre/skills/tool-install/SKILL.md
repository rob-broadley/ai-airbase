---
name: tool-install
description: Load when any tool is missing or needs installing. Required by devex, full-reviewer, concurrency-reviewer, dead-code-detector, dependency-reviewer, and security-reviewer before installing analysis or audit tools.
license: AGPL-3.0-or-later
allowed-tools: execute
---

> **No root access.** `sudo`, `apt`, `dnf`, and all system package managers are unavailable. Never attempt them. All installs must go through `uv`, `nix`, or `npm`.

Two package managers are available as the bootstrap layer: **uv** and **Nix**. Once a language runtime is installed, its own toolchain should be used for the rest of that ecosystem.

| Use when                                                                         | Tool                                  |
| -------------------------------------------------------------------------------- | ------------------------------------- |
| Tool is a Python package or Python-based CLI (check PyPI first)                  | `uv`                                  |
| Tool is a language runtime (Go, Node.js, Java, Rust, …) or not available on PyPI | `nix`                                 |
| JavaScript and TypeScript runtime is installed — installing a package globally   | `npm install -g`                      |
| Python runtime is installed — installing a tool                                  | `uv tool install`                     |
| Java runtime is installed — installing project tools                             | `mvn`, `gradle`, or `nix profile add` |
| C# runtime is installed — installing a tool                                      | `dotnet tool install --global`        |
| C++ toolchain is installed — installing or resolving packages                    | `cmake`, `conan`, or `vcpkg`          |
| Go runtime is installed — installing a Go tool                                   | `go install`                          |
| Rust and Cargo are installed — installing a Rust tool                            | `cargo install`                       |

Use **uv** for Python-based tools and **Nix** for language runtimes and non-Python tools. Once a native toolchain is available, prefer it for that ecosystem.

## Language runtime quick reference

| Language                      | Runtime and package guidance                                                                                |
| ----------------------------- | ----------------------------------------------------------------------------------------------------------- |
| **JavaScript and TypeScript** | Install Node.js with Nix when needed, then use `npm` for ecosystem tools.                                   |
| **Python**                    | Use `uv python install` for interpreters and `uv tool install` for Python-based tools.                      |
| **Java**                      | Install a JDK with Nix, then use Maven or Gradle according to the project.                                  |
| **C#**                        | Install the matching `dotnet-sdk`, then use `dotnet` CLI tooling and global tools where appropriate.        |
| **C++**                       | Install a compiler and CMake with Nix, then follow the project's `conan`, `vcpkg`, or plain CMake workflow. |
| **Go**                        | Install the Go toolchain with Nix and then use `go install` for Go-native tools.                            |
| **Rust**                      | Install `rustup` with Nix, select the required toolchain, then use `cargo install` for Cargo-native tools.  |

## uv

Installed tools are placed in `$XDG_DATA_HOME/uv/bin` and are available on `PATH` immediately.

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

`uv` can manage Python interpreter installations side-by-side:

```sh
uv python install 3.12 3.13 3.14   # install multiple versions at once
uv python list                      # show available and installed versions
```

Installed Pythons are used automatically by `uv run`, `uv venv`, and similar commands.

## Nix

[Nix](https://nixos.org/) is available for on-demand tool installation. `nixpkgs` is
pinned to a known-good revision so installs are reproducible and served from the binary
cache without compiling from source.

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
nix profile add nixpkgs#ripgrep nixpkgs#fd nixpkgs#bat
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

## JavaScript and TypeScript tools

If the required Node.js version is not already present, install it with Nix first:

```sh
nix profile add nixpkgs#nodejs_22
```

The npm global prefix is configured to `$XDG_DATA_HOME/npm`, so `npm install -g` works without root.

```sh
npm install -g <package>          # install latest
npm install -g <package>@1.2.3    # install specific version
npm list -g --depth=0             # check what is installed
npm update -g <package>           # upgrade
```

## Python tools

Use `uv` for Python-native tools and interpreters:

```sh
uv python install 3.12
uv tool install <package>
uv tool upgrade <package>
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
go install <module-path>@latest   # install latest
go install <module-path>@v1.2.3   # install specific version
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

Never write tool caches inside the repository working directory. If a cache or data environment variable is unset or points inside the workspace, redirect it to a directory under `$HOME`. Always check `.github/copilot-instructions.md` for project-specific overrides.
