// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/rob-broadley/ai-airbase/marshal/internal/config"
	"github.com/rob-broadley/ai-airbase/marshal/internal/container"
)

// newCreateCmd returns the cobra.Command for the "create" subcommand, which
// saves the project mount configuration and creates the container. This is the
// entry point for setting up a new project; subsequent invocations of marshal
// read the saved mounts from config rather than accepting them as flags.
func newCreateCmd(deps Deps, projectFlag *string) *cobra.Command {
	var mountFlags []string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new container for the project",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCreate(cmd, deps, *projectFlag, mountFlags)
		},
	}
	cmd.Flags().StringArrayVarP(&mountFlags, "mount", "m", nil, "Directory to bind mount into /workspace/<basename> (repeatable)")
	return cmd
}

// runCreate implements the "create" subcommand: it validates the project, errors
// if the container already exists, saves the mount configuration, pulls the
// image if needed, and creates the container.
func runCreate(cmd *cobra.Command, deps Deps, projectFlag string, mountFlagValues []string) error {
	project, containerName, err := resolveContainer(deps, projectFlag)
	if err != nil {
		return err
	}

	exists, err := container.Exists(deps.Runner, containerName)
	if err != nil {
		return fmt.Errorf("checking container: %w", err)
	}
	if exists {
		return fmt.Errorf("project %s already has a container; use 'marshal recreate' to rebuild with existing configuration, or 'marshal remove' then 'marshal create' to reconfigure mounts", project)
	}

	cwd, err := deps.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	cfg, err := config.Load(project)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// resolveMountPaths updates cfg.Mounts when flags are provided; otherwise
	// cfg.Mounts stays as loaded (empty for a fresh project).
	if _, _, err = resolveMountPaths(cwd, mountFlagValues, cfg); err != nil {
		return err
	}
	// If no explicit mounts were configured, fall back to CWD and persist it.
	if len(cfg.Mounts) == 0 {
		cfg.Mounts = []string{cwd}
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

	if err := createContainerWithVolumes(deps.Runner, deps.logger(), p.containerName, p.image, p.mountSpecs, p.userConfig, p.workdir, p.cmd); err != nil {
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
	_, containerName, err := resolveContainer(deps, projectFlag)
	if err != nil {
		return err
	}

	exists, err := container.Exists(deps.Runner, containerName)
	if err != nil {
		return fmt.Errorf("checking container: %w", err)
	}
	if !exists {
		return fmt.Errorf("container %s does not exist", containerName)
	}

	running, err := container.IsRunning(deps.Runner, containerName)
	if err != nil {
		return fmt.Errorf("checking running state: %w", err)
	}
	if !running {
		fmt.Fprintf(cmd.OutOrStdout(), "container %s is already stopped\n", containerName)
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
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(cmd, deps, *projectFlag)
		},
	}
}

// runStatus implements the "status" subcommand: it resolves the project,
// queries the container state, and prints project name, container name,
// running status, image, and creation time.
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
	fmt.Fprintf(w, "Project:   %s\n", project)
	fmt.Fprintf(w, "Container: %s\n", containerName)

	if !status.Exists {
		fmt.Fprintf(w, "Status:    absent\n")
		fmt.Fprintf(w, "Image:     -\n")
		fmt.Fprintf(w, "Created:   -\n")
		return nil
	}

	statusStr := "stopped"
	if status.Running {
		statusStr = "running"
	}
	fmt.Fprintf(w, "Status:    %s\n", statusStr)
	fmt.Fprintf(w, "Image:     %s\n", status.Image)
	fmt.Fprintf(w, "Created:   %s\n", status.Created)
	return nil
}

// newRecreateCmd returns the cobra.Command for the "recreate" subcommand,
// which pulls the latest image, removes the existing container, and creates a
// fresh one while preserving the per-project Nix store volume.
func newRecreateCmd(deps Deps, projectFlag *string) *cobra.Command {
	return &cobra.Command{
		Use:   "recreate",
		Short: "Atomically replace the project container with a fresh one",
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

	if err := removeAndRecreateContainer(deps.Runner, deps.logger(), p.containerName, p.image, p.mountSpecs, p.userConfig, p.workdir, p.cmd); err != nil {
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
		return fmt.Errorf("container %s does not exist", containerName)
	}

	deps.logger().Info("removing container", "container", containerName)
	if err := container.Remove(deps.Runner, containerName); err != nil {
		return fmt.Errorf("removing container: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "container %s removed\n", containerName)

	// Best-effort cleanup — continue past individual failures.
	var errs []error
	if err := container.RemoveProjectVolumes(deps.Runner, containerName); err != nil {
		errs = append(errs, fmt.Errorf("removing project volumes: %w", err))
	}
	if err := config.Delete(project); err != nil {
		errs = append(errs, fmt.Errorf("removing project config: %w", err))
	}
	return errors.Join(errs...)
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
