// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd_test

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/rob-broadley/ai-airbase/marshal/internal/cmd"
	"github.com/rob-broadley/ai-airbase/marshal/internal/config"
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

	// Then an error is returned
	assertError(t, err)
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
