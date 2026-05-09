// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd_test

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/rob-broadley/ai-airbase/marshal/internal/cmd"
)

// TestRecreate_CreateFails_ExistingContainerPreserved verifies the core
// atomicity guarantee: when the new container cannot be created (e.g. image
// incompatibility, resource exhaustion), the existing container is left
// completely untouched — "podman rm <originalName>" must NOT be called.
//
// Acceptance criterion: If createContainerWithVolumes fails after the check,
// the old container is not removed.
func TestRecreate_CreateFails_ExistingContainerPreserved(t *testing.T) {
	// Given an existing stopped container and a runner whose "create" fails
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{
		exists:            true,
		running:           false,
		imageExistsResult: true,
		runErrors:         map[string]error{"create": fmt.Errorf("image incompatible")},
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
	root.SetArgs([]string{"--project", "myapp", "recreate"})

	// When recreate is executed
	err := root.Execute()

	// Then an error is returned
	assertError(t, err)
	// And the original container was NOT removed (old container is preserved).
	// Note: cleanup of the pending container may call rm, so we check by name.
	if runner.rmCalledFor("marshal-myapp") {
		t.Error("original container must NOT be removed when pending-container creation fails")
	}
}

// TestRecreate_CreateSucceeds_RenameIsCalled verifies that after the pending
// container is successfully created, "podman rename" is called to promote it
// to the canonical container name.
//
// Acceptance criterion: on a successful recreate, podman rename is invoked
// to promote the pending container to the real name.
func TestRecreate_CreateSucceeds_RenameIsCalled(t *testing.T) {
	// Given an existing stopped container
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{
		exists:            true,
		running:           false,
		imageExistsResult: true,
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
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "recreate"})

	// When recreate is executed successfully
	assertNoError(t, root.Execute())

	// Then podman rename was called to promote pending → canonical name
	expectedPendingName := fmt.Sprintf("marshal-myapp-pending-%d", os.Getpid())
	if !runner.renameCalledWith(expectedPendingName, "marshal-myapp") {
		t.Errorf("expected 'podman rename %s marshal-myapp'; calls: %v", expectedPendingName, runner.calls)
	}
}

// TestRecreate_CreateFails_NoExistingContainer_RmNotCalled verifies that when
// there is no existing container and the create attempt fails, "podman rm" is
// never called against the (absent) original container.
//
// Acceptance criterion: rm is not called for the original when create fails and
// no original container exists.
func TestRecreate_CreateFails_NoExistingContainer_RmNotCalled(t *testing.T) {
	// Given no existing container and a runner whose "create" fails
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{
		exists:            false,
		imageExistsResult: true,
		runErrors:         map[string]error{"create": fmt.Errorf("disk full")},
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
	root.SetArgs([]string{"--project", "myapp", "recreate"})

	// When recreate is executed
	err := root.Execute()

	// Then an error is returned and rm is never called for the original name
	assertError(t, err)
	if runner.rmCalledFor("marshal-myapp") {
		t.Error("'podman rm marshal-myapp' must not be called when no container exists and create fails")
	}
}

// ---------------------------------------------------------------------------
// Rename failure: error must include the pending container name for recovery
// ---------------------------------------------------------------------------

// TestRecreate_RenameFails_ErrorMentionsPendingName verifies that when rename
// fails after the old container has already been removed, the returned error
// includes the pending container name so the user can recover manually.
func TestRecreate_RenameFails_ErrorMentionsPendingName(t *testing.T) {
	// Given an existing stopped container and a runner whose "rename" fails
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{
		exists:            true,
		running:           false,
		imageExistsResult: true,
		runErrors:         map[string]error{"rename": fmt.Errorf("rename permission denied")},
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
	root.SetArgs([]string{"--project", "myapp", "recreate"})

	// When recreate is executed
	err := root.Execute()

	// Then an error is returned containing the pending container name
	assertError(t, err)
	expectedPendingName := fmt.Sprintf("marshal-myapp-pending-%d", os.Getpid())
	if !strings.Contains(err.Error(), expectedPendingName) {
		t.Errorf("expected error to contain pending container name %q for manual recovery, got: %v", expectedPendingName, err)
	}
}

// TestRecreate_RenameFails_AttemptsPendingCleanup verifies that when rename
// fails, a best-effort cleanup of the pending container is attempted so it
// does not linger as an orphan.
func TestRecreate_RenameFails_AttemptsPendingCleanup(t *testing.T) {
	// Given an existing stopped container and a runner whose "rename" fails
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{
		exists:            true,
		running:           false,
		imageExistsResult: true,
		runErrors:         map[string]error{"rename": fmt.Errorf("rename permission denied")},
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
	root.SetArgs([]string{"--project", "myapp", "recreate"})

	// When recreate is executed (error is expected — ignore it here)
	_ = root.Execute()

	// Then rm is attempted for the pending container
	expectedPendingName := fmt.Sprintf("marshal-myapp-pending-%d", os.Getpid())
	if !runner.rmCalledFor(expectedPendingName) {
		t.Errorf("expected 'podman rm %s' to be called after rename failure; calls: %v", expectedPendingName, runner.calls)
	}
}

// ---------------------------------------------------------------------------
// Concurrent recreate: pending name must be unique per PID
// ---------------------------------------------------------------------------

// TestRecreate_StalePendingContainer_DoesNotCollide verifies that the current
// recreate invocation uses a PID-unique pending name and does not reference
// the pending name of any other PID.
//
// Note: the fakeRunner cannot simulate a pre-existing Podman container, so
// this test proves isolation by construction — the current invocation only
// ever constructs and operates on its own PID-suffixed pending name, never
// on any other PID's staging name.
func TestRecreate_StalePendingContainer_DoesNotCollide(t *testing.T) {
	// Given an existing stopped container plus a stale pending container from
	// a different invocation (PID 99999, assumed not to be the current PID).
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	stalePendingName := "marshal-myapp-pending-99999"

	runner := &fakeRunner{
		exists:            true,
		running:           false,
		imageExistsResult: true,
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
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "recreate"})

	// When recreate is executed successfully
	assertNoError(t, root.Execute())

	// Then the stale pending container from a different PID is never removed
	if runner.rmCalledFor(stalePendingName) {
		t.Errorf("stale pending container %q from another invocation must not be removed", stalePendingName)
	}

	// And the current invocation uses its own PID-based pending name
	expectedPendingName := fmt.Sprintf("marshal-myapp-pending-%d", os.Getpid())
	if !runner.renameCalledWith(expectedPendingName, "marshal-myapp") {
		t.Errorf("expected rename from %q to %q; calls: %v", expectedPendingName, "marshal-myapp", runner.calls)
	}
}
