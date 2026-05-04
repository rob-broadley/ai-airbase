// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd_test

import (
	"bytes"
	"testing"

	"github.com/rob-broadley/ai-airbase/marshal/internal/cmd"
)

// TestDefaultCmd_CredentialMountsIncluded verifies that the default command mounts
// credential directories when creating a container.
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

	// Then both credential mounts appear in the create args
	copilotMount := cf.expectedMount("copilot", "/home/copilot/.copilot")
	ghcMount := cf.expectedMount("config/github-copilot", "/home/copilot/.config/github-copilot")

	if !runner.createArgsContain(copilotMount) {
		t.Errorf("expected copilot mount %q in create args\ngot: %v", copilotMount, runner.createArgs())
	}
	if !runner.createArgsContain(ghcMount) {
		t.Errorf("expected github-copilot mount %q in create args\ngot: %v", ghcMount, runner.createArgs())
	}
}

// TestRecreate_CredentialMountsIncluded verifies that the recreate command also
// mounts credential directories when creating a container.
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

	// Then both credential mounts appear in the create args
	copilotMount := cf.expectedMount("copilot", "/home/copilot/.copilot")
	ghcMount := cf.expectedMount("config/github-copilot", "/home/copilot/.config/github-copilot")

	if !runner.createArgsContain(copilotMount) {
		t.Errorf("expected copilot mount %q in recreate create args\ngot: %v", copilotMount, runner.createArgs())
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

	// Then the credential mount host paths are identical
	copilotMount := cf.expectedMount("copilot", "/home/copilot/.copilot")
	ghcMount := cf.expectedMount("config/github-copilot", "/home/copilot/.config/github-copilot")

	if !sliceContains(alphaArgs, copilotMount) {
		t.Errorf("alpha: expected shared copilot mount %q\ngot: %v", copilotMount, alphaArgs)
	}
	if !sliceContains(betaArgs, copilotMount) {
		t.Errorf("beta: expected shared copilot mount %q\ngot: %v", copilotMount, betaArgs)
	}
	if !sliceContains(alphaArgs, ghcMount) {
		t.Errorf("alpha: expected shared github-copilot mount %q\ngot: %v", ghcMount, alphaArgs)
	}
	if !sliceContains(betaArgs, ghcMount) {
		t.Errorf("beta: expected shared github-copilot mount %q\ngot: %v", ghcMount, betaArgs)
	}
}
