// SPDX-License-Identifier: AGPL-3.0-or-later
// Package cmd implements the marshal cobra command tree and wires up
// all CLI subcommands with their shared dependencies.
package cmd

import (
	"os"
	"os/exec"
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
	// ResolveImage returns the container image to use. Defaults to defaultImage,
	// which reads MARSHAL_IMAGE from the environment and falls back to the
	// published revetment image. Override in tests to fix the image name without
	// touching the environment.
	ResolveImage func() string
}

// saveConfig returns the effective config-save function: the injected one or config.Save.
func (d Deps) saveConfig() func(string, *config.Config) error {
	if d.SaveConfig != nil {
		return d.SaveConfig
	}
	return config.Save
}

// resolveImage returns the effective image resolver: the injected one or the
// package-level defaultImage free function (which reads MARSHAL_IMAGE).
func (d Deps) resolveImage() string {
	if d.ResolveImage != nil {
		return d.ResolveImage()
	}
	return defaultImage()
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
		ResolveImage:        defaultImage,
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

// defaultImage returns the container image to use: the MARSHAL_IMAGE environment
// variable when set, otherwise the default published image.
func defaultImage() string {
	if img := os.Getenv("MARSHAL_IMAGE"); img != "" {
		return img
	}
	return "ghcr.io/rob-broadley/ai-airbase/revetment:latest"
}
