// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd_test

import (
	"bytes"
	"errors"
	"os"
	"testing"

	"github.com/rob-broadley/ai-airbase/marshal/internal/cmd"
	"github.com/rob-broadley/ai-airbase/marshal/internal/config"
)

// TestCreate_MkdirAll_CalledBeforeEnsureProjectVolume verifies that when
// marshal create is run with --mask .venv and the host directory does not yet
// exist, provisionMaskVolumes calls MkdirAll with the correct host path and
// mode before invoking EnsureProjectVolume (which issues podman volume
// commands).
func TestCreate_MkdirAll_CalledBeforeEnsureProjectVolume(t *testing.T) {
	// Given a create command for project "myapp" with --mask .venv,
	// and the host directory /projects/myapp/.venv does not yet exist
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: false, imageExistsResult: true}
	var mkdirCalls []mkdirCall

	deps := newHostDirDeps(t, runner, func(path string, perm os.FileMode) error {
		mkdirCalls = append(mkdirCalls, mkdirCall{
			path:                    path,
			perm:                    perm,
			runnerCallsAtInvocation: len(runner.calls),
		})
		return nil
	})

	root := cmd.NewRootCmd(deps)
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "create", "--mask", ".venv"})

	// When the create subcommand runs
	assertNoError(t, root.Execute())

	// Then MkdirAll is called exactly once with the correct host path and mode
	assertMkdirCalledOnceWithPathAndPerm(t, mkdirCalls, "/projects/myapp/.venv", os.FileMode(0o755))

	// And MkdirAll was invoked before EnsureProjectVolume (the first podman
	// volume command in the runner call log)
	assertMkdirCalledBeforeFirstVolumeCommand(t, runner, mkdirCalls, "create")
}

// TestRecreate_MkdirAll_CalledBeforeEnsureProjectVolume verifies that when
// marshal recreate is run on a project with a saved mask config, provisionMaskVolumes
// calls MkdirAll with the host path and mode 0o755 before invoking
// EnsureProjectVolume (which issues podman volume commands).
func TestRecreate_MkdirAll_CalledBeforeEnsureProjectVolume(t *testing.T) {
	// Given a project config with masks ["/projects/myapp/.venv"] saved and a recreate command
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := config.Save("myapp", &config.Config{
		Mounts: []string{"/projects/myapp"},
		Masks:  []string{"/projects/myapp/.venv"},
	}); err != nil {
		t.Fatalf("saving config: %v", err)
	}

	runner := &fakeRunner{exists: false, imageExistsResult: true}
	var mkdirCalls []mkdirCall

	deps := newHostDirDeps(t, runner, func(path string, perm os.FileMode) error {
		mkdirCalls = append(mkdirCalls, mkdirCall{
			path:                    path,
			perm:                    perm,
			runnerCallsAtInvocation: len(runner.calls),
		})
		return nil
	})

	root := cmd.NewRootCmd(deps)
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "recreate"})

	// When the recreate subcommand runs
	assertNoError(t, root.Execute())

	// Then MkdirAll is called with the host path and mode 0o755
	assertMkdirCalledOnceWithPathAndPerm(t, mkdirCalls, "/projects/myapp/.venv", os.FileMode(0o755))

	// And MkdirAll was invoked before EnsureProjectVolume (the first podman
	// volume command in the runner call log)
	assertMkdirCalledBeforeFirstVolumeCommand(t, runner, mkdirCalls, "recreate")
}

// TestCreate_MkdirAllFailure_AbortsProvisioning verifies that when MkdirAll
// returns an error during mask volume provisioning, the create command returns
// an error and podman create is never invoked.
func TestCreate_MkdirAllFailure_AbortsProvisioning(t *testing.T) {
	// Given a create command with --mask .venv and MkdirAll injected to return
	// a permission-denied error
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: false, imageExistsResult: true}
	mkdirErr := errors.New("permission denied")

	deps := newHostDirDeps(t, runner, func(path string, perm os.FileMode) error {
		return mkdirErr
	})

	root := cmd.NewRootCmd(deps)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "create", "--mask", ".venv"})

	// When the create subcommand runs
	err := root.Execute()

	// Then an error is returned
	assertError(t, err)

	// And podman create is never called
	if runner.calledSubcommand("create") {
		t.Errorf("expected podman create NOT to be called after MkdirAll failure; calls: %v", runner.calls)
	}
}

// TestRecreate_MkdirAllFailure_AbortsProvisioning verifies that when MkdirAll
// returns an error during mask volume provisioning in the recreate path, the
// recreate command returns an error and podman create is never invoked.
func TestRecreate_MkdirAllFailure_AbortsProvisioning(t *testing.T) {
	// Given a recreate command for a project with a saved mask config
	// containing one mask, and MkdirAll is injected to return a
	// permission-denied error
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := config.Save("myapp", &config.Config{
		Mounts: []string{"/projects/myapp"},
		Masks:  []string{"/projects/myapp/.venv"},
	}); err != nil {
		t.Fatalf("saving config: %v", err)
	}

	runner := &fakeRunner{exists: false, imageExistsResult: true}
	mkdirErr := errors.New("permission denied")

	deps := newHostDirDeps(t, runner, func(path string, perm os.FileMode) error {
		return mkdirErr
	})

	root := cmd.NewRootCmd(deps)
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "recreate"})

	// When the recreate subcommand runs
	err := root.Execute()

	// Then an error is returned
	assertError(t, err)

	// And podman create is never called
	if runner.calledSubcommand("create") {
		t.Errorf("expected podman create NOT to be called after MkdirAll failure; calls: %v", runner.calls)
	}
}

// TestCreate_MultipleMasks_SecondMkdirAllFailure_AbortsProvisioning verifies
// that when a create command is run with two masks and MkdirAll succeeds on
// the first call but returns an error on the second call, provisionMaskVolumes
// calls MkdirAll exactly twice (once per mask), returns an error, and podman
// create is never invoked.
func TestCreate_MultipleMasks_SecondMkdirAllFailure_AbortsProvisioning(t *testing.T) {
	// Given a create command with --mask .venv and --mask node_modules,
	// and MkdirAll is injected to succeed on the first call and return a
	// permission-denied error on the second call
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: false, imageExistsResult: true}
	mkdirErr := errors.New("permission denied")
	var mkdirCallCount int

	deps := newHostDirDeps(t, runner, func(path string, perm os.FileMode) error {
		mkdirCallCount++
		if mkdirCallCount >= 2 {
			return mkdirErr
		}
		return nil
	})

	root := cmd.NewRootCmd(deps)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "create", "--mask", ".venv", "--mask", "node_modules"})

	// When the create subcommand runs
	err := root.Execute()

	// Then MkdirAll is called exactly twice (once per mask)
	if mkdirCallCount != 2 {
		t.Errorf("expected MkdirAll to be called exactly twice, got %d calls", mkdirCallCount)
	}

	// And an error is returned
	assertError(t, err)

	// And podman create is never called
	if runner.calledSubcommand("create") {
		t.Errorf("expected podman create NOT to be called after second MkdirAll failure; calls: %v", runner.calls)
	}
}

// TestCreate_MultipleMasks_BothSucceed_ContainerCreated verifies that when a
// create command is run with two masks and both MkdirAll calls succeed,
// provisionMaskVolumes calls MkdirAll exactly twice (once per mask) and the
// container is created normally.
func TestCreate_MultipleMasks_BothSucceed_ContainerCreated(t *testing.T) {
	// Given a create command with --mask .venv and --mask node_modules,
	// and both MkdirAll calls succeed
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: false, imageExistsResult: true}
	var mkdirCallCount int

	deps := newHostDirDeps(t, runner, func(path string, perm os.FileMode) error {
		mkdirCallCount++
		return nil
	})

	root := cmd.NewRootCmd(deps)
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "create", "--mask", ".venv", "--mask", "node_modules"})

	// When the create subcommand runs
	assertNoError(t, root.Execute())

	// Then MkdirAll is called exactly twice (once per mask)
	if mkdirCallCount != 2 {
		t.Errorf("expected MkdirAll to be called exactly twice, got %d calls", mkdirCallCount)
	}

	// And podman create is called
	if !runner.calledSubcommand("create") {
		t.Errorf("expected podman create to be called when both MkdirAll calls succeed; calls: %v", runner.calls)
	}
}

// TestCreate_ExistingDirectory_MkdirAllIdempotent verifies that when the host
// directory already exists, MkdirAll (which is idempotent for existing
// directories) returns nil and provisioning continues normally, creating the
// container.
func TestCreate_ExistingDirectory_MkdirAllIdempotent(t *testing.T) {
	// Given the host directory already exists (MkdirAll is idempotent — returns nil)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: false, imageExistsResult: true}

	// noopMkdirAll simulates an existing directory: os.MkdirAll is idempotent
	// and returns nil when the path already exists.
	deps := newHostDirDeps(t, runner, noopMkdirAll)

	root := cmd.NewRootCmd(deps)
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "create", "--mask", ".venv"})

	// When the create subcommand runs
	assertNoError(t, root.Execute())

	// Then provisioning continues normally and the container is created
	if !runner.calledSubcommand("create") {
		t.Error("expected podman create to be called when MkdirAll succeeds idempotently")
	}
}
