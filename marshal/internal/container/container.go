// SPDX-License-Identifier: AGPL-3.0-or-later
// Package container provides a thin wrapper around the Podman CLI.
// It handles container lifecycle (create, start, stop, remove, inspect),
// image operations (existence checks and pulls), and mount resolution,
// delegating process execution to an injectable Runner.
package container

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ---------------------------------------------------------------------------
// Constants — eliminate magic values and keep command construction DRY.
// ---------------------------------------------------------------------------

// podmanBin is the name (or path) of the Podman executable. It is a package-
// level variable rather than a constant so that internal tests can substitute a
// fake binary without requiring a real Podman daemon.
var podmanBin = "podman"

const (
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

	// labelVolumePrefix is the image-label prefix used to declare named volume
	// mounts. Labels of the form io.ai-airbase.volume.<suffix>=<containerPath>
	// map a volume name suffix to its mount path. Marshal cross-references these
	// labels with the image's VOLUME declarations to determine which paths need
	// persistent named volumes and what to call them.
	labelVolumePrefix = "io.ai-airbase.volume."

	// ContainerUserHome is the home directory of the container user inside the image.
	// All credential mount targets and the HOME environment variable must agree with this value.
	ContainerUserHome = "/home/copilot"

	// ContainerCopilotDir is the container-side path for the Copilot extension state directory.
	ContainerCopilotDir = ContainerUserHome + "/.copilot"

	// ContainerGitConfigFile is the XDG user-level git config path inside the container.
	// This overrides the system /etc/gitconfig baked into the image.
	ContainerGitConfigFile = ContainerUserHome + "/.config/git/config"
)

// DefaultContainerCmd is the command run inside the container when started
// by marshal. It launches Copilot CLI routing through the mission-control agent.
var DefaultContainerCmd = []string{"copilot", "--agent=mission-control"}

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

// ImageVolumeSpec describes a volume declared by an image: the name suffix
// (from the io.ai-airbase.volume.* label) and the container-side mount path
// (from the VOLUME declaration). The volume name is derived at runtime by
// prepending the container name and a hyphen.
type ImageVolumeSpec struct {
	Suffix        string // e.g. "nix-store"
	ContainerPath string // e.g. "/nix/store"
}

// NamedVolumeMount describes a named Podman volume and its container mount point.
// Unlike a MountSpec (bind mount), this refers to a Podman-managed named volume.
type NamedVolumeMount struct {
	Name          string // e.g. "marshal-myapp-nix-store"
	ContainerPath string // e.g. "/nix/store"
	HostPath      string // host path shadowed by this volume (mask volumes only; empty for image volumes)
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
	RunStreaming(name string, stdout, stderr io.Writer, args ...string) error
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

// RunStreaming executes name with the given args, streaming stdout and stderr
// to the provided writers. Unlike Run, output is not captured — it flows
// directly to stdout and stderr for real-time progress display.
func (p PodmanRunner) RunStreaming(name string, stdout, stderr io.Writer, args ...string) error {
	cmd := exec.Command(name, args...)
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

// WorkdirFromMounts returns the container working directory given the resolved
// workspace mounts. When there is exactly one workspace mount the working directory
// is set to that mount's container path, so the user lands directly inside their
// project without having to cd. For zero or multiple mounts the working directory
// is /workspace, leaving navigation to the user.
func WorkdirFromMounts(mounts []MountSpec) string {
	if len(mounts) == 1 {
		return mounts[0].ContainerPath
	}
	return workspaceDir
}

// ImageVolumeSpecs inspects image and returns the volumes it declares that have
// a corresponding io.ai-airbase.volume.* label. It cross-references the image's
// VOLUME declarations (the set of paths that need persistence) with the label
// map (the naming convention for each path). Only paths present in both are
// returned; VOLUME paths without a matching label are silently ignored.
// Results are sorted by ContainerPath for deterministic ordering.
func ImageVolumeSpecs(r Runner, image string) ([]ImageVolumeSpec, error) {
	volOut, err := r.Run(podmanBin, "image", "inspect", "--format", "{{json .Config.Volumes}}", image)
	if err != nil {
		return nil, fmt.Errorf("inspecting image volumes: %w", err)
	}
	var volumePaths map[string]struct{}
	if err := json.Unmarshal(bytes.TrimSpace(volOut), &volumePaths); err != nil {
		return nil, fmt.Errorf("parsing image volumes: %w", err)
	}

	labOut, err := r.Run(podmanBin, "image", "inspect", "--format", "{{json .Config.Labels}}", image)
	if err != nil {
		return nil, fmt.Errorf("inspecting image labels: %w", err)
	}
	var labels map[string]string
	if err := json.Unmarshal(bytes.TrimSpace(labOut), &labels); err != nil {
		return nil, fmt.Errorf("parsing image labels: %w", err)
	}

	// Build reverse map: container path → volume name suffix.
	pathToSuffix := make(map[string]string, len(labels))
	for k, v := range labels {
		if suffix, ok := strings.CutPrefix(k, labelVolumePrefix); ok {
			pathToSuffix[v] = suffix
		}
	}

	var specs []ImageVolumeSpec
	for path := range volumePaths {
		if suffix, ok := pathToSuffix[path]; ok {
			specs = append(specs, ImageVolumeSpec{Suffix: suffix, ContainerPath: path})
		}
	}
	sort.Slice(specs, func(i, j int) bool { return specs[i].ContainerPath < specs[j].ContainerPath })
	return specs, nil
}

// maskVolumeSpec computes the NamedVolumeMount for a single mask path.
// maskAbsPath is the absolute host path of the mask (from cfg.Masks).
// mountRoot is the absolute host path of the mount that contains this mask.
// containerMountRoot is the container-side path where the mount lands (e.g. /workspace/myapp).
//
// Volume names use the scheme: <containerName>-mask-<encodedMountBase>-<encodedRelPath>
// Both components use the same two-step injective encoding (hyphens doubled,
// then slashes converted to hyphens). Including the mount basename ensures
// that masking the same relative subpath under two different mounts produces
// distinct volume names. Mount basenames are guaranteed unique by
// checkBasenameConflicts, so this scheme is collision-free.
func maskVolumeSpec(containerName, maskAbsPath, mountRoot, containerMountRoot string) NamedVolumeMount {
	relPath := strings.TrimPrefix(maskAbsPath, mountRoot+"/")
	// Encode mount basename: only hyphen-doubling is needed (no slashes in a basename).
	mountBase := strings.ReplaceAll(filepath.Base(mountRoot), "-", "--")
	// Encode relative path using the same two-step injective mapping.
	// Order matters: double existing hyphens before converting slashes,
	// so "src/vendor" and "src-vendor" remain distinct.
	suffix := strings.ReplaceAll(relPath, "-", "--")
	suffix = strings.ReplaceAll(suffix, "/", "-")
	return NamedVolumeMount{
		Name:          containerName + "-mask-" + mountBase + "-" + suffix,
		ContainerPath: filepath.Join(containerMountRoot, relPath),
		HostPath:      maskAbsPath,
	}
}

// findContainingMountIndex returns the index into mountPaths of the mount that
// contains abs as a strict subdirectory, or -1 if no mount contains abs.
func findContainingMountIndex(abs string, mountPaths []string) int {
	for i, mp := range mountPaths {
		if strings.HasPrefix(abs, mp+string(filepath.Separator)) {
			return i
		}
	}
	return -1
}

// BuildMaskVolumes returns the NamedVolumeMount specs for all configured mask
// paths. Each mask is matched to its containing project mount so that the
// correct container-side mount root can be passed to maskVolumeSpec. If any
// saved mask no longer falls inside a configured project mount, the function
// fails closed with an error instead of silently skipping it.
func BuildMaskVolumes(containerName string, masks, mountPaths []string, mountSpecs []MountSpec) ([]NamedVolumeMount, error) {
	var volumes []NamedVolumeMount
	for _, m := range masks {
		i := findContainingMountIndex(m, mountPaths)
		if i < 0 {
			return nil, fmt.Errorf("mask %q is no longer inside any configured mount — remove or update the mask in the project config", m)
		}
		volumes = append(volumes, maskVolumeSpec(containerName, m, mountPaths[i], mountSpecs[i].ContainerPath))
	}
	return volumes, nil
}

// EnsureProjectVolume creates a named Podman volume called name if it does not
// already exist, labelling it with the project label so RemoveProjectVolumes
// can find it. It returns true when the volume was newly created, and false
// when it already existed. Any unexpected error is returned as the error value.
func EnsureProjectVolume(r Runner, name, containerName string) (bool, error) {
	out, err := r.Run(podmanBin, "volume", "ls", "--filter", "name="+name, "--format", "{{.Name}}")
	if err != nil {
		return false, fmt.Errorf("checking volume: %w", err)
	}
	alreadyExists := false
	for _, line := range splitLines(string(out)) {
		if line == name {
			alreadyExists = true
			break
		}
	}
	_, err = r.Run(podmanBin, "volume", "create",
		"--ignore",
		"--label", labelProject+"="+containerName,
		name,
	)
	if err != nil {
		return false, fmt.Errorf("creating volume: %w", err)
	}
	return !alreadyExists, nil
}

// RemoveProjectVolumes removes all named Podman volumes that carry the project
// label for containerName. It uses a single label-filter query to discover them,
// then removes all in one batch call. When no labelled volumes exist the
// function returns nil without issuing a remove call.
func RemoveProjectVolumes(r Runner, containerName string) error {
	out, err := r.Run(podmanBin, "volume", "ls",
		"--filter", "label="+labelProject+"="+containerName,
		"--format", "{{.Name}}")
	if err != nil {
		return fmt.Errorf("listing project volumes: %w", err)
	}
	names := splitLines(string(out))
	if len(names) == 0 {
		return nil
	}
	args := append([]string{"volume", "rm"}, names...)
	_, err = r.Run(podmanBin, args...)
	return err
}

// Exists returns true when a container with containerName is present
// (running or stopped). Uses an anchored regex filter (^name$) to prevent
// prefix-match false positives: without anchoring, a container named
// "marshal-foo" would falsely match when "marshal-foobar" exists.
func Exists(r Runner, containerName string) (bool, error) {
	return queryContainerState(r, containerName, true)
}

// IsRunning returns true when containerName is currently running.
// Uses an anchored regex filter (^name$) to prevent prefix-match false
// positives: without anchoring, a container named "marshal-foo" would
// falsely match when "marshal-foobar" exists.
func IsRunning(r Runner, containerName string) (bool, error) {
	return queryContainerState(r, containerName, false)
}

// Create creates a container from image but does NOT start it.
// The container is not auto-removed (--rm=false).
// --userns=keep-id maps the host user's UID/GID into the container namespace.
// --tty allocates a pseudo-TTY and --interactive sets OpenStdin=true so that
// podman start --attach --interactive properly connects stdin to the PTY.
// Each MountSpec becomes a -v flag with a :Z SELinux relabelling suffix.
// Each NamedVolumeMount becomes a -v flag without a :Z suffix (named volumes
// must not carry SELinux relabelling).
// uc.UID/GID are passed as --user; uc.HomeDir is exported via -e HOME.
// workdir sets the container's working directory via -w.
// The container will run the image's default CMD when started.
func Create(r Runner, containerName, image string, mounts []MountSpec, namedVolumes []NamedVolumeMount, uc UserConfig, workdir string, cmd []string) error {
	args := []string{
		"create",
		"--name", containerName,
		"--userns=keep-id",
		"--tty",
		"--interactive",
		"--security-opt", "no-new-privileges",
	}
	args = append(args, userIdentityArgs(uc)...)
	for _, m := range mounts {
		args = append(args, "-v", mountFlag(m))
	}
	for _, nv := range namedVolumes {
		args = append(args, "-v", nv.Name+":"+nv.ContainerPath)
	}
	args = append(args,
		"--label", labelManagedBy+"="+toolName,
		"--label", labelProject+"="+containerName,
		"--label", labelImage+"="+image,
		"-w", workdir, image,
	)
	args = append(args, cmd...)
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

// ForceRemove forcibly removes containerName without checking running state.
// Unlike Remove, it does not call IsRunning first; --force instructs the
// runtime to stop and remove the container in a single step, making it safe
// to call regardless of running state. Use for best-effort cleanup of staging
// containers where a separate IsRunning check would be redundant or unreliable.
func ForceRemove(r Runner, name string) error {
	_, err := r.Run(podmanBin, "rm", "--force", name)
	return err
}

// Rename renames the container identified by from to the name given by to.
// It runs `podman rename <from> <to>`, following the same calling convention
// as all other operations in this package.
func Rename(r Runner, from, to string) error {
	_, err := r.Run(podmanBin, "rename", from, to)
	return err
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
// It runs `podman image exists <image>` through r.Run: exit 0 → present,
// exit 1 → absent. Any other failure is returned as an error.
func ImageExists(r Runner, image string) (bool, error) {
	out, err := r.Run(podmanBin, "image", "exists", image)
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == imageAbsentExitCode {
		return false, nil
	}
	if len(out) > 0 {
		return false, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return false, err
}

// PullImage pulls image from a registry, streaming progress to stdout and stderr.
// It runs `podman pull --retry=1 <image>` via r.RunStreaming so the same
// Runner intercept point used by all other operations handles this call.
func PullImage(r Runner, image string, stdout, stderr io.Writer) error {
	return r.RunStreaming(podmanBin, stdout, stderr, "pull", "--retry=1", image)
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

// parseInspectOutput extracts image and created from raw podman inspect output.
// It uses the last non-empty line, so any leading warning lines written to
// stderr (e.g. from CombinedOutput) are skipped gracefully.
func parseInspectOutput(raw string) (image, created string) {
	line := lastNonEmptyLine(raw)
	parts := strings.SplitN(line, inspectSeparator, 2)
	image = strings.TrimSpace(parts[0])
	if len(parts) > 1 {
		created = strings.TrimSpace(parts[1])
	}
	return image, created
}

// lastNonEmptyLine returns the last line in s that contains non-whitespace
// characters. Returns an empty string if s is empty or all-whitespace.
func lastNonEmptyLine(s string) string {
	var last string
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			last = line
		}
	}
	return strings.TrimSpace(last)
}

// queryContainerState runs podman ps filtered to an exact container name match.
// When allContainers is true, --all is included so stopped containers are found.
func queryContainerState(r Runner, containerName string, allContainers bool) (bool, error) {
	args := []string{"ps"}
	if allContainers {
		args = append(args, "--all")
	}
	args = append(args,
		"--filter", "name=^"+regexp.QuoteMeta(containerName)+"$",
		"--format", formatNames,
	)
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

// splitLines splits s by newlines and returns non-empty, trimmed lines.
func splitLines(s string) []string {
	var result []string
	for _, line := range strings.Split(strings.TrimSpace(s), "\n") {
		if n := strings.TrimSpace(line); n != "" {
			result = append(result, n)
		}
	}
	return result
}
