// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd_test

import (
	"bytes"
	"testing"

	"github.com/rob-broadley/ai-airbase/marshal/internal/cmd"
	"github.com/rob-broadley/ai-airbase/marshal/internal/container"
)

// ---------------------------------------------------------------------------
// Credential mount location tests — XDG config/data split
// ---------------------------------------------------------------------------

// TestDefaultCmd_ConfigFilesFromXDGConfig verifies that user-editable Copilot
// config files are bind-mounted from XDG_CONFIG_HOME/marshal/copilot/, not
// from XDG_DATA_HOME, so backup tools and dotfile managers handle them correctly.
func TestDefaultCmd_ConfigFilesFromXDGConfig(t *testing.T) {
	// Given marshal is run with separate config and data fakes
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cf := newCredFakes(t)
	runner := &fakeRunner{exists: false}
	deps := cmd.Deps{
		Runner:                runner,
		ExecFn:                (&fakeExec{}).exec,
		Getwd:                 func() (string, error) { return "/projects/myapp", nil },
		Getuid:                func() int { return 1001 },
		Getgid:                func() int { return 1002 },
		EnsureSharedDataDir:   cf.dataDirFn,
		EnsureSharedConfigDir: cf.configDirFn,
	}

	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "myapp"})

	// When the default command is executed
	assertNoError(t, root.Execute())

	args := runner.createArgs()

	// Then each config file is mounted from XDG_CONFIG, not XDG_DATA
	for _, tc := range []struct {
		file          string
		containerPath string
	}{
		{"copilot/settings.json", container.ContainerCopilotDir + "/settings.json"},
		{"copilot/mcp-config.json", container.ContainerCopilotDir + "/mcp-config.json"},
		{"copilot/copilot-instructions.md", container.ContainerCopilotDir + "/copilot-instructions.md"},
		{"copilot/permissions-config.json", container.ContainerCopilotDir + "/permissions-config.json"},
		{"copilot/config.json", container.ContainerCopilotDir + "/config.json"},
	} {
		want := cf.expectedConfigMount(tc.file, tc.containerPath)
		if !sliceContains(args, want) {
			t.Errorf("expected config-side mount %q in create args\ngot: %v", want, args)
		}
		// And NOT from XDG_DATA
		notWant := cf.expectedDataMount(tc.file, tc.containerPath)
		if sliceContains(args, notWant) {
			t.Errorf("config file %s must not be mounted from XDG_DATA; found %q in create args", tc.file, notWant)
		}
	}
}

// TestDefaultCmd_GitConfigMounted verifies that the user git config is
// bind-mounted from XDG_CONFIG_HOME/marshal/git/config so the user's git
// identity and preferences override the image's /etc/gitconfig.
func TestDefaultCmd_GitConfigMounted(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cf := newCredFakes(t)
	runner := &fakeRunner{exists: false}
	deps := cmd.Deps{
		Runner:                runner,
		ExecFn:                (&fakeExec{}).exec,
		Getwd:                 func() (string, error) { return "/projects/myapp", nil },
		Getuid:                func() int { return 1001 },
		Getgid:                func() int { return 1002 },
		EnsureSharedDataDir:   cf.dataDirFn,
		EnsureSharedConfigDir: cf.configDirFn,
	}

	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "myapp"})
	assertNoError(t, root.Execute())

	args := runner.createArgs()
	want := cf.expectedConfigMount("git/config", container.ContainerGitConfigFile)
	if !sliceContains(args, want) {
		t.Errorf("expected git config mount %q in create args\ngot: %v", want, args)
	}
}

// TestDefaultCmd_SessionStoreMounted verifies that session-store.db is
// bind-mounted from the per-project XDG_DATA_HOME/marshal/projects/<project>/
// path so conversation history is preserved across container recreates.
func TestDefaultCmd_SessionStoreMounted(t *testing.T) {
	// Given deps configured for project "myapp"
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cf := newCredFakes(t)
	runner := &fakeRunner{exists: false}
	deps := cmd.Deps{
		Runner:                runner,
		ExecFn:                (&fakeExec{}).exec,
		Getwd:                 func() (string, error) { return "/projects/myapp", nil },
		Getuid:                func() int { return 1001 },
		Getgid:                func() int { return 1002 },
		EnsureSharedDataDir:   cf.dataDirFn,
		EnsureSharedConfigDir: cf.configDirFn,
	}

	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "myapp"})

	// When the default command is executed
	assertNoError(t, root.Execute())

	// Then session-store.db is mounted from the per-project data directory
	args := runner.createArgs()
	want := cf.expectedDataMount("projects/myapp/session-store.db", container.ContainerCopilotDir+"/session-store.db")
	if !sliceContains(args, want) {
		t.Errorf("expected session-store mount %q in create args\ngot: %v", want, args)
	}
}

// TestDefaultCmd_SessionStateMountedPerProject verifies that the session-state
// directory is bind-mounted from a per-project path under XDG_DATA_HOME so
// checkpoint history survives container recreates.
func TestDefaultCmd_SessionStateMountedPerProject(t *testing.T) {
	// Given deps configured for project "myapp"
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cf := newCredFakes(t)
	runner := &fakeRunner{exists: false}
	deps := cmd.Deps{
		Runner:                runner,
		ExecFn:                (&fakeExec{}).exec,
		Getwd:                 func() (string, error) { return "/projects/myapp", nil },
		Getuid:                func() int { return 1001 },
		Getgid:                func() int { return 1002 },
		EnsureSharedDataDir:   cf.dataDirFn,
		EnsureSharedConfigDir: cf.configDirFn,
	}

	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "myapp"})

	// When the default command is executed
	assertNoError(t, root.Execute())

	// Then session-state is mounted from a per-project path under XDG_DATA_HOME
	args := runner.createArgs()
	want := cf.expectedDataMount("projects/myapp/session-state", container.ContainerCopilotDir+"/session-state")
	if !sliceContains(args, want) {
		t.Errorf("expected per-project session-state mount %q in create args\ngot: %v", want, args)
	}
}

// TestCredentialMounts_SessionStateIsolatedByProject verifies that two
// different projects receive different session-state and session-store.db
// host paths.
func TestCredentialMounts_SessionStateIsolatedByProject(t *testing.T) {
	// Given two separate projects "alpha" and "beta" sharing the same config fakes
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cf := newCredFakes(t)

	argsFor := func(project string) []string {
		runner := &fakeRunner{exists: false}
		deps := cmd.Deps{
			Runner:                runner,
			ExecFn:                (&fakeExec{}).exec,
			Getwd:                 func() (string, error) { return "/projects/" + project, nil },
			Getuid:                func() int { return 1001 },
			Getgid:                func() int { return 1001 },
			EnsureSharedDataDir:   cf.dataDirFn,
			EnsureSharedConfigDir: cf.configDirFn,
		}
		root := cmd.NewRootCmd(deps)
		root.SetArgs([]string{"--project", project})
		if err := root.Execute(); err != nil {
			t.Fatalf("project %s: unexpected error: %v", project, err)
		}
		return runner.createArgs()
	}

	// When the default command is executed for each project
	alphaArgs := argsFor("alpha")
	betaArgs := argsFor("beta")

	// Then each project's session paths are distinct and not shared
	for _, tc := range []struct {
		alphaSubdir   string
		betaSubdir    string
		containerPath string
	}{
		{"projects/alpha/session-state", "projects/beta/session-state", container.ContainerCopilotDir + "/session-state"},
		{"projects/alpha/session-store.db", "projects/beta/session-store.db", container.ContainerCopilotDir + "/session-store.db"},
	} {
		alphaMount := cf.expectedDataMount(tc.alphaSubdir, tc.containerPath)
		betaMount := cf.expectedDataMount(tc.betaSubdir, tc.containerPath)

		if !sliceContains(alphaArgs, alphaMount) {
			t.Errorf("alpha: expected mount %q\ngot: %v", alphaMount, alphaArgs)
		}
		if !sliceContains(betaArgs, betaMount) {
			t.Errorf("beta: expected mount %q\ngot: %v", betaMount, betaArgs)
		}
		if sliceContains(alphaArgs, betaMount) {
			t.Errorf("alpha: must not have beta's mount %q", betaMount)
		}
		if sliceContains(betaArgs, alphaMount) {
			t.Errorf("beta: must not have alpha's mount %q", alphaMount)
		}
	}
}

// TestCredentialMounts_ConfigSharedAcrossProjects verifies that config files
// are identical across different projects (shared state).
func TestCredentialMounts_ConfigSharedAcrossProjects(t *testing.T) {
	// Given two separate projects "alpha" and "beta" sharing the same config fakes
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cf := newCredFakes(t)

	argsFor := func(project string) []string {
		runner := &fakeRunner{exists: false}
		deps := cmd.Deps{
			Runner:                runner,
			ExecFn:                (&fakeExec{}).exec,
			Getwd:                 func() (string, error) { return "/projects/" + project, nil },
			Getuid:                func() int { return 1001 },
			Getgid:                func() int { return 1001 },
			EnsureSharedDataDir:   cf.dataDirFn,
			EnsureSharedConfigDir: cf.configDirFn,
		}
		root := cmd.NewRootCmd(deps)
		root.SetArgs([]string{"--project", project})
		if err := root.Execute(); err != nil {
			t.Fatalf("project %s: unexpected error: %v", project, err)
		}
		return runner.createArgs()
	}

	// When the default command is executed for each project
	alphaArgs := argsFor("alpha")
	betaArgs := argsFor("beta")

	// Then the same config mounts appear for both projects
	shared := []struct {
		subdir        string
		containerPath string
	}{
		{"copilot/settings.json", container.ContainerCopilotDir + "/settings.json"},
		{"copilot/mcp-config.json", container.ContainerCopilotDir + "/mcp-config.json"},
		{"copilot/copilot-instructions.md", container.ContainerCopilotDir + "/copilot-instructions.md"},
	}

	for _, tc := range shared {
		mount := cf.expectedConfigMount(tc.subdir, tc.containerPath)
		if !sliceContains(alphaArgs, mount) {
			t.Errorf("alpha: expected shared mount %q\ngot: %v", mount, alphaArgs)
		}
		if !sliceContains(betaArgs, mount) {
			t.Errorf("beta: expected shared mount %q\ngot: %v", mount, betaArgs)
		}
	}
}

// ---------------------------------------------------------------------------
// Recreate — credential mounts included
// ---------------------------------------------------------------------------

// TestRecreate_CredentialMountsIncluded verifies that the recreate command also
// mounts credential files when creating a container.
func TestRecreate_CredentialMountsIncluded(t *testing.T) {
	// Given deps configured for project "myapp" with the recreate subcommand
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cf := newCredFakes(t)
	runner := &fakeRunner{exists: false}
	deps := cmd.Deps{
		Runner:                runner,
		ExecFn:                (&fakeExec{}).exec,
		Getwd:                 func() (string, error) { return "/projects/myapp", nil },
		Getuid:                func() int { return 1001 },
		Getgid:                func() int { return 1002 },
		EnsureSharedDataDir:   cf.dataDirFn,
		EnsureSharedConfigDir: cf.configDirFn,
	}

	root := cmd.NewRootCmd(deps)
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "recreate"})

	// When recreate is executed
	assertNoError(t, root.Execute())

	// Then credential mounts (settings and session-store) are included
	args := runner.createArgs()
	settingsMount := cf.expectedConfigMount("copilot/settings.json", container.ContainerCopilotDir+"/settings.json")
	sessionMount := cf.expectedDataMount("projects/myapp/session-store.db", container.ContainerCopilotDir+"/session-store.db")

	if !sliceContains(args, settingsMount) {
		t.Errorf("recreate: expected settings mount %q\ngot: %v", settingsMount, args)
	}
	if !sliceContains(args, sessionMount) {
		t.Errorf("recreate: expected session-store mount %q\ngot: %v", sessionMount, args)
	}
}

