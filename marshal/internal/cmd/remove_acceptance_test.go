// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/rob-broadley/ai-airbase/marshal/internal/cmd"
)

// TestRemove_RunningContainer verifies that remove stops and removes a running
// container and reports success.
func TestRemove_RunningContainer(t *testing.T) {
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

	// When the remove subcommand is executed
	root.SetArgs([]string{"--project", "myapp", "remove"})
	assertNoError(t, root.Execute())

	// Then stop and rm are called and the output mentions removed
	if !runner.calledSubcommand("stop") {
		t.Error("expected 'podman stop' to be called")
	}
	if !runner.calledSubcommand("rm") {
		t.Error("expected 'podman rm' to be called")
	}
	assertContains(t, buf.String(), "removed")
}

// TestRemove_StoppedContainer verifies that remove omits the stop call and
// removes a stopped container directly.
func TestRemove_StoppedContainer(t *testing.T) {
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

	// When the remove subcommand is executed
	root.SetArgs([]string{"--project", "myapp", "remove"})
	assertNoError(t, root.Execute())

	// Then stop is NOT called but rm is
	if runner.calledSubcommand("stop") {
		t.Error("expected 'podman stop' NOT to be called for stopped container")
	}
	if !runner.calledSubcommand("rm") {
		t.Error("expected 'podman rm' to be called")
	}
}

// TestRemove_AbsentContainer verifies that remove returns an error when the
// container does not exist.
func TestRemove_AbsentContainer(t *testing.T) {
	// Given no container exists
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

	// When the remove subcommand is executed
	root.SetArgs([]string{"--project", "myapp", "remove"})

	// Then an error is returned
	assertError(t, root.Execute())
}

// TestRemove_RemoveFails verifies that an error from podman rm is propagated
// back to the caller.
func TestRemove_RemoveFails(t *testing.T) {
	// Given an existing stopped container and a runner that fails on "rm"
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{
		exists:    true,
		running:   false,
		runErrors: map[string]error{"rm": fmt.Errorf("rm failed")},
	}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
	}

	// When the remove subcommand is executed
	root := cmd.NewRootCmd(deps)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "remove"})

	// Then an error is returned
	assertError(t, root.Execute())
}

// TestRemove_AlsoRemovesNixStoreVolume verifies that marshal remove also runs
// "podman volume rm marshal-<project>-nix" to clean up the per-project Nix
// store volume.
func TestRemove_AlsoRemovesNixStoreVolume(t *testing.T) {
	// Given a stopped container for project "myapp"
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

	root := cmd.NewRootCmd(deps)
	root.SetOut(&bytes.Buffer{})

	// When the remove subcommand is executed
	root.SetArgs([]string{"--project", "myapp", "remove"})
	assertNoError(t, root.Execute())

	// Then "podman volume rm marshal-myapp-nix" was called
	found := false
	for _, call := range runner.calls {
		if len(call) >= 4 && call[0] == "podman" && call[1] == "volume" && call[2] == "rm" && call[3] == "marshal-myapp-nix" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'podman volume rm marshal-myapp-nix' to be called; got calls: %v", runner.calls)
	}
}
