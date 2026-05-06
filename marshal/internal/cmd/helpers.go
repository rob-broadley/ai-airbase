// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rob-broadley/ai-airbase/marshal/internal/config"
	"github.com/rob-broadley/ai-airbase/marshal/internal/container"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// buildCredentialMounts returns MountSpec values that bind host credential files
// into the container. These mounts are shared across all projects.
//
// Authentication tokens come from the GitHub Copilot config directory, which is
// mounted whole so Copilot CLI can read and update its token cache.
//
// User settings (settings.json) and MCP server configuration (mcp-config.json)
// are mounted as individual files. This preserves the agents/ and skills/
// directories baked into the container image, which would otherwise be hidden
// by a whole-directory bind mount.
func buildCredentialMounts(deps Deps) ([]container.MountSpec, error) {
	specs := make([]container.MountSpec, 0, 3)

	// GitHub Copilot auth tokens — mount the whole directory.
	ghcPath, err := deps.EnsureSharedDataDir("config/github-copilot")
	if err != nil {
		return nil, fmt.Errorf("ensuring credential dir config/github-copilot: %w", err)
	}
	specs = append(specs, container.MountSpec{
		HostPath:      ghcPath,
		ContainerPath: container.ContainerGHCopilotDir,
	})

	// User settings and MCP config — individual files so that the image's
	// baked-in agents/ and skills/ are not hidden by a whole-directory mount.
	copilotDir, err := deps.EnsureSharedDataDir("copilot")
	if err != nil {
		return nil, fmt.Errorf("ensuring credential dir copilot: %w", err)
	}

	type configFile struct {
		name           string
		containerPath  string
		defaultContent []byte
	}
	files := []configFile{
		{
			name:           "settings.json",
			containerPath:  container.ContainerCopilotDir + "/settings.json",
			defaultContent: []byte("{}\n"),
		},
		{
			name:           "mcp-config.json",
			containerPath:  container.ContainerCopilotDir + "/mcp-config.json",
			defaultContent: []byte(`{"mcpServers":{}}` + "\n"),
		},
		{
			name:           "copilot-instructions.md",
			containerPath:  container.ContainerCopilotDir + "/copilot-instructions.md",
			defaultContent: []byte{},
		},
	}

	for _, f := range files {
		hostPath := filepath.Join(copilotDir, f.name)
		if err := ensureConfigFile(hostPath, f.defaultContent); err != nil {
			return nil, fmt.Errorf("ensuring config file %s: %w", f.name, err)
		}
		specs = append(specs, container.MountSpec{
			HostPath:      hostPath,
			ContainerPath: f.containerPath,
		})
	}
	return specs, nil
}

// ensureConfigFile creates the file at path with defaultContent if it does not
// already exist. Uses O_EXCL so a concurrent create wins cleanly — the file is
// left with whatever content the other writer put there. Returns an error if
// path exists as a directory, since Podman cannot bind-mount a file over a
// directory. The file is created with 0o600 (owner-read/write) permissions.
func ensureConfigFile(path string, defaultContent []byte) error {
	fi, err := os.Stat(path)
	if err == nil {
		if fi.IsDir() {
			return fmt.Errorf("config path exists but is a directory: %s", path)
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return nil // another process created it concurrently
		}
		return err
	}
	defer f.Close()
	_, err = f.Write(defaultContent)
	return err
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
