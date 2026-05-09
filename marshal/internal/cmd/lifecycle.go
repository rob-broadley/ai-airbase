// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

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

// createContainerWithVolumes provisions named volumes then creates the
// container. It is the single implementation of the
// "provision → log → create" sequence shared by prepareContainer,
// runCreate, and removeAndRecreateContainer.
func createContainerWithVolumes(runner container.Runner, log *slog.Logger, containerName, image string, mountSpecs []container.MountSpec, uc container.UserConfig, workdir string) error {
	namedVolumes, err := provisionVolumes(runner, log, containerName, image)
	if err != nil {
		return err
	}
	log.Info("creating container", "container", containerName)
	if err := container.Create(runner, containerName, image, mountSpecs, namedVolumes, uc, workdir); err != nil {
		return fmt.Errorf("creating container: %w", err)
	}
	return nil
}

// removeAndRecreateContainer atomically replaces an existing container with a
// freshly created one. To avoid leaving the user with no container when creation
// fails, the new container is first created under a temporary "-pending" name.
// Only once creation succeeds is the old container removed and the temporary
// container renamed to the canonical name. If creation fails the pending
// container is cleaned up (best-effort) and the original is left untouched.
// Per-project volumes are provisioned using the real container name so cached
// packages survive the rebuild.
func removeAndRecreateContainer(runner container.Runner, log *slog.Logger, containerName, image string, mountSpecs []container.MountSpec, uc container.UserConfig, workdir string) error {
	exists, err := container.Exists(runner, containerName)
	if err != nil {
		return fmt.Errorf("checking container: %w", err)
	}

	// Provision named volumes using the real container name so that existing
	// volume data (e.g. Nix store) is preserved across recreates.
	namedVolumes, err := provisionVolumes(runner, log, containerName, image)
	if err != nil {
		return err
	}

	// Create the replacement under a temporary name first.
	pendingName := fmt.Sprintf("%s-pending-%d", containerName, os.Getpid())
	log.Info("creating container", "container", pendingName)
	if err := container.Create(runner, pendingName, image, mountSpecs, namedVolumes, uc, workdir); err != nil {
		// Creation failed — clean up any partial pending container (best-effort)
		// and leave the original container untouched.
		_ = container.Remove(runner, pendingName)
		return fmt.Errorf("creating container: %w", err)
	}

	// Creation succeeded — now it is safe to remove the old container.
	if exists {
		log.Info("removing container", "container", containerName)
		if err := container.Remove(runner, containerName); err != nil {
			return fmt.Errorf("removing container: %w", err)
		}
	}

	// Promote the pending container to the canonical name.
	if err := container.Rename(runner, pendingName, containerName); err != nil {
		if cleanupErr := container.Remove(runner, pendingName); cleanupErr != nil {
			log.Warn("failed to remove stranded pending container", "container", pendingName, "error", cleanupErr)
		}
		return fmt.Errorf("renaming container %s to %s: %w; recover with: podman rename %s %s", pendingName, containerName, err, pendingName, containerName)
	}
	return nil
}

// prepareContainer ensures the container exists (creating it when absent),
// pulling the image if needed. It returns the container name and whether the
// container is currently running. Callers use the returned state to decide
// between start-and-attach vs attach (for the default command) or
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
		if err := createContainerWithVolumes(deps.Runner, deps.logger(), p.containerName, p.image, p.mountSpecs, p.userConfig, p.workdir); err != nil {
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

// ensureContainerAndStart ensures the container exists then connects to PID 1.
// For a new or stopped container it calls ExecFn with
// ["podman", "start", "--attach", "--interactive", containerName] so the
// current process is replaced by the attached start and the copilot process
// running as PID 1 receives stdin/stdout directly.
// For an already-running container it calls ExecFn with
// ["podman", "attach", containerName] to join the existing PID 1 session.
func ensureContainerAndStart(cmd *cobra.Command, deps Deps, projectFlag string) error {
	p, err := resolveContainerParams(deps, projectFlag)
	if err != nil {
		return err
	}
	containerName, running, err := prepareContainer(cmd, deps, p)
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
func ensureContainerAndExec(cmd *cobra.Command, deps Deps, projectFlag string) error {
	p, err := resolveContainerParams(deps, projectFlag)
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
