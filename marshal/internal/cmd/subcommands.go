// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/rob-broadley/ai-airbase/marshal/internal/container"
)

// newCreateCmd returns the cobra.Command for the "create" subcommand, which
// saves the project mount configuration and creates the container. This is the
// entry point for setting up a new project; subsequent invocations of marshal
// read the saved mounts from config rather than accepting them as flags.
func newCreateCmd(deps Deps, projectFlag *string) *cobra.Command {
	var mountFlags []string
	var maskFlags []string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new container for the project, pulling the image if not present",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCreate(cmd, deps, *projectFlag, mountFlags, maskFlags)
		},
	}
	cmd.Flags().StringArrayVarP(&mountFlags, "mount", "m", nil, "Directory to bind mount into /workspace/<basename> (repeatable)")
	cmd.Flags().StringArrayVar(&maskFlags, "mask", nil, "Hide a host subdirectory from the agent by overlaying it with a named volume (repeatable). Path resolves from CWD; must fall inside a configured mount.")
	return cmd
}

// runCreate implements the "create" subcommand: it validates the project, errors
// if the container already exists, saves the mount configuration, pulls the
// image if needed, and creates the container.
func runCreate(cmd *cobra.Command, deps Deps, projectFlag string, mountFlagValues, maskFlagValues []string) error {
	project, containerName, err := resolveContainer(deps, projectFlag)
	if err != nil {
		return err
	}

	exists, err := container.Exists(deps.Runner, containerName)
	if err != nil {
		return fmt.Errorf("checking container: %w", err)
	}
	if exists {
		return fmt.Errorf("project %s already has a container; use 'marshal recreate' to rebuild with existing configuration, or 'marshal remove' then 'marshal create' to reconfigure mounts or masks", project)
	}

	cwd, err := deps.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	cfg, err := deps.loadConfig()(project)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// resolveMountPaths updates cfg.Mounts when flags are provided; otherwise
	// cfg.Mounts stays as loaded (empty for a fresh project).
	if _, err = resolveMountPaths(cwd, mountFlagValues, cfg); err != nil {
		return err
	}
	// If no explicit mounts were configured, fall back to CWD and persist it.
	if len(cfg.Mounts) == 0 {
		cfg.Mounts = []string{cwd}
	}
	if _, err = resolveMaskPaths(cwd, cfg.Mounts, maskFlagValues, cfg); err != nil {
		return err
	}
	if err := deps.saveConfig()(project, cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	p, err := resolveContainerParams(deps, projectFlag)
	if err != nil {
		return err
	}

	if err := pullImageIfMissing(cmd, deps, p.image); err != nil {
		return err
	}

	if err := createContainerWithVolumes(deps.mkdirAll(), deps.Runner, deps.logger(), p); err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "container %s created\n", p.containerName)
	return nil
}

// newStopCmd returns the cobra.Command for the "stop" subcommand, which stops
// the running container for the resolved project.
func newStopCmd(deps Deps, projectFlag *string) *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the running container for the project",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStop(cmd, deps, *projectFlag)
		},
	}
}

// runStop implements the "stop" subcommand: it resolves the project, verifies
// the container exists and is running, stops it, and reports the outcome.
func runStop(cmd *cobra.Command, deps Deps, projectFlag string) error {
	project, containerName, err := resolveContainer(deps, projectFlag)
	if err != nil {
		return err
	}

	exists, err := container.Exists(deps.Runner, containerName)
	if err != nil {
		return fmt.Errorf("checking container: %w", err)
	}
	if !exists {
		return fmt.Errorf("project %s has no container", project)
	}

	running, err := container.IsRunning(deps.Runner, containerName)
	if err != nil {
		return fmt.Errorf("checking running state: %w", err)
	}
	if !running {
		fmt.Fprintf(cmd.OutOrStdout(), "project %s container is already stopped\n", project)
		return nil
	}

	deps.logger().Info("stopping container", "container", containerName)
	if err := container.Stop(deps.Runner, containerName); err != nil {
		return fmt.Errorf("stopping container: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "container %s stopped\n", containerName)
	return nil
}

// newStatusCmd returns the cobra.Command for the "status" subcommand, which
// prints the current state of the container for the resolved project.
func newStatusCmd(deps Deps, projectFlag *string) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the current state of the container for the project",
		Long: `Show the current state of the container for the project.

Prints project name, container name, running status, image details, version,
creation time, and mount/mask configuration.

When the container exists, mount and mask entries are annotated with:
  ✓  active                  — configured entry is mounted from the configured host path at the expected container destination
  ✗  missing                 — configured entry has no corresponding container mount
  ?  not in project config   — Mounts: untracked bind mount; host source path shown
                               Masks:  untracked volume; host-equivalent path shown,
                                       derived from parent bind mount (falls back to
                                       container path if no parent bind mount is found)

When the container is absent, configured paths are listed without a symbol prefix.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(cmd, deps, *projectFlag)
		},
	}
}

// runStatus implements the "status" subcommand: it resolves the project,
// queries the container state, and prints project name, container name,
// running status, image ref, image ID, image digest, version, creation time,
// and configured mounts and masks. When the container exists, mount and mask
// entries are annotated with reconciliation symbols (✓/✗/?). When absent,
// raw config paths are shown with no symbol prefix.
func runStatus(cmd *cobra.Command, deps Deps, projectFlag string) error {
	project, containerName, err := resolveContainer(deps, projectFlag)
	if err != nil {
		return err
	}

	status, err := container.GetStatus(deps.Runner, containerName)
	if err != nil {
		return fmt.Errorf("getting container status: %w", err)
	}

	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "Project:      %s\n", project)
	fmt.Fprintf(w, "Container:    %s\n", containerName)

	if !status.Exists {
		fmt.Fprintf(w, "Status:       absent\n")
		fmt.Fprintf(w, "Image Ref:    -\n")
		fmt.Fprintf(w, "Image ID:     -\n")
		fmt.Fprintf(w, "Image Digest: -\n")
		fmt.Fprintf(w, "Version:      -\n")
		fmt.Fprintf(w, "Created:      -\n")
		cfg, err := deps.loadConfig()(project)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		if len(cfg.Mounts) > 0 {
			fmt.Fprintf(w, "Mounts:\n")
			for _, m := range cfg.Mounts {
				fmt.Fprintf(w, "  %s\n", sanitizeForTerminal(m))
			}
		} else {
			fmt.Fprintf(w, "Mounts:       none\n")
		}
		if len(cfg.Masks) > 0 {
			fmt.Fprintf(w, "Masks:\n")
			for _, m := range cfg.Masks {
				fmt.Fprintf(w, "  %s\n", sanitizeForTerminal(m))
			}
		} else {
			fmt.Fprintf(w, "Masks:        none\n")
		}
		return nil
	}

	statusStr := "stopped"
	if status.Running {
		statusStr = "running"
	}
	if status.Version == "" {
		deps.logger().Debug("image version label absent", "container", containerName)
	}
	version := sanitizeForTerminal(status.Version)
	if version == "" {
		if status.Version != "" {
			deps.logger().Debug("image version label stripped (control characters only)", "container", containerName)
		}
		version = "-"
	}
	if status.ImageDigest == "" {
		deps.logger().Debug("image digest absent", "container", containerName)
	}
	imageDigest := sanitizeForTerminal(status.ImageDigest)
	if imageDigest == "" {
		if status.ImageDigest != "" {
			deps.logger().Debug("image digest stripped (control characters only)", "container", containerName)
		}
		imageDigest = "-"
	}
	if status.ImageRef == "" {
		deps.logger().Debug("image ref absent", "container", containerName)
	}
	imageRef := sanitizeForTerminal(status.ImageRef)
	if imageRef == "" {
		if status.ImageRef != "" {
			deps.logger().Debug("image ref stripped (control characters only)", "container", containerName)
		}
		imageRef = "-"
	}
	image := sanitizeForTerminal(status.Image)
	if image == "" {
		image = "-"
	}
	created := sanitizeForTerminal(status.Created)
	if created == "" {
		if status.Created != "" {
			deps.logger().Debug("container created timestamp stripped (control characters only)", "container", containerName)
		}
		created = "-"
	}
	fmt.Fprintf(w, "Status:       %s\n", statusStr)
	fmt.Fprintf(w, "Image Ref:    %s\n", imageRef)
	fmt.Fprintf(w, "Image ID:     %s\n", image)
	fmt.Fprintf(w, "Image Digest: %s\n", imageDigest)
	fmt.Fprintf(w, "Version:      %s\n", version)
	fmt.Fprintf(w, "Created:      %s\n", created)

	cfg, err := deps.loadConfig()(project)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	actualMounts, err := container.GetMounts(deps.Runner, containerName)
	if err != nil {
		return fmt.Errorf("getting container mounts: %w", err)
	}
	renderMountsAndMasks(deps.logger(), w, cfg.Mounts, cfg.Masks, actualMounts)
	return nil
}

// newRecreateCmd returns the cobra.Command for the "recreate" subcommand,
// which pulls the latest image, removes the existing container, and creates a
// fresh one while preserving the per-project Nix store volume.
func newRecreateCmd(deps Deps, projectFlag *string) *cobra.Command {
	return &cobra.Command{
		Use:   "recreate",
		Short: "Atomically replace the project container with a fresh one",
		Long: `Pulls the latest agent image, removes the existing container, and creates a fresh replacement.

Mount and mask configuration is read from the saved project config — re-specifying --mount or --mask is not supported on recreate. To change mounts or masks, run 'marshal remove' then 'marshal create' with the new flags.

Named volumes (Nix store, uv cache, and any mask volumes) are preserved across recreate. Container filesystem state that is not in a named volume or bind mount is lost.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRecreate(cmd, deps, *projectFlag)
		},
	}
}

// runRecreate implements the "recreate" subcommand: it pulls the latest image,
// resolves mounts from saved config, removes the existing container (if any),
// and creates a replacement, leaving the Nix store volume intact.
func runRecreate(cmd *cobra.Command, deps Deps, projectFlag string) error {
	p, err := resolveContainerParams(deps, projectFlag)
	if err != nil {
		return err
	}

	// Always pull before touching the container. If the pull fails but a local
	// image exists (e.g. offline), warn and continue. If no local image exists,
	// return an error and leave the old container intact.
	if err := pullImageWithFallback(cmd, deps, p.image); err != nil {
		return err
	}

	if err := removeAndRecreateContainer(deps.mkdirAll(), deps.Runner, deps.logger(), p); err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "container %s recreated\n", p.containerName)
	return nil
}

// newRemoveCmd returns the cobra.Command for the "remove" subcommand, which
// stops and permanently removes the container and its Nix store volume for
// the resolved project.
func newRemoveCmd(deps Deps, projectFlag *string) *cobra.Command {
	return &cobra.Command{
		Use:   "remove",
		Short: "Stop and permanently remove the container for the project",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRemove(cmd, deps, *projectFlag)
		},
	}
}

// runRemove implements the "remove" subcommand: it stops the container if
// running, removes the container, removes the associated Nix store volume,
// and deletes the saved project config so a subsequent create starts clean.
func runRemove(cmd *cobra.Command, deps Deps, projectFlag string) error {
	project, containerName, err := resolveContainer(deps, projectFlag)
	if err != nil {
		return err
	}

	exists, err := container.Exists(deps.Runner, containerName)
	if err != nil {
		return fmt.Errorf("checking container: %w", err)
	}
	if !exists {
		return fmt.Errorf("project %s has no container", project)
	}

	deps.logger().Info("removing container", "container", containerName)
	if err := container.Remove(deps.Runner, containerName); err != nil {
		return fmt.Errorf("removing container: %w", err)
	}

	// Best-effort cleanup — continue past individual failures.
	var errs []error
	if err := container.RemoveProjectVolumes(deps.Runner, containerName); err != nil {
		errs = append(errs, fmt.Errorf("removing project volumes: %w", err))
	}
	if err := deps.deleteConfig()(project); err != nil {
		errs = append(errs, fmt.Errorf("removing project config: %w", err))
	}
	joinedErr := errors.Join(errs...)
	if joinedErr != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "container %s removed (cleanup partially failed: %v)\n", containerName, joinedErr)
		return joinedErr
	}
	fmt.Fprintf(cmd.OutOrStdout(), "container %s removed\n", containerName)
	return nil
}

// newShellCmd returns the cobra.Command for the "shell" subcommand, which
// opens an interactive bash shell in the managed container, creating and
// starting it if needed.
func newShellCmd(deps Deps, projectFlag *string) *cobra.Command {
	return &cobra.Command{
		Use:   "shell",
		Short: "Open an interactive shell inside the container",
		RunE: func(cmd *cobra.Command, args []string) error {
			return ensureContainerAndExec(cmd, deps, *projectFlag)
		},
	}
}

// newListCmd returns the cobra.Command for the "list" subcommand, which
// prints all registered projects and their container status.
func newListCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all registered projects and their container status",
		Long: `List all registered projects and their Podman container status.

Prints a table with three columns — NAME, STATUS, and CONFIG — sorted
in case-sensitive lexicographic order by project name. The header is always
printed, even when there are no projects.

STATUS values:
  running  the container is currently running
  stopped  the container exists but is not running
  absent   no container exists for this project yet
  unknown  the Podman query failed (a warning is printed to stderr)

CONFIG values:
  ok     the project config file is valid
  error  the project config file could not be loaded (a warning is printed to stderr)

Exit codes:
  0  success, including per-project Podman failures (STATUS "unknown") and
     config-load failures (CONFIG "error")
  1  the projects config directory is unreadable

The --project / -p flag is accepted but has no effect on the output.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd, deps)
		},
	}
}

// runList implements the "list" subcommand: it prints a header row and one
// row per registered project with its container status. The NAME column is
// padded to the width of the longest project name (or the header word "NAME"
// if shorter), with at least 2 spaces separating it from the STATUS column.
// When a per-project Podman query fails, the row shows "unknown", a warning is
// written to stderr naming the project, and the command still exits 0.
func runList(cmd *cobra.Command, deps Deps) error {
	projects, warnings, err := deps.listProjects()()
	if err != nil {
		return fmt.Errorf("listing projects: %w", err)
	}

	for _, w := range warnings {
		deps.logger().Warn(w)
	}

	// Compute column width: max of len("NAME") and all project name lengths.
	colWidth := len("NAME")
	for _, p := range projects {
		if len(p) > colWidth {
			colWidth = len(p)
		}
	}

	const statusColWidth = 7 // width of the longest STATUS value ("running", "stopped", "unknown")

	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "%-*s  %-*s  %s\n", colWidth, "NAME", statusColWidth, "STATUS", "CONFIG")

	for _, project := range projects {
		configStatus := "ok"
		if _, loadErr := deps.loadConfig()(project); loadErr != nil {
			if errors.Is(loadErr, os.ErrNotExist) {
				loadErr = fmt.Errorf("config file disappeared: %w", loadErr)
			}
			deps.logger().Warn("config problem for project", "project", project, "error", sanitizeForTerminal(loadErr.Error()))
			configStatus = "error"
		}
		status, statusErr := containerStatusStringErr(deps.Runner, containerNameForProject(project))
		if statusErr != nil {
			deps.logger().Warn("failed to query container for project", "project", project, "container", containerNameForProject(project), "error", sanitizeForTerminal(statusErr.Error()))
		}
		fmt.Fprintf(w, "%-*s  %-*s  %s\n", colWidth, project, statusColWidth, status, configStatus)
	}
	return nil
}

// containerStatusStringErr returns the display status and the first error
// encountered while querying the container state. It returns ("unknown", err)
// when an error occurs so that callers can render an explicit unknown row while
// also detecting the failure.
func containerStatusStringErr(runner container.Runner, containerName string) (string, error) {
	exists, err := container.Exists(runner, containerName)
	if err != nil {
		return "unknown", err
	}
	if !exists {
		return "absent", nil
	}
	running, err := container.IsRunning(runner, containerName)
	if err != nil {
		return "unknown", err
	}
	if !running {
		return "stopped", nil
	}
	return "running", nil
}

// newPullCmd returns the cobra.Command for the "pull" subcommand, which
// explicitly pulls the latest revetment image regardless of local state.
func newPullCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "pull",
		Short: "Pull the latest revetment container image",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPull(cmd, deps)
		},
	}
}

// runPull implements the "pull" subcommand: it resolves the image name, pulls
// it, and prints a confirmation message on success.
func runPull(cmd *cobra.Command, deps Deps) error {
	image := deps.resolveImage()
	deps.logger().Info("pulling image", "image", image)
	if err := container.PullImage(deps.Runner, image, cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
		return fmt.Errorf("pulling image: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Image pulled successfully: %s\n", image)
	return nil
}
