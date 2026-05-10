// SPDX-License-Identifier: AGPL-3.0-or-later
// Acceptance tests verifying that progress messages are emitted during slow
// operations so the user can see what marshal is doing.
package cmd_test

import (
	"bytes"
	"testing"

	"github.com/rob-broadley/ai-airbase/marshal/internal/cmd"
)

// TestCreate_LogsCreatingContainer verifies that the create subcommand logs
// a "creating container" progress message so the user is not left wondering
// why the CLI appears to hang.
func TestCreate_LogsCreatingContainer(t *testing.T) {
	// Given a project with no existing container and an injected progress logger
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	var logBuf bytes.Buffer
	runner := &fakeRunner{exists: false}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
		Logger:              cmd.NewCLILogger(&logBuf),
	}

	root := cmd.NewRootCmd(deps)
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "create"})

	// When create is executed
	assertNoError(t, root.Execute())

	// Then a "creating container" progress message appears in the log output
	assertContains(t, logBuf.String(), "creating container")
}

// TestDefaultCmd_LogsCreatingContainer verifies that the default command also
// logs progress when it creates a container on first run.
func TestDefaultCmd_LogsCreatingContainer(t *testing.T) {
	// Given no container exists and an injected progress logger
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	var logBuf bytes.Buffer
	runner := &fakeRunner{exists: false}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
		Logger:              cmd.NewCLILogger(&logBuf),
	}

	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "myapp"})

	// When the default command is executed
	assertNoError(t, root.Execute())

	// Then a "creating container" progress message appears in the log output
	assertContains(t, logBuf.String(), "creating container")
}

// TestRecreate_LogsRemovingAndCreating verifies that recreate logs both the
// removal and creation steps so the user sees activity throughout the operation.
// With the two-step rename approach, the removal is logged as "removing retired
// container" when the retiring container is force-removed after successful
// promotion.
func TestRecreate_LogsRemovingAndCreating(t *testing.T) {
	// Given an existing container and an injected progress logger
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	var logBuf bytes.Buffer
	runner := &fakeRunner{exists: true}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
		Logger:              cmd.NewCLILogger(&logBuf),
	}

	root := cmd.NewRootCmd(deps)
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "recreate"})

	// When recreate is executed
	assertNoError(t, root.Execute())

	// Then creation and removal progress messages appear
	got := logBuf.String()
	assertContains(t, got, "removing retired container")
	assertContains(t, got, "creating container")
}

// TestCreate_LogsProvisioningVolume verifies that when a container image
// declares named volumes, a "provisioning volume" progress message is logged
// for each one so the user can see volume setup activity.
func TestCreate_LogsProvisioningVolume(t *testing.T) {
	// Given an image that declares a labelled volume
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	var logBuf bytes.Buffer
	runner := &fakeRunner{
		exists:                 false,
		imageInspectVolumeJSON: `{"/work/npm":{}}`,
		imageInspectLabelJSON:  `{"io.ai-airbase.volume.npm":"/work/npm"}`,
	}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
		Logger:              cmd.NewCLILogger(&logBuf),
	}

	root := cmd.NewRootCmd(deps)
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "create"})

	// When create is executed
	assertNoError(t, root.Execute())

	// Then a "provisioning volume" progress message appears in the log output
	assertContains(t, logBuf.String(), "provisioning volume")
}

// a "pulling image" progress message is logged before the pull starts.
func TestCreate_LogsPullingImage(t *testing.T) {
	// Given an absent image that needs to be pulled
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	var logBuf bytes.Buffer
	runner := &fakeRunner{
		exists:            false,
		imageExistsResult: false,
	}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
		ResolveImage:        func() string { return "ghcr.io/example/revetment:latest" },
		Logger:              cmd.NewCLILogger(&logBuf),
	}

	root := cmd.NewRootCmd(deps)
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "create"})

	// When create is executed
	assertNoError(t, root.Execute())

	// Then a "pulling image" progress message appears before the pull
	assertContains(t, logBuf.String(), "pulling image")
}
