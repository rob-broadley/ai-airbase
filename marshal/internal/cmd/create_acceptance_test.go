// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rob-broadley/ai-airbase/marshal/internal/cmd"
	"github.com/rob-broadley/ai-airbase/marshal/internal/config"
	"github.com/rob-broadley/ai-airbase/marshal/internal/customisations"
)

// TestCreate_CreatesContainerWithCWD verifies that marshal create creates the
// container mounting the current working directory when no --mount flags are
// provided.
func TestCreate_CreatesContainerWithCWD(t *testing.T) {
	// Given no container exists and no mounts configured
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: false, imageExistsResult: true}
	fe := &fakeExec{}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              fe.exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
	}

	root := cmd.NewRootCmd(deps)
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "create"})

	// When the create subcommand is executed
	assertNoError(t, root.Execute())

	// Then podman create is called
	if !runner.calledSubcommand("create") {
		t.Error("expected 'podman create' to be called")
	}

	// And CWD is mounted under /workspace/myapp
	if !runner.createArgsContain("/projects/myapp:/workspace/myapp:Z") {
		t.Errorf("expected '/projects/myapp:/workspace/myapp:Z' in create args, got %v", runner.createArgs())
	}

	// And config was saved with CWD as the mount
	cfg, err := config.Load("myapp")
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if len(cfg.Mounts) != 1 || cfg.Mounts[0] != "/projects/myapp" {
		t.Errorf("expected config.Mounts = ['/projects/myapp'], got %v", cfg.Mounts)
	}
}

// TestCreate_MountFlagAbsolute verifies that an absolute --mount path is mounted
// under /workspace/<basename> and CWD is not mounted.
func TestCreate_MountFlagAbsolute(t *testing.T) {
	// Given --mount /abs/shared-lib
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: false, imageExistsResult: true}
	fe := &fakeExec{}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              fe.exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
	}

	root := cmd.NewRootCmd(deps)
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "create", "--mount", "/abs/shared-lib"})

	// When the create subcommand is executed
	assertNoError(t, root.Execute())

	// Then the path is mounted under /workspace/shared-lib and CWD is not mounted
	if !runner.createArgsContain("/abs/shared-lib:/workspace/shared-lib:Z") {
		t.Errorf("expected '/abs/shared-lib:/workspace/shared-lib:Z' in create args, got %v", runner.createArgs())
	}
	if runner.createArgsContain("/projects/myapp:/workspace/myapp:Z") {
		t.Error("CWD should NOT be mounted when --mount flags are provided")
	}
}

// TestCreate_MultipleMountFlags verifies that multiple --mount flags each get
// mounted under /workspace/<basename>.
func TestCreate_MultipleMountFlags(t *testing.T) {
	// Given two --mount flags
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: false, imageExistsResult: true}
	fe := &fakeExec{}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              fe.exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
	}

	root := cmd.NewRootCmd(deps)
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "create", "--mount", "/abs/lib1", "--mount", "/abs/lib2"})

	// When the create subcommand is executed
	assertNoError(t, root.Execute())

	// Then both paths are mounted
	if !runner.createArgsContain("/abs/lib1:/workspace/lib1:Z") {
		t.Errorf("expected '/abs/lib1:/workspace/lib1:Z' in create args, got %v", runner.createArgs())
	}
	if !runner.createArgsContain("/abs/lib2:/workspace/lib2:Z") {
		t.Errorf("expected '/abs/lib2:/workspace/lib2:Z' in create args, got %v", runner.createArgs())
	}
}

// TestCreate_PersistsMountsToConfig verifies that --mount paths are saved to
// the project config for future use by other commands.
func TestCreate_PersistsMountsToConfig(t *testing.T) {
	// Given --mount /abs/shared-lib with a writable config dir
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	runner := &fakeRunner{exists: false, imageExistsResult: true}
	fe := &fakeExec{}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              fe.exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
	}

	root := cmd.NewRootCmd(deps)
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "create", "--mount", "/abs/shared-lib"})

	// When the create subcommand is executed
	assertNoError(t, root.Execute())

	// Then the mount path is persisted to config
	cfg, err := config.Load("myapp")
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if len(cfg.Mounts) != 1 || cfg.Mounts[0] != "/abs/shared-lib" {
		t.Errorf("expected config.Mounts = ['/abs/shared-lib'], got %v", cfg.Mounts)
	}
}

// TestCreate_RelativeMountResolved verifies that a relative --mount path is
// resolved against the current working directory.
func TestCreate_RelativeMountResolved(t *testing.T) {
	// Given --mount reldir with CWD set
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: false, imageExistsResult: true}
	fe := &fakeExec{}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              fe.exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
	}

	root := cmd.NewRootCmd(deps)
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "create", "--mount", "reldir"})

	// When the create subcommand is executed
	assertNoError(t, root.Execute())

	// Then the relative path is resolved to an absolute path
	if !runner.createArgsContain("/projects/myapp/reldir:/workspace/reldir:Z") {
		t.Errorf("expected '/projects/myapp/reldir:/workspace/reldir:Z' in create args, got %v", runner.createArgs())
	}
}

// TestCreate_ErrorsWhenContainerExists verifies that create returns an
// actionable error when a container already exists for the project.
func TestCreate_ErrorsWhenContainerExists(t *testing.T) {
	// Given the container already exists
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: true}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
	}

	root := cmd.NewRootCmd(deps)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "create"})

	// When the create subcommand is executed
	err := root.Execute()

	// Then an error is returned mentioning recreate and remove as alternatives
	assertError(t, err)
	assertContains(t, err.Error(), "already")
	assertContains(t, err.Error(), "recreate")
	assertContains(t, err.Error(), "remove")
}

// TestCreate_MountSaveFails verifies that a config save failure is propagated
// with a message mentioning "saving config".
func TestCreate_MountSaveFails(t *testing.T) {
	// Given a SaveConfig function that returns an error
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: false, imageExistsResult: true}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
		SaveConfig:          func(string, *config.Config) error { return fmt.Errorf("disk full") },
	}

	root := cmd.NewRootCmd(deps)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "create", "--mount", "/some/path"})

	// When the create subcommand is executed
	err := root.Execute()

	// Then an error is returned containing "saving config"
	assertError(t, err)
	assertContains(t, err.Error(), "saving config")
}

// TestCreate_PrintsSuccessMessage verifies that a successful create prints
// the container name to stdout.
func TestCreate_PrintsSuccessMessage(t *testing.T) {
	// Given no container exists
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: false, imageExistsResult: true}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
	}

	buf := &bytes.Buffer{}
	root := cmd.NewRootCmd(deps)
	root.SetOut(buf)
	root.SetArgs([]string{"--project", "myapp", "create"})

	// When the create subcommand is executed
	assertNoError(t, root.Execute())

	// Then a success message is printed
	assertContains(t, buf.String(), "marshal-myapp")
	assertContains(t, buf.String(), "created")
}

// TestCreate_InvalidProjectName verifies that create returns an error for
// project names that fail validation.
func TestCreate_InvalidProjectName(t *testing.T) {
	// Given a project name that contains spaces (which fails validation)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: false}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
	}

	root := cmd.NewRootCmd(deps)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "invalid name with spaces", "create"})

	// When create is executed
	err := root.Execute()

	// Then an error is returned mentioning the invalid project name
	assertError(t, err)
	assertContains(t, err.Error(), "invalid project")
}

// TestCreate_MountWithColonErrors verifies that create returns an error when
// a mount path contains ':', which would be misinterpreted as a mount-spec separator.
func TestCreate_MountWithColonErrors(t *testing.T) {
	// Given a --mount path containing a colon, which would be misinterpreted as a mount-spec separator
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: false}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
	}

	root := cmd.NewRootCmd(deps)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "create", "--mount", "/bad:path"})

	// When create is executed
	err := root.Execute()

	// Then an error is returned containing the offending character
	assertError(t, err)
	assertContains(t, err.Error(), ":")
}

// TestCreate_BasenameConflictErrors verifies that create returns an error when
// two mount paths share the same basename, which would cause a collision under
// /workspace/ in the container.
func TestCreate_BasenameConflictErrors(t *testing.T) {
	// Given two --mount paths that share the same basename, which would collide under /workspace/
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: false}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
	}

	root := cmd.NewRootCmd(deps)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "create", "--mount", "/a/foo", "--mount", "/b/foo"})

	// When create is executed
	err := root.Execute()

	// Then an error containing "conflict" is returned
	assertError(t, err)
	assertContains(t, err.Error(), "conflict")
}

// TestNonCreateCmds_DoNotAcceptMountFlag verifies that --mount is unknown on
// all commands except create, so users get a clear error rather than silent
// flag mishandling.
func TestNonCreateCmds_DoNotAcceptMountFlag(t *testing.T) {
	// Given --mount passed to a command that is not "create"
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	for _, args := range [][]string{
		{"--mount", "/some/path"},
		{"shell", "--mount", "/some/path"},
		{"recreate", "--mount", "/some/path"},
	} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			runner := &fakeRunner{exists: true, running: true}
			deps := cmd.Deps{
				Runner:              runner,
				ExecFn:              (&fakeExec{}).exec,
				Getwd:               func() (string, error) { return "/projects/myapp", nil },
				Getuid:              stubGetuid,
				Getgid:              stubGetgid,
				EnsureSharedDataDir: stubEnsureSharedDataDir(t),
			}

			root := cmd.NewRootCmd(deps)
			root.SetErr(&bytes.Buffer{})
			root.SetArgs(append([]string{"--project", "myapp"}, args...))

			// When the command is executed
			err := root.Execute()

			// Then an error is returned (--mount is not a known flag)
			assertError(t, err)
		})
	}
}

// TestCreate_NestedMountErrors verifies that create returns an error when one
// mount path is nested inside another, which would defeat the --mask security
// guarantee by exposing the inner subtree unmasked at a second container path.
func TestCreate_NestedMountErrors(t *testing.T) {
	// Given two --mount paths where the inner is strictly nested inside the outer
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: false}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
	}

	root := cmd.NewRootCmd(deps)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{
		"--project", "myapp", "create",
		"--mount", "/home/user/project",
		"--mount", "/home/user/project/secrets",
	})

	// When create is executed
	err := root.Execute()

	// Then an error containing "nested" is returned
	assertError(t, err)
	assertContains(t, err.Error(), "nested")
}

// TestCreate_LoadConfigErrorPropagated verifies that runCreate honours the
// injected LoadConfig dependency: when the stub returns an error, runCreate
// must propagate it rather than falling through to config.Load directly.
func TestCreate_LoadConfigErrorPropagated(t *testing.T) {
	// Given a LoadConfig stub that always returns an error
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: false, imageExistsResult: true}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
		LoadConfig: func(project string) (*config.Config, error) {
			return nil, fmt.Errorf("injected load failure")
		},
	}

	root := cmd.NewRootCmd(deps)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "create"})

	// When the create subcommand is executed
	err := root.Execute()

	// Then the injected LoadConfig error is propagated
	assertError(t, err)
	assertContains(t, err.Error(), "injected load failure")
}

// TestCreate_ImagePullFails verifies that create propagates an error when the
// image is absent and cannot be pulled (registry unreachable).
func TestCreate_ImagePullFails(t *testing.T) {
	// Given the image is absent and the registry is unreachable
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{
		exists:            false,
		imageExistsResult: false,
		runErrors:         map[string]error{"pull": errors.New("registry unreachable")},
	}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
		ResolveImage:        func() string { return "ghcr.io/example/revetment:latest" },
	}

	root := cmd.NewRootCmd(deps)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "create"})

	// When create is executed
	err := root.Execute()

	// Then the pull error is propagated
	assertError(t, err)
}

// TestCreate_WithCustomPort verifies that custom port is validated, saved to config,
// and correctly used in podman create command.
func TestCreate_WithCustomPort(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: false, imageExistsResult: true}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
	}

	root := cmd.NewRootCmd(deps)
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "create", "--port", "5000"})

	// When create is executed with custom port
	assertNoError(t, root.Execute())

	// Then config was saved with the custom port
	cfg, err := config.Load("myapp")
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if cfg.Port != 5000 {
		t.Errorf("expected config.Port = 5000, got %d", cfg.Port)
	}

	// And podman create maps port 5000 to container 4096
	if !runner.createArgsContain("127.0.0.1:5000:4096") {
		t.Errorf("expected '127.0.0.1:5000:4096' in create args, got %v", runner.createArgs())
	}
}

// TestCreate_InvalidPortRange verifies that create rejects invalid ports (<1024 or >65535).
func TestCreate_InvalidPortRange(t *testing.T) {
	for _, invalidPort := range []string{"80", "1023", "65536"} {
		t.Run("port_"+invalidPort, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())

			runner := &fakeRunner{exists: false, imageExistsResult: true}
			deps := cmd.Deps{
				Runner:              runner,
				ExecFn:              (&fakeExec{}).exec,
				Getwd:               func() (string, error) { return "/projects/myapp", nil },
				Getuid:              stubGetuid,
				Getgid:              stubGetgid,
				EnsureSharedDataDir: stubEnsureSharedDataDir(t),
			}

			root := cmd.NewRootCmd(deps)
			root.SetOut(&bytes.Buffer{})
			root.SetErr(&bytes.Buffer{})
			root.SetArgs([]string{"--project", "myapp", "create", "--port", invalidPort})

			// When create is executed with invalid port
			err := root.Execute()

			// Then an error is returned
			assertError(t, err)
			assertContains(t, err.Error(), "must be in range 1024-65535")
		})
	}
}

// TestCreate_PortConflictWithOtherProject verifies that create rejects ports already taken by other projects.
func TestCreate_PortConflictWithOtherProject(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	// Pre-register another project config with port 5000
	otherCfg := &config.Config{
		Mounts: []string{},
		Masks:  []string{},
		Port:   5000,
	}
	if err := config.Save("otherproject", otherCfg); err != nil {
		t.Fatalf("saving other config: %v", err)
	}

	runner := &fakeRunner{exists: false, imageExistsResult: true}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
	}

	root := cmd.NewRootCmd(deps)
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "create", "--port", "5000"})

	// When create is executed with a port already taken by otherproject
	err := root.Execute()

	// Then an error is returned
	assertError(t, err)
	assertContains(t, err.Error(), `port 5000 is already configured for project "otherproject"`)
}

// TestCreate_PortConflictWithHost verifies that create rejects ports already bound on the host.
func TestCreate_PortConflictWithHost(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: false, imageExistsResult: true}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
		IsPortBound:         func(port int) bool { return port == 5000 },
	}

	root := cmd.NewRootCmd(deps)
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "create", "--port", "5000"})

	// When create is executed with a port bound on host
	err := root.Execute()

	// Then an error is returned
	assertError(t, err)
	assertContains(t, err.Error(), "port 5000 is already in use on the host")
}

// TestCreate_AutoPortAllocation verifies that when no port is specified,
// create auto-allocates the first free port (starting at 4096), persists it, and maps it.
func TestCreate_AutoPortAllocation(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: false, imageExistsResult: true}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
		// Mock 4096 as bound, but 4097 as free
		IsPortBound: func(port int) bool { return port == 4096 },
	}

	root := cmd.NewRootCmd(deps)
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "create"})

	// When create is executed with no explicit port
	assertNoError(t, root.Execute())

	// Then config was saved with the auto-allocated port 4097
	cfg, err := config.Load("myapp")
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if cfg.Port != 4097 {
		t.Errorf("expected config.Port = 4097, got %d", cfg.Port)
	}

	// And podman create maps port 4097
	if !runner.createArgsContain("127.0.0.1:4097:4096") {
		t.Errorf("expected '127.0.0.1:4097:4096' in create args, got %v", runner.createArgs())
	}
}

// TestCreate_NoPortFlag_SavesConfigExactlyOnce verifies that when `marshal
// create` is run without `--port`, the project config is persisted exactly
// once. The first save must already contain the auto-allocated port — a
// separate, follow-up save (as previously triggered by ensurePortIsConfigured
// inside resolveContainerParams) would briefly leave the on-disk config with
// Port: 0 and waste a file write on every fresh create.
func TestCreate_NoPortFlag_SavesConfigExactlyOnce(t *testing.T) {
	// Given a SaveConfig spy that counts invocations and records the last saved config
	var saveCalls int
	var lastSavedProject string
	var lastSavedCfg *config.Config

	runner := &fakeRunner{exists: false, imageExistsResult: true}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
		SaveConfig: func(project string, cfg *config.Config) error {
			saveCalls++
			lastSavedProject = project
			lastSavedCfg = cfg
			// Also persist to disk so the second loadConfig inside
			// resolveContainerParams sees the saved state — otherwise
			// the spy would mask a real double-save by reading back an
			// empty config.
			return config.Save(project, cfg)
		},
	}

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	root := cmd.NewRootCmd(deps)
	root.SetOut(&bytes.Buffer{})
	// When `marshal create` is executed without --port
	root.SetArgs([]string{"--project", "myapp", "create"})

	assertNoError(t, root.Execute())

	// Then SaveConfig is called exactly once (not twice)
	if saveCalls != 1 {
		t.Errorf("expected SaveConfig to be called exactly once, got %d calls", saveCalls)
	}

	// And the last saved config has a non-zero port (auto-allocated before the save)
	if lastSavedCfg == nil {
		t.Fatal("expected SaveConfig to have been called with a non-nil config")
	}
	if lastSavedCfg.Port == 0 {
		t.Errorf("expected last saved config to have a non-zero Port, got %d", lastSavedCfg.Port)
	}
	if lastSavedProject != "myapp" {
		t.Errorf("expected saved project name to be %q, got %q", "myapp", lastSavedProject)
	}
}

// TestCreate_PerProjectDirsExist verifies that marshal create creates the
// per-project host directories for each entry in MountDirs under
// XDG_DATA_HOME/marshal/projects/<project>/opencode/.
func TestCreate_PerProjectDirsExist(t *testing.T) {
	// Given XDG_DATA_HOME is set to a fresh temp directory
	xdgDataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgDataHome)

	// And EnsureSharedDataDir mirrors the real hostinfo behaviour by
	// prepending "marshal/" to the subdir, so provisionProjectDir creates
	// per-project directories at the expected XDG_DATA_HOME path.
	// Directories are created with 0o755 so hardenProjectDir must correct
	// them to 0o700 — verifying the production code's permission enforcement.
	cf := newCredFakes(t)
	cf.dataBase = xdgDataHome
	cf.dataDirFn = func(subdir string) (string, error) {
		cf.calls = append(cf.calls, "data:"+subdir)
		p := filepath.Join(xdgDataHome, "marshal", subdir)
		if err := os.MkdirAll(p, 0o755); err != nil {
			return "", err
		}
		return p, nil
	}
	runner := &fakeRunner{exists: false, imageExistsResult: true}
	deps := newCredentialTestDeps(cf, runner)
	deps.XDGDataHome = func() string { return xdgDataHome }

	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "myapp", "create"})

	// When the user runs `marshal create`
	assertNoError(t, root.Execute())

	// Then projects/myapp directories exist under the data directory
	wantBase := filepath.Join(xdgDataHome, "marshal", "projects", "myapp")
	for _, entry := range customisations.MountDirs() {
		p := filepath.Join(wantBase, entry.HostSubdir)
		info, err := os.Stat(p)
		if err != nil {
			t.Fatalf("expected %s to exist: %v", p, err)
		}
		if !info.IsDir() {
			t.Errorf("expected %s to be a directory", p)
		}
		// And all directories have 0o700 permissions
		if perm := info.Mode().Perm(); perm != 0o700 {
			t.Errorf("expected %s to have permissions 0o700, got %04o", p, perm)
		}
	}
}
