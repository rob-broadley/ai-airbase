// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd_test

import (
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
