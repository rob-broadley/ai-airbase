// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/rob-broadley/ai-airbase/marshal/internal/cmd"
	"github.com/rob-broadley/ai-airbase/marshal/internal/config"
)

// volumeCreateCalledFor checks if "podman volume create ... <name>" was called.
func volumeCreateCalledFor(runner *fakeRunner, name string) bool {
	for _, call := range runner.calls {
		if len(call) >= 2 && call[0] == "podman" && call[1] == "volume" {
			for _, arg := range call[2:] {
				if arg == name {
					return true
				}
			}
		}
	}
	return false
}

// TestMaskVolumes_SingleMask verifies that marshal create --mask .venv
// causes podman volume create ... marshal-myapp-mask-myapp-.venv to be called.
func TestMaskVolumes_SingleMask(t *testing.T) {
	// Given no container exists; CWD /projects/myapp is the single mount root
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

	// Then podman volume create is called for marshal-myapp-mask-myapp-.venv
	wantVolume := "marshal-myapp-mask-myapp-.venv"
	if !volumeCreateCalledFor(runner, wantVolume) {
		t.Errorf("expected 'podman volume create ... %s' to be called; calls: %v", wantVolume, runner.calls)
	}
	// And podman create received -v <volume>:<container-path>
	wantVFlag := "marshal-myapp-mask-myapp-.venv:/workspace/myapp/.venv"
	if !runner.createArgsContain(wantVFlag) {
		t.Errorf("expected 'podman create' to receive arg %q; createArgs: %v", wantVFlag, runner.createArgs())
	}
}

// TestMaskVolumes_PathSeparatorSanitised verifies that marshal create
// --mask src/vendor produces volume marshal-myapp-mask-myapp-src-vendor (/ → -).
func TestMaskVolumes_PathSeparatorSanitised(t *testing.T) {
	// Given no container exists; CWD is the single mount root
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
	root.SetArgs([]string{"--project", "myapp", "create", "--mask", "src/vendor"})

	// When the create subcommand is executed
	assertNoError(t, root.Execute())

	// Then podman volume create is called for marshal-myapp-mask-myapp-src-vendor
	wantVolume := "marshal-myapp-mask-myapp-src-vendor"
	if !volumeCreateCalledFor(runner, wantVolume) {
		t.Errorf("expected 'podman volume create ... %s' to be called; calls: %v", wantVolume, runner.calls)
	}
	// And podman create received -v <volume>:<container-path>
	wantVFlag := "marshal-myapp-mask-myapp-src-vendor:/workspace/myapp/src/vendor"
	if !runner.createArgsContain(wantVFlag) {
		t.Errorf("expected 'podman create' to receive arg %q; createArgs: %v", wantVFlag, runner.createArgs())
	}
}

// TestMaskVolumes_TwoMasks verifies that two --mask flags each trigger
// a separate volume create call.
func TestMaskVolumes_TwoMasks(t *testing.T) {
	// Given no container exists and two mask flags
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

	// Then two mask volume creates are called
	for _, wantVolume := range []string{
		"marshal-myapp-mask-myapp-.venv",
		"marshal-myapp-mask-myapp-node_modules",
	} {
		if !volumeCreateCalledFor(runner, wantVolume) {
			t.Errorf("expected 'podman volume create ... %s' to be called; calls: %v", wantVolume, runner.calls)
		}
	}
	// And podman create received -v flags for both masks
	for _, wantVFlag := range []string{
		"marshal-myapp-mask-myapp-.venv:/workspace/myapp/.venv",
		"marshal-myapp-mask-myapp-node_modules:/workspace/myapp/node_modules",
	} {
		if !runner.createArgsContain(wantVFlag) {
			t.Errorf("expected 'podman create' to receive arg %q; createArgs: %v", wantVFlag, runner.createArgs())
		}
	}
}

// TestMaskVolumes_NoMaskNoExtraVolumeCreate verifies that marshal create
// with no --mask flag does not produce any mask volume create calls beyond
// those for image-declared volumes.
func TestMaskVolumes_NoMask_NoMaskVolumes(t *testing.T) {
	// Given no container exists and no --mask flag
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

	// Then no mask volume is created
	if volumeCreateCalledFor(runner, "marshal-myapp-mask-") {
		t.Error("expected no mask volume create calls, but found one")
	}
	// Count volume create calls — only the image-declared-volume path should run
	// (which uses volume ls + volume create for each image spec; with empty
	// imageInspectVolumeJSON default we get zero image-declared volumes too).
	for _, call := range runner.calls {
		if len(call) >= 2 && call[0] == "podman" && call[1] == "volume" && len(call) >= 3 && call[2] == "create" {
			// Check none of the volume names start with "marshal-myapp-mask-"
			for _, arg := range call[3:] {
				if len(arg) > 0 && arg[0] != '-' {
					// last non-flag arg is the volume name
					if len(arg) >= len("marshal-myapp-mask-") &&
						arg[:len("marshal-myapp-mask-")] == "marshal-myapp-mask-" {
						t.Errorf("unexpected mask volume create call for %q", arg)
					}
				}
			}
		}
	}
}

// TestMaskVolumes_RecreateProvisionsMaskVolumes verifies that marshal
// recreate on a project with masks in config provisions the mask volumes
// before creating the replacement container.
func TestMaskVolumes_Recreate_ProvisionsMaskVolumes(t *testing.T) {
	// Given a project config with a mask already saved
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := config.Save("myapp", &config.Config{
		Mounts: []string{"/projects/myapp"},
		Masks:  []string{"/projects/myapp/.venv"},
	}); err != nil {
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
		MkdirAll:            noopMkdirAll,
	}

	root := cmd.NewRootCmd(deps)
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "recreate"})

	// When recreate is executed
	assertNoError(t, root.Execute())

	// Then the mask volume is provisioned
	wantVolume := "marshal-myapp-mask-myapp-.venv"
	if !volumeCreateCalledFor(runner, wantVolume) {
		t.Errorf("expected 'podman volume create ... %s' to be called during recreate; calls: %v", wantVolume, runner.calls)
	}
	// And podman create received -v <volume>:<container-path>
	wantVFlag := "marshal-myapp-mask-myapp-.venv:/workspace/myapp/.venv"
	if !runner.createArgsContain(wantVFlag) {
		t.Errorf("expected 'podman create' to receive arg %q; createArgs: %v", wantVFlag, runner.createArgs())
	}
}

// TestMaskVolumes_LabelApplied verifies that the mask volume create call
// carries the io.ai-airbase.project label (via EnsureProjectVolume path).
func TestMaskVolumes_LabelApplied(t *testing.T) {
	// Given no container exists and a single mask
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

	// Then the volume create call carries the project label
	wantVolume := "marshal-myapp-mask-myapp-.venv"
	wantLabel := "io.ai-airbase.project=marshal-myapp"
	labelFound := false
	for _, call := range runner.calls {
		if len(call) >= 2 && call[0] == "podman" && call[1] == "volume" {
			hasName := false
			hasLabel := false
			for i, arg := range call {
				if arg == wantVolume {
					hasName = true
				}
				if arg == "--label" && i+1 < len(call) && call[i+1] == wantLabel {
					hasLabel = true
				}
			}
			if hasName && hasLabel {
				labelFound = true
				break
			}
		}
	}
	if !labelFound {
		t.Errorf("expected volume create for %q to carry label %q; calls: %v", wantVolume, wantLabel, runner.calls)
	}
	// And podman create received -v <volume>:<container-path>
	wantVFlag := "marshal-myapp-mask-myapp-.venv:/workspace/myapp/.venv"
	if !runner.createArgsContain(wantVFlag) {
		t.Errorf("expected 'podman create' to receive arg %q; createArgs: %v", wantVFlag, runner.createArgs())
	}
}

// TestCreate_MaskVolumeProvisionFailure verifies that a mask-volume creation
// failure aborts create before any podman create call and reports the host path.
func TestCreate_MaskVolumeProvisionFailure(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{
		exists:            false,
		imageExistsResult: true,
		runErrors:         map[string]error{"volume": errors.New("volume create failed")},
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
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "create", "--mask", ".venv"})

	err := root.Execute()
	assertError(t, err)
	assertContains(t, err.Error(), "/projects/myapp/.venv")
	if runner.calledSubcommand("create") {
		t.Errorf("expected podman create NOT to be called after mask volume provisioning failure; calls: %v", runner.calls)
	}
}

// TestRecreate_MaskVolumeProvisionFailure verifies that a mask-volume creation
// failure aborts recreate before any podman create call and reports the host path.
func TestRecreate_MaskVolumeProvisionFailure(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := config.Save("myapp", &config.Config{
		Mounts: []string{"/projects/myapp"},
		Masks:  []string{"/projects/myapp/.venv"},
	}); err != nil {
		t.Fatalf("saving config: %v", err)
	}

	runner := &fakeRunner{
		exists:            true,
		running:           false,
		imageExistsResult: true,
		runErrors:         map[string]error{"volume": errors.New("volume create failed")},
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
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "recreate"})

	err := root.Execute()
	assertError(t, err)
	assertContains(t, err.Error(), "/projects/myapp/.venv")
	if runner.calledSubcommand("create") {
		t.Errorf("expected podman create NOT to be called after mask volume provisioning failure; calls: %v", runner.calls)
	}
}
