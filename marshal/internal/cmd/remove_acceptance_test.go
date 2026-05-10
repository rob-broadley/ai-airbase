// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd_test

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/rob-broadley/ai-airbase/marshal/internal/cmd"
	"github.com/rob-broadley/ai-airbase/marshal/internal/config"
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
	err := root.Execute()

	// Then an error is returned with the project-facing message.
	assertError(t, err)
	assertContains(t, err.Error(), "project myapp has no container")
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

// TestRemove_RemovesProjectVolumes verifies that marshal remove queries for
// project-labelled volumes via "podman volume ls" and removes them.
func TestRemove_RemovesProjectVolumes(t *testing.T) {
	// Given a stopped container with two labelled project volumes
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{
		exists: true, running: false,
		projectVolumes: []string{"marshal-myapp-nix-store", "marshal-myapp-nix-profile"},
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

	// When the remove subcommand is executed
	root.SetArgs([]string{"--project", "myapp", "remove"})
	assertNoError(t, root.Execute())

	// Then "podman volume ls" was called to discover project volumes
	volumeLsCalled := false
	for _, call := range runner.calls {
		if len(call) >= 3 && call[0] == "podman" && call[1] == "volume" && call[2] == "ls" {
			volumeLsCalled = true
			break
		}
	}
	if !volumeLsCalled {
		t.Error("expected 'podman volume ls' to be called to discover project volumes")
	}

	// And "podman volume rm" was called with both project volumes
	volumeRmFound := false
	for _, call := range runner.calls {
		if len(call) >= 3 && call[0] == "podman" && call[1] == "volume" && call[2] == "rm" {
			if sliceContains(call[3:], "marshal-myapp-nix-store") && sliceContains(call[3:], "marshal-myapp-nix-profile") {
				volumeRmFound = true
				break
			}
		}
	}
	if !volumeRmFound {
		t.Errorf("expected 'podman volume rm' with both project volumes; got calls: %v", runner.calls)
	}
}

// TestRemove_ProjectVolumeRemoveFails verifies that an error from "podman volume ls"
// is propagated back to the caller with a message mentioning "removing project volumes".
func TestRemove_ProjectVolumeRemoveFails(t *testing.T) {
	// Given a stopped container and a runner that fails on the "volume" subcommand
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{
		exists:    true,
		running:   false,
		runErrors: map[string]error{"volume": errors.New("volume ls failed")},
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
	root.SetArgs([]string{"--project", "myapp", "remove"})

	// When the remove subcommand is executed
	err := root.Execute()

	// Then an error is returned containing "removing project volumes"
	assertError(t, err)
	assertContains(t, err.Error(), "removing project volumes")
}

// TestRemove_DeletesProjectConfig verifies that marshal remove deletes the
// saved project config file so a subsequent create starts clean.
func TestRemove_DeletesProjectConfig(t *testing.T) {
	// Given a saved config for the project
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := config.Save("myapp", &config.Config{Mounts: []string{"/large/dataset"}}); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	runner := &fakeRunner{exists: true}
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

	// When remove is executed
	root.SetArgs([]string{"--project", "myapp", "remove"})
	assertNoError(t, root.Execute())

	// Then the config file is gone — Load returns an empty config
	cfg, err := config.Load("myapp")
	if err != nil {
		t.Fatalf("Load after remove failed: %v", err)
	}
	if len(cfg.Mounts) != 0 {
		t.Errorf("expected empty mounts after remove, got: %v", cfg.Mounts)
	}
}

// TestRemove_ContinuesPastVolumeFailure verifies that when RemoveProjectVolumes
// returns an error, runRemove still calls config.Delete (cleanup continues) and
// returns an aggregated error that mentions "removing project volumes".
func TestRemove_ContinuesPastVolumeFailure(t *testing.T) {
	// Given a stopped container, a saved project config, and a runner that
	// fails on the "volume" subcommand (RemoveProjectVolumes).
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := config.Save("myapp", &config.Config{Mounts: []string{"/large/dataset"}}); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	runner := &fakeRunner{
		exists:    true,
		running:   false,
		runErrors: map[string]error{"volume": errors.New("volume ls failed")},
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
	root.SetArgs([]string{"--project", "myapp", "remove"})

	// When remove is executed
	err := root.Execute()

	// Then an error is returned (aggregated from volume failure)
	assertError(t, err)
	if !runner.calledSubcommand("rm") {
		t.Error("expected 'podman rm' to be called")
	}
	assertContains(t, err.Error(), "removing project volumes")

	// And config.Delete was still called — the config must be gone
	cfg, loadErr := config.Load("myapp")
	if loadErr != nil {
		t.Fatalf("Load after remove failed: %v", loadErr)
	}
	if len(cfg.Mounts) != 0 {
		t.Errorf("expected config to be deleted despite volume failure, got mounts: %v", cfg.Mounts)
	}
}

// TestRemove_ContainerRemoveFails_VolumesAndConfigUntouched verifies that when
// container.Remove fails, RemoveProjectVolumes and config.Delete are NOT called
// (the container still exists, so its state must remain consistent).
//
// Acceptance criterion 1: container remove fails → volumes and config untouched.
func TestRemove_ContainerRemoveFails_VolumesAndConfigUntouched(t *testing.T) {
	// Given a saved config and a runner that fails on "rm"
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := config.Save("myapp", &config.Config{Mounts: []string{"/large/dataset"}}); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	runner := &fakeRunner{
		exists:    true,
		running:   false,
		runErrors: map[string]error{"rm": fmt.Errorf("podman unavailable")},
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
	root.SetArgs([]string{"--project", "myapp", "remove"})

	// When the remove subcommand is executed
	err := root.Execute()

	// Then an error is returned
	assertError(t, err)

	// And "podman volume rm" was NOT called — volumes are still consistent
	for _, call := range runner.calls {
		if len(call) >= 3 && call[0] == "podman" && call[1] == "volume" && call[2] == "rm" {
			t.Error("expected 'podman volume rm' NOT to be called when container remove fails")
		}
	}

	// And config.Delete was NOT called — saved mounts must still be present
	cfg, loadErr := config.Load("myapp")
	if loadErr != nil {
		t.Fatalf("Load after failed remove: %v", loadErr)
	}
	if len(cfg.Mounts) == 0 {
		t.Error("expected config to remain intact when container remove fails, but mounts were gone")
	}
}

// TestRemove_VolumeRemoveFails_PrintsQualifiedMessage verifies that when container
// remove succeeds but volume remove fails, stdout reports that the container was
// removed and cleanup only partially succeeded.
//
// Acceptance criterion 2: container remove succeeds, volume remove fails →
// qualified success message printed, config.Delete still attempted, error returned.
func TestRemove_VolumeRemoveFails_PrintsQualifiedMessage(t *testing.T) {
	// Given a stopped container and a runner that fails on the "volume" subcommand
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{
		exists:    true,
		running:   false,
		runErrors: map[string]error{"volume": errors.New("volume ls failed")},
	}
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
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "remove"})

	// When the remove subcommand is executed
	err := root.Execute()

	// Then an error is returned (volume cleanup failed)
	assertError(t, err)

	// And stdout qualifies the success to reflect the partial cleanup failure.
	assertContains(t, buf.String(), "removed (cleanup partially failed:")
}

// TestRemove_ConfigDeleteFails_PrintsQualifiedMessage verifies that when container
// remove and volume remove both succeed but config.Delete fails, stdout reports
// partial cleanup failure and an error is returned.
//
// Acceptance criterion 3: container remove succeeds, config delete fails →
// qualified success message printed, error returned.
func TestRemove_ConfigDeleteFails_PrintsQualifiedMessage(t *testing.T) {
	// Given a runner that succeeds and a DeleteConfig stub that returns an error.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: true, running: false}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
		DeleteConfig:        func(string) error { return errors.New("permission denied") },
	}

	buf := &bytes.Buffer{}
	root := cmd.NewRootCmd(deps)
	root.SetOut(buf)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "remove"})

	// When the remove subcommand is executed
	err := root.Execute()

	// Then an error is returned (config cleanup failed)
	assertError(t, err)

	// And stdout qualifies the success to reflect the partial cleanup failure.
	assertContains(t, buf.String(), "removed (cleanup partially failed:")
}

// --project flag contains an invalid project name (e.g. path traversal).
func TestRemove_InvalidProjectName(t *testing.T) {
	// Given a runner that would succeed if reached
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
	root.SetErr(&bytes.Buffer{})

	// When the remove subcommand is executed with an invalid project name
	root.SetArgs([]string{"--project", "../evil", "remove"})
	err := root.Execute()

	// Then an error is returned containing "invalid project"
	assertError(t, err)
	assertContains(t, err.Error(), "invalid project")
}
