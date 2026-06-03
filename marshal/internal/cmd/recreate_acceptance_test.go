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

// TestRecreate_ExistingRunning verifies that recreate replaces the container
// when it is currently running. With the two-step rename approach, the running
// container is renamed aside and then force-removed rather than being explicitly
// stopped first — podman rm --force handles shutdown internally.
func TestRecreate_ExistingRunning(t *testing.T) {
	// Given a running container
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: true, running: true, imageExistsResult: true}
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

	// When the recreate subcommand is executed
	root.SetArgs([]string{"--project", "myapp", "recreate"})
	assertNoError(t, root.Execute())

	// Then create is called to build the replacement container
	if !runner.calledSubcommand("create") {
		t.Error("expected 'podman create' to be called")
	}
	// And the old container is eventually removed (via force-rm on the retiring name)
	if !runner.calledSubcommand("rm") {
		t.Error("expected 'podman rm' to be called (retiring container force-removed)")
	}
	// And start is NOT called (container left in stopped state for next invocation)
	if runner.calledSubcommand("start") {
		t.Error("expected 'podman start' NOT to be called (container left stopped for next invocation)")
	}
}

// TestRecreate_ExistingStopped verifies that recreate removes, creates and
// starts the container when it is stopped (no stop call needed).
func TestRecreate_ExistingStopped(t *testing.T) {
	// Given a stopped container
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: true, running: false, imageExistsResult: true}
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

	// When the recreate subcommand is executed
	root.SetArgs([]string{"--project", "myapp", "recreate"})
	assertNoError(t, root.Execute())

	// Then stop is NOT called but rm and create are (no start)
	if runner.calledSubcommand("stop") {
		t.Error("expected 'podman stop' NOT to be called for a stopped container")
	}
	if !runner.calledSubcommand("rm") {
		t.Error("expected 'podman rm' to be called")
	}
	if !runner.calledSubcommand("create") {
		t.Error("expected 'podman create' to be called")
	}
	if runner.calledSubcommand("start") {
		t.Error("expected 'podman start' NOT to be called")
	}
}

// TestRecreate_NoContainer verifies that recreate creates and starts a new
// container when no existing container is present.
func TestRecreate_NoContainer(t *testing.T) {
	// Given no container exists
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: false, running: false, imageExistsResult: true}
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

	// When the recreate subcommand is executed
	root.SetArgs([]string{"--project", "myapp", "recreate"})
	assertNoError(t, root.Execute())

	// Then rm is NOT called but create is (no start)
	if runner.calledSubcommand("rm") {
		t.Error("expected 'podman rm' NOT to be called when container absent")
	}
	if !runner.calledSubcommand("create") {
		t.Error("expected 'podman create' to be called")
	}
	if runner.calledSubcommand("start") {
		t.Error("expected 'podman start' NOT to be called")
	}
}

// TestRecreate_UsesSavedMounts verifies that recreate applies mounts saved in
// the project config without requiring --mount flags.
func TestRecreate_UsesSavedMounts(t *testing.T) {
	// Given a project config with saved mounts
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := config.Save("myapp", &config.Config{Mounts: []string{"/abs/lib"}}); err != nil {
		t.Fatalf("saving config: %v", err)
	}

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

	// When recreate is executed without --mount flags
	root.SetArgs([]string{"--project", "myapp", "recreate"})
	assertNoError(t, root.Execute())

	// Then the saved mount appears in the create args
	if !runner.createArgsContain("/abs/lib:/workspace/lib:Z") {
		t.Errorf("expected saved mount in create args, got %v", runner.createArgs())
	}
}

// TestRecreate_PullsBeforeRemovingContainer verifies that recreate always pulls
// the image before touching the existing container.
func TestRecreate_PullsBeforeRemovingContainer(t *testing.T) {
	// Given an existing stopped container
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: true, running: false}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
		ResolveImage:        func() string { return "ghcr.io/rob-broadley/ai-airbase/revetment:latest" },
	}

	root := cmd.NewRootCmd(deps)
	root.SetOut(&bytes.Buffer{})

	// When the recreate subcommand is executed
	root.SetArgs([]string{"--project", "myapp", "recreate"})
	assertNoError(t, root.Execute())

	// Then PullImage is called and the container is removed
	if !runner.pullImageCalled {
		t.Error("expected PullImage to be called during recreate")
	}
	if !runner.calledSubcommand("rm") {
		t.Error("expected container to be removed after successful pull")
	}
}

// TestRecreate_PullFails_NoLocalImage_ErrorReturned verifies that recreate
// returns an error and leaves the existing container untouched when pull fails
// and no local image exists.
func TestRecreate_PullFails_NoLocalImage_ErrorReturned(t *testing.T) {
	// Given pull will fail and no local image exists
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	pullErr := errors.New("registry unavailable")
	runner := &fakeRunner{
		exists:            true,
		running:           false,
		pullImageErr:      pullErr,
		imageExistsResult: false,
	}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
		ResolveImage:        func() string { return "ghcr.io/rob-broadley/ai-airbase/revetment:latest" },
	}

	root := cmd.NewRootCmd(deps)
	root.SetErr(&bytes.Buffer{})

	// When the recreate subcommand is executed
	root.SetArgs([]string{"--project", "myapp", "recreate"})
	err := root.Execute()

	// Then an error is returned and the container is not removed
	assertError(t, err)
	if runner.calledSubcommand("rm") {
		t.Error("expected container NOT to be removed when pull fails and no local image")
	}
}

// TestRecreate_PullFails_LocalImageExists_WarnAndProceed verifies that recreate
// warns and proceeds with the remove/create cycle when pull fails but a local
// image is available.
func TestRecreate_PullFails_LocalImageExists_WarnAndProceed(t *testing.T) {
	// Given pull will fail but a local image exists
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	pullErr := errors.New("registry unavailable")
	runner := &fakeRunner{
		exists:            true,
		running:           false,
		pullImageErr:      pullErr,
		imageExistsResult: true,
	}
	var logBuf bytes.Buffer
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
		Logger:              cmd.NewCLILogger(&logBuf),
		ResolveImage:        func() string { return "ghcr.io/rob-broadley/ai-airbase/revetment:latest" },
	}

	root := cmd.NewRootCmd(deps)
	root.SetOut(&bytes.Buffer{})

	// When the recreate subcommand is executed
	root.SetArgs([]string{"--project", "myapp", "recreate"})
	assertNoError(t, root.Execute())

	// Then a warning is emitted via the progress logger and the container is recreated
	if !runner.pullImageCalled {
		t.Error("expected PullImage to be called")
	}
	if !runner.calledSubcommand("rm") {
		t.Error("expected container to be removed after fallback to local image")
	}
	if !runner.calledSubcommand("create") {
		t.Error("expected container to be created after fallback to local image")
	}
	assertContains(t, logBuf.String(), "pull failed")
}

// TestRecreate_PullSuccess_ProceedsWithRemoveAndCreate verifies that recreate
// performs the pull → remove → create cycle when pull succeeds (no start — left stopped).
func TestRecreate_PullSuccess_ProceedsWithRemoveAndCreate(t *testing.T) {
	// Given an existing stopped container and a successful pull
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: true, running: false}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
		ResolveImage:        func() string { return "ghcr.io/rob-broadley/ai-airbase/revetment:latest" },
	}

	root := cmd.NewRootCmd(deps)
	root.SetOut(&bytes.Buffer{})

	// When the recreate subcommand is executed
	root.SetArgs([]string{"--project", "myapp", "recreate"})
	assertNoError(t, root.Execute())

	// Then pull, rm and create are all called (no start — container left stopped)
	if !runner.pullImageCalled {
		t.Error("expected PullImage to be called")
	}
	if !runner.calledSubcommand("rm") {
		t.Error("expected 'podman rm' to be called after successful pull")
	}
	if !runner.calledSubcommand("create") {
		t.Error("expected 'podman create' to be called after successful pull")
	}
	if runner.calledSubcommand("start") {
		t.Error("expected 'podman start' NOT to be called (container left stopped for next invocation)")
	}
}

// TestRecreate_ContainerAbsent verifies that recreate creates a fresh container
// when none exists (no existing container to remove).
func TestRecreate_ContainerAbsent(t *testing.T) {
	// Given no existing container and an available image
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: false, imageExistsResult: true, imageExistsAfterPull: true}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
	}

	// When the recreate subcommand is executed
	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "myapp", "recreate"})
	assertNoError(t, root.Execute())

	// Then podman create is called to create a fresh container
	if !runner.calledSubcommand("create") {
		t.Error("expected 'podman create' to be called")
	}
}

// TestRecreate_ExistsCheckFails verifies that an error from the Exists check
// is propagated back to the caller.
func TestRecreate_ExistsCheckFails(t *testing.T) {
	// Given a runner that fails on the "ps-all" subcommand used by the Exists check
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{
		imageExistsResult: true,
		runErrors:         map[string]error{"ps-all": fmt.Errorf("ps failed")},
	}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
	}

	// When the recreate subcommand is executed
	root := cmd.NewRootCmd(deps)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "recreate"})

	// Then an error is returned
	assertError(t, root.Execute())
}

// TestRecreate_RemoveFails verifies that when the retiring container cannot be
// force-removed, recreate still succeeds — the cleanup is best-effort. With the
// two-step rename approach, the old container is renamed aside to a retiring
// name and then force-removed as a cleanup step. A failure there is logged as a
// warning, not returned as an error, so the user's container is correctly
// placed at the canonical name.
func TestRecreate_RemoveFails(t *testing.T) {
	// Given an existing stopped container and a runner that fails on "rm"
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{
		exists:            true,
		running:           false,
		imageExistsResult: true,
		runErrors:         map[string]error{"rm": fmt.Errorf("rm failed")},
	}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
	}

	// When the recreate subcommand is executed
	root := cmd.NewRootCmd(deps)
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "recreate"})

	// Then the operation succeeds despite the rm failure — the retiring
	// container cleanup is best-effort (logged as a warning, not an error).
	assertNoError(t, root.Execute())

	// And the canonical container name was never explicitly removed
	if runner.rmCalledFor("marshal-myapp") {
		t.Error("expected canonical container NOT to be explicitly rm'd")
	}
}

// TestRecreate_CreateFails verifies that an error from podman create is
// propagated back to the caller.
func TestRecreate_CreateFails(t *testing.T) {
	// Given no existing container and a runner that fails on "create"
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	// Container already absent (or removed); inject create error.
	runner := &fakeRunner{
		exists:            false,
		imageExistsResult: true,
		runErrors:         map[string]error{"create": fmt.Errorf("create failed")},
	}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
	}

	// When the recreate subcommand is executed
	root := cmd.NewRootCmd(deps)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "recreate"})

	// Then an error is returned
	assertError(t, root.Execute())
}

// TestRecreate_CreateSucceeds_NoStart verifies that recreate succeeds and does
// NOT call podman start — the container is left stopped for the next invocation.
func TestRecreate_CreateSucceeds_NoStart(t *testing.T) {
	// Given no existing container and a runner that creates successfully
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{
		exists:            false,
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

	// When the recreate subcommand is executed
	root := cmd.NewRootCmd(deps)
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "recreate"})
	assertNoError(t, root.Execute())

	// Then the container is created but not started
	if runner.calledSubcommand("start") {
		t.Error("expected 'podman start' NOT to be called by recreate")
	}
}

// TestRecreate_PreservesCustomPort verifies that recreate preserves and reuse
// the custom port configured in the project's config.
func TestRecreate_PreservesCustomPort(t *testing.T) {
	// Given an existing configuration with a custom port
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{
		exists:            false,
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

	// Save existing configuration with port 5000
	cfg := &config.Config{
		Mounts: []string{"/projects/myapp"},
		Port:   5000,
	}
	if err := config.Save("myapp", cfg); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	// When the recreate subcommand is executed
	root := cmd.NewRootCmd(deps)
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "recreate"})
	assertNoError(t, root.Execute())

	// Then the container is created with port 5000
	if !runner.createArgsContain("127.0.0.1:5000:4096") {
		t.Errorf("expected '127.0.0.1:5000:4096' in recreate args, got %v", runner.createArgs())
	}
}
