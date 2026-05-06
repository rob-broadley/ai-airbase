// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd_test

import (
	"bytes"
	"testing"

	"github.com/rob-broadley/ai-airbase/marshal/internal/cmd"
)

// TestDefaultCmd_CredentialMountsIncluded verifies that the default command mounts
// credential files when creating a container, and that the copilot agents/skills
// directory is NOT bind-mounted (it is baked into the image).
func TestDefaultCmd_CredentialMountsIncluded(t *testing.T) {
	// Given marshal is run for any project with credential fakes injected
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cf := newCredFakes(t)
	runner := &fakeRunner{exists: false}
	fe := &fakeExec{}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              fe.exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              func() int { return 1001 },
		Getgid:              func() int { return 1002 },
		EnsureSharedDataDir: cf.ensureFn,
	}

	// When the root command is executed
	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "myapp"})
	assertNoError(t, root.Execute())

	// Then the individual config file mounts appear in the create args
	settingsMount := cf.expectedMount("copilot/settings.json", "/home/copilot/.copilot/settings.json")
	mcpMount := cf.expectedMount("copilot/mcp-config.json", "/home/copilot/.copilot/mcp-config.json")
	ghcMount := cf.expectedMount("config/github-copilot", "/home/copilot/.config/github-copilot")

	if !runner.createArgsContain(settingsMount) {
		t.Errorf("expected settings mount %q in create args\ngot: %v", settingsMount, runner.createArgs())
	}
	if !runner.createArgsContain(mcpMount) {
		t.Errorf("expected mcp-config mount %q in create args\ngot: %v", mcpMount, runner.createArgs())
	}
	if !runner.createArgsContain(ghcMount) {
		t.Errorf("expected github-copilot mount %q in create args\ngot: %v", ghcMount, runner.createArgs())
	}

	// And the whole copilot directory is NOT mounted (agents/skills stay from image)
	wholeDirMount := cf.expectedMount("copilot", "/home/copilot/.copilot")
	if runner.createArgsContain(wholeDirMount) {
		t.Errorf("expected copilot whole-dir mount to be absent, but found %q in create args", wholeDirMount)
	}
}

// TestRecreate_CredentialMountsIncluded verifies that the recreate command also
// mounts credential files when creating a container.
func TestRecreate_CredentialMountsIncluded(t *testing.T) {
	// Given marshal recreate is run with credential fakes injected
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cf := newCredFakes(t)
	runner := &fakeRunner{exists: false}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              func() int { return 1001 },
		Getgid:              func() int { return 1002 },
		EnsureSharedDataDir: cf.ensureFn,
	}

	// When the recreate subcommand is executed
	root := cmd.NewRootCmd(deps)
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "recreate"})
	assertNoError(t, root.Execute())

	// Then the individual config file mounts appear in the create args
	settingsMount := cf.expectedMount("copilot/settings.json", "/home/copilot/.copilot/settings.json")
	mcpMount := cf.expectedMount("copilot/mcp-config.json", "/home/copilot/.copilot/mcp-config.json")
	ghcMount := cf.expectedMount("config/github-copilot", "/home/copilot/.config/github-copilot")

	if !runner.createArgsContain(settingsMount) {
		t.Errorf("expected settings mount %q in recreate create args\ngot: %v", settingsMount, runner.createArgs())
	}
	if !runner.createArgsContain(mcpMount) {
		t.Errorf("expected mcp-config mount %q in recreate create args\ngot: %v", mcpMount, runner.createArgs())
	}
	if !runner.createArgsContain(ghcMount) {
		t.Errorf("expected github-copilot mount %q in recreate create args\ngot: %v", ghcMount, runner.createArgs())
	}
}

// TestCredentialMounts_SharedAcrossProjects verifies that credential mount host
// paths are identical across different projects.
func TestCredentialMounts_SharedAcrossProjects(t *testing.T) {
	// Given the same EnsureSharedDataDir is used for two different projects
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cf := newCredFakes(t)

	runProject := func(project string) []string {
		runner := &fakeRunner{exists: false}
		deps := cmd.Deps{
			Runner:              runner,
			ExecFn:              (&fakeExec{}).exec,
			Getwd:               func() (string, error) { return "/projects/" + project, nil },
			Getuid:              func() int { return 1001 },
			Getgid:              func() int { return 1001 },
			EnsureSharedDataDir: cf.ensureFn,
		}
		root := cmd.NewRootCmd(deps)
		root.SetArgs([]string{"--project", project})
		if err := root.Execute(); err != nil {
			t.Fatalf("project %s: unexpected error: %v", project, err)
		}
		return runner.createArgs()
	}

	// When both projects are run
	alphaArgs := runProject("alpha")
	betaArgs := runProject("beta")

	// Then the credential mount host paths are identical across projects
	settingsMount := cf.expectedMount("copilot/settings.json", "/home/copilot/.copilot/settings.json")
	mcpMount := cf.expectedMount("copilot/mcp-config.json", "/home/copilot/.copilot/mcp-config.json")
	ghcMount := cf.expectedMount("config/github-copilot", "/home/copilot/.config/github-copilot")

	for _, tc := range []struct {
		name  string
		args  []string
		mount string
	}{
		{"alpha settings", alphaArgs, settingsMount},
		{"beta settings", betaArgs, settingsMount},
		{"alpha mcp-config", alphaArgs, mcpMount},
		{"beta mcp-config", betaArgs, mcpMount},
		{"alpha ghc", alphaArgs, ghcMount},
		{"beta ghc", betaArgs, ghcMount},
	} {
		if !sliceContains(tc.args, tc.mount) {
			t.Errorf("%s: expected shared mount %q\ngot: %v", tc.name, tc.mount, tc.args)
		}
	}
}
