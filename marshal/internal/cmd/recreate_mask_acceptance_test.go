// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd_test

import (
	"bytes"
	"testing"

	"github.com/rob-broadley/ai-airbase/marshal/internal/cmd"
	"github.com/rob-broadley/ai-airbase/marshal/internal/config"
)

// volumeRmCalledFor checks whether "podman volume rm ... <name>" appeared in
// the recorded calls. Unlike the container-level rmCalledFor method, this
// specifically checks for the "volume rm" sub-subcommand so container rm calls
// are not mistakenly matched.
func volumeRmCalledFor(runner *fakeRunner, name string) bool {
	for _, call := range runner.calls {
		if len(call) >= 3 && call[0] == "podman" && call[1] == "volume" && call[2] == "rm" {
			for _, arg := range call[3:] {
				if arg == name {
					return true
				}
			}
		}
	}
	return false
}

// TestRecreateMask_MaskVolumeNotRemovedOnRecreate verifies that marshal
// recreate does NOT call "podman volume rm" for the mask volume. Mask volumes
// are preserved across recreates (like image-declared volumes), so the agent's
// scratch state survives a container rebuild.
func TestRecreateMask_MaskVolumeNotRemovedOnRecreate(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := config.Save("myapp", &config.Config{
		Mounts: []string{"/projects/myapp"},
		Masks:  []string{"/projects/myapp/.venv"},
	}); err != nil {
		t.Fatalf("saving config: %v", err)
	}

	runner := &fakeRunner{exists: true, running: false, imageExistsResult: true}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
		MkdirAll:            noopMkdirAll,
	}

	root := cmd.NewRootCmd(deps)
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "recreate"})

	assertNoError(t, root.Execute())

	// Mask volume must NOT be removed — it is preserved across recreates.
	wantVolume := "marshal-myapp-mask-myapp-.venv"
	if volumeRmCalledFor(runner, wantVolume) {
		t.Errorf("expected 'podman volume rm' NOT to be called for mask volume %s; calls: %v",
			wantVolume, runner.calls)
	}
}

// TestRecreateMask_MaskVolumeProvisionedOnRecreate verifies that the mask
// volume is still provisioned (created if absent) during recreate, so that the
// new container always has its scratch space available.
func TestRecreateMask_MaskVolumeProvisionedOnRecreate(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := config.Save("myapp", &config.Config{
		Mounts: []string{"/projects/myapp"},
		Masks:  []string{"/projects/myapp/.venv"},
	}); err != nil {
		t.Fatalf("saving config: %v", err)
	}

	runner := &fakeRunner{exists: true, running: false, imageExistsResult: true}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
		MkdirAll:            noopMkdirAll,
	}

	root := cmd.NewRootCmd(deps)
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "recreate"})

	assertNoError(t, root.Execute())

	// Mask volume must be provisioned so the new container has its scratch space.
	wantVolume := "marshal-myapp-mask-myapp-.venv"
	if !volumeCreateCalledFor(runner, wantVolume) {
		t.Errorf("expected 'podman volume create ... %s' to be called during recreate; calls: %v",
			wantVolume, runner.calls)
	}
}

// TestRecreateMask_ExistingContainer_MaskVolumesMounted verifies that when an
// existing (stopped) container is recreated with a mask configured, the full
// double-rename sequence is executed and the new container's podman create call
// includes the mask volume -v flag.
func TestRecreateMask_ExistingContainer_MaskVolumesMounted(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := config.Save("myapp", &config.Config{
		Mounts: []string{"/projects/myapp"},
		Masks:  []string{"/projects/myapp/.venv"},
	}); err != nil {
		t.Fatalf("saving config: %v", err)
	}

	runner := &fakeRunner{exists: true, running: false, imageExistsResult: true}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
		MkdirAll:            noopMkdirAll,
	}

	root := cmd.NewRootCmd(deps)
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "recreate"})

	assertNoError(t, root.Execute())

	// The canonical name was renamed aside (retiring) as part of the double-rename
	// sequence — the existing container steps out of the way before the new one
	// is promoted.
	if !runner.renameCalledWithToPrefix("marshal-myapp", "marshal-myapp-retiring-") {
		t.Error("expected marshal-myapp to be renamed to a retiring- name; calls:", runner.calls)
	}

	// A pending container was created and then promoted to the canonical name.
	if !runner.renameCalledFromPrefix("marshal-myapp-pending-", "marshal-myapp") {
		t.Error("expected a pending container to be renamed to marshal-myapp; calls:", runner.calls)
	}

	// The new container's create call must carry the mask volume -v flag so that
	// the agent's scratch space is wired in on the real recreate path.
	wantFlag := "marshal-myapp-mask-myapp-.venv:/workspace/myapp/.venv"
	if !runner.createArgsContain(wantFlag) {
		t.Errorf("expected podman create to include -v flag %q; create args: %v",
			wantFlag, runner.createArgs())
	}
}

// TestRecreateMask_PreExistingMaskVolume_NotRemovedAndProvisionedIdempotently
// verifies that when marshal recreate is run and the mask volume already exists
// (pre-provisioned), the recreate path:
//  1. Does NOT remove the mask volume (the volume is preserved, not wiped).
//  2. Still calls podman volume create --ignore for the volume (idempotent provision).
//  3. Completes without error.
func TestRecreateMask_PreExistingMaskVolume_NotRemovedAndProvisionedIdempotently(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := config.Save("myapp", &config.Config{
		Mounts: []string{"/projects/myapp"},
		Masks:  []string{"/projects/myapp/.venv"},
	}); err != nil {
		t.Fatalf("saving config: %v", err)
	}

	// fakeRunner: container exists, image exists, and the mask volume is
	// pre-existing so EnsureProjectVolume returns created=false.
	runner := &fakeRunner{
		exists:            true,
		running:           false,
		imageExistsResult: true,
		projectVolumes:    []string{"marshal-myapp-mask-myapp-.venv"},
	}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
		MkdirAll:            noopMkdirAll,
	}

	root := cmd.NewRootCmd(deps)
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "recreate"})

	// Then: no error
	assertNoError(t, root.Execute())

	wantVolume := "marshal-myapp-mask-myapp-.venv"

	// The pre-existing volume must NOT be removed.
	if volumeRmCalledFor(runner, wantVolume) {
		t.Errorf("expected 'podman volume rm' NOT to be called for pre-existing mask volume %s; calls: %v",
			wantVolume, runner.calls)
	}

	// Idempotent provision must still run (--ignore makes it safe).
	if !volumeCreateCalledFor(runner, wantVolume) {
		t.Errorf("expected 'podman volume create ... %s' to be called even when volume pre-exists; calls: %v",
			wantVolume, runner.calls)
	}
}

// TestRecreateMask_NoMasks_NoVolumeRm verifies that marshal recreate on a
// project with NO masks does not call "podman volume rm" for any mask volume.
func TestRecreateMask_NoMasks_NoVolumeRm(t *testing.T) {
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

	root := cmd.NewRootCmd(deps)
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "recreate"})

	assertNoError(t, root.Execute())

	// No mask volume rm should be called.
	for _, call := range runner.calls {
		if len(call) >= 3 && call[0] == "podman" && call[1] == "volume" && call[2] == "rm" {
			for _, arg := range call[3:] {
				if len(arg) > len("marshal-myapp-mask-") &&
					arg[:len("marshal-myapp-mask-")] == "marshal-myapp-mask-" {
					t.Errorf("expected no mask volume rm, but found: %v", call)
				}
			}
		}
	}
}
