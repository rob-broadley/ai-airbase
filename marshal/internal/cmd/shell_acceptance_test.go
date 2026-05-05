// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/rob-broadley/ai-airbase/marshal/internal/cmd"
)

// TestShell_ExecsBashInRunningContainer verifies that marshal shell execs
// /bin/bash into an already-running container without calling podman start.
func TestShell_ExecsBashInRunningContainer(t *testing.T) {
	// Given the container exists and is running
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: true, running: true}
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

	// When the shell subcommand is executed
	root.SetArgs([]string{"--project", "myapp", "shell"})
	assertNoError(t, root.Execute())

	// Then podman start is NOT called via runner
	if runner.calledSubcommand("start") {
		t.Error("expected 'podman start' NOT to be called via runner for running container")
	}
	// And ExecFn is called with podman exec -it marshal-myapp /bin/bash
	if !fe.called {
		t.Fatal("expected exec to be called")
	}
	if !sliceContains(fe.argv, "exec") {
		t.Errorf("expected 'exec' in exec argv, got %v", fe.argv)
	}
	if !sliceContains(fe.argv, "marshal-myapp") {
		t.Errorf("expected 'marshal-myapp' in exec argv, got %v", fe.argv)
	}
	if !sliceContains(fe.argv, "/bin/bash") {
		t.Errorf("expected '/bin/bash' in exec argv, got %v", fe.argv)
	}
}

// TestShell_CreatesAndStartsWhenAbsent verifies that marshal shell creates and
// starts a managed container when it does not yet exist, then execs /bin/bash.
func TestShell_CreatesAndStartsWhenAbsent(t *testing.T) {
	// Given the container does not exist
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: false}
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

	// When the shell subcommand is executed
	root.SetArgs([]string{"--project", "myapp", "shell"})
	assertNoError(t, root.Execute())

	// Then podman create is called via runner
	if !runner.calledSubcommand("create") {
		t.Error("expected 'podman create' to be called via runner")
	}
	// And podman start is called via runner
	if !runner.calledSubcommand("start") {
		t.Error("expected 'podman start' to be called via runner")
	}
	// And ExecFn is called with podman exec -it marshal-myapp /bin/bash
	if !fe.called {
		t.Fatal("expected exec to be called")
	}
	if !sliceContains(fe.argv, "exec") {
		t.Errorf("expected 'exec' in exec argv, got %v", fe.argv)
	}
	if !sliceContains(fe.argv, "/bin/bash") {
		t.Errorf("expected '/bin/bash' in exec argv, got %v", fe.argv)
	}
}

// TestShell_StartsAndExecsWhenStopped verifies that marshal shell starts the
// managed container and execs /bin/bash when the container exists but is stopped.
func TestShell_StartsAndExecsWhenStopped(t *testing.T) {
	// Given the container exists but is stopped
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: true, running: false}
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

	// When the shell subcommand is executed
	root.SetArgs([]string{"--project", "myapp", "shell"})
	assertNoError(t, root.Execute())

	// Then podman create is NOT called
	if runner.calledSubcommand("create") {
		t.Error("expected 'podman create' NOT to be called for stopped container")
	}
	// And podman start IS called via runner
	if !runner.calledSubcommand("start") {
		t.Error("expected 'podman start' to be called via runner")
	}
	// And ExecFn is called with podman exec -it marshal-myapp /bin/bash
	if !fe.called {
		t.Fatal("expected exec to be called")
	}
	if !sliceContains(fe.argv, "exec") {
		t.Errorf("expected 'exec' in exec argv, got %v", fe.argv)
	}
	if !sliceContains(fe.argv, "/bin/bash") {
		t.Errorf("expected '/bin/bash' in exec argv, got %v", fe.argv)
	}
}

// TestShell_StartFails verifies that an error from podman start is propagated
// back to the caller with a message mentioning "starting container".
func TestShell_StartFails(t *testing.T) {
	// Given a stopped container and a runner that fails on "start"
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{
		exists:    true,
		running:   false,
		runErrors: map[string]error{"start": errors.New("failed to start")},
	}
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
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "shell"})

	// When the shell subcommand is executed
	err := root.Execute()

	// Then an error is returned containing "starting container"
	assertError(t, err)
	assertContains(t, err.Error(), "starting container")
}

// TestShell_ExecFails verifies that an error from the ExecFn (podman exec) is
// propagated back to the caller.
func TestShell_ExecFails(t *testing.T) {
	// Given a running container and an ExecFn that returns an error
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: true, running: true}
	fe := &fakeExec{err: errors.New("exec failed")}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              fe.exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
	}

	root := cmd.NewRootCmd(deps)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "shell"})

	// When the shell subcommand is executed
	err := root.Execute()

	// Then an error is returned
	assertError(t, err)
}
