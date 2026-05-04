// SPDX-License-Identifier: AGPL-3.0-or-later
// Package container provides a thin wrapper around the Podman CLI.
// It handles container lifecycle (create, start, stop, remove, inspect),
// image operations (existence checks and pulls), and mount resolution,
// delegating process execution to an injectable Runner.
package container

import (
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// ---------------------------------------------------------------------------
// Constants — eliminate magic values and keep command construction DRY.
// ---------------------------------------------------------------------------

const (
	podmanBin = "podman"

	// workspaceDir is the container-side path where project directories are mounted.
	workspaceDir       = "/workspace"
	formatNames        = "{{.Names}}"
	formatImageCreated = "{{.Image}}|{{.Created}}"
	inspectSeparator   = "|"

	// imageAbsentExitCode is the exit code that `podman image exists` returns
	// when the named image is not present in the local store.
	imageAbsentExitCode = 1

	// toolName is the value written to the managed-by label so containers
	// created by this tool can be identified programmatically.
	toolName = "marshal"

	// Label keys applied to every container created by this tool.
	labelManagedBy = "io.ai-airbase.managed-by"
	labelProject   = "io.ai-airbase.project"
	labelImage     = "io.ai-airbase.image"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// Status describes the current state of a named container.
type Status struct {
	Image   string
	Created string
	Exists  bool
	Running bool
}

// MountSpec describes a single bind-mount: a host path mapped to a container path.
type MountSpec struct {
	HostPath      string
	ContainerPath string
}

// UserConfig carries the identity that the container process should run as.
// UID and GID map to --user <UID>:<GID>; HomeDir is exported as HOME=<HomeDir>.
type UserConfig struct {
	HomeDir string // container-side home path, e.g. "/home/copilot"
	UID     int
	GID     int
}

// ---------------------------------------------------------------------------
// Runner interface + PodmanRunner implementation
// ---------------------------------------------------------------------------

// Runner abstracts the execution of an external command so that tests can
// inject a fake without needing a real Podman daemon.
type Runner interface {
	Run(name string, args ...string) ([]byte, error)
	ImageExists(image string) (bool, error)
	PullImage(image string, stdout, stderr io.Writer) error
}

// PodmanRunner is the real Runner implementation that calls exec.Command.
type PodmanRunner struct{}

// Run executes name with the given args and returns combined stdout+stderr output.
// When the command fails and produced output, the output is appended to the
// returned error so callers automatically see the underlying Podman message.
func (p PodmanRunner) Run(name string, args ...string) ([]byte, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil && len(out) > 0 {
		return out, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return out, err
}

// ImageExists reports whether image is present in the local Podman image store.
// It runs `podman image exists <image>`: exit 0 → present, exit 1 → absent.
// Any other failure is returned as an error.
func (p PodmanRunner) ImageExists(image string) (bool, error) {
	err := exec.Command(podmanBin, "image", "exists", image).Run()
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == imageAbsentExitCode {
		return false, nil
	}
	return false, err
}

// PullImage pulls image from a registry, streaming progress to stdout and
// stderr. It runs `podman pull --retry=1 <image>` and returns nil on success.
// --retry=1 limits to one retry so transient failures respond quickly without
// the default three-retry wait.
func (p PodmanRunner) PullImage(image string, stdout, stderr io.Writer) error {
	cmd := exec.Command(podmanBin, "pull", "--retry=1", image)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

// ---------------------------------------------------------------------------
// Public functions
// ---------------------------------------------------------------------------

// ResolveMounts converts a list of host paths into MountSpec values.
// When mounts is empty, cwd is mounted at /workspace/<basename(cwd)>.
// Otherwise each path is mounted at /workspace/<basename(path)> and cwd is
// NOT mounted.
func ResolveMounts(cwd string, mounts []string) []MountSpec {
	if len(mounts) == 0 {
		return []MountSpec{{HostPath: cwd, ContainerPath: filepath.Join(workspaceDir, filepath.Base(cwd))}}
	}
	specs := make([]MountSpec, 0, len(mounts))
	for _, p := range mounts {
		specs = append(specs, MountSpec{
			HostPath:      p,
			ContainerPath: filepath.Join(workspaceDir, filepath.Base(p)),
		})
	}
	return specs
}

// WorkdirFromMounts returns the container working directory to use given
// the resolved workspace mounts. When there is exactly one workspace mount,
// the working directory is set to that mount's container path so the user
// lands directly inside their project. For zero or multiple mounts the
// working directory is /workspace.
func WorkdirFromMounts(mounts []MountSpec) string {
	if len(mounts) == 1 {
		return mounts[0].ContainerPath
	}
	return workspaceDir
}

// Exists returns true when a container with containerName is present
// (running or stopped).
func Exists(r Runner, containerName string) (bool, error) {
	return queryContainerNames(r, containerName, true)
}

// IsRunning returns true when containerName is currently running.
func IsRunning(r Runner, containerName string) (bool, error) {
	return queryContainerNames(r, containerName, false)
}

// Create creates a container from image but does NOT start it.
// The container is not auto-removed (--rm=false).
// --userns=keep-id maps the host user's UID/GID into the container namespace.
// --tty allocates a pseudo-TTY and --interactive sets OpenStdin=true so that
// podman start --attach --interactive properly connects stdin to the PTY.
// Each MountSpec becomes a -v flag with a :Z SELinux relabelling suffix.
// uc.UID/GID are passed as --user; uc.HomeDir is exported via -e HOME.
// workdir sets the container's working directory via -w.
// The container will run the image's default CMD when started.
func Create(r Runner, containerName, image string, mounts []MountSpec, uc UserConfig, workdir string) error {
	args := []string{
		"create",
		"--name", containerName,
		"--userns=keep-id",
		"--tty",
		"--interactive",
	}
	args = append(args, userIdentityArgs(uc)...)
	for _, m := range mounts {
		args = append(args, "-v", mountFlag(m))
	}
	args = append(args,
		"--label", labelManagedBy+"="+toolName,
		"--label", labelProject+"="+containerName,
		"--label", labelImage+"="+image,
		"-w", workdir, image,
	)
	_, err := r.Run(podmanBin, args...)
	return err
}

// Start starts an already-created container.
func Start(r Runner, containerName string) error {
	_, err := r.Run(podmanBin, "start", containerName)
	return err
}

// Stop stops a running container.
func Stop(r Runner, containerName string) error {
	_, err := r.Run(podmanBin, "stop", containerName)
	return err
}

// Remove stops the container if running, then removes it permanently.
func Remove(r Runner, containerName string) error {
	running, err := IsRunning(r, containerName)
	if err != nil {
		return fmt.Errorf("checking running state: %w", err)
	}
	if running {
		if err := Stop(r, containerName); err != nil {
			return fmt.Errorf("stopping container: %w", err)
		}
	}
	if _, err = r.Run(podmanBin, "rm", containerName); err != nil {
		return fmt.Errorf("removing container: %w", err)
	}
	return nil
}

// GetStatus returns the full Status of a container.
// When the container does not exist, Status.Exists is false and all other
// fields are zero values.
func GetStatus(r Runner, containerName string) (Status, error) {
	exists, err := Exists(r, containerName)
	if err != nil {
		return Status{}, fmt.Errorf("checking existence: %w", err)
	}
	if !exists {
		return Status{Exists: false}, nil
	}

	running, err := IsRunning(r, containerName)
	if err != nil {
		return Status{}, fmt.Errorf("checking running state: %w", err)
	}

	out, err := r.Run(podmanBin, "inspect", "--format", formatImageCreated, containerName)
	if err != nil {
		return Status{}, fmt.Errorf("inspecting container: %w", err)
	}

	image, created := parseInspectOutput(string(out))

	return Status{
		Exists:  true,
		Running: running,
		Image:   image,
		Created: created,
	}, nil
}

// ImageExists reports whether image is present in the local Podman image store.
// It delegates to r.ImageExists, providing the same container.Op(r, …) calling
// convention used by all other operations in this package.
func ImageExists(r Runner, image string) (bool, error) {
	return r.ImageExists(image)
}

// PullImage pulls image from a registry, streaming progress to stdout and stderr.
// It delegates to r.PullImage, providing the same container.Op(r, …) calling
// convention used by all other operations in this package.
func PullImage(r Runner, image string, stdout, stderr io.Writer) error {
	return r.PullImage(image, stdout, stderr)
}

// Exec replaces the current process with an interactive `podman exec -it` session
// running command inside containerName. execFn is typically deps.ExecFn (a
// syscall.Exec wrapper) or a test spy. command must contain at least one element.
func Exec(execFn func([]string) error, containerName string, command ...string) error {
	args := append([]string{"podman", "exec", "-it", containerName}, command...)
	return execFn(args)
}

// ---------------------------------------------------------------------------
// Helpers — internal helpers used within this package only.
// ---------------------------------------------------------------------------

// mountFlag formats m as the value for a single -v flag:
// "hostPath:containerPath:Z". The :Z suffix requests SELinux relabelling so
// the container process can read and write the bind-mounted directory.
func mountFlag(m MountSpec) string {
	return m.HostPath + ":" + m.ContainerPath + ":Z"
}

// userIdentityArgs returns the --user and -e HOME flags that tell the
// container process which UID/GID to run as and where its home directory is.
func userIdentityArgs(uc UserConfig) []string {
	return []string{
		"--user", fmt.Sprintf("%d:%d", uc.UID, uc.GID),
		"-e", "HOME=" + uc.HomeDir,
		"--passwd-entry", fmt.Sprintf("copilot:x:%d:%d::%s:/bin/bash", uc.UID, uc.GID, uc.HomeDir),
	}
}

// queryContainerNames runs `podman ps [--all] --filter name=^X$ --format {{.Names}}`
// and reports whether containerName appears as an exact line in the output.
// Pass includeAll=true to match stopped containers too (Exists semantics),
// or false to match only running containers (IsRunning semantics).
//
// The --filter uses an anchored regex (^name$) because podman's name filter
// performs prefix/substring matching by default, which would cause a container
// named "marshal-foo" to falsely match when "marshal-foobar" exists. The
// anchored regex combined with an exact-line check eliminates both false
// positives.
func queryContainerNames(r Runner, containerName string, includeAll bool) (bool, error) {
	args := []string{"ps"}
	if includeAll {
		args = append(args, "--all")
	}
	// Anchor the filter to prevent prefix-match false positives.
	args = append(args, "--filter", "name=^"+regexp.QuoteMeta(containerName)+"$", "--format", formatNames)
	out, err := r.Run(podmanBin, args...)
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.TrimSpace(line) == containerName {
			return true, nil
		}
	}
	return false, nil
}

// parseInspectOutput splits "image|created\n" into its two parts.
func parseInspectOutput(raw string) (image, created string) {
	line := strings.TrimSpace(raw)
	parts := strings.SplitN(line, inspectSeparator, 2)
	image = strings.TrimSpace(parts[0])
	if len(parts) > 1 {
		created = strings.TrimSpace(parts[1])
	}
	return
}
