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

Project names must start and end with an alphanumeric character and contain only alphanumerics, hyphens, underscores, and dots (1–128 characters).
Names ending with `-pending-<pid>-<nano>` or `-retiring-<pid>-<nano>` are reserved for internal use.

______________________________________________________________________

## Commands

### marshal

Start or attach to the project container, launching Copilot CLI.

If the container does not exist, marshal creates it (pulling the image if needed) and starts it. If the container is stopped, marshal starts it and
attaches. If the container is already running, marshal attaches to the existing PID 1 session.

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

If no `--mount` flags are given, the current directory is used as the sole mount. Each `--mount` path is bind-mounted inside the container at
`/workspace/<basename>`. Paths are saved to the project config file and used by all subsequent commands automatically.

Errors if a container for the project already exists. To rebuild an existing project, use `marshal recreate`.
To change mounts or masks, run `marshal remove` then `marshal create` with the new `--mount` and `--mask` flags.

**Usage**

```
marshal create [flags]
```

**Flags**

#### `--mount <path>` / `-m`

Directory to bind mount into `/workspace/<basename>`. Repeatable.
When any `--mount` flag is given, only the listed paths are mounted —
the current directory is not added automatically.
Default: current directory (only when no `--mount` flags are given).

Each mount path must not contain `:`. No two paths may share the same
basename (each mount lands at `/workspace/<basename>`, so duplicate
basenames would collide). Nested mounts — where one path is a strict
subdirectory of another — are also rejected, as nesting would allow the
outer mount to expose files that a `--mask` on the inner tree is intended
to hide.

#### `--mask <path>`

Subdirectory to hide from the agent by shadowing it with a named Podman volume. Repeatable. The named volume starts empty on first use; any data
written by the agent into the masked directory persists in the volume and survives `marshal recreate`.

Paths are resolved like shell paths — relative to your current working directory, not relative to any mount root. marshal looks up which configured
mount contains the resolved path and mounts the named volume at the corresponding container path, making the host contents at that path invisible to
the agent. Absolute paths are accepted if they fall under a configured mount.

Each mask is resolved independently, so `--mask` works with any number of `--mount` flags.

Named volumes follow the scheme `marshal-<project>-mask-<mount-basename>-<encoded-rel-path>` where `<mount-basename>` is the last path component of
the containing mount and `<encoded-rel-path>` is the mask path relative to that mount, with hyphens doubled and slashes converted to hyphens (for
example, with mount `/projects/myapp`, masking `.venv` produces `marshal-myapp-mask-myapp-.venv`, masking `src/vendor` produces
`marshal-myapp-mask-myapp-src-vendor`, and masking `src-vendor` produces `marshal-myapp-mask-myapp-src--vendor`). The mount basename component
prevents volume name collisions when multiple mounts share the same relative subpath. Volume names are visible in `podman volume ls` output, which
means masked path names are visible to anyone who can list Podman volumes — accept this as a trade-off when path names are sensitive.

The path must not contain `:`, must not resolve to a mount root itself or escape the mount (for example, via `..`), and must fall under a configured
mount. Absolute paths are accepted when they resolve to a path inside a configured mount. Duplicate paths, paths that point to a regular file on the
host, and paths where one mask is a subdirectory of another are also rejected.

Mask volumes are preserved across `marshal recreate` (like the Nix store and uv tool cache), so any data written by the agent into the masked
directory survives a container rebuild. `marshal remove` deletes mask volumes automatically along with all other per-project volumes.

> [!NOTE]
> Masks are set at create time. The `--mask` flag is not available on `marshal recreate` — to change a project's masks, remove the project with
> `marshal remove` and recreate it with the new `--mask` flags.

> [!NOTE]
> If `marshal create` fails after some mask volumes were already created (for example, a second `podman volume create` call fails), the project config
> was saved before container creation began and any partial volumes are left in place. Running `marshal create` again is safe — volume creation uses
> `--ignore`, so re-running is idempotent and will not duplicate or corrupt existing volumes. If you want to abandon the project entirely, run
> `marshal remove`, which cleans up all label-tagged volumes including any that were partially provisioned.

**Examples**

```bash
# Create with the current directory as the only mount
marshal create

# Create with additional mounts (project directory must be listed explicitly)
marshal create --mount . --mount ../shared-lib --mount ~/configs

# Create a named project with an absolute path mount
marshal create --project my-app --mount /abs/path/to/project

# Mask the virtual environment and node_modules from the agent (single mount, default)
marshal create --mask .venv --mask node_modules

# Multi-mount project: mask paths resolve against CWD — here CWD is inside my-app
marshal create --mount ~/work/my-app --mount ~/shared-lib --mask .venv

# Absolute mask path — useful when CWD is not under any project mount
marshal create --mount ~/work/my-app --mask ~/work/my-app/.venv
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

When the container exists:

```
Project:      <project>
Container:    marshal-<project>
Status:       running | stopped | absent
Image Ref:    <image-ref> | -
Image ID:     <image-id> | -
Image Digest: <digest> | -
Version:      <version> | -
Created:      <timestamp> | -
Mounts:
  <symbol> <host-path>
  ...
Masks:
  <symbol> <host-path>
  ...
```

When the container is absent, entries are shown without symbol prefixes:

```
Project:      <project>
Container:    marshal-<project>
Status:       absent
Image Ref:    -
Image ID:     -
Image Digest: -
Version:      -
Created:      -
Mounts:
  <host-path>
  ...
Masks:
  <host-path>
  ...
```

When the container is absent, `Image Ref`, `Image ID`, `Image Digest`, `Version`, and `Created` are printed as `-`. When the image has no
`org.opencontainers.image.version` label, `Version` is printed as `-`. When the image digest is unavailable (e.g. for locally built images),
`Image Digest` is printed as `-`.

`Image Ref` shows the human-readable OCI image reference (e.g. `ghcr.io/rob-broadley/ai-airbase/revetment:latest`), sourced from the `ImageName` field
of `podman container inspect`. `Image ID` shows the raw local image identifier — a 64-character hex string with no prefix, sourced from the `Image`
field of `podman container inspect`. `Image Digest` shows the registry manifest digest, prefixed with `sha256:`, sourced from the `ImageDigest` field
of `podman container inspect`. Image ID and Image Digest identify the same image through different mechanisms and are not interchangeable.

When the container exists, each mount and mask entry is prefixed with a status symbol: `✓` (active — mounted from the configured host path at the
expected container path), `✗` (missing — configured but absent from the container), or `?` (not in project config — Mounts: untracked bind mount,
shown by host source path; Masks: untracked volume, shown by host-equivalent path derived from the parent bind mount, falling back to container path
if no parent bind mount is found). The symbols are also described in `marshal status --help`.

`Mounts:` lists the configured bind mounts by host path. When the container exists, each entry is prefixed with a status symbol: `✓` the bind mount is
active with the correct host source and container destination; `✗` the configured mount has no matching bind mount in the container. Bind mounts under
`/workspace/` that are present in the container but not in the project config are appended with `?` (shown by host path). When no entries are present
in this section, the field is printed as a single line: `Mounts:       none`.

`Masks:` lists the configured mask volumes by host path. When the container exists, each entry is prefixed with a status symbol: `✓` the named volume
is mounted at the expected container path; `✗` the configured mask has no corresponding volume mount in the container. `✓` and `✗` entries are shown
by host path, while volume mounts under `/workspace/` that are not accounted for by any configured mask are appended with `?` (shown by
host-equivalent path, derived from the parent bind mount; falls back to the container path if no parent bind mount can be found). When no entries are
present in this section, the field is printed as a single line: `Masks:        none`.

When the container is absent, `Mounts:` and `Masks:` list the paths saved in the project config file with no symbol prefix and no legend line. If the
config has no entries, the `none` placeholder is used.

**Examples**

```bash
marshal status
```

```
Project:      my-app
Container:    marshal-my-app
Status:       running
Image Ref:    ghcr.io/rob-broadley/ai-airbase/revetment:latest
Image ID:     20232757d1f59e6e733cd1cd3d8a35a87e24524a17b75543499dddc6c8a4369c
Image Digest: sha256:2a4a9ad4a3b974af6820af557240fced4c7395ab393ac1ea3b76ac71663a7921
Version:      0.1.1
Created:      2026-05-01 09:14:32
Mounts:
  ✓ /home/user/project
Masks:
  ✓ /home/user/project/secrets
```

```bash
marshal status --project other-project
```

```
Project:      other-project
Container:    marshal-other-project
Status:       absent
Image Ref:    -
Image ID:     -
Image Digest: -
Version:      -
Created:      -
Mounts:
  /home/user/other-project
Masks:
  /home/user/other-project/private
```

______________________________________________________________________

### marshal recreate

Atomically replace the project container with a fresh one.

marshal always pulls the latest image before replacing the container. If the pull fails but a local copy exists (for example, when offline), marshal
warns and continues with the local image. If no local image exists and the pull fails, marshal returns an error and leaves the old container intact.

The replacement uses a double-rename sequence (pending → canonical) to minimise the window during which no container is present at the canonical name.

**What is preserved across recreate**

- Three per-project tool volumes: the Nix store (`/nix/store`), the Nix user profile (`~/.local/state/nix`), and the uv tool cache
  (`~/.local/share/uv`). Tools installed by agents persist across recreates.
- Conversation history and agent checkpoints (`session-store.db` and `session-state/`) stored under `$XDG_DATA_HOME/marshal/projects/<project>/` on
  the host.
- Mask volumes — any data written by the agent into each masked directory is preserved across recreates.

**What is not preserved**

- Any state stored only in the container filesystem (not in a named volume or host bind mount), including the Copilot CLI binary. If the Copilot CLI
  updated itself via `/update` during a session, the image-bundled version is restored on the next `marshal recreate`.

> [!NOTE]
> The Copilot CLI binary is a pre-built native binary at `/opt/copilot/bin/copilot`. Running `/update` updates the CLI in-session (the binary
> bootstraps itself and stores updated state elsewhere), but the bundled binary is restored when `marshal recreate` pulls a new image. To get a
> permanently updated version, run `marshal recreate`.

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

Stop and permanently remove the container and all per-project state.

marshal stops the container if running, removes it, removes all three per-project tool volumes (Nix store, Nix profile, and uv tool cache), and
deletes the saved project config file. The revetment image is left untouched.

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

The container is created (pulling the image if needed) and started automatically if it does not exist or is stopped. When you exit the shell the
container keeps running; use `marshal stop` to stop it.

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

Each file records the directories to bind-mount into the container and any masked subdirectories:

```toml
mounts = [
    "/home/user/work/my-app",
    "/home/user/work/shared-lib",
]

masks = [
    "/home/user/work/my-app/.venv",
    "/home/user/work/my-app/node_modules",
]
```

`mounts` lists the host directories bind-mounted into `/workspace/`. `masks` lists the absolute host paths of subdirectories shadowed by empty named
volumes — these paths correspond to the `--mask` values passed to `marshal create`, resolved to absolute form.

marshal writes this file when you run `marshal create`. You do not normally need to edit it by hand. To change the mounts or masks for a project, run
`marshal remove` then `marshal create` with the new `--mount` and `--mask` flags.

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

## Container networking

The revetment container has unrestricted outbound network access. This is intentional: agents need network access to install packages, pull
dependencies, and perform research tasks.

______________________________________________________________________

## Exit codes

| Code | Meaning                                      |
| ---- | -------------------------------------------- |
| `0`  | Command completed successfully               |
| `1`  | An error occurred; details written to stderr |
