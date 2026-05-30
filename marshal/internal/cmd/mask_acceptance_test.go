// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rob-broadley/ai-airbase/marshal/internal/cmd"
	"github.com/rob-broadley/ai-airbase/marshal/internal/config"
)

// TestCreate_MaskFlag_SingleMask:
// marshal create --mask .venv saves .venv as a resolved absolute path under
// the mount root in cfg.Masks and succeeds.
func TestCreate_MaskFlag_SingleMask(t *testing.T) {
	// Given no container exists; CWD is used as the single mount root
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: false, imageExistsResult: true}
	fe := &fakeExec{}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              fe.exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
		MkdirAll:            noopMkdirAll,
	}

	root := cmd.NewRootCmd(deps)
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "create", "--mask", ".venv"})

	// When the create subcommand is executed
	assertNoError(t, root.Execute())

	// Then the container is created
	if !runner.calledSubcommand("create") {
		t.Error("expected 'podman create' to be called")
	}

	// And cfg.Masks contains the resolved absolute path
	cfg, err := config.Load("myapp")
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	wantMask := "/projects/myapp/.venv"
	if len(cfg.Masks) != 1 || cfg.Masks[0] != wantMask {
		t.Errorf("expected cfg.Masks = [%q], got %v", wantMask, cfg.Masks)
	}
}

// TestCreate_MaskFlag_MultipleMasks:
// marshal create --mask .venv --mask node_modules saves both masks.
func TestCreate_MaskFlag_MultipleMasks(t *testing.T) {
	// Given no container exists; CWD is used as the single mount root
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: false, imageExistsResult: true}
	fe := &fakeExec{}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              fe.exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
		MkdirAll:            noopMkdirAll,
	}

	root := cmd.NewRootCmd(deps)
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "create", "--mask", ".venv", "--mask", "node_modules"})

	// When the create subcommand is executed
	assertNoError(t, root.Execute())

	// Then cfg.Masks contains both resolved absolute paths
	cfg, err := config.Load("myapp")
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if len(cfg.Masks) != 2 {
		t.Fatalf("expected 2 masks, got %d: %v", len(cfg.Masks), cfg.Masks)
	}
	wantMasks := map[string]bool{
		"/projects/myapp/.venv":        true,
		"/projects/myapp/node_modules": true,
	}
	for _, m := range cfg.Masks {
		if !wantMasks[m] {
			t.Errorf("unexpected mask %q", m)
		}
	}
}

// TestCreate_MaskFlag_InvalidMasks:
// An invalid mask returns an error and does NOT create the container.
func TestCreate_MaskFlag_InvalidMasks(t *testing.T) {
	for _, tc := range []struct {
		name    string
		masks   []string
		wantErr string
	}{
		{
			name:    "absolute path not under mount rejected",
			masks:   []string{"/abs"},
			wantErr: "no configured mount",
		},
		{
			name:    "colon in path rejected",
			masks:   []string{"node:mod"},
			wantErr: "must not contain",
		},
		{
			name:    "dotdot escape rejected",
			masks:   []string{"../../other"},
			wantErr: "no configured mount",
		},
		{
			name:    "duplicate masks rejected",
			masks:   []string{".venv", ".venv"},
			wantErr: "duplicate",
		},
		{
			name:    "nested masks rejected",
			masks:   []string{"src", "src/vendor"},
			wantErr: "nested inside",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Given no container exists
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())

			runner := &fakeRunner{exists: false, imageExistsResult: true}
			fe := &fakeExec{}
			deps := cmd.Deps{
				Runner:              runner,
				ExecFn:              fe.exec,
				Getwd:               func() (string, error) { return "/projects/myapp", nil },
				Getuid:              stubGetuid,
				Getgid:              stubGetgid,
				EnsureSharedDataDir: stubEnsureSharedDataDir(t),
			}

			args := []string{"--project", "myapp", "create"}
			for _, m := range tc.masks {
				args = append(args, "--mask", m)
			}

			root := cmd.NewRootCmd(deps)
			root.SetErr(&bytes.Buffer{})
			root.SetArgs(args)

			// When the create subcommand is executed
			err := root.Execute()

			// Then an error is returned
			assertError(t, err)
			assertContains(t, err.Error(), tc.wantErr)

			// And the container is NOT created
			if runner.calledSubcommand("create") {
				t.Error("expected 'podman create' NOT to be called on invalid mask")
			}
		})
	}
}

// TestCreate_MaskFlag_NoMask:
// marshal create with no --mask flag succeeds with an empty cfg.Masks.
func TestCreate_MaskFlag_NoMask(t *testing.T) {
	// Given no container exists and no --mask flag is supplied
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: false, imageExistsResult: true}
	fe := &fakeExec{}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              fe.exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
	}

	root := cmd.NewRootCmd(deps)
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "create"})

	// When the create subcommand is executed
	assertNoError(t, root.Execute())

	// Then cfg.Masks is empty (zero masks stored)
	cfg, err := config.Load("myapp")
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if len(cfg.Masks) != 0 {
		t.Errorf("expected cfg.Masks to be empty, got %v", cfg.Masks)
	}
}

// TestCreate_MaskFlag_MultiMount:
// marshal create --mount ./foo --mount ./bar --mask bar/.venv succeeds with
// two project mounts; the mask resolves relative to CWD under the second mount.
func TestCreate_MaskFlag_MultiMount(t *testing.T) {
	// Given no container exists; CWD is /projects/myapp; two mounts: foo and bar
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: false, imageExistsResult: true}
	fe := &fakeExec{}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              fe.exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
		MkdirAll:            noopMkdirAll,
	}

	root := cmd.NewRootCmd(deps)
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{
		"--project", "myapp",
		"create",
		"--mount", "./foo",
		"--mount", "./bar",
		"--mask", "bar/.venv",
	})

	// When the create subcommand is executed
	assertNoError(t, root.Execute())

	// Then the container is created
	if !runner.calledSubcommand("create") {
		t.Error("expected 'podman create' to be called")
	}

	// And cfg.Masks contains the mask resolved relative to CWD under the bar mount
	cfg, err := config.Load("myapp")
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	wantMask := "/projects/myapp/bar/.venv"
	if len(cfg.Masks) != 1 || cfg.Masks[0] != wantMask {
		t.Errorf("expected cfg.Masks = [%q], got %v", wantMask, cfg.Masks)
	}

	wantContainerPath := "/workspace/" + filepath.Base("/projects/myapp/bar") + "/.venv"
	if !runner.createArgsContain(wantContainerPath) {
		t.Errorf("expected podman create to include container path %q", wantContainerPath)
	}

}

// TestNonCreateCmds_DoNotAcceptMaskFlag:
// --mask is NOT available on recreate, shell, stop, status, remove.
func TestNonCreateCmds_DoNotAcceptMaskFlag(t *testing.T) {
	// Given --mask passed to a command that is not "create"
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	for _, tc := range []struct {
		cmd  string
		args []string
	}{
		{cmd: "recreate", args: []string{"recreate", "--mask", ".venv"}},
		{cmd: "shell", args: []string{"shell", "--mask", ".venv"}},
		{cmd: "stop", args: []string{"stop", "--mask", ".venv"}},
		{cmd: "status", args: []string{"status", "--mask", ".venv"}},
		{cmd: "remove", args: []string{"remove", "--mask", ".venv"}},
	} {
		t.Run(strings.Join(tc.args, "_"), func(t *testing.T) {
			runner := &fakeRunner{exists: true, running: true}
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
			root.SetArgs(append([]string{"--project", "myapp"}, tc.args...))

			// When the command is executed
			err := root.Execute()

			if err == nil {
				t.Fatalf("subcommand %q: expected error for unknown --mask flag, got nil", tc.cmd)
			}
			if !strings.Contains(err.Error(), "unknown flag: --mask") {
				t.Errorf("subcommand %q: expected 'unknown flag: --mask' error, got: %v", tc.cmd, err)
			}
			if len(runner.calls) != 0 {
				t.Errorf("subcommand %q: expected no podman calls, got: %v", tc.cmd, runner.calls)
			}
		})
	}
}
