// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd_test

import (
	"bytes"
	"testing"

	"github.com/rob-broadley/ai-airbase/marshal/internal/cmd"
)

// TestRemove_DoesNotCallEnsureSharedDataDir verifies that the remove command
// does not touch shared data directories so credential dirs survive remove cycles.
func TestRemove_DoesNotCallEnsureSharedDataDir(t *testing.T) {
	// Given a running container with credential fakes injected
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cf := newCredFakes(t)
	runner := &fakeRunner{exists: true, running: false}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              func() int { return 1001 },
		Getgid:              func() int { return 1001 },
		EnsureSharedDataDir: cf.ensureFn,
	}

	// When the remove subcommand is executed
	root := cmd.NewRootCmd(deps)
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "remove"})
	assertNoError(t, root.Execute())

	// Then EnsureSharedDataDir was never called
	if len(cf.calls) != 0 {
		t.Errorf("remove must not call EnsureSharedDataDir; got calls: %v", cf.calls)
	}
}

// TestStop_DoesNotCallEnsureSharedDataDir verifies that the stop command does
// not touch shared data directories.
func TestStop_DoesNotCallEnsureSharedDataDir(t *testing.T) {
	// Given a running container with credential fakes injected
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cf := newCredFakes(t)
	runner := &fakeRunner{exists: true, running: true}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              func() int { return 1001 },
		Getgid:              func() int { return 1001 },
		EnsureSharedDataDir: cf.ensureFn,
	}

	// When the stop subcommand is executed
	root := cmd.NewRootCmd(deps)
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "stop"})
	assertNoError(t, root.Execute())

	// Then EnsureSharedDataDir was never called
	if len(cf.calls) != 0 {
		t.Errorf("stop must not call EnsureSharedDataDir; got calls: %v", cf.calls)
	}
}
