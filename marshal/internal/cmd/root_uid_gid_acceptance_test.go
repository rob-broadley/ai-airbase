// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd_test

import (
	"strings"
	"testing"

	"github.com/rob-broadley/ai-airbase/marshal/internal/cmd"
)

// TestBuildUserConfig_RefusesRootUID verifies that running marshal as root
// (UID 0) is rejected before a container is created.
//
// Given  the calling user has UID 0
// When   the default command is executed
// Then   an error is returned that mentions root or UID 0
func TestBuildUserConfig_RefusesRootUID(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	cf := newCredFakes(t)
	runner := &fakeRunner{exists: false, imageExistsResult: true}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              func() int { return 0 },
		Getgid:              func() int { return 1001 },
		EnsureSharedDataDir: cf.dataDirFn,
	}

	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"start", "--project", "myapp"})
	err := root.Execute()

	assertError(t, err)
	if !strings.Contains(strings.ToLower(err.Error()), "root") &&
		!strings.Contains(err.Error(), "UID 0") &&
		!strings.Contains(err.Error(), "uid 0") {
		t.Errorf("expected error to mention root or UID 0, got: %v", err)
	}

	// Container must NOT have been created.
	if runner.calledSubcommand("create") {
		t.Error("expected podman create NOT to be called when UID is 0")
	}
}

// TestBuildUserConfig_AllowsGIDZero verifies that GID 0 is NOT rejected.
// On some Linux distributions (e.g. Fedora) regular users have the root group
// as their primary GID, so refusing GID 0 would block legitimate users.
//
// Given  the calling user has UID 1001 and GID 0
// When   the default command is executed
// Then   no error is returned for the UID/GID combination
func TestBuildUserConfig_AllowsGIDZero(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	cf := newCredFakes(t)
	runner := &fakeRunner{exists: false, imageExistsResult: true}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              func() int { return 1001 },
		Getgid:              func() int { return 0 },
		EnsureSharedDataDir: cf.dataDirFn,
	}

	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"start", "--project", "myapp"})
	// The command will fail (no real podman), but the failure must NOT be a
	// "must not run as root" error — GID 0 alone should not be rejected.
	err := root.Execute()
	if err != nil &&
		(strings.Contains(strings.ToLower(err.Error()), "gid 0") ||
			(strings.Contains(strings.ToLower(err.Error()), "root") &&
				strings.Contains(strings.ToLower(err.Error()), "gid"))) {
		t.Errorf("GID 0 should be allowed for non-root users, got: %v", err)
	}
}

// TestBuildUserConfig_RefusesRootUID_Recreate verifies that the recreate
// subcommand also refuses UID 0, ensuring no subcommand bypasses the guard.
//
// Given  the calling user has UID 0
// When   the recreate subcommand is executed
// Then   an error is returned and no container is created
func TestBuildUserConfig_RefusesRootUID_Recreate(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	cf := newCredFakes(t)
	runner := &fakeRunner{exists: false, imageExistsResult: true}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              func() int { return 0 },
		Getgid:              func() int { return 1001 },
		EnsureSharedDataDir: cf.dataDirFn,
	}

	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "myapp", "recreate"})
	err := root.Execute()

	assertError(t, err)
	if runner.calledSubcommand("create") {
		t.Error("expected podman create NOT to be called when UID is 0 (recreate path)")
	}
}
