// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd_test

import (
	"bytes"
	"fmt"
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

	// Then podman rename was called to promote pending → canonical name.
	// The pending name contains a non-deterministic PID+nanosecond suffix so
	// we use a prefix match rather than an exact name match.
	if !runner.renameCalledFromPrefix("marshal-myapp-pending-", "marshal-myapp") {
		t.Errorf("expected 'podman rename marshal-myapp-pending-... marshal-myapp'; calls: %v", runner.calls)
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
// Two-step rename: verify the aside→promote→retire sequence
// ---------------------------------------------------------------------------

// TestRecreate_OldRenamedAside_PendingPromotedToCanonical verifies the full
// success path of the two-step rename sequence introduced to eliminate the
// data-loss window that existed in the old create→remove→rename sequence.
//
// The new sequence is:
//  1. Create pending container (new image, staging name)
//  2. Rename old → retiring (reversible aside)
//  3. Rename pending → canonical (promotion)
//  4. Force-remove retiring (best-effort cleanup)
//
// Acceptance criterion: on success, the old container is renamed aside to a
// retiring name, the pending container is promoted to the canonical name, and
// the retiring container is force-removed.
func TestRecreate_OldRenamedAside_PendingPromotedToCanonical(t *testing.T) {
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

	// Then the old container was renamed aside to a retiring name (step 2)
	if !runner.renameCalledWithToPrefix("marshal-myapp", "marshal-myapp-retiring-") {
		t.Errorf("expected 'podman rename marshal-myapp marshal-myapp-retiring-...' (aside step); calls: %v", runner.calls)
	}

	// And the pending container was promoted to the canonical name (step 3)
	if !runner.renameCalledFromPrefix("marshal-myapp-pending-", "marshal-myapp") {
		t.Errorf("expected 'podman rename marshal-myapp-pending-... marshal-myapp' (promotion step); calls: %v", runner.calls)
	}

	// And the retiring container was force-removed (step 4)
	if !runner.rmCalledForPrefix("marshal-myapp-retiring-") {
		t.Errorf("expected 'podman rm --force marshal-myapp-retiring-...' (retire cleanup); calls: %v", runner.calls)
	}

	// And the canonical container was never explicitly removed
	if runner.rmCalledFor("marshal-myapp") {
		t.Error("canonical container must never be explicitly rm'd in the two-step rename path")
	}
}

// TestRecreate_PromotionFails_OldRestoredFromRetiring verifies that when the
// promotion rename (pending→canonical) fails, the retiring container is renamed
// back to the canonical name so the user is never left without a container.
//
// Acceptance criterion: if the promotion rename fails, the retiring container is
// restored to the canonical name before the error is returned.
func TestRecreate_PromotionFails_OldRestoredFromRetiring(t *testing.T) {
	// Given an existing stopped container and a runner whose promotion rename fails.
	// We inject failure only on the pending→canonical rename so the aside rename
	// (marshal-myapp → retiring) succeeds and the restoration rename
	// (retiring → marshal-myapp) also succeeds.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{
		exists:            true,
		running:           false,
		imageExistsResult: true,
		renameErrorFn: func(from, to string) error {
			// Fail only the promotion rename (pending → canonical).
			// The aside rename (canonical → retiring) and the restoration
			// rename (retiring → canonical) must succeed.
			if strings.HasPrefix(from, "marshal-myapp-pending-") {
				return fmt.Errorf("rename permission denied")
			}
			return nil
		},
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

	// And the retiring container was renamed back to the canonical name (restoration)
	if !runner.renameCalledFromPrefix("marshal-myapp-retiring-", "marshal-myapp") {
		t.Errorf("expected retiring container to be restored to canonical name after promotion failure; calls: %v", runner.calls)
	}

	// And the pending container was cleaned up (force-removed)
	if !runner.rmCalledForPrefix("marshal-myapp-pending-") {
		t.Errorf("expected pending container to be force-removed after promotion failure; calls: %v", runner.calls)
	}
}

// TestRecreate_RemoveAside_Fails_PendingCleaned verifies that when the
// rename-aside step (old→retiring) fails, the pending container is
// force-removed so it does not linger as an orphan.
//
// Acceptance criterion: if the aside rename fails, the pending container is
// force-removed and an error is returned.
func TestRecreate_RemoveAside_Fails_PendingCleaned(t *testing.T) {
	// Given an existing stopped container and a runner whose aside rename fails.
	// The pending container was already created successfully before the aside
	// rename is attempted.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{
		exists:            true,
		running:           false,
		imageExistsResult: true,
		renameErrorFn: func(from, to string) error {
			// Fail only the aside rename (canonical → retiring).
			if strings.HasPrefix(to, "marshal-myapp-retiring-") {
				return fmt.Errorf("rename aside failed")
			}
			return nil
		},
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

	// And the pending container was force-removed to avoid leaking a staging container
	if !runner.rmCalledForPrefix("marshal-myapp-pending-") {
		t.Errorf("expected pending container to be force-removed after aside-rename failure; calls: %v", runner.calls)
	}

	// And the canonical container was not removed
	if runner.rmCalledFor("marshal-myapp") {
		t.Error("canonical container must not be removed when aside rename fails")
	}
}

// ---------------------------------------------------------------------------
// Rename failure: error must include the pending container name for recovery
// ---------------------------------------------------------------------------

// TestRecreate_RenameFails_ErrorMentionsPendingName verifies that when the
// promotion rename (pending→canonical) fails, the returned error includes the
// pending container name prefix so the user can recover manually.
func TestRecreate_RenameFails_ErrorMentionsPendingName(t *testing.T) {
	// Given an existing stopped container and a runner whose promotion rename fails.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{
		exists:            true,
		running:           false,
		imageExistsResult: true,
		renameErrorFn: func(from, to string) error {
			if strings.HasPrefix(from, "marshal-myapp-pending-") {
				return fmt.Errorf("rename permission denied")
			}
			return nil
		},
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

	// Then an error is returned containing the pending container name prefix
	assertError(t, err)
	if !strings.Contains(err.Error(), "marshal-myapp-pending-") {
		t.Errorf("expected error to contain pending container name prefix %q for manual recovery, got: %v",
			"marshal-myapp-pending-", err)
	}
}

// TestRecreate_RenameFails_AttemptsPendingCleanup verifies that when the
// promotion rename (pending→canonical) fails, a best-effort force-removal of
// the pending container is attempted so it does not linger as an orphan.
func TestRecreate_RenameFails_AttemptsPendingCleanup(t *testing.T) {
	// Given an existing stopped container and a runner whose promotion rename fails.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{
		exists:            true,
		running:           false,
		imageExistsResult: true,
		renameErrorFn: func(from, to string) error {
			if strings.HasPrefix(from, "marshal-myapp-pending-") {
				return fmt.Errorf("rename permission denied")
			}
			return nil
		},
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

	// Then force-rm is attempted for the pending container.
	// Prefix match because the pending name includes a non-deterministic
	// PID+nanosecond suffix.
	if !runner.rmCalledForPrefix("marshal-myapp-pending-") {
		t.Errorf("expected force-rm of marshal-myapp-pending-... after rename failure; calls: %v", runner.calls)
	}
}

// ---------------------------------------------------------------------------
// Concurrent recreate: pending name must be unique per invocation
// ---------------------------------------------------------------------------

// TestRecreate_StalePendingContainer_DoesNotCollide verifies that the current
// recreate invocation uses a per-invocation unique pending name and never
// operates on the staging containers of other invocations.
//
// Isolation by construction: the pending name is formed as
// "<container>-pending-<PID>-<nanosecond>". The nanosecond component makes
// each invocation's pending name unique even across PID reuse or PID namespace
// sharing. Because fakeRunner cannot simulate pre-existing Podman containers
// independently, this test proves isolation structurally — the current
// invocation only ever constructs and operates on its own PID+nano-suffixed
// pending name, never touching any other invocation's staging container.
func TestRecreate_StalePendingContainer_DoesNotCollide(t *testing.T) {
	// Given an existing stopped container plus a stale pending container from
	// a different invocation (using a fixed legacy PID name that the new
	// PID+nano format will never produce).
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

	// Then the stale pending container from a different invocation is never removed
	if runner.rmCalledFor(stalePendingName) {
		t.Errorf("stale pending container %q from another invocation must not be removed", stalePendingName)
	}

	// And the current invocation uses its own PID+nano-based pending name
	// (prefix match because the exact nanosecond suffix is non-deterministic)
	if !runner.renameCalledFromPrefix("marshal-myapp-pending-", "marshal-myapp") {
		t.Errorf("expected rename from marshal-myapp-pending-... to marshal-myapp; calls: %v", runner.calls)
	}
}
