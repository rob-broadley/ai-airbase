// SPDX-License-Identifier: AGPL-3.0-or-later
// Package cmd implements the marshal cobra command tree and wires up
// all CLI subcommands with their shared dependencies.
package cmd

import (
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/rob-broadley/ai-airbase/marshal/internal/config"
	"github.com/rob-broadley/ai-airbase/marshal/internal/container"
	"github.com/rob-broadley/ai-airbase/marshal/internal/hostinfo"
)

// ExecFunc replaces the current process with a new one (like syscall.Exec).
type ExecFunc func(argv []string) error

// Deps holds injectable dependencies so commands can be tested without real
// Podman or a real working directory.
type Deps struct {
	Runner                container.Runner
	ExecFn                ExecFunc
	Getwd                 func() (string, error)
	Getuid                func() int
	Getgid                func() int
	EnsureSharedDataDir   func(subdir string) (string, error)
	EnsureSharedConfigDir func(subdir string) (string, error)
	SaveConfig            func(project string, cfg *config.Config) error // defaults to config.Save
	DeleteConfig          func(project string) error                     // defaults to config.Delete
	LoadConfig            func(project string) (*config.Config, error)   // defaults to config.Load
	// ListProjects returns all registered project names and any per-file warnings.
	// Defaults to config.ListProjects.
	ListProjects func() ([]string, []string, error)
	// LookupGitConfig reads a git configuration key (e.g. "user.name") from the
	// host and returns its trimmed value, or an empty string if unset or on error.
	// Defaults to hostinfo.LookupHostGitConfig.
	LookupGitConfig func(key string) string
	// ResolveImage returns the container image to use. Defaults to defaultImage,
	// which reads MARSHAL_IMAGE from the environment and falls back to the
	// published revetment image. Override in tests to fix the image name without
	// touching the environment.
	ResolveImage func() string
	// MkdirAll creates a directory named path, along with any necessary parents.
	// Defaults to os.MkdirAll. Override in tests to avoid touching the real
	// filesystem when CWD paths are fake.
	MkdirAll func(path string, perm fs.FileMode) error
	// Logger receives progress messages during slow operations (image pulls,
	// container creation, volume provisioning). When nil, a discard logger is
	// used so callers that do not inject a logger are not affected.
	Logger *slog.Logger
	// IsPortBound checks if a TCP port is already bound on host loopback.
	// Defaults to standard net.Listen-based check.
	IsPortBound func(port int) bool
	// SharedDataPath returns the resolved host path for a subdirectory
	// under XDG_DATA_HOME/marshal/ without creating the directory.
	// Defaults to hostinfo.SharedDataPath.
	SharedDataPath func(subdir string) string
	// XDGDataHome returns the XDG data home directory path.
	// Defaults to hostinfo.XDGDataHome.
	XDGDataHome func() string
}

// sharedDataPath returns the injected SharedDataPath or the
// real hostinfo.SharedDataPath.
func (d Deps) sharedDataPath() func(string) string {
	if d.SharedDataPath != nil {
		return d.SharedDataPath
	}
	return hostinfo.SharedDataPath
}

// xdgDataHome returns the injected XDGDataHome or the real one.
func (d Deps) xdgDataHome() func() string {
	if d.XDGDataHome != nil {
		return d.XDGDataHome
	}
	return hostinfo.XDGDataHome
}

// deleteConfig returns the effective config-delete function: the injected one or config.Delete.
func (d Deps) deleteConfig() func(string) error {
	if d.DeleteConfig != nil {
		return d.DeleteConfig
	}
	return config.Delete
}

// saveConfig returns the effective config-save function: the injected one or config.Save.
func (d Deps) saveConfig() func(string, *config.Config) error {
	if d.SaveConfig != nil {
		return d.SaveConfig
	}
	return config.Save
}

// loadConfig returns the effective config-load function: the injected one or config.Load.
func (d Deps) loadConfig() func(string) (*config.Config, error) {
	if d.LoadConfig != nil {
		return d.LoadConfig
	}
	return config.Load
}

// listProjects returns the effective projects lister: the injected one or config.ListProjects.
func (d Deps) listProjects() func() ([]string, []string, error) {
	if d.ListProjects != nil {
		return d.ListProjects
	}
	return config.ListProjects
}

// ensureSharedConfigDirFn returns the injected EnsureSharedConfigDir or the
// real hostinfo.EnsureSharedConfigDir. Tests that set XDG_CONFIG_HOME to a temp
// directory get safe isolation without needing to inject this function.
func (d Deps) ensureSharedConfigDirFn() func(string) (string, error) {
	if d.EnsureSharedConfigDir != nil {
		return d.EnsureSharedConfigDir
	}
	return hostinfo.EnsureSharedConfigDir
}

// lookupGitConfigFn returns the injected LookupGitConfig or a no-op that
// returns empty string. Production code always sets LookupGitConfig explicitly
// in Execute(); the no-op fallback keeps tests hermetic by avoiding real git
// subprocess calls when the dep is not injected.
func (d Deps) lookupGitConfigFn() func(string) string {
	if d.LookupGitConfig != nil {
		return d.LookupGitConfig
	}
	return func(string) string { return "" }
}

// isPortBound returns the effective port-binding check: the injected one or defaultIsPortBound.
func (d Deps) isPortBound() func(int) bool {
	if d.IsPortBound != nil {
		return d.IsPortBound
	}
	return defaultIsPortBound
}

// logger returns the injected Logger or a discard logger when none is set.
// All code should use this accessor rather than accessing Logger directly.
func (d Deps) logger() *slog.Logger {
	if d.Logger != nil {
		return d.Logger
	}
	return slog.New(slog.DiscardHandler)
}

// ensureSharedDataDir returns the injected EnsureSharedDataDir or the
// real hostinfo.EnsureSharedDataDir. Tests that set XDG_DATA_HOME to a temp
// directory get safe isolation without needing to inject this function.
func (d Deps) ensureSharedDataDir() func(string) (string, error) {
	if d.EnsureSharedDataDir != nil {
		return d.EnsureSharedDataDir
	}
	return hostinfo.EnsureSharedDataDir
}

// package-level defaultImage free function (which reads MARSHAL_IMAGE).
func (d Deps) resolveImage() string {
	if d.ResolveImage != nil {
		return d.ResolveImage()
	}
	return defaultImage()
}

// mkdirAll returns the effective MkdirAll function: the injected one or os.MkdirAll.
func (d Deps) mkdirAll() func(string, fs.FileMode) error {
	if d.MkdirAll != nil {
		return d.MkdirAll
	}
	return os.MkdirAll
}

// NewRootCmd builds the root cobra.Command tree with the supplied dependencies.
func NewRootCmd(deps Deps) *cobra.Command {
	var projectFlag string

	root := &cobra.Command{
		Use:   "marshal",
		Short: "A sandbox for running OpenCode — one container per project, managed for you.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return ensureContainerAndStart(cmd, deps, projectFlag)
		},
	}

	root.PersistentFlags().StringVarP(&projectFlag, "project", "p", "", "Project name (default: current directory name)")

	root.AddCommand(
		newCreateCmd(deps, &projectFlag),
		newListCmd(deps),
		newStopCmd(deps, &projectFlag),
		newStatusCmd(deps, &projectFlag),
		newRecreateCmd(deps, &projectFlag),
		newRemoveCmd(deps, &projectFlag),
		newShellCmd(deps, &projectFlag),
		newPullCmd(deps),
	)

	return root
}

// Execute is the real entry point used by main.go. version is embedded at build
// time via -ldflags and exposed through cobra's --version flag and version subcommand.
func Execute(version string) {
	deps := Deps{
		Runner:                container.PodmanRunner{},
		ExecFn:                realExec,
		Getwd:                 os.Getwd,
		Getuid:                os.Getuid,
		Getgid:                os.Getgid,
		EnsureSharedDataDir:   hostinfo.EnsureSharedDataDir,
		EnsureSharedConfigDir: hostinfo.EnsureSharedConfigDir,
		SharedDataPath:        hostinfo.SharedDataPath,
		SaveConfig:            config.Save,
		LoadConfig:            config.Load,
		LookupGitConfig:       hostinfo.LookupHostGitConfig,
		ResolveImage:          defaultImage,
		Logger:                NewCLILogger(os.Stderr),
	}
	rootCmd := NewRootCmd(deps)
	rootCmd.Version = version
	rootCmd.SilenceErrors = true
	// SilenceUsage prevents cobra from printing the full help text on every runtime error.
	rootCmd.SilenceUsage = true
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
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

func defaultIsPortBound(port int) bool {
	address := fmt.Sprintf("127.0.0.1:%d", port)
	l, err := net.Listen("tcp", address)
	if err != nil {
		return true
	}
	_ = l.Close()
	return false
}
