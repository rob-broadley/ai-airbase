# AI-Airbase

> A multi-agent sandbox for GitHub Copilot — containerised, isolated, always ready.

## What is AI-Airbase?

**AI-Airbase** gives you an isolated, persistent container for running GitHub Copilot agents on any project. Each project gets its own container, pre-loaded with a group of expert agents and their tools. Your working directory is mounted in automatically, and the container stays around between sessions — so the environment is always ready.

You are the pilot. GitHub Copilot is your co-pilot, managing the agents inside the container. A CLI running on your host machine manages the container lifecycle — creating, starting, stopping, recreating — so you can focus on the work.

## Components

### The Plane — Your Project

Your project is the aircraft. The cadre are the ground crew getting it mission-ready. When the work is done, you — the pilot — take the controls.

### Cadre — The Agents

A cadre is a trained core group — in military use, the specialists that form the heart of a unit. The agents inside each container are the cadre: expert AI agents, each with a specific area of expertise and the tools for the job. They work on the project while it's in the revetment.

### Revetment — The Container

A revetment is a hardened blast bay on an airbase: a reinforced enclosure where aircraft are parked and maintained. Any incident stays inside; the surrounding base stays protected. Here it's the container: an isolated environment where the cadre work on your project, keeping their activity away from your host machine. Each project gets its own revetment.

### Marshal — The CLI

An aircraft marshaller guides aircraft to their stands and revetments — directing where each aircraft goes, when it moves, how long it stays. The `marshal` command does the same for your projects: it guides each one to its revetment, manages the container lifecycle, and clears the way for a fresh start when you need it. It runs on your host machine and never inside the container itself.

### AI-Airbase — The Project

AI-Airbase is the whole thing: the CLI, the containers, the agents, the concept. An AI airbase.

## Requirements

- [Podman](https://podman.io/) installed and available on your `PATH`
- A [GitHub Copilot subscription](https://github.com/features/copilot/plans)

## Installation

**Download the binary**

Grab the latest release from the [releases page](https://github.com/rob-broadley/ai-airbase/releases), then move it into place:

```sh
mkdir -p ~/.local/bin
mv marshal ~/.local/bin/marshal
chmod +x ~/.local/bin/marshal
```

If `~/.local/bin` isn't already on your `PATH`, add it to your shell config:

```sh
export PATH="$HOME/.local/bin:$PATH"
```

The container image is pulled automatically on first use.

## Concepts

### Projects

A **project** maps to a single named container (a revetment). By default the project name is derived from the current working directory's name, so running `marshal` inside `~/work/my-app` creates and manages a container called `marshal-my-app`. The cadre of agents inside that revetment is scoped to that project.

You can override this with the `--project` flag or the `MARSHAL_PROJECT` environment variable to share a container across directories or give it a more meaningful name.

### Container Lifecycle

| State       | Meaning                                                  |
| ----------- | -------------------------------------------------------- |
| **running** | Container is up and ready                                |
| **stopped** | Container exists but is not running (state is preserved) |
| **absent**  | No container exists for this project                     |

Containers are **persistent by default** — stopping a container does not remove it. Use `marshal remove` to remove it entirely, or `marshal recreate` to remove it and start fresh.

## Usage

`marshal create` sets up a new project — it saves the mount configuration and creates the container. Run it once per project.

`marshal` with no subcommand is the main day-to-day workflow: it attaches to the existing project container (or creates one with a CWD mount if no config exists yet) and launches Copilot CLI.

```sh
# First-time setup — mount the project and an adjacent shared library:
marshal create --mount ../shared-lib

# Or with no extra mounts (current directory is used):
marshal create

# Subsequently, just run marshal to attach:
marshal
```

Mount paths are saved to config by `create`, so all subsequent commands use them automatically.

### Global Options

| Flag               | Short | Description                                    |
| ------------------ | ----- | ---------------------------------------------- |
| `--project <name>` | `-p`  | Project name (default: current directory name) |
| `--help`           | `-h`  | Show help                                      |
| `--version`        | `-v`  | Show version                                   |

The project name can also be set via the `MARSHAL_PROJECT` environment variable. The `--project` flag takes precedence.

`marshal create` has its own `--mount` flag for specifying extra bind mounts — see the [`marshal create`](#marshal-create) section below.

______________________________________________________________________

### `marshal create`

Set up a new project: saves the mount configuration and creates the container. This is the first command to run for any new project.

If no `--mount` flags are given, the current directory is used as the sole mount. When `--mount` flags are provided, each specified path is mounted inside the container working directory.

Errors if the project already exists. To change mounts, use `marshal remove` then `marshal create`.

```sh
marshal create
marshal create --mount ../shared-lib --mount ~/configs
marshal create --project my-app --mount /abs/path
```

| Flag             | Short | Description                                             |
| ---------------- | ----- | ------------------------------------------------------- |
| `--mount <path>` | `-m`  | Directory to bind mount into the container (repeatable) |

______________________________________________________________________

### `marshal stop`

Stop the running container. The container and its state are preserved — run `marshal` again to bring it back.

```sh
marshal stop
marshal stop --project my-app
```

______________________________________________________________________

### `marshal status`

Show the current state of the container for this project.

```sh
marshal status
marshal status --project my-app
```

Example output:

```
Project:    my-app
Container:  marshal-my-app
Status:     running
Image:      revetment:latest
Created:    2026-05-01 09:14:32
```

______________________________________________________________________

### `marshal recreate`

Remove the existing container and create a fresh one. Useful when you want a clean environment or after updating the revetment image.

The per-project tool cache (Nix store) is preserved across recreate — tools fetched by agents do not need to be re-downloaded.

Conversation history and agent checkpoints (`session-store.db` and `session-state/`) are stored under `$XDG_DATA_HOME/marshal/projects/<project>/` on the host and bind-mounted into the container. They are not stored on the container volume, so they also survive a recreate.

```sh
marshal recreate
marshal recreate --project my-app
```

______________________________________________________________________

### `marshal remove`

Stop and permanently remove the container and its tool cache for this project. The revetment image is left untouched.

```sh
marshal remove
marshal remove --project my-app
```

______________________________________________________________________

### `marshal shell`

Open an interactive bash shell inside the same persistent container instead of launching Copilot CLI. Useful for debugging, inspecting the environment, or running commands manually. The container is created and started automatically if needed; when you exit the shell the container keeps running.

```sh
marshal shell
marshal shell --project my-app
```

______________________________________________________________________

### `marshal pull`

Pull or refresh the revetment container image without touching the container. Useful for updating the image ahead of an `marshal recreate`.

```sh
marshal pull
marshal pull --project my-app
```

______________________________________________________________________

## Configuration

marshal stores per-project configuration in `$XDG_CONFIG_HOME/marshal/` — typically `~/.config/marshal/` on Linux. Each project has its own file:

```
~/.config/marshal/
└── projects/
    ├── my-app.toml
    └── other-project.toml
```

Configuration grows with the tool, but at minimum each project file records the directories to bind mount. An example:

```toml
mounts = [
    "/home/user/work/shared-lib",
    "/home/user/configs",
]
```

You don't normally need to edit these files by hand — `marshal` writes them when you run `marshal create`, saving either the explicit `--mount` paths or the current directory when no mounts are specified.

## Environment Variables

| Variable          | Description                                                                          |
| ----------------- | ------------------------------------------------------------------------------------ |
| `MARSHAL_PROJECT` | Default project name (overridden by `--project`)                                     |
| `MARSHAL_IMAGE`   | Container image to use (default: `ghcr.io/rob-broadley/ai-airbase/revetment:latest`) |

## Container Naming

Containers are named `marshal-<project>`. For example, a project named `my-app` creates a container called `marshal-my-app`. This makes it easy to inspect or interact with revetment containers directly via `podman` if needed.

## Roadmap

- [x] Container lifecycle management (`create`, `stop`, `remove`, `recreate`)
- [x] Per-project named containers
- [x] CWD bind mounted as working directory
- [x] Extra bind mounts via `marshal create --mount`
- [x] On-demand tool installation via Nix inside the container
- [x] Per-project tool cache — preserved across `recreate`, removed with `remove`
- [ ] Bundled Copilot CLI agents inside the image
