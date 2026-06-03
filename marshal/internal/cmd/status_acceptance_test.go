// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/rob-broadley/ai-airbase/marshal/internal/cmd"
	"github.com/rob-broadley/ai-airbase/marshal/internal/config"
)

// TestStatus_Running verifies that status reports project, container name,
// running state, image, image ref, image digest, version, and creation date for a running container.
func TestStatus_Running(t *testing.T) {
	// Given a running container with image, digest, version label, image ref, and creation metadata
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: true, running: true, image: "20232757d1f59e6e733cd1cd3d8a35a87e24524a17b75543499dddc6c8a4369c", created: "2024-06-01", imageVersion: "1.2.3", imageDigest: "sha256:abc123", imageRef: "ghcr.io/rob-broadley/ai-airbase/revetment:latest"}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
		LoadConfig: func(project string) (*config.Config, error) {
			return &config.Config{}, nil
		},
	}

	buf := &bytes.Buffer{}
	root := cmd.NewRootCmd(deps)
	root.SetOut(buf)

	// When the status subcommand is executed
	root.SetArgs([]string{"--project", "myapp", "status"})
	assertNoError(t, root.Execute())

	// Then the output contains project, container, state, image, image ref, digest, version, and created date
	out := buf.String()
	assertContains(t, out, "myapp")
	assertContains(t, out, "marshal-myapp")
	assertContains(t, out, "running")
	assertContains(t, out, "Image ID:     20232757d1f59e6e733cd1cd3d8a35a87e24524a17b75543499dddc6c8a4369c")
	assertContains(t, out, "Image Ref:    ghcr.io/rob-broadley/ai-airbase/revetment:latest")
	assertContains(t, out, "Image Digest: sha256:abc123")
	assertContains(t, out, "Version:      1.2.3")
	assertContains(t, out, "Created:      2024-06-01")
	// Image Ref: must appear between Status: and Image ID:
	statusIdx := strings.Index(out, "Status:")
	imageRefIdx := strings.Index(out, "Image Ref:")
	imageIDIdx := strings.Index(out, "Image ID:")
	if statusIdx < 0 || imageRefIdx < 0 || imageIDIdx < 0 {
		t.Fatalf("expected Status:, Image Ref:, and Image ID: lines in output; got:\n%s", out)
	}
	if statusIdx >= imageRefIdx || imageRefIdx >= imageIDIdx {
		t.Errorf("expected Image Ref: between Status: and Image ID: in output; got:\n%s", out)
	}
}

// TestStatus_Running_ImageDigestAbsent verifies that when the container exists
// but the image has no digest (e.g. a locally built image), the Image Digest:
// line shows a dash placeholder.
func TestStatus_Running_ImageDigestAbsent(t *testing.T) {
	// Given a running container whose image has no digest
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: true, running: true, image: "20232757d1f59e6e733cd1cd3d8a35a87e24524a17b75543499dddc6c8a4369c", created: "2024-06-01", imageVersion: "1.2.3", imageDigest: ""}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
		LoadConfig: func(project string) (*config.Config, error) {
			return &config.Config{}, nil
		},
	}

	buf := &bytes.Buffer{}
	root := cmd.NewRootCmd(deps)
	root.SetOut(buf)

	// When the status subcommand is executed
	root.SetArgs([]string{"--project", "myapp", "status"})
	assertNoError(t, root.Execute())

	// Then the Image Digest: line shows a dash, and surrounding fields are unaffected
	out := buf.String()
	assertContains(t, out, "Status:       running")
	assertContains(t, out, "Image ID:     20232757d1f59e6e733cd1cd3d8a35a87e24524a17b75543499dddc6c8a4369c")
	assertContains(t, out, "Image Ref:    -")
	assertContains(t, out, "Image Digest: -")
	assertContains(t, out, "Version:      1.2.3")
}

// TestStatus_Running_VersionLabelNotSet verifies that when the container exists
// but the org.opencontainers.image.version label is absent (e.g. a locally
// built image), the Version: line shows a dash placeholder.
func TestStatus_Running_VersionLabelNotSet(t *testing.T) {
	// Given a running container whose image has no version label
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: true, running: true, image: "20232757d1f59e6e733cd1cd3d8a35a87e24524a17b75543499dddc6c8a4369c", created: "2024-06-01", imageVersion: ""}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
		LoadConfig: func(project string) (*config.Config, error) {
			return &config.Config{}, nil
		},
	}

	buf := &bytes.Buffer{}
	root := cmd.NewRootCmd(deps)
	root.SetOut(buf)

	// When the status subcommand is executed
	root.SetArgs([]string{"--project", "myapp", "status"})
	assertNoError(t, root.Execute())

	// Then the Version: and Image Digest: lines are present and show dash placeholders
	out := buf.String()
	assertContains(t, out, "Image Ref:    -")
	assertContains(t, out, "Image Digest: -")
	assertContains(t, out, "Version:      -")
}

// TestStatus_Absent verifies that when no container exists, all output fields
// show dash placeholders.
func TestStatus_Absent(t *testing.T) {
	// Given no container exists
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: false, running: false}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
		LoadConfig: func(project string) (*config.Config, error) {
			return &config.Config{}, nil
		},
	}

	buf := &bytes.Buffer{}
	root := cmd.NewRootCmd(deps)
	root.SetOut(buf)

	// When the status subcommand is executed
	root.SetArgs([]string{"--project", "myapp", "status"})
	assertNoError(t, root.Execute())

	// Then the output reports absent and all fields show dash placeholders
	out := buf.String()
	assertContains(t, out, "absent")
	assertContains(t, out, "Image ID:     -")
	assertContains(t, out, "Image Ref:    -")
	assertContains(t, out, "Image Digest: -")
	assertContains(t, out, "Version:      -")
	assertContains(t, out, "Created:      -")
}

// TestStatus_Stopped verifies that status reports stopped state for a
// non-running container.
func TestStatus_Stopped(t *testing.T) {
	// Given a stopped container
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: true, running: false, image: "20232757d1f59e6e733cd1cd3d8a35a87e24524a17b75543499dddc6c8a4369c", created: "2024-06-01"}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
		LoadConfig: func(project string) (*config.Config, error) {
			return &config.Config{}, nil
		},
	}

	buf := &bytes.Buffer{}
	root := cmd.NewRootCmd(deps)
	root.SetOut(buf)

	// When the status subcommand is executed
	root.SetArgs([]string{"--project", "myapp", "status"})
	assertNoError(t, root.Execute())

	// Then the output contains stopped, image metadata, and empty mount/mask placeholders
	out := buf.String()
	assertContains(t, out, "Status:       stopped")
	assertContains(t, out, "Image ID:     20232757d1f59e6e733cd1cd3d8a35a87e24524a17b75543499dddc6c8a4369c")
	assertContains(t, out, "Image Ref:    -")
	assertContains(t, out, "Image Digest: -")
	assertContains(t, out, "Mounts:       none")
	assertContains(t, out, "Masks:        none")
}

// TestStatus_GetStatusFails verifies that an error from GetStatus (ps-all) is
// propagated back to the caller.
func TestStatus_GetStatusFails(t *testing.T) {
	// Given a runner that fails on the "ps-all" subcommand
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{runErrors: map[string]error{"ps-all": fmt.Errorf("ps failed")}}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
	}

	// When the status subcommand is executed
	root := cmd.NewRootCmd(deps)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "status"})

	// Then an error is returned
	assertError(t, root.Execute())
}

// TestStatus_LoadConfigFails_AbsentContainer verifies that a config loading
// error in the absent-container branch is propagated back to the caller.
func TestStatus_LoadConfigFails_AbsentContainer(t *testing.T) {
	// Given an absent container whose config load fails
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: false}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
		LoadConfig: func(project string) (*config.Config, error) {
			return nil, fmt.Errorf("config failed")
		},
	}

	root := cmd.NewRootCmd(deps)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "status"})

	// When the status subcommand is executed
	err := root.Execute()

	// Then an error is returned containing "loading config"
	assertError(t, err)
	assertContains(t, err.Error(), "loading config")
}

// TestStatus_LoadConfigFails_RunningContainer verifies that a config loading
// error in the existing-container branch is propagated back to the caller.
func TestStatus_LoadConfigFails_RunningContainer(t *testing.T) {
	// Given a running container whose config load fails
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: true, running: true, image: "abc123", created: "2024-06-01"}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
		LoadConfig: func(project string) (*config.Config, error) {
			return nil, fmt.Errorf("config failed")
		},
	}

	root := cmd.NewRootCmd(deps)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "status"})

	// When the status subcommand is executed
	err := root.Execute()

	// Then an error is returned containing "loading config"
	assertError(t, err)
	assertContains(t, err.Error(), "loading config")
}

// TestStatus_GetMountsFails verifies that an error from GetMounts for an
// existing container is propagated back to the caller.
func TestStatus_GetMountsFails(t *testing.T) {
	// Given a running container whose mounts inspect call fails
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{
		exists:    true,
		running:   true,
		image:     "abc123",
		created:   "2024-06-01",
		runErrors: map[string]error{"inspect-mounts": fmt.Errorf("inspect mounts failed")},
	}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
		LoadConfig: func(project string) (*config.Config, error) {
			return &config.Config{Mounts: []string{}, Masks: []string{}}, nil
		},
	}

	// When the status subcommand is executed
	root := cmd.NewRootCmd(deps)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "status"})

	// Then an error is returned
	err := root.Execute()
	assertError(t, err)
	assertContains(t, err.Error(), "getting container mounts")
}

// TestStatus_InvalidProjectName verifies that status returns an error when the
// --project flag contains an invalid project name (e.g. path traversal).
func TestStatus_InvalidProjectName(t *testing.T) {
	// Given a runner that would succeed if reached
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

	// When the status subcommand is executed with an invalid project name
	root.SetArgs([]string{"--project", "../evil", "status"})
	err := root.Execute()

	// Then an error is returned containing "invalid project"
	assertError(t, err)
	assertContains(t, err.Error(), "invalid project")
}

// ---------------------------------------------------------------------------
// Mounts and Masks section
// ---------------------------------------------------------------------------

// TestStatus_MountsMasks_AbsentContainer_WithConfig verifies that when the
// container is absent but the project config has mounts and masks configured,
// the status output lists them without symbols or a legend line.
func TestStatus_MountsMasks_AbsentContainer_WithConfig(t *testing.T) {
	// Given a project with configured mounts and masks, and the container is absent
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: false, running: false}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
		LoadConfig: func(project string) (*config.Config, error) {
			return &config.Config{
				Mounts: []string{"/home/user/project"},
				Masks:  []string{"/home/user/project/secrets"},
			}, nil
		},
	}

	buf := &bytes.Buffer{}
	root := cmd.NewRootCmd(deps)
	root.SetOut(buf)

	// When the status subcommand is executed
	root.SetArgs([]string{"--project", "myapp", "status"})
	assertNoError(t, root.Execute())

	// Then the output contains a Mounts section with the configured path
	// and a Masks section with the configured path — no symbols, no legend
	out := buf.String()
	assertContains(t, out, "Mounts:\n  /home/user/project\n")
	assertContains(t, out, "Masks:\n  /home/user/project/secrets\n")
	assertNotContains(t, out, "✓")
	assertNotContains(t, out, "✗")
	assertNotContains(t, out, "?")
	assertNotContains(t, out, "active")
	assertNotContains(t, out, "missing")
	assertNotContains(t, out, "not managed")
}

// TestStatus_MountsMasks_RunningContainer_AllActive verifies that when the
// container is running and all configured mounts and masks are present in the
// container, the status output shows each entry with an active symbol.
func TestStatus_MountsMasks_RunningContainer_AllActive(t *testing.T) {
	// Given a running container with a bind mount for the configured mount
	// and a volume mount for the configured mask, both active
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{
		exists:  true,
		running: true,
		image:   "abc123",
		created: "2024-06-01",
		containerMountsJSON: `[
			{"Type":"bind","Source":"/home/user/project","Destination":"/workspace/project"},
			{"Type":"volume","Source":"marshal-myapp-project-secrets","Destination":"/workspace/project/secrets"}
		]`,
	}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
		LoadConfig: func(project string) (*config.Config, error) {
			return &config.Config{
				Mounts: []string{"/home/user/project"},
				Masks:  []string{"/home/user/project/secrets"},
			}, nil
		},
	}

	buf := &bytes.Buffer{}
	root := cmd.NewRootCmd(deps)
	root.SetOut(buf)

	// When the status subcommand is executed
	root.SetArgs([]string{"--project", "myapp", "status"})
	assertNoError(t, root.Execute())

	// Then the output contains active symbols for both mount and mask; no legend line
	out := buf.String()
	assertContains(t, out, "Mounts:\n  ✓ /home/user/project\n")
	assertContains(t, out, "Masks:\n  ✓ /home/user/project/secrets\n")
	assertNotContains(t, out, "✗")
	assertNotContains(t, out, "?")
	assertNotContains(t, out, "active")
	assertNotContains(t, out, "missing")
	assertNotContains(t, out, "not managed")
}

// TestStatus_MountsMasks_AbsentContainer_EmptyConfig verifies that when the
// container is absent and the project config has no mounts or masks, the
// status output shows "none" placeholders for both sections.
func TestStatus_MountsMasks_AbsentContainer_EmptyConfig(t *testing.T) {
	// Given a project with no configured mounts or masks, and the container is absent
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: false, running: false}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
		LoadConfig: func(project string) (*config.Config, error) {
			return &config.Config{Mounts: []string{}, Masks: []string{}}, nil
		},
	}

	buf := &bytes.Buffer{}
	root := cmd.NewRootCmd(deps)
	root.SetOut(buf)

	// When the status subcommand is executed
	root.SetArgs([]string{"--project", "myapp", "status"})
	assertNoError(t, root.Execute())

	// Then the output contains "none" placeholders for Mounts and Masks, with no symbols or legend
	out := buf.String()
	assertContains(t, out, "Mounts:       none")
	assertContains(t, out, "Masks:        none")
	assertNotContains(t, out, "✓")
	assertNotContains(t, out, "✗")
	assertNotContains(t, out, "?")
	assertNotContains(t, out, "active")
	assertNotContains(t, out, "missing")
	assertNotContains(t, out, "not managed")
}

// TestStatus_MountsMasks_RunningContainer_MissingAndUntracked verifies that
// when the container is running but a configured mount is absent and there are
// untracked bind and volume mounts, the status output marks the missing entry
// with ✗ and labels the untracked entries with ?.
func TestStatus_MountsMasks_RunningContainer_MissingAndUntracked(t *testing.T) {
	// Given a running container where:
	//   - /home/user/other is configured but not mounted (missing)
	//   - /home/user/extra is bound into /workspace/extra (untracked bind)
	//   - a volume covers /workspace/project/logs (untracked volume)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{
		exists:  true,
		running: true,
		image:   "abc123",
		created: "2024-06-01",
		containerMountsJSON: `[
			{"Type":"bind","Source":"/home/user/extra","Destination":"/workspace/extra"},
			{"Type":"volume","Source":"some-vol","Destination":"/workspace/project/logs"}
		]`,
	}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
		LoadConfig: func(project string) (*config.Config, error) {
			return &config.Config{
				Mounts: []string{"/home/user/other"},
				Masks:  []string{},
			}, nil
		},
	}

	buf := &bytes.Buffer{}
	root := cmd.NewRootCmd(deps)
	root.SetOut(buf)

	// When the status subcommand is executed
	root.SetArgs([]string{"--project", "myapp", "status"})
	assertNoError(t, root.Execute())

	// Then the output contains:
	//   - ✗ for the configured-but-missing mount
	//   - ? for the untracked bind mount (shown by host path)
	//   - ? for the untracked volume (shown by container destination)
	//   - no legend line
	out := buf.String()
	assertContains(t, out, "✗ /home/user/other")
	assertContains(t, out, "? /home/user/extra")
	assertContains(t, out, "? /workspace/project/logs")
}

// TestStatus_UntrackedBindMount_SourceSanitized verifies that control characters
// in an untracked bind mount's host source path are stripped before terminal output.
func TestStatus_UntrackedBindMount_SourceSanitized(t *testing.T) {
	// Given a running container with an untracked bind mount whose Source contains
	// an ANSI escape sequence (ESC [ 2 J — clear-screen)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{
		exists:  true,
		running: true,
		image:   "abc123",
		created: "2024-06-01",
		containerMountsJSON: `[
			{"Type":"bind","Source":"/home/user/evil\u001b[2J","Destination":"/workspace/evil"}
		]`,
	}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
		LoadConfig: func(project string) (*config.Config, error) {
			return &config.Config{Mounts: []string{}, Masks: []string{}}, nil
		},
	}

	buf := &bytes.Buffer{}
	root := cmd.NewRootCmd(deps)
	root.SetOut(buf)

	// When the status subcommand is executed
	root.SetArgs([]string{"--project", "myapp", "status"})
	assertNoError(t, root.Execute())

	// Then the ESC character is absent from the output — the path is sanitised
	out := buf.String()
	assertNotContains(t, out, "\x1b")
	assertContains(t, out, "/home/user/evil[2J")
}

// TestStatus_DisplaysCustomPort verifies that status output displays the custom port configured
// in the project's config.
func TestStatus_DisplaysCustomPort(t *testing.T) {
	// Given a running container with custom port 5000 configured
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: true, running: true}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
		LoadConfig: func(project string) (*config.Config, error) {
			return &config.Config{
				Port: 5000,
			}, nil
		},
	}

	buf := &bytes.Buffer{}
	root := cmd.NewRootCmd(deps)
	root.SetOut(buf)

	// When the status subcommand is executed
	root.SetArgs([]string{"--project", "myapp", "status"})
	assertNoError(t, root.Execute())

	// Then the output displays Port: 5000
	out := buf.String()
	assertContains(t, out, "Port:         5000")
}

// TestStatus_UntrackedVolumeMount_DestinationSanitized verifies that control
// characters in an untracked volume mount's container destination path are
// stripped before terminal output.
func TestStatus_UntrackedVolumeMount_DestinationSanitized(t *testing.T) {
	// Given a running container with an untracked volume mount whose Destination
	// contains an ANSI escape sequence
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{
		exists:  true,
		running: true,
		image:   "abc123",
		created: "2024-06-01",
		containerMountsJSON: `[
			{"Type":"volume","Source":"some-vol","Destination":"/workspace/evil\u001b[2J"}
		]`,
	}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
		LoadConfig: func(project string) (*config.Config, error) {
			return &config.Config{Mounts: []string{}, Masks: []string{}}, nil
		},
	}

	buf := &bytes.Buffer{}
	root := cmd.NewRootCmd(deps)
	root.SetOut(buf)

	// When the status subcommand is executed
	root.SetArgs([]string{"--project", "myapp", "status"})
	assertNoError(t, root.Execute())

	// Then the ESC character is absent from the output — the path is sanitised
	out := buf.String()
	assertNotContains(t, out, "\x1b")
	assertContains(t, out, "? /workspace/evil[2J")
}

// TestStatus_AbsentContainer_ConfiguredMountSanitized verifies that control
// characters in a configured mount path are stripped before terminal output
// in the absent-container branch.
func TestStatus_AbsentContainer_ConfiguredMountSanitized(t *testing.T) {
	// Given an absent container whose config contains a mount path with an ANSI
	// escape sequence embedded in it
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: false, running: false}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
		LoadConfig: func(project string) (*config.Config, error) {
			return &config.Config{
				Mounts: []string{"/home/user/evil\x1b[2J"},
				Masks:  []string{},
			}, nil
		},
	}

	buf := &bytes.Buffer{}
	root := cmd.NewRootCmd(deps)
	root.SetOut(buf)

	// When the status subcommand is executed
	root.SetArgs([]string{"--project", "myapp", "status"})
	assertNoError(t, root.Execute())

	// Then the ESC character is absent from the output — the path is sanitised
	out := buf.String()
	assertNotContains(t, out, "\x1b")
	assertContains(t, out, "/home/user/evil[2J")
}

// TestStatus_MountsMasks_StoppedContainer verifies that when the container
// exists but is not running, every configured mount and mask entry is shown
// with a ✗ (missing) symbol, because GetMounts returns an empty list for a
// stopped container.
func TestStatus_MountsMasks_StoppedContainer(t *testing.T) {
	// Given a stopped container with configured mounts and masks, and no active
	// container mounts (stopped containers have no live mounts)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{
		exists:              true,
		running:             false,
		image:               "abc123",
		created:             "2024-06-01",
		containerMountsJSON: "[]",
	}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
		LoadConfig: func(project string) (*config.Config, error) {
			return &config.Config{
				Mounts: []string{"/home/user/project"},
				Masks:  []string{"/home/user/project/secrets"},
			}, nil
		},
	}

	buf := &bytes.Buffer{}
	root := cmd.NewRootCmd(deps)
	root.SetOut(buf)

	// When the status subcommand is executed
	root.SetArgs([]string{"--project", "myapp", "status"})
	assertNoError(t, root.Execute())

	// Then the output reports stopped and each configured mount/mask shows ✗
	// (missing), with no ✓ (active) or ? (untracked) symbols anywhere
	out := buf.String()
	assertContains(t, out, "Status:       stopped")
	assertContains(t, out, "✗ /home/user/project")
	assertContains(t, out, "✗ /home/user/project/secrets")
	assertNotContains(t, out, "✓")
	assertNotContains(t, out, "?")
}

// TestStatus_MountsMasks_RunningContainer_UntrackedMaskShowsHostPath verifies
// that a ? untracked volume mask entry displays the host-equivalent path
// (derived by reversing the configured bind mount mapping) rather than the raw
// container destination path.
func TestStatus_MountsMasks_RunningContainer_UntrackedMaskShowsHostPath(t *testing.T) {
	// Given a running container where:
	//   - /home/user/project is a configured mount, active (bound to /workspace/project)
	//   - a volume covers /workspace/project/secrets (untracked — not in config masks)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{
		exists:  true,
		running: true,
		image:   "abc123",
		created: "2024-06-01",
		containerMountsJSON: `[
			{"Type":"bind","Source":"/home/user/project","Destination":"/workspace/project"},
			{"Type":"volume","Source":"some-vol","Destination":"/workspace/project/secrets"}
		]`,
	}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
		LoadConfig: func(project string) (*config.Config, error) {
			return &config.Config{
				Mounts: []string{"/home/user/project"},
				Masks:  []string{},
			}, nil
		},
	}

	buf := &bytes.Buffer{}
	root := cmd.NewRootCmd(deps)
	root.SetOut(buf)

	// When the status subcommand is executed
	root.SetArgs([]string{"--project", "myapp", "status"})
	assertNoError(t, root.Execute())

	// Then the ? mask entry shows the host-equivalent path, not the container path
	out := buf.String()
	assertContains(t, out, "? /home/user/project/secrets")
	assertNotContains(t, out, "? /workspace/project/secrets")
}

// TestStatus_MountsMasks_RunningContainer_UntrackedMaskUnderUntrackedMount
// verifies that when an untracked volume's container destination falls under
// an untracked bind mount (one not in the configured mounts), the ? mask entry
// shows the host-equivalent path derived from the actual bind mount rather than
// the raw container destination.
func TestStatus_MountsMasks_RunningContainer_UntrackedMaskUnderUntrackedMount(t *testing.T) {
	// Given a running container where:
	//   - there are no configured mounts (empty config)
	//   - one actual bind mount: source /home/rob/repos/devenv → destination /workspace/devenv
	//   - one actual volume: destination /workspace/devenv/screenshots (untracked)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{
		exists:  true,
		running: true,
		image:   "abc123",
		created: "2024-06-01",
		containerMountsJSON: `[
			{"Type":"bind","Source":"/home/rob/repos/devenv","Destination":"/workspace/devenv"},
			{"Type":"volume","Source":"some-vol","Destination":"/workspace/devenv/screenshots"}
		]`,
	}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
		LoadConfig: func(project string) (*config.Config, error) {
			return &config.Config{
				Mounts: []string{},
				Masks:  []string{},
			}, nil
		},
	}

	buf := &bytes.Buffer{}
	root := cmd.NewRootCmd(deps)
	root.SetOut(buf)

	// When the status subcommand is executed
	root.SetArgs([]string{"--project", "myapp", "status"})
	assertNoError(t, root.Execute())

	// Then the ? mask entry shows the host-equivalent path derived from the
	// actual (untracked) bind mount, not the raw container destination
	out := buf.String()
	assertContains(t, out, "? /home/rob/repos/devenv/screenshots")
	assertNotContains(t, out, "? /workspace/devenv/screenshots")
}

func TestStatus_AbsentContainer_ConfiguredMaskSanitized(t *testing.T) {
	// Given an absent container whose config contains a mask path with an ANSI
	// escape sequence embedded in it
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: false, running: false}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
		LoadConfig: func(project string) (*config.Config, error) {
			return &config.Config{
				Mounts: []string{},
				Masks:  []string{"/home/user/evil\x1b[2J"},
			}, nil
		},
	}

	buf := &bytes.Buffer{}
	root := cmd.NewRootCmd(deps)
	root.SetOut(buf)

	// When the status subcommand is executed
	root.SetArgs([]string{"--project", "myapp", "status"})
	assertNoError(t, root.Execute())

	// Then the ESC character is absent from the output — the path is sanitised
	out := buf.String()
	assertNotContains(t, out, "\x1b")
	assertContains(t, out, "/home/user/evil[2J")
}
