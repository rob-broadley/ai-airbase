// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd

import (
	"fmt"
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
	pullErr := container.PullImage(deps.Runner, image, cmd.OutOrStdout(), cmd.ErrOrStderr())
	if pullErr != nil {
		localExists, existsErr := container.ImageExists(deps.Runner, image)
		if existsErr != nil || !localExists {
			if existsErr != nil {
				return fmt.Errorf("failed to pull image %s: %w; also failed to check local copy: %v", image, pullErr, existsErr)
			}
			return fmt.Errorf("failed to pull image %s: %w — check connectivity or run 'podman pull %s' manually", image, pullErr, image)
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: pull failed for %s: %v — using local image\n", image, pullErr)
	}
	return nil
}

// provisionVolumes inspects image to discover its declared volumes, ensures
// each named volume exists (creating it with the project label when absent),
// and returns the mounts to pass to Create.
func provisionVolumes(runner container.Runner, containerName, image string) ([]container.NamedVolumeMount, error) {
	specs, err := container.ImageVolumeSpecs(runner, image)
	if err != nil {
		return nil, fmt.Errorf("discovering image volumes: %w", err)
	}
	mounts := make([]container.NamedVolumeMount, 0, len(specs))
	for _, spec := range specs {
		name := containerName + "-" + spec.Suffix
		if err := container.EnsureProjectVolume(runner, name, containerName); err != nil {
			return nil, fmt.Errorf("ensuring volume %s: %w", name, err)
		}
		mounts = append(mounts, container.NamedVolumeMount{Name: name, ContainerPath: spec.ContainerPath})
	}
	return mounts, nil
}

// removeAndRecreateContainer removes an existing container (when present) and
// creates a fresh one with the supplied image, mounts, user config, and workdir.
// The container is left in the stopped/created state; it will be started and
// attached on the next marshal invocation. Per-project volumes labelled with
// the project name are intentionally left intact so that cached packages survive
// the rebuild.
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
	namedVolumes, err := provisionVolumes(runner, containerName, image)
	if err != nil {
		return err
	}
	if err := container.Create(runner, containerName, image, mountSpecs, namedVolumes, uc, workdir); err != nil {
		return fmt.Errorf("creating container: %w", err)
	}
	return nil
}

// prepareContainer ensures the container exists (creating it when absent),
// handling the mountsChanged case: when the container is running with different
// mounts it returns an actionable error; when stopped it removes and recreates.
// It returns the container name and whether the container is currently running.
// Callers use the returned state to decide between start-and-attach vs attach
// (for the default command) or start-then-exec (for the shell command).
func prepareContainer(cmd *cobra.Command, deps Deps, p containerParams, mountsChanged bool) (containerName string, running bool, err error) {
	exists, err := container.Exists(deps.Runner, p.containerName)
	if err != nil {
		return "", false, fmt.Errorf("checking container: %w", err)
	}

	if !exists {
		if err := pullImageIfMissing(cmd, deps, p.image); err != nil {
			return "", false, err
		}
		namedVolumes, err := provisionVolumes(deps.Runner, p.containerName, p.image)
		if err != nil {
			return "", false, err
		}
		if err := container.Create(deps.Runner, p.containerName, p.image, p.mountSpecs, namedVolumes, p.userConfig, p.workdir); err != nil {
			return "", false, fmt.Errorf("creating container: %w", err)
		}
		return p.containerName, false, nil
	}

	isRunning, err := container.IsRunning(deps.Runner, p.containerName)
	if err != nil {
		return "", false, fmt.Errorf("checking running state: %w", err)
	}

	if mountsChanged {
		if isRunning {
			return "", false, fmt.Errorf(
				"container %s is running with different mounts; stop it first with `marshal stop` or use `marshal recreate`",
				p.containerName,
			)
		}
		if err := pullImageIfMissing(cmd, deps, p.image); err != nil {
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
func ensureContainerAndStart(cmd *cobra.Command, deps Deps, projectFlag string, mountFlagValues []string) error {
	p, mountsChanged, err := resolveContainerParams(deps, projectFlag, mountFlagValues)
	if err != nil {
		return err
	}
	containerName, running, err := prepareContainer(cmd, deps, p, mountsChanged)
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
func ensureContainerAndExec(cmd *cobra.Command, deps Deps, projectFlag string, mountFlagValues []string) error {
	p, mountsChanged, err := resolveContainerParams(deps, projectFlag, mountFlagValues)
	if err != nil {
		return err
	}
	containerName, running, err := prepareContainer(cmd, deps, p, mountsChanged)
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
