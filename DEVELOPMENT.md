# Contributing to AI-Airbase

You don't need Go installed locally — all toolchain commands run inside `sapper`, a Podman container defined in `dev/Containerfile` (Fedora minimal + Go 1.26 + golangci-lint + mdformat). Podman is the only host dependency.

Build the builder image once before anything else:

```sh
make dev-image
```

## Getting started

```sh
git clone https://github.com/rob-broadley/ai-airbase
cd ai-airbase
make dev-image   # first time only
make build       # produces ./bin/marshal
```

```sh
make test        # run tests (race detector on)
make check       # fmt + vet + lint quality gate
make install     # build + copy to ~/.local/bin
```

## Makefile reference

| Target              | What it does                                               |
| ------------------- | ---------------------------------------------------------- |
| `make dev-image`    | Build the `sapper` image with OCI labels (one-time setup)  |
| `make image`        | Build the `revetment` container image with OCI labels      |
| `make build`        | Build the binary with version embedded via `git describe`  |
| `make test`         | Run tests with race detector                               |
| `make coverage`     | Run tests; produce `coverage.txt` and `coverage.html`      |
| `make fmt`          | Format all Go code with gofmt                              |
| `make fmt-check`    | Check formatting without modifying (used by `make check`)  |
| `make fmt-md`       | Format Markdown with mdformat                              |
| `make fmt-md-check` | Check Markdown formatting without modifying                |
| `make vet`          | Run `go vet`                                               |
| `make lint`         | Run golangci-lint                                          |
| `make check`        | Full quality gate: fmt-check + fmt-md-check + vet + lint   |
| `make tidy`         | Run `go mod tidy`                                          |
| `make install`      | Build + copy to `~/.local/bin/marshal`                     |
| `make clean`        | Remove binary and coverage files                           |
| `make cache-clean`  | Remove named Podman volumes for Go module and build caches |

Both `make dev-image` and `make image` automatically inject three OCI build args computed from the current git state:

| Variable   | Source                         | Example                     |
| ---------- | ------------------------------ | --------------------------- |
| `VERSION`  | `git describe --tags --always` | `v0.1.0` or `v0.1.0-3-gabc` |
| `REVISION` | `git rev-parse HEAD`           | `abc1234...`                |
| `CREATED`  | `date -u +%Y-%m-%dT%H:%M:%SZ`  | `2026-05-03T21:00:00Z`      |

`make image` (revetment only) also injects:

| Variable          | Source                  | Example  |
| ----------------- | ----------------------- | -------- |
| `COPILOT_VERSION` | `.copilot-version` file | `1.0.42` |

To update to the latest published Copilot CLI version (queries the npm registry and rewrites `.copilot-version`):

```sh
make update-copilot-version
```

To pin a specific version for a one-off build without changing `.copilot-version`:

```sh
make image COPILOT_VERSION=1.0.43
```

## Caches

Go modules and build artifacts live in named Podman volumes so they survive across runs — no manual setup needed.

| Volume                  | Mount path              | Purpose              |
| ----------------------- | ----------------------- | -------------------- |
| `marshal-gomod-cache`   | `/root/go/pkg/mod`      | Downloaded modules   |
| `marshal-gobuild-cache` | `/root/.cache/go-build` | Compiled build cache |

To wipe them and start fresh:

```sh
make cache-clean
```

## Building the marshal container image

The image is defined in `revetment/Containerfile`. Use `make image` to build it with the correct OCI labels and build args:

```sh
make image
```

For local development, point marshal at your local build rather than the published image:

```sh
export MARSHAL_IMAGE=revetment
marshal
```

Without this, `marshal recreate` will try to pull from `ghcr.io` on every run.

## Running tests

```sh
make test
```

For a coverage report:

```sh
make coverage
# open coverage.html in the project root
```

To run a single package directly:

```sh
podman run --rm -v .:/workspace:Z -w /workspace sapper go test -race ./marshal/internal/config/...
```

## Linting

Linter config is in `marshal/.golangci.yml`. Run the full quality gate before pushing:

```sh
make check
```

Or just the linter:

```sh
make lint
```

## Version info

The binary embeds a version string at build time via `git describe`:

```sh
marshal --version
```

Builds via `make build` get a proper version like `v0.3.1-2-gabcdef`. Direct `go build` without ldflags defaults to `"dev"`.

## Project structure

```
ai-airbase/
├── marshal/
│   ├── .golangci.yml
│   ├── go.mod
│   ├── go.sum
│   ├── cmd/main.go               # entry point
│   └── internal/
│       ├── cmd/                  # cobra commands and dependency injection
│       ├── config/               # XDG config dirs, TOML loading, project resolution
│       └── container/            # Runner interface, PodmanRunner, container lifecycle
├── cadre/                        # agents and skills
├── dev/Containerfile             # sapper image (build toolchain)
├── revetment/Containerfile       # revetment container image (agent sandbox)
└── Makefile
```

`marshal/internal/container` defines a `Runner` interface — `PodmanRunner` is the real implementation, tests use a fake. Business logic never spawns real containers.

`marshal/internal/config` handles all persistence: per-project TOML files under `$XDG_CONFIG_HOME/marshal/`.

## Development workflow

This project follows ATDD/TDD. The short version: don't write implementation code before you have a failing test.

For a new feature or fix:

1. **Understand the problem first** — identify what you're actually trying to solve, including edge cases, before touching code.
1. **Write a failing acceptance test** — describe the behaviour from the outside (CLI args in, observable output out).
1. **Write failing unit tests** — for the internal logic that the acceptance test exercises.
1. **Make them pass** — write the minimum code to go green.
1. **Refactor** — tidy structure and naming without changing behaviour; tests should still pass.

For small changes (typo fixes, obvious one-liners), use judgement. When in doubt, start with a test.

If something is hard to test, that's usually a design smell worth fixing rather than working around.

## Adding dependencies

```sh
podman run --rm -v .:/workspace:Z -w /workspace/marshal sapper go get github.com/some/package
make tidy
```

Commit both `marshal/go.mod` and `marshal/go.sum`.

## Code style

- Depend on interfaces, not concrete types — `container.Runner` is the reference.
- Keep packages focused. Don't reach across packages when you can inject a dependency.
- Standard Go conventions: short names in narrow scopes, descriptive names where scope is wide, errors wrapped with context.
- Comment the _why_, not the _what_.

## Troubleshooting

**`sapper` image not found**
Run `make dev-image`. Required once per machine (or after `dev/Containerfile` changes).

**Podman permission denied on workspace**
Make sure the volume mount uses `:Z` for SELinux relabelling. The Makefile handles this; if you're running podman manually, add `:Z` to the `-v` flag.

**Race detector failure**
A race detector report is a genuine data race, not a fluke. Read the goroutine trace in the output.

## Contributing

PRs welcome. Check `README.md` for planned work, or open an issue before building something new.
