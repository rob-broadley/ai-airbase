// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd

import (
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/rob-broadley/ai-airbase/marshal/internal/container"
)

// isRegistryImage reports whether image is a registry-hosted reference that
// can be pulled. Any image reference containing a '/' is treated as a registry
// reference (e.g. "ghcr.io/org/img", "myregistry/img", "localhost/img").
// Bare names with no '/' (e.g. "revetment", "revetment:latest") are local-only
// images that must be built on the host and cannot be pulled.
func isRegistryImage(image string) bool {
	return strings.Contains(image, "/")
}

// pullImageIfMissing skips the pull when the image already exists locally.
// When the image is absent and is a registry reference it delegates to
// pullImageWithFallback. When the image is absent and is a local-only name
// (no registry hostname) it returns an actionable error.
func pullImageIfMissing(cmd *cobra.Command, deps Deps, image string) error {
	imagePresent, err := container.ImageExists(deps.Runner, image)
	if err != nil {
		return fmt.Errorf("checking image: %w", err)
	}
	if imagePresent {
		return nil
	}
	if !isRegistryImage(image) {
		return fmt.Errorf("local image %q not found — build it with 'podman build'", image)
	}
	return pullImageWithFallback(cmd, deps, image)
}

// pullImageWithFallback pulls image and handles failure gracefully: if the pull
// fails but a local copy exists (e.g. offline) it warns and continues; if no
// local copy exists it returns an actionable error. For local-only image names
// (no registry hostname) the pull is skipped and existence is verified instead.
func pullImageWithFallback(cmd *cobra.Command, deps Deps, image string) error {
	if !isRegistryImage(image) {
		exists, err := container.ImageExists(deps.Runner, image)
		if err != nil {
			return fmt.Errorf("checking image: %w", err)
		}
		if !exists {
			return fmt.Errorf("local image %q not found — build it with 'podman build'", image)
		}
		return nil
	}
	deps.logger().Info("pulling image", "image", image)
	pullErr := container.PullImage(deps.Runner, image, cmd.OutOrStdout(), cmd.ErrOrStderr())
	if pullErr != nil {
		localExists, existsErr := container.ImageExists(deps.Runner, image)
		if existsErr != nil || !localExists {
			if existsErr != nil {
				return fmt.Errorf("failed to pull image %s: %w; also failed to check local copy: %v", image, pullErr, existsErr)
			}
			return fmt.Errorf("failed to pull image %s: %w — check connectivity or run 'podman pull %s' manually", image, pullErr, image)
		}
		deps.logger().Warn("pull failed, using local image", "image", image, "error", pullErr)
	}
	return nil
}

// provisionVolumes inspects image to discover its declared volumes, ensures
// each named volume exists (creating it with the project label when absent),
// and returns the mounts to pass to Create. "provisioning volume" is logged
// only for volumes that are newly created; pre-existing volumes are silently
// reused so that recreate does not produce spurious log output.
func provisionVolumes(runner container.Runner, log *slog.Logger, containerName, image string) ([]container.NamedVolumeMount, error) {
	specs, err := container.ImageVolumeSpecs(runner, image)
	if err != nil {
		return nil, fmt.Errorf("discovering image volumes: %w", err)
	}
	mounts := make([]container.NamedVolumeMount, 0, len(specs))
	for _, spec := range specs {
		name := containerName + "-" + spec.Suffix
		created, err := container.EnsureProjectVolume(runner, name, containerName)
		if err != nil {
			return nil, fmt.Errorf("ensuring volume %s: %w", name, err)
		}
		if created {
			log.Info("provisioning volume", "volume", name)
		}
		mounts = append(mounts, container.NamedVolumeMount{Name: name, ContainerPath: spec.ContainerPath})
	}
	return mounts, nil
}

// provisionMaskVolumes ensures the host directory exists for each mask spec,
// then ensures a named volume exists for it.
// MkdirAll is called first so that Podman finds the directory already present
// with correct ownership, preventing it from creating a root-mapped directory.
// It reuses EnsureProjectVolume (which applies the project label) so that
// RemoveProjectVolumes will clean up mask volumes on remove.
// Volumes are provisioned one at a time; if any step fails the function returns
// immediately, leaving previously provisioned volumes in place. The caller is
// responsible for cleanup if the overall create/recreate fails.
func provisionMaskVolumes(mkdirAll func(string, fs.FileMode) error, runner container.Runner, log *slog.Logger, containerName string, maskSpecs []container.NamedVolumeMount) error {
	for _, spec := range maskSpecs {
		if err := mkdirAll(spec.HostPath, 0o755); err != nil {
			return fmt.Errorf("creating host directory %s: %w", spec.HostPath, err)
		}
		created, err := container.EnsureProjectVolume(runner, spec.Name, containerName)
		if err != nil {
			return fmt.Errorf("ensuring mask volume %s (host path: %s): %w", spec.Name, spec.HostPath, err)
		}
		if created {
			log.Info("provisioning mask volume", "volume", spec.Name)
		} else {
			log.Debug("reusing existing mask volume", "volume", spec.Name)
		}
	}
	return nil
}

// createContainerWithVolumes provisions named volumes then creates the
// container. It is the single implementation of the
// "provision → log → create" sequence shared by prepareContainer and
// runCreate. removeAndRecreateContainer performs an equivalent inline
// provisioning sequence.
func createContainerWithVolumes(mkdirAll func(string, fs.FileMode) error, runner container.Runner, log *slog.Logger, p containerParams) error {
	namedVolumes, err := provisionVolumes(runner, log, p.containerName, p.image)
	if err != nil {
		return err
	}
	if err := provisionMaskVolumes(mkdirAll, runner, log, p.containerName, p.maskVolumes); err != nil {
		return err
	}
	log.Info("creating container", "container", p.containerName)
	allNamedVolumes := slices.Concat(namedVolumes, p.maskVolumes)
	if err := container.Create(runner, p.containerName, p.image, p.port, p.mountSpecs, allNamedVolumes, p.userConfig, p.workdir, p.cmd); err != nil {
		return fmt.Errorf("creating container: %w", err)
	}
	return nil
}

// tryForceRemove force-removes name and logs a warning when it fails.
// It is intentionally best-effort: callers must not rely on the removal
// having succeeded when this function returns.
func tryForceRemove(runner container.Runner, log *slog.Logger, name, warnMsg string) {
	if err := container.ForceRemove(runner, name); err != nil {
		log.Warn(warnMsg, "container", name, "error", err)
	}
}

// removeAndRecreateContainer atomically replaces an existing container with a
// freshly created one using a double-rename sequence that closes the
// availability gap in the old create→remove→rename approach.
//
// The sequence is:
//  1. Create pending container under a staging name (PID+nano suffix).
//  2. Rename old → retiring (reversible aside; if this fails pending is cleaned).
//  3. Rename pending → canonical (if this fails, retiring is restored and
//     pending is cleaned up before returning the error).
//  4. Force-remove retiring container (best-effort; failure is only a warning).
//
// Per-project volumes are provisioned using the real container name so cached
// packages survive the rebuild.
func removeAndRecreateContainer(mkdirAll func(string, fs.FileMode) error, runner container.Runner, log *slog.Logger, p containerParams) error {
	exists, err := container.Exists(runner, p.containerName)
	if err != nil {
		return fmt.Errorf("checking container: %w", err)
	}

	// Provision named volumes using the real container name so that existing
	// volume data (e.g. Nix store) is preserved across recreates.
	namedVolumes, err := provisionVolumes(runner, log, p.containerName, p.image)
	if err != nil {
		return err
	}
	if err := provisionMaskVolumes(mkdirAll, runner, log, p.containerName, p.maskVolumes); err != nil {
		return err
	}

	// Build the PID+nanosecond suffix shared by both staging names so they
	// sort together and a single listing can identify both.
	suffix := fmt.Sprintf("%d-%d", os.Getpid(), time.Now().UnixNano())
	pendingName := fmt.Sprintf("%s-pending-%s", p.containerName, suffix)
	retiringName := fmt.Sprintf("%s-retiring-%s", p.containerName, suffix)

	// Step 1 — create the replacement under the staging name.
	log.Info("creating container", "container", pendingName)
	allNamedVolumes := slices.Concat(namedVolumes, p.maskVolumes)
	if err := container.Create(runner, pendingName, p.image, p.port, p.mountSpecs, allNamedVolumes, p.userConfig, p.workdir, p.cmd); err != nil {
		// Creation failed — clean up any partial pending container (best-effort)
		// and leave the original container completely untouched.
		tryForceRemove(runner, log, pendingName, "failed to clean up pending container after creation failure")
		return fmt.Errorf("creating container: %w", err)
	}

	// Step 2 — rename old → retiring so we can reverse if promotion fails.
	if exists {
		if err := container.Rename(runner, p.containerName, retiringName); err != nil {
			// Aside failed — pending container was created but the old container
			// is still at the canonical name, so just clean up the pending one.
			tryForceRemove(runner, log, pendingName, "failed to clean up pending container after rename-aside failure")
			return fmt.Errorf("renaming existing container aside: %w", err)
		}
	}

	// Step 3 — promote pending → canonical.
	if err := container.Rename(runner, pendingName, p.containerName); err != nil {
		// Promotion failed — restore the retiring container to the canonical
		// name so the user is never left without a container.
		if exists {
			if restoreErr := container.Rename(runner, retiringName, p.containerName); restoreErr != nil {
				log.Warn("failed to restore retiring container after promotion failure",
					"container", retiringName, "error", restoreErr)
			}
		}
		tryForceRemove(runner, log, pendingName, "failed to clean up pending container after promotion failure")
		return fmt.Errorf("renaming container %s to %s: %w; recover with: podman rename %s %s",
			pendingName, p.containerName, err, pendingName, p.containerName)
	}

	// Step 4 — force-remove the retiring container (best-effort cleanup).
	if exists {
		log.Info("removing retired container", "container", retiringName)
		tryForceRemove(runner, log, retiringName, "failed to remove retired container")
	}

	return nil
}

// prepareContainer ensures the container exists (creating it when absent),
// pulling the image if needed. It returns the container name and whether the
// container is currently running. Callers use the returned state to decide
// whether they can reuse the running container (for the default command), or
// start-then-exec (for the shell command).
func prepareContainer(cmd *cobra.Command, deps Deps, p containerParams) (containerName string, running bool, err error) {
	exists, err := container.Exists(deps.Runner, p.containerName)
	if err != nil {
		return "", false, fmt.Errorf("checking container: %w", err)
	}

	if !exists {
		if err := pullImageIfMissing(cmd, deps, p.image); err != nil {
			return "", false, err
		}
		if err := createContainerWithVolumes(deps.mkdirAll(), deps.Runner, deps.logger(), p); err != nil {
			return "", false, err
		}
		return p.containerName, false, nil
	}

	isRunning, err := container.IsRunning(deps.Runner, p.containerName)
	if err != nil {
		return "", false, fmt.Errorf("checking running state: %w", err)
	}

	return p.containerName, isRunning, nil
}

// ensureContainerAndStart ensures the container exists then starts it in the background
// if it is stopped or non-existent, or prints informational status if it is already running.
func ensureContainerAndStart(cmd *cobra.Command, deps Deps, projectFlag string) error {
	project, _, err := resolveContainer(deps, projectFlag)
	if err != nil {
		return err
	}
	paths, err := ensureHostState(deps, project)
	if err != nil {
		return err
	}
	p, err := resolveContainerParams(deps, projectFlag, paths)
	if err != nil {
		return err
	}
	containerName, running, err := prepareContainer(cmd, deps, p)
	if err != nil {
		return err
	}
	if running {
		fmt.Fprintf(cmd.OutOrStdout(), "container %s is already running\nOpenCode Web is available at http://127.0.0.1:%d/\n", containerName, p.port)
		return nil
	}
	if err := container.Start(deps.Runner, containerName); err != nil {
		return fmt.Errorf("starting container: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "container %s started\nOpenCode Web is available at http://127.0.0.1:%d/\n", containerName, p.port)
	return nil
}

// ensureContainerAndExec ensures the container exists and is running, then
// replaces the current process with an interactive `podman exec` session
// running /bin/bash inside the container.
// When the container is stopped it is started via the runner first.
// This function is used exclusively by the shell subcommand.
func ensureContainerAndExec(cmd *cobra.Command, deps Deps, projectFlag string) error {
	project, _, err := resolveContainer(deps, projectFlag)
	if err != nil {
		return err
	}
	paths, err := ensureHostState(deps, project)
	if err != nil {
		return err
	}
	p, err := resolveContainerParams(deps, projectFlag, paths)
	if err != nil {
		return err
	}
	containerName, running, err := prepareContainer(cmd, deps, p)
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
