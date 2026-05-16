// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd_test

import (
	"bytes"
	"testing"

	"github.com/rob-broadley/ai-airbase/marshal/internal/cmd"
)

// TestRemoveMask_MaskVolumeRemovedViaLabelFilter verifies that when
// marshal remove is run and a mask volume carries the project label, the mask
// volume is deleted as part of the cleanup path in RemoveProjectVolumes.
func TestRemoveMask_MaskVolumeRemovedViaLabelFilter(t *testing.T) {
	// Given a stopped container with a mask volume that carries the project label
	// (pre-configured via projectVolumes so the fakeRunner returns it when
	// "podman volume ls --filter label=..." is queried by RemoveProjectVolumes).
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{
		exists:         true,
		running:        false,
		projectVolumes: []string{"marshal-myapp-mask-myapp-.venv"},
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
	root.SetArgs([]string{"--project", "myapp", "remove"})

	// When marshal remove is executed
	assertNoError(t, root.Execute())

	// Then podman volume rm was called for the mask volume
	wantVolume := "marshal-myapp-mask-myapp-.venv"
	if !volumeRmCalledFor(runner, wantVolume) {
		t.Errorf("expected 'podman volume rm ... %s' to be called during remove; calls: %v",
			wantVolume, runner.calls)
	}
}

// TestRemoveMask_RemoveProjectVolumesUsesContainerNameFilter verifies that
// RemoveProjectVolumes is invoked with the container name so that the label
// filter "io.ai-airbase.project=marshal-myapp" picks up the mask volume. The
// fakeRunner returns the pre-configured mask volume name for that label query,
// and "podman volume rm" is called for it.
func TestRemoveMask_RemoveProjectVolumesUsesContainerNameFilter(t *testing.T) {
	// Given a stopped container with a mask volume that carries the project label
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{
		exists:         true,
		running:        false,
		projectVolumes: []string{"marshal-myapp-mask-myapp-.venv"},
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
	root.SetArgs([]string{"--project", "myapp", "remove"})

	// When marshal remove is executed
	assertNoError(t, root.Execute())

	// Then "podman volume ls" was called with the project label filter for the
	// canonical container name (marshal-myapp) so RemoveProjectVolumes discovers
	// the mask volume via label lookup.
	wantFilter := "label=io.ai-airbase.project=marshal-myapp"
	filterFound := false
	for _, call := range runner.calls {
		if len(call) >= 3 && call[0] == "podman" && call[1] == "volume" && call[2] == "ls" {
			for _, arg := range call[3:] {
				if arg == wantFilter {
					filterFound = true
					break
				}
			}
		}
		if filterFound {
			break
		}
	}
	if !filterFound {
		t.Errorf("expected 'podman volume ls' with filter %q; calls: %v", wantFilter, runner.calls)
	}

	// And podman volume rm was called for the mask volume returned by that query
	wantVolume := "marshal-myapp-mask-myapp-.venv"
	if !volumeRmCalledFor(runner, wantVolume) {
		t.Errorf("expected 'podman volume rm ... %s' to be called; calls: %v", wantVolume, runner.calls)
	}
}

// TestRemoveMask_MultiMaskProject_AllVolumesRemoved verifies that when
// marshal remove is run on a project with two mask volumes, both volumes
// appear in the podman volume rm call so neither is left behind.
func TestRemoveMask_MultiMaskProject_AllVolumesRemoved(t *testing.T) {
	// Given a stopped container whose project label covers two mask volumes.
	// No config is needed — remove uses label-based volume discovery, not config.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{
		exists:  true,
		running: false,
		projectVolumes: []string{
			"marshal-myapp-mask-myapp-.venv",
			"marshal-myapp-mask-myapp-node_modules",
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
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "remove"})

	// When marshal remove is executed
	assertNoError(t, root.Execute())

	// Then both mask volumes are removed.
	for _, wantVolume := range []string{
		"marshal-myapp-mask-myapp-.venv",
		"marshal-myapp-mask-myapp-node_modules",
	} {
		if !volumeRmCalledFor(runner, wantVolume) {
			t.Errorf("expected 'podman volume rm ... %s' to be called during remove; calls: %v",
				wantVolume, runner.calls)
		}
	}
}
