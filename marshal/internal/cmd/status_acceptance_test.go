// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/rob-broadley/ai-airbase/marshal/internal/cmd"
)

// TestStatus_Running verifies that status reports project, container name,
// running state, image, and creation date for a running container.
func TestStatus_Running(t *testing.T) {
	// Given a running container with image and creation metadata
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: true, running: true, image: "ghcr.io/rob-broadley/ai-airbase/revetment:latest", created: "2024-06-01"}
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

	// When the status subcommand is executed
	root.SetArgs([]string{"--project", "myapp", "status"})
	assertNoError(t, root.Execute())

	// Then the output contains project, container, state, image and created date
	out := buf.String()
	assertContains(t, out, "myapp")
	assertContains(t, out, "marshal-myapp")
	assertContains(t, out, "running")
	assertContains(t, out, "ghcr.io/rob-broadley/ai-airbase/revetment:latest")
	assertContains(t, out, "2024-06-01")
}

// TestStatus_Stopped verifies that status reports stopped state for a
// non-running container.
func TestStatus_Stopped(t *testing.T) {
	// Given a stopped container
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: true, running: false, image: "ghcr.io/rob-broadley/ai-airbase/revetment:latest", created: "2024-06-01"}
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

	// When the status subcommand is executed
	root.SetArgs([]string{"--project", "myapp", "status"})
	assertNoError(t, root.Execute())

	// Then the output contains stopped
	out := buf.String()
	assertContains(t, out, "stopped")
}

// TestStatus_Absent verifies that status reports absent and a dash placeholder
// when no container exists.
func TestStatus_Absent(t *testing.T) {
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

	buf := &bytes.Buffer{}
	root := cmd.NewRootCmd(deps)
	root.SetOut(buf)

	// When the status subcommand is executed
	root.SetArgs([]string{"--project", "myapp", "status"})
	assertNoError(t, root.Execute())

	// Then the output reports absent and uses a dash placeholder
	out := buf.String()
	assertContains(t, out, "absent")
	assertContains(t, out, "-")
}

// TestStatus_GetStatusFails verifies that an error from GetStatus (ps-all) is
// propagated back to the caller.
func TestStatus_GetStatusFails(t *testing.T) {
	// Given a runner that fails on the "ps-all" subcommand
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{runErrors: map[string]error{"ps-all": fmt.Errorf("ps failed")}}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
	}

	// When the status subcommand is executed
	root := cmd.NewRootCmd(deps)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "status"})

	// Then an error is returned
	assertError(t, root.Execute())
}
