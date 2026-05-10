# AI-Airbase

A containerised, isolated environment for running GitHub Copilot agents on your projects.

## What it is

AI-Airbase gives each project its own persistent Podman container — the **revetment** — pre-loaded with GitHub Copilot CLI, a suite of code analysis tools, and Nix for on-demand package installation. Inside every revetment lives a **cadre**: a group of specialist AI agents covering feature delivery, code review, refactoring, documentation, and more. The **marshal** CLI runs on your host machine to manage the container lifecycle — creating, starting, stopping, and recreating revetments — without ever running inside the container itself. Your project directory is bind-mounted in automatically, so the environment is ready whenever you are.

The naming follows a military airbase metaphor: a revetment is the hardened blast bay where aircraft are maintained, the cadre are the trained ground crew, and marshal guides each aircraft to its stand.

## Components

| Component     | What it does                                                                     |
| ------------- | -------------------------------------------------------------------------------- |
| **Marshal**   | Host-side CLI — manages the container lifecycle for each project                 |
| **Revetment** | Container image — Fedora-based sandbox with Copilot CLI, analysis tools, and Nix |
| **Cadre**     | Specialist AI agents bundled inside every revetment container                    |

## Prerequisites

- [Podman](https://podman.io/) installed and available on your `PATH`
- A [GitHub Copilot subscription](https://github.com/features/copilot/plans) with GitHub Copilot CLI access

## Installation

**Download the binary**

Grab the latest `marshal` binary from the [releases page](https://github.com/rob-broadley/ai-airbase/releases) and move it into place:

```bash
mkdir -p ~/.local/bin
mv marshal ~/.local/bin/marshal
chmod +x ~/.local/bin/marshal
```

If `~/.local/bin` is not on your `PATH`, add it to your shell config:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

**Build from source**

```bash
git clone https://github.com/rob-broadley/ai-airbase
cd ai-airbase
make dev-image   # build the toolchain container (one time)
make install     # compile marshal and copy to ~/.local/bin
```

The revetment container image is pulled automatically the first time you run `marshal`.

## Quick start

```bash
# 1. Navigate to your project directory
cd ~/work/my-app

# 2. Create the revetment for this project
marshal create

# 3. Attach and launch GitHub Copilot CLI
marshal
```

`marshal` attaches to the container and starts GitHub Copilot CLI with `mission-control` as the active agent. Describe your task; `mission-control` routes it to the right specialist agents and coordinates the work.

```bash
# 4. Stop the container when you are done
marshal stop

# 5. Attach again any time — the container and its state are preserved
marshal
```

To mount additional directories alongside your project, pass `--mount` flags to `marshal create`. When any `--mount` flag is given, only the listed paths are mounted — include your project directory explicitly:

```bash
marshal create --mount . --mount ../shared-lib --mount ~/configs
```

## Further reading

- [marshal CLI reference](docs/marshal.md) — all subcommands, flags, configuration, and environment variables
- [Cadre agent reference](docs/agents.md) — all bundled agents and skills
- [DEVELOPMENT.md](DEVELOPMENT.md) — contributing, project structure, and build system

## License

[AGPL-3.0-or-later](LICENSE)
