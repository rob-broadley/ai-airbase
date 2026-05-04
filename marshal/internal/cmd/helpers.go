// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd

import (
	"fmt"

	"github.com/rob-broadley/ai-airbase/marshal/internal/container"
)

// ---------------------------------------------------------------------------
// Container-image path convention
// ---------------------------------------------------------------------------

// containerUserHome is the home directory inside the container image.
// All credential mount targets and the HOME environment variable must agree
// with this value. Change it here when the image convention changes.
const containerUserHome = "/home/copilot"

// Credential container paths are derived from containerUserHome so that a
// single change keeps everything consistent.
const (
	containerCopilotDir   = containerUserHome + "/.copilot"
	containerGHCopilotDir = containerUserHome + "/.config/github-copilot"
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
		{"copilot", containerCopilotDir},
		{"config/github-copilot", containerGHCopilotDir},
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
// HomeDir is set to containerUserHome, which matches the container image convention.
func buildUserConfig(deps Deps) container.UserConfig {
	return container.UserConfig{
		UID:     deps.Getuid(),
		GID:     deps.Getgid(),
		HomeDir: containerUserHome,
	}
}
