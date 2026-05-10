# marshal CLI reference

`marshal` manages Podman containers (revetments) for GitHub Copilot CLI — one container per project — from your host machine.

## Synopsis

```
marshal [flags]
marshal <command> [flags]
```

## Global flags

These flags apply to every command.

| Flag               | Short | Default                | Description               |
| ------------------ | ----- | ---------------------- | ------------------------- |
| `--project <name>` | `-p`  | Current directory name | Project name              |
| `--help`           | `-h`  | —                      | Show help for the command |
| `--version`        | `-v`  | —                      | Print the marshal version |

`MARSHAL_PROJECT` sets the project name when `--project` is not given. The flag takes precedence over the environment variable.

Project names must start with an alphanumeric character and contain only alphanumerics, hyphens, underscores, and dots (1–128 characters). Names ending with `-pending-<pid>-<nano>` or `-retiring-<pid>-<nano>` are reserved for internal use.

______________________________________________________________________

## Commands

### marshal

Start or attach to the project container, launching Copilot CLI.

If the container does not exist, marshal creates it (pulling the image if needed) and starts it. If the container is stopped, marshal starts it and attaches. If the container is already running, marshal attaches to the existing PID 1 session.

The current process is replaced by the `podman start` or `podman attach` process.

**Usage**

```
marshal [flags]
```

**Flags**

Inherits [global flags](#global-flags) only.

**Examples**

```bash
# Start or attach using the current directory as the project name
marshal

# Attach to a named project regardless of working directory
marshal --project my-app
```

______________________________________________________________________

### marshal create

Save the mount configuration and create the container. Run this once per project before using any other lifecycle commands.

If no `--mount` flags are given, the current directory is used as the sole mount. Each `--mount` path is bind-mounted inside the container at `/workspace/<basename>`. Paths are saved to the project config file and used by all subsequent commands automatically.

Errors if a container for the project already exists. To rebuild an existing project, use `marshal recreate`. To change mounts, run `marshal remove` then `marshal create` with the new `--mount` flags.

**Usage**

```
marshal create [flags]
```

**Flags**

| Flag             | Short | Default                                                  | Description                                                                                                                                                                               |
| ---------------- | ----- | -------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `--mount <path>` | `-m`  | None (defaults to current directory when no flags given) | Directory to bind mount into `/workspace/<basename>`. Repeatable. When any `--mount` flag is given, only the listed paths are mounted — the current directory is not added automatically. |

**Examples**

```bash
# Create with the current directory as the only mount
marshal create

# Create with additional mounts (project directory must be listed explicitly)
marshal create --mount . --mount ../shared-lib --mount ~/configs

# Create a named project with an absolute path mount
marshal create --project my-app --mount /abs/path/to/project
```

______________________________________________________________________

### marshal stop

Stop the running container. The container and its state are preserved; run `marshal` to bring it back.

Exits without error if the container is already stopped.

**Usage**

```
marshal stop [flags]
```

**Flags**

Inherits [global flags](#global-flags) only.

**Examples**

```bash
# Stop the container for the current directory project
marshal stop

# Stop a named project
marshal stop --project my-app
```

______________________________________________________________________

### marshal status

Print the current state of the container for this project.

**Usage**

```
marshal status [flags]
```

**Flags**

Inherits [global flags](#global-flags) only.

**Output**

```
Project:   <project>
Container: marshal-<project>
Status:    running | stopped | absent
Image:     <image> | -
Created:   <timestamp> | -
```

When the container is absent, `Image` and `Created` are printed as `-`.

**Examples**

```bash
marshal status
```

```
Project:   my-app
Container: marshal-my-app
Status:    running
Image:     ghcr.io/rob-broadley/ai-airbase/revetment:latest
Created:   2026-05-01 09:14:32
```

```bash
marshal status --project other-project
```

```
Project:   other-project
Container: marshal-other-project
Status:    absent
Image:     -
Created:   -
```

______________________________________________________________________

### marshal recreate

Atomically replace the project container with a fresh one.

marshal always pulls the latest image before replacing the container. If the pull fails but a local copy exists (for example, when offline), marshal warns and continues with the local image. If no local image exists and the pull fails, marshal returns an error and leaves the old container intact.

The replacement uses a double-rename sequence (pending → canonical) to minimise the window during which no container is present at the canonical name.

**What is preserved across recreate**

- The per-project Nix store volume — tools fetched by agents do not need to be re-downloaded.
- Conversation history and agent checkpoints (`session-store.db` and `session-state/`) stored under `$XDG_DATA_HOME/marshal/projects/<project>/` on the host.

**What is not preserved**

- Any state stored only in the container filesystem (not in a named volume or host bind mount).

**Usage**

```
marshal recreate [flags]
```

**Flags**

Inherits [global flags](#global-flags) only.

**Examples**

```bash
# Recreate the container for the current directory project
marshal recreate

# Recreate a named project
marshal recreate --project my-app
```

______________________________________________________________________

### marshal remove

Stop and permanently remove the container and its tool cache for this project.

marshal stops the container if running, removes it, removes the associated Nix store volume, and deletes the saved project config file. The revetment image is left untouched.

After `remove`, the project has no container and no config. Use `marshal create` to start fresh.

**Usage**

```
marshal remove [flags]
```

**Flags**

Inherits [global flags](#global-flags) only.

**Examples**

```bash
# Remove the container for the current directory project
marshal remove

# Remove a named project
marshal remove --project my-app
```

______________________________________________________________________

### marshal shell

Open an interactive bash shell inside the container.

The container is created (pulling the image if needed) and started automatically if it does not exist or is stopped. When you exit the shell the container keeps running; use `marshal stop` to stop it.

The current process is replaced by a `podman exec` session running `/bin/bash`.

**Usage**

```
marshal shell [flags]
```

**Flags**

Inherits [global flags](#global-flags) only.

**Examples**

```bash
# Open a shell in the current directory project
marshal shell

# Open a shell in a named project
marshal shell --project my-app
```

______________________________________________________________________

### marshal pull

Pull or refresh the revetment container image without touching any container.

`pull` is image-global — it is not project-scoped and does not use the `--project` flag. Run it to update the image ahead of a `marshal recreate`.

**Usage**

```
marshal pull [flags]
```

**Flags**

Inherits [global flags](#global-flags) only. The `--project` flag has no effect on this command.

**Examples**

```bash
# Pull the latest revetment image
marshal pull

# Pull a custom image (set MARSHAL_IMAGE first)
MARSHAL_IMAGE=registry.example.com/team/revetment:v2 marshal pull
```

______________________________________________________________________

### marshal completion

Generate shell completion scripts.

**Usage**

```
marshal completion <shell>
```

**Supported shells**

| Shell      | Subcommand                      |
| ---------- | ------------------------------- |
| Bash       | `marshal completion bash`       |
| Zsh        | `marshal completion zsh`        |
| Fish       | `marshal completion fish`       |
| PowerShell | `marshal completion powershell` |

**Examples**

```bash
# Bash — load for every new session (Linux)
marshal completion bash > /etc/bash_completion.d/marshal

# Bash — load in the current session only
source <(marshal completion bash)

# Zsh — load for every new session (Linux)
marshal completion zsh > "${fpath[1]}/_marshal"

# Fish — load for every new session
marshal completion fish > ~/.config/fish/completions/marshal.fish
```

Run `marshal completion <shell> --help` for shell-specific instructions including macOS paths.

______________________________________________________________________

## Configuration

marshal stores per-project configuration as TOML files under `$XDG_CONFIG_HOME/marshal/` (typically `~/.config/marshal/` on Linux).

```
~/.config/marshal/
└── projects/
    ├── my-app.toml
    └── other-project.toml
```

Each file records the directories to bind-mount into the container:

```toml
mounts = [
    "/home/user/work/my-app",
    "/home/user/work/shared-lib",
]
```

marshal writes this file when you run `marshal create`. You do not normally need to edit it by hand. To change the mounts for a project, run `marshal remove` then `marshal create` with the new `--mount` flags.

______________________________________________________________________

## Environment variables

| Variable          | Description                                                                          |
| ----------------- | ------------------------------------------------------------------------------------ |
| `MARSHAL_PROJECT` | Default project name. Overridden by `--project`.                                     |
| `MARSHAL_IMAGE`   | Container image to use. Default: `ghcr.io/rob-broadley/ai-airbase/revetment:latest`. |

______________________________________________________________________

## Container naming

Containers are named `marshal-<project>`. A project named `my-app` creates a container called `marshal-my-app`.

You can inspect or interact with these containers directly via `podman` when needed:

```bash
podman ps --filter name=marshal-my-app
podman logs marshal-my-app
```

______________________________________________________________________

## Exit codes

| Code | Meaning                                      |
| ---- | -------------------------------------------- |
| `0`  | Command completed successfully               |
| `1`  | An error occurred; details written to stderr |
