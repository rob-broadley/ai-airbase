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

To hide a subdirectory from the agent — for example a large generated directory that should not be read or modified — pass `--mask` with a path. Relative paths resolve against your current directory, so for a single-mount project `--mask .venv` works as-is. Absolute paths are also accepted when they fall under a configured mount:

```bash
marshal create --mask .venv --mask node_modules

# When CWD is outside the project mount, use an absolute path for --mask
marshal create --mount ~/work/my-app --mask ~/work/my-app/.venv
```

The agent sees an empty, writable directory at each masked path; the host contents are untouched. See the [marshal CLI reference](docs/marshal.md) for the full list of constraints and behaviour.

> [!NOTE]
> `--mask` is only accepted on `marshal create`. To change masks, run `marshal remove` then `marshal create` with the new flags. The masked path must be a directory that falls under a configured mount. Mask volumes persist across `marshal recreate` and are deleted by `marshal remove`.

## Security and trust

### Sandbox model

The revetment is a rootless Podman container. Copilot CLI runs as your host user's UID with `--security-opt no-new-privileges`, so the process cannot escalate privileges or access the host filesystem beyond the directories explicitly mounted via `--mount`. There is no root access to the host, and the container boundary provides meaningful containment for agent activity.

### Network access

The container has full outbound network access by design. Agents need this to install packages (via Nix, uv, and similar tools), query APIs, and perform research tasks. There is no outbound network restriction applied by marshal.

### Per-action confirmations

`/allow all` is a GitHub Copilot CLI command that removes confirmation prompts for all tool use. Without it, Copilot asks for your permission before each action; with it, the agent proceeds without pausing.

Inside a revetment, the practical consequences are: the agent can make network calls to external services, modify or delete any file in the mounted project directories, and run arbitrary code — all without prompting you. The container boundary remains intact; the agent cannot access the host beyond what is mounted and cannot gain elevated privileges. What is removed is the human checkpoint layer.

`/allow all` is appropriate for low-stakes sessions where uninterrupted throughput matters more than per-action oversight. For sessions involving sensitive files, destructive operations, or external API calls you want to review, omit `/allow all` so each action requires your confirmation.

______________________________________________________________________

## Further reading

- [marshal CLI reference](docs/marshal.md) — all subcommands, flags, configuration, and environment variables
- [Cadre agent reference](docs/agents.md) — all bundled agents and skills
- [DEVELOPMENT.md](DEVELOPMENT.md) — contributing, project structure, and build system

## License

[AGPL-3.0-or-later](LICENSE)
