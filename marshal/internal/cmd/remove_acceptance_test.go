// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/rob-broadley/ai-airbase/marshal/internal/cmd"
	"github.com/rob-broadley/ai-airbase/marshal/internal/config"
)

// recordingSlogHandler is an slog.Handler spy that captures every record it
// is asked to handle. Mirrors the pattern of recordingHandler in helpers_test.go
// but lives in this external test package so remove_acceptance_test.go can use
// it without reaching into the internal cmd package test file.
type recordingSlogHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *recordingSlogHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *recordingSlogHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r.Clone())
	return nil
}

func (h *recordingSlogHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *recordingSlogHandler) WithGroup(_ string) slog.Handler      { return h }

func (h *recordingSlogHandler) recordCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.records)
}

func (h *recordingSlogHandler) describeRecords() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.records) == 0 {
		return "(no records)"
	}
	parts := make([]string, 0, len(h.records))
	for _, r := range h.records {
		parts = append(parts, fmt.Sprintf("{level=%s msg=%q}", r.Level, r.Message))
	}
	return strings.Join(parts, ", ")
}

// findInfoRecord returns the first INFO-level record whose message equals
// msg exactly.
func (h *recordingSlogHandler) findInfoRecord(msg string) (slog.Record, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, r := range h.records {
		if r.Level == slog.LevelInfo && r.Message == msg {
			return r, true
		}
	}
	return slog.Record{}, false
}

// recordAttr returns the string form of the attribute named key on r, or the
// empty string if the attribute is not set.
func recordAttr(r slog.Record, key string) string {
	var got string
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == key {
			got = fmt.Sprintf("%v", a.Value.Any())
			return false
		}
		return true
	})
	return got
}

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

// ---------------------------------------------------------------------------
// Host-side project directory cleanup
// ---------------------------------------------------------------------------

// TestRemove_DeletesHostProjectDirectory verifies that running marshal remove
// deletes the host-side project directory and all its nested contents.
func TestRemove_DeletesHostProjectDirectory(t *testing.T) {
	// Given a project "myapp" has a container and its host-side data directory "projects/myapp" exists with nested directories and files
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	tempDir := t.TempDir()
	ensureDataDir := func(subdir string) (string, error) {
		p := filepath.Join(tempDir, subdir)
		if err := os.MkdirAll(p, 0o700); err != nil {
			return "", err
		}
		return p, nil
	}

	projectDir, err := ensureDataDir("projects/myapp")
	if err != nil {
		t.Fatalf("failed to create projects/myapp: %v", err)
	}

	// Create nested config, share, state dirs and files
	subdirs := []string{"config", "share", "state"}
	for _, sd := range subdirs {
		dirPath := filepath.Join(projectDir, "opencode", sd)
		if err := os.MkdirAll(dirPath, 0o700); err != nil {
			t.Fatalf("failed to create nested dir %s: %v", sd, err)
		}
		filePath := filepath.Join(dirPath, sd+"_file.txt")
		if err := os.WriteFile(filePath, []byte("data"), 0o600); err != nil {
			t.Fatalf("failed to write file %s: %v", filePath, err)
		}
	}

	runner := &fakeRunner{exists: true, running: true}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: ensureDataDir,
		SharedDataPath:      func(subdir string) string { return filepath.Join(tempDir, subdir) },
	}

	buf := &bytes.Buffer{}
	root := cmd.NewRootCmd(deps)
	root.SetOut(buf)

	// When the "remove" subcommand is executed for project "myapp"
	root.SetArgs([]string{"--project", "myapp", "remove"})
	assertNoError(t, root.Execute())

	// Then the container is stopped and removed, and the host-side data directory "projects/myapp" along with all nested contents is recursively deleted from the host.
	if !runner.calledSubcommand("stop") {
		t.Error("expected 'podman stop' to be called")
	}
	if !runner.calledSubcommand("rm") {
		t.Error("expected 'podman rm' to be called")
	}
	if _, err := os.Stat(projectDir); !os.IsNotExist(err) {
		t.Errorf("expected host-side project directory %s to be deleted, but os.Stat returned: %v", projectDir, err)
	}
}

// TestRemove_HostDirectoryUnremovable_OtherCleanupProceeds verifies that if
// deleting the host-side project directory fails, a warning is printed to stderr
// containing the text "Failed to clean up host directory", the failure is aggregated,
// and container/volume/config cleanup continues.
func TestRemove_HostDirectoryUnremovable_OtherCleanupProceeds(t *testing.T) {
	// Given a host-side project directory with a nested file
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := config.Save("myapp", &config.Config{Mounts: []string{"/data"}}); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	tempDir := t.TempDir()
	ensureDataDir := func(subdir string) (string, error) {
		p := filepath.Join(tempDir, subdir)
		if err := os.MkdirAll(p, 0o700); err != nil {
			return "", err
		}
		return p, nil
	}

	projectDir, err := ensureDataDir("projects/myapp")
	if err != nil {
		t.Fatalf("failed to create projects/myapp: %v", err)
	}

	// Create a nested file
	filePath := filepath.Join(projectDir, "locked_file.txt")
	if err := os.WriteFile(filePath, []byte("locked"), 0o600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	runner := &fakeRunner{exists: true, running: false}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: ensureDataDir,
		SharedDataPath:      func(subdir string) string { return filepath.Join(tempDir, subdir) },
		RemoveAll:           func(string) error { return errors.New("simulated failure") },
	}

	outBuf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	root := cmd.NewRootCmd(deps)
	root.SetOut(outBuf)
	root.SetErr(errBuf)

	// When marshal remove is executed
	root.SetArgs([]string{"--project", "myapp", "remove"})
	err = root.Execute()

	// Then the operation fails (because we aggregate the delete error)
	assertError(t, err)

	// And the warning is printed to stderr
	assertContains(t, errBuf.String(), "Failed to clean up host directory")

	// And the error is aggregated
	assertContains(t, err.Error(), "removing host directory")

	// And the rest of the best-effort cleanup still succeeded (e.g. config deleted)
	cfg, loadErr := config.Load("myapp")
	if loadErr != nil {
		t.Fatalf("Load after remove failed: %v", loadErr)
	}
	if len(cfg.Mounts) != 0 {
		t.Errorf("expected config to be deleted despite host dir cleanup failure, got mounts: %v", cfg.Mounts)
	}
}

// TestRemove_ProjectDirIsSymlink_AbortsAndPreservesTarget verifies that if
// the host-side project directory is a symlink pointing outside the marshal
// data dir, marshal remove aborts with a symlink-related error and does NOT
// delete the symlink target or its contents.
func TestRemove_ProjectDirIsSymlink_RefusesToDeleteTarget(t *testing.T) {
	// Given the host-side project directory "projects/myapp" is a symlink
	// pointing to a target directory outside the marshal data dir, and the
	// target contains a sentinel file
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	tempDir := t.TempDir()
	targetDir := t.TempDir() // outside the marshal data dir

	// Plant a sentinel file inside the symlink target so we can prove the
	// target was not deleted.
	sentinelPath := filepath.Join(targetDir, "must-survive.txt")
	if err := os.WriteFile(sentinelPath, []byte("survives"), 0o600); err != nil {
		t.Fatalf("failed to write sentinel: %v", err)
	}

	// Create the symlink: marshal data dir's "projects/myapp" -> targetDir.
	projectLink := filepath.Join(tempDir, "projects", "myapp")
	if err := os.MkdirAll(filepath.Dir(projectLink), 0o700); err != nil {
		t.Fatalf("failed to create projects dir: %v", err)
	}
	if err := os.Symlink(targetDir, projectLink); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	runner := &fakeRunner{exists: true, running: false}
	deps := cmd.Deps{
		Runner:         runner,
		ExecFn:         (&fakeExec{}).exec,
		Getwd:          func() (string, error) { return "/projects/myapp", nil },
		Getuid:         stubGetuid,
		Getgid:         stubGetgid,
		SharedDataPath: func(subdir string) string { return filepath.Join(tempDir, subdir) },
	}

	root := cmd.NewRootCmd(deps)
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})

	// When marshal remove is executed
	root.SetArgs([]string{"--project", "myapp", "remove"})
	err := root.Execute()

	// Then the command aborts with an error containing "symlink"
	assertError(t, err)
	assertContains(t, strings.ToLower(err.Error()), "symlink")

	// And the symlink target directory and its sentinel file still exist
	if _, statErr := os.Stat(targetDir); statErr != nil {
		t.Errorf("expected symlink target %s to still exist, got: %v", targetDir, statErr)
	}
	if _, statErr := os.Stat(sentinelPath); statErr != nil {
		t.Errorf("expected sentinel file %s to still exist, got: %v", sentinelPath, statErr)
	}
}

// TestRemove_OneProject_DoesNotAffectOthersHostData verifies that removing
// project "alpha" does not delete "beta" files on the host.
func TestRemove_OneProject_DoesNotAffectOthersHostData(t *testing.T) {
	// Given two projects, "alpha" and "beta", with host-side directories
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	tempDir := t.TempDir()
	ensureDataDir := func(subdir string) (string, error) {
		p := filepath.Join(tempDir, subdir)
		if err := os.MkdirAll(p, 0o700); err != nil {
			return "", err
		}
		return p, nil
	}

	alphaDir, err := ensureDataDir("projects/alpha")
	if err != nil {
		t.Fatalf("failed to create projects/alpha: %v", err)
	}
	betaDir, err := ensureDataDir("projects/beta")
	if err != nil {
		t.Fatalf("failed to create projects/beta: %v", err)
	}

	// Write files inside both
	alphaFile := filepath.Join(alphaDir, "file.txt")
	betaFile := filepath.Join(betaDir, "file.txt")
	if err := os.WriteFile(alphaFile, []byte("alpha"), 0o600); err != nil {
		t.Fatalf("failed to write alpha file: %v", err)
	}
	if err := os.WriteFile(betaFile, []byte("beta"), 0o600); err != nil {
		t.Fatalf("failed to write beta file: %v", err)
	}

	runner := &fakeRunner{exists: true, running: false}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/alpha", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: ensureDataDir,
		SharedDataPath:      func(subdir string) string { return filepath.Join(tempDir, subdir) },
	}

	buf := &bytes.Buffer{}
	root := cmd.NewRootCmd(deps)
	root.SetOut(buf)

	// When marshal remove is executed for alpha
	root.SetArgs([]string{"--project", "alpha", "remove"})
	assertNoError(t, root.Execute())

	// Then alphaDir is deleted
	if _, err := os.Stat(alphaDir); !os.IsNotExist(err) {
		t.Errorf("expected alphaDir to be deleted, got err: %v", err)
	}

	// But betaDir remains untouched
	if _, err := os.Stat(betaDir); err != nil {
		t.Errorf("expected betaDir to remain untouched, got err: %v", err)
	}
	content, err := os.ReadFile(betaFile)
	if err != nil {
		t.Fatalf("failed to read beta file: %v", err)
	}
	if string(content) != "beta" {
		t.Errorf("expected beta file content to be 'beta', got: %q", string(content))
	}
}

// TestRemove_HostProjectDirectoryRemovalIsLogged verifies that an INFO log
// entry is emitted when marshal remove successfully deletes the host-side
// project directory. This makes the destructive directory removal as visible
// in the log trail as the container removal (which is already logged at INFO).
func TestRemove_HostProjectDirectoryRemovalIsLogged(t *testing.T) {
	// Given a project "myapp" with a running container and an existing
	// host-side data directory "projects/myapp"
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	tempDir := t.TempDir()
	ensureDataDir := func(subdir string) (string, error) {
		p := filepath.Join(tempDir, subdir)
		if err := os.MkdirAll(p, 0o700); err != nil {
			return "", err
		}
		return p, nil
	}

	projectDir, err := ensureDataDir("projects/myapp")
	if err != nil {
		t.Fatalf("failed to create projects/myapp: %v", err)
	}

	runner := &fakeRunner{exists: true, running: true}
	spy := &recordingSlogHandler{}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: ensureDataDir,
		SharedDataPath:      func(subdir string) string { return filepath.Join(tempDir, subdir) },
		Logger:              slog.New(spy),
	}

	buf := &bytes.Buffer{}
	root := cmd.NewRootCmd(deps)
	root.SetOut(buf)

	// When marshal remove is executed for project "myapp"
	root.SetArgs([]string{"--project", "myapp", "remove"})
	assertNoError(t, root.Execute())

	// Then the host project directory is removed (sanity check)
	if _, statErr := os.Stat(projectDir); !os.IsNotExist(statErr) {
		t.Errorf("expected host project directory to be removed, got err: %v", statErr)
	}

	// And the logger received an INFO-level record whose message indicates
	// the host project directory was removed for "myapp".
	rec, ok := spy.findInfoRecord("removed host project directory")
	if !ok {
		t.Fatalf("expected an INFO log record indicating host project directory removal, got %d record(s): %s",
			spy.recordCount(), spy.describeRecords())
	}
	if got := recordAttr(rec, "project"); got != "myapp" {
		t.Errorf("expected INFO log to carry project=myapp, got %q", got)
	}
}

// TestRemove_HostDirectoryDeleteFails_LogsNotRemoved verifies that when the
// host-side project directory cannot be removed, the misleading
// "removed host project directory" INFO log is NOT emitted — only the warning
// to stderr and the aggregated error are. A spurious success log would
// contradict the warning and confuse post-mortem analysis.
func TestRemove_HostDirectoryDeleteFails_LogsNotRemoved(t *testing.T) {
	// Given a host-side project directory with a nested file
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := config.Save("myapp", &config.Config{Mounts: []string{"/data"}}); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	tempDir := t.TempDir()
	ensureDataDir := func(subdir string) (string, error) {
		p := filepath.Join(tempDir, subdir)
		if err := os.MkdirAll(p, 0o700); err != nil {
			return "", err
		}
		return p, nil
	}

	projectDir, err := ensureDataDir("projects/myapp")
	if err != nil {
		t.Fatalf("failed to create projects/myapp: %v", err)
	}

	// Create a nested file
	filePath := filepath.Join(projectDir, "locked_file.txt")
	if err := os.WriteFile(filePath, []byte("locked"), 0o600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	runner := &fakeRunner{exists: true, running: false}
	spy := &recordingSlogHandler{}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: ensureDataDir,
		SharedDataPath:      func(subdir string) string { return filepath.Join(tempDir, subdir) },
		RemoveAll:           func(string) error { return errors.New("simulated failure") },
		Logger:              slog.New(spy),
	}

	outBuf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	root := cmd.NewRootCmd(deps)
	root.SetOut(outBuf)
	root.SetErr(errBuf)

	// When marshal remove is executed
	root.SetArgs([]string{"--project", "myapp", "remove"})
	assertError(t, root.Execute())

	// Sanity: the warning to stderr still fired
	assertContains(t, errBuf.String(), "Failed to clean up host directory")

	// Then no INFO record claiming the directory was removed is emitted.
	if _, ok := spy.findInfoRecord("removed host project directory"); ok {
		t.Errorf("expected NO 'removed host project directory' INFO log on the failure path, got records: %s", spy.describeRecords())
	}
}
