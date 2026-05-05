// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd

import (
	"fmt"

	"github.com/rob-broadley/ai-airbase/marshal/internal/config"
	"github.com/rob-broadley/ai-airbase/marshal/internal/container"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// buildCredentialMounts calls EnsureSharedDataDir for each persistent credential
// subdirectory and returns the corresponding MountSpec slice.
// These mounts are shared across all projects — the host path is independent of
// the project name.
func buildCredentialMounts(deps Deps) ([]container.MountSpec, error) {
	type credDir struct {
		subdir        string
		containerPath string
	}
	dirs := []credDir{
		{"copilot", container.ContainerCopilotDir},
		{"config/github-copilot", container.ContainerGHCopilotDir},
	}

	specs := make([]container.MountSpec, 0, len(dirs))
	for _, d := range dirs {
		hostPath, err := deps.EnsureSharedDataDir(d.subdir)
		if err != nil {
			return nil, fmt.Errorf("ensuring credential dir %s: %w", d.subdir, err)
		}
		specs = append(specs, container.MountSpec{
			HostPath:      hostPath,
			ContainerPath: d.containerPath,
		})
	}
	return specs, nil
}

// buildUserConfig constructs the container.UserConfig for the calling user.
// HomeDir is set to ContainerUserHome, which matches the container image convention.
func buildUserConfig(deps Deps) container.UserConfig {
	return container.UserConfig{
		UID:     deps.Getuid(),
		GID:     deps.Getgid(),
		HomeDir: container.ContainerUserHome,
	}
}

// resolveContainer resolves the project name from projectFlag and deps,
// validates it, and returns both the project name and the derived container
// name. Returns an "invalid project" error when the resolved name fails
// validation.
func resolveContainer(deps Deps, projectFlag string) (project, containerName string, err error) {
	project = config.ResolveProject(projectFlag, deps.Getwd)
	if err = config.ValidateProjectName(project); err != nil {
		return "", "", fmt.Errorf("invalid project: %w", err)
	}
	return project, containerNameForProject(project), nil
}
