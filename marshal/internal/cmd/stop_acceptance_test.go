// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd_test

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/rob-broadley/ai-airbase/marshal/internal/cmd"
)

// TestStop_RunningContainer verifies that stop calls podman stop and reports
// the container as stopped.
func TestStop_RunningContainer(t *testing.T) {
	// Given a running container
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: true, running: true}
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
	root.SetErr(buf)

	// When the stop subcommand is executed
	root.SetArgs([]string{"--project", "myapp", "stop"})
	assertNoError(t, root.Execute())

	// Then podman stop is called and the output mentions stopped
	if !runner.calledSubcommand("stop") {
		t.Error("expected 'podman stop' to be called")
	}
	assertContains(t, buf.String(), "stopped")
}

// TestStop_AlreadyStopped verifies that stop is a no-op and reports already
// stopped when the container is not running.
func TestStop_AlreadyStopped(t *testing.T) {
	// Given a stopped container
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: true, running: false}
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

	// When the stop subcommand is executed
	root.SetArgs([]string{"--project", "myapp", "stop"})
	assertNoError(t, root.Execute())

	// Then podman stop is NOT called and the output uses the project-facing message.
	if runner.calledSubcommand("stop") {
		t.Error("expected 'podman stop' NOT to be called for already-stopped container")
	}
	assertContains(t, buf.String(), "project myapp container is already stopped")
}

// TestStop_AbsentContainer verifies that stop returns an error when the
// container does not exist.
func TestStop_AbsentContainer(t *testing.T) {
	// Given no container exists
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: false, running: false}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
	}

	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "myapp", "stop"})
	// cobra prints errors by default; suppress to keep test output clean
	root.SetErr(&bytes.Buffer{})

	// When the stop subcommand is executed
	err := root.Execute()

	// Then an error is returned with the project-facing message.
	assertError(t, err)
	assertContains(t, err.Error(), "project myapp has no container")
}

// TestStop_IsRunningCheckFails verifies that an error from the IsRunning check
// is propagated back to the caller.
func TestStop_IsRunningCheckFails(t *testing.T) {
	// Given an existing container whose IsRunning check (ps) returns an error
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{
		exists:    true,
		running:   false,
		runErrors: map[string]error{"ps": fmt.Errorf("ps failed")},
	}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
	}

	// When the stop subcommand is executed
	root := cmd.NewRootCmd(deps)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "stop"})

	// Then an error is returned
	assertError(t, root.Execute())
}

// TestStop_InvalidProjectName verifies that stop returns an error when the
// --project flag contains an invalid project name (e.g. path traversal).
func TestStop_InvalidProjectName(t *testing.T) {
	// Given a runner that would succeed if reached
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

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

	// When the stop subcommand is executed with an invalid project name
	root.SetArgs([]string{"--project", "../evil", "stop"})
	err := root.Execute()

	// Then an error is returned containing "invalid project"
	assertError(t, err)
	assertContains(t, err.Error(), "invalid project")
}

// TestStop_StopCommandFails verifies that an error from podman stop is
// propagated back to the caller with a message mentioning "stopping container".
func TestStop_StopCommandFails(t *testing.T) {
	// Given a running container and a runner that fails on "stop"
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{
		exists:    true,
		running:   true,
		runErrors: map[string]error{"stop": errors.New("timeout stopping container")},
	}
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
	root.SetArgs([]string{"--project", "myapp", "stop"})

	// When the stop subcommand is executed
	err := root.Execute()

	// Then an error is returned containing "stopping container"
	assertError(t, err)
	assertContains(t, err.Error(), "stopping container")
}
