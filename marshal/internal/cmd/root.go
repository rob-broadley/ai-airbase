// SPDX-License-Identifier: AGPL-3.0-or-later
// Package cmd implements the marshal cobra command tree and wires up
// all CLI subcommands with their shared dependencies.
package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/rob-broadley/ai-airbase/marshal/internal/config"
	"github.com/rob-broadley/ai-airbase/marshal/internal/container"
)

// ExecFunc replaces the current process with a new one (like syscall.Exec).
type ExecFunc func(argv []string) error

// Deps holds injectable dependencies so commands can be tested without real
// Podman or a real working directory.
type Deps struct {
	Runner              container.Runner
	ExecFn              ExecFunc
	Getwd               func() (string, error)
	Getuid              func() int
	Getgid              func() int
	EnsureSharedDataDir func(subdir string) (string, error)
	SaveConfig          func(project string, cfg *config.Config) error // defaults to config.Save
	// ResolveImage returns the container image to use. Defaults to resolveImage,
	// which reads MARSHAL_IMAGE from the environment and falls back to the
	// published revetment image. Override in tests to fix the image name without
	// touching the environment.
	ResolveImage func() string
	Stdout       io.Writer // defaults to os.Stdout when nil
	Stderr       io.Writer // defaults to os.Stderr when nil
}

// stdout returns the effective stdout writer: the injected one or os.Stdout.
func (d Deps) stdout() io.Writer {
	if d.Stdout != nil {
		return d.Stdout
	}
	return os.Stdout
}

// stderr returns the effective stderr writer: the injected one or os.Stderr.
func (d Deps) stderr() io.Writer {
	if d.Stderr != nil {
		return d.Stderr
	}
	return os.Stderr
}

// saveConfig returns the effective config-save function: the injected one or config.Save.
func (d Deps) saveConfig() func(string, *config.Config) error {
	if d.SaveConfig != nil {
		return d.SaveConfig
	}
	return config.Save
}

// resolveImage returns the effective image resolver: the injected one or the
// package-level resolveImage free function (which reads MARSHAL_IMAGE).
func (d Deps) resolveImage() string {
	if d.ResolveImage != nil {
		return d.ResolveImage()
	}
	return resolveImage()
}

// NewRootCmd builds the root cobra.Command tree with the supplied dependencies.
func NewRootCmd(deps Deps) *cobra.Command {
	var projectFlag string
	var mountFlags []string

	root := &cobra.Command{
		Use:   "marshal",
		Short: "A sandbox for running GitHub Copilot CLI — one container per project, managed for you.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return ensureContainerAndStart(cmd, deps, projectFlag, mountFlags)
		},
	}

	root.PersistentFlags().StringVarP(&projectFlag, "project", "p", "", "Project name (default: current directory name)")
	root.PersistentFlags().StringArrayVarP(&mountFlags, "mount", "m", nil, "Extra directory to bind mount (repeatable)")

	root.AddCommand(
		newStopCmd(deps, &projectFlag),
		newStatusCmd(deps, &projectFlag),
		newRecreateCmd(deps, &projectFlag, &mountFlags),
		newRemoveCmd(deps, &projectFlag),
		newShellCmd(deps, &projectFlag, &mountFlags),
		newPullCmd(deps),
	)

	return root
}

// Execute is the real entry point used by main.go. version is embedded at build
// time via -ldflags and exposed through cobra's --version flag and version subcommand.
func Execute(version string) {
	deps := Deps{
		Runner:              container.PodmanRunner{},
		ExecFn:              realExec,
		Getwd:               os.Getwd,
		Getuid:              os.Getuid,
		Getgid:              os.Getgid,
		EnsureSharedDataDir: config.EnsureSharedDataDir,
		SaveConfig:          config.Save,
		ResolveImage:        resolveImage,
		Stdout:              os.Stdout,
		Stderr:              os.Stderr,
	}
	rootCmd := NewRootCmd(deps)
	rootCmd.Version = version
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// realExec resolves argv[0] via PATH and replaces the current process image
// using syscall.Exec, passing the current environment unchanged.
func realExec(argv []string) error {
	path, err := exec.LookPath(argv[0])
	if err != nil {
		return err
	}
	return syscall.Exec(path, argv, os.Environ())
}

// containerNameForProject returns the Podman container name for a given project.
func containerNameForProject(project string) string {
	return "marshal-" + project
}

// resolveImage returns the container image to use: the MARSHAL_IMAGE environment
// variable when set, otherwise the default published image.
func resolveImage() string {
	if img := os.Getenv("MARSHAL_IMAGE"); img != "" {
		return img
	}
	return "ghcr.io/rob-broadley/ai-airbase/revetment:latest"
}

// resolveMountPaths returns the final list of host mount paths for a project.
// When flag overrides are supplied their relative paths are resolved against cwd
// and the result is persisted to the project config. Otherwise the saved config
// mounts are returned unchanged. An empty return value is valid — the caller's
// container.ResolveMounts will then default to CWD→/workspace.
func resolveMountPaths(save func(string, *config.Config) error, cwd string, mountFlagValues []string, project string, cfg *config.Config) ([]string, error) {
	if len(mountFlagValues) > 0 {
		mounts := make([]string, 0, len(mountFlagValues))
		for _, m := range mountFlagValues {
			if !filepath.IsAbs(m) {
				m = filepath.Clean(filepath.Join(cwd, m))
			}
			mounts = append(mounts, m)
		}
		cfg.Mounts = mounts
		if err := save(project, cfg); err != nil {
			return nil, fmt.Errorf("saving config: %w", err)
		}
		return mounts, nil
	}
	return cfg.Mounts, nil
}

// pullImageIfMissing skips the pull when the image already exists locally.
// When the image is absent it delegates to pullImageWithFallback.
func pullImageIfMissing(deps Deps, image string) error {
	imagePresent, err := container.ImageExists(deps.Runner, image)
	if err != nil {
		return fmt.Errorf("checking image: %w", err)
	}
	if imagePresent {
		return nil
	}
	return pullImageWithFallback(deps, image)
}

// pullImageRefresh always attempts a pull regardless of local state, then
// delegates failure handling to pullImageWithFallback.
func pullImageRefresh(deps Deps, image string) error {
	return pullImageWithFallback(deps, image)
}

// pullImageWithFallback pulls image and handles failure gracefully: if the pull
// fails but a local copy exists (e.g. offline) it warns and continues; if no
// local copy exists it returns an actionable error.
func pullImageWithFallback(deps Deps, image string) error {
	pullErr := container.PullImage(deps.Runner, image, deps.stdout(), io.Discard)
	if pullErr != nil {
		localExists, existsErr := container.ImageExists(deps.Runner, image)
		if existsErr != nil || !localExists {
			return fmt.Errorf("failed to pull image %s: %w — check connectivity or run 'podman pull %s' manually", image, pullErr, image)
		}
		fmt.Fprintf(deps.stderr(), "warning: pull failed for %s: %v — using local image\n", image, pullErr)
	}
	return nil
}

// mountsEqual reports whether two mount-path slices contain the same paths
// regardless of order. Both slices are copied before sorting so the originals
// are not mutated.
func mountsEqual(a, b []string) bool {
	aSorted := slices.Clone(a)
	bSorted := slices.Clone(b)
	slices.Sort(aSorted)
	slices.Sort(bSorted)
	return slices.Equal(aSorted, bSorted)
}

// containerParams holds the resolved parameters needed to create or interact
// with a managed container.
type containerParams struct {
	containerName string
	image         string
	workdir       string
	mountSpecs    []container.MountSpec
	userConfig    container.UserConfig
	mountsChanged bool // true when -m flags produced a different mount set than what was saved in config
}

// resolveContainerParams resolves the project name, working directory, config,
// mounts, credentials, and user identity into a single containerParams value.
// It also detects whether the --mount flags differ from the mounts saved in
// config, recording the result in containerParams.mountsChanged.
// It is the single source of truth for how a container is configured and is
// called by both prepareContainer and runRecreate.
func resolveContainerParams(deps Deps, projectFlag string, mountFlagValues []string) (containerParams, error) {
	project := config.ResolveProject(projectFlag, deps.Getwd)
	containerName := containerNameForProject(project)

	cwd, err := deps.Getwd()
	if err != nil {
		return containerParams{}, fmt.Errorf("getting working directory: %w", err)
	}

	cfg, err := config.Load(project)
	if err != nil {
		return containerParams{}, fmt.Errorf("loading config: %w", err)
	}

	oldMountPaths := slices.Clone(cfg.Mounts)

	mounts, err := resolveMountPaths(deps.saveConfig(), cwd, mountFlagValues, project, cfg)
	if err != nil {
		return containerParams{}, err
	}

	mountsChanged := len(mountFlagValues) > 0 && !mountsEqual(oldMountPaths, mounts)

	image := deps.resolveImage()
	mountSpecs := container.ResolveMounts(cwd, mounts)

	workdir := container.WorkdirFromMounts(mountSpecs)

	credMounts, err := buildCredentialMounts(deps)
	if err != nil {
		return containerParams{}, err
	}
	mountSpecs = append(mountSpecs, credMounts...)

	uc := buildUserConfig(deps)

	return containerParams{
		containerName: containerName,
		image:         image,
		mountSpecs:    mountSpecs,
		userConfig:    uc,
		workdir:       workdir,
		mountsChanged: mountsChanged,
	}, nil
}

// removeAndRecreateContainer removes an existing container (when present) and
// creates a fresh one with the supplied image, mounts, user config, and workdir.
// The container is left in the stopped/created state; it will be started and
// attached on the next marshal invocation. 
func removeAndRecreateContainer(runner container.Runner, containerName, image string, mountSpecs []container.MountSpec, uc container.UserConfig, workdir string) error {
	exists, err := container.Exists(runner, containerName)
	if err != nil {
		return fmt.Errorf("checking container: %w", err)
	}
	if exists {
		if err := container.Remove(runner, containerName); err != nil {
			return fmt.Errorf("removing container: %w", err)
		}
	}
	if err := container.Create(runner, containerName, image, mountSpecs, uc, workdir); err != nil {
		return fmt.Errorf("creating container: %w", err)
	}
	return nil
}

// prepareContainer resolves the project, loads config, assembles mounts and
// user config, and ensures the container exists (creating it when absent).
// When --mount flags produce a different set of mounts than those saved in
// config, it returns an actionable error if the container is running, or
// automatically removes and recreates the container if it is stopped.
// It returns the container name and whether the container is currently running.
// Callers use the returned state to decide between start-and-attach vs attach
// (for the default command) or start-then-exec (for the shell command).
func prepareContainer(deps Deps, projectFlag string, mountFlagValues []string) (containerName string, running bool, err error) {
	p, err := resolveContainerParams(deps, projectFlag, mountFlagValues)
	if err != nil {
		return "", false, err
	}

	exists, err := container.Exists(deps.Runner, p.containerName)
	if err != nil {
		return "", false, fmt.Errorf("checking container: %w", err)
	}

	if !exists {
		if err := pullImageIfMissing(deps, p.image); err != nil {
			return "", false, err
		}
		if err := container.Create(deps.Runner, p.containerName, p.image, p.mountSpecs, p.userConfig, p.workdir); err != nil {
			return "", false, fmt.Errorf("creating container: %w", err)
		}
		return p.containerName, false, nil
	}

	isRunning, err := container.IsRunning(deps.Runner, p.containerName)
	if err != nil {
		return "", false, fmt.Errorf("checking running state: %w", err)
	}

	if p.mountsChanged {
		if isRunning {
			return "", false, fmt.Errorf(
				"container %s is running with different mounts; stop it first with `marshal stop` or use `marshal recreate`",
				p.containerName,
			)
		}
		if err := pullImageIfMissing(deps, p.image); err != nil {
			return "", false, err
		}
		if err := removeAndRecreateContainer(deps.Runner, p.containerName, p.image, p.mountSpecs, p.userConfig, p.workdir); err != nil {
			return "", false, err
		}
		return p.containerName, false, nil
	}

	return p.containerName, isRunning, nil
}

// ensureContainerAndStart ensures the container exists then connects to PID 1.
// For a new or stopped container it calls ExecFn with
// ["podman", "start", "--attach", "--interactive", containerName] so the
// current process is replaced by the attached start and the copilot process
// running as PID 1 receives stdin/stdout directly.
// For an already-running container it calls ExecFn with
// ["podman", "attach", containerName] to join the existing PID 1 session.
func ensureContainerAndStart(_ *cobra.Command, deps Deps, projectFlag string, mountFlagValues []string) error {
	containerName, running, err := prepareContainer(deps, projectFlag, mountFlagValues)
	if err != nil {
		return err
	}
	if running {
		return deps.ExecFn([]string{"podman", "attach", containerName})
	}
	return deps.ExecFn([]string{"podman", "start", "--attach", "--interactive", containerName})
}

// ensureContainerAndExec ensures the container exists and is running, then
// replaces the current process with an interactive `podman exec` session
// running /bin/bash inside the container.
// When the container is stopped it is started via the runner first.
// This function is used exclusively by the shell subcommand.
func ensureContainerAndExec(_ *cobra.Command, deps Deps, projectFlag string, mountFlagValues []string) error {
	containerName, running, err := prepareContainer(deps, projectFlag, mountFlagValues)
	if err != nil {
		return err
	}
	if !running {
		if err := container.Start(deps.Runner, containerName); err != nil {
			return fmt.Errorf("starting container: %w", err)
		}
	}
	return container.Exec(deps.ExecFn, containerName, "/bin/bash")
}
