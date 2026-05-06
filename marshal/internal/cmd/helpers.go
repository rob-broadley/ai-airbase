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
// into the container. These mounts are shared across all projects unless noted.
//
// Config files (settings, mcp-config, git config, etc.) come from XDG_CONFIG_HOME
// so backup tools and dotfile managers handle them.
//
// Session-store.db and session-state/ are both per-project under XDG_DATA_HOME
// so conversation history and checkpoints survive container recreates.
//
// The agents/ and skills/ directories baked into the container image are left
// untouched — no whole-directory ~/.copilot mount is used.
func buildCredentialMounts(deps Deps, project string) ([]container.MountSpec, error) {
	specs := make([]container.MountSpec, 0, 8)
	ensureConfigDir := deps.ensureSharedConfigDirFn()

	// User git config — overrides /etc/gitconfig baked into the image.
	gitConfigDir, err := ensureConfigDir("git")
	if err != nil {
		return nil, fmt.Errorf("ensuring config dir git: %w", err)
	}
	gitConfigPath := filepath.Join(gitConfigDir, "config")
	if err := ensureConfigFile(gitConfigPath, []byte{}); err != nil {
		return nil, fmt.Errorf("ensuring git config file: %w", err)
	}
	specs = append(specs, container.MountSpec{
		HostPath:      gitConfigPath,
		ContainerPath: container.ContainerGitConfigFile,
	})

	// User-editable config files — individual file mounts from XDG_CONFIG.
	configDir, err := ensureConfigDir("copilot")
	if err != nil {
		return nil, fmt.Errorf("ensuring config dir copilot: %w", err)
	}

	type configFile struct {
		name           string
		containerPath  string
		defaultContent []byte
	}
	configFiles := []configFile{
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
		{
			name:           "permissions-config.json",
			containerPath:  container.ContainerCopilotDir + "/permissions-config.json",
			defaultContent: []byte("{}\n"),
		},
		{
			name:           "config.json",
			containerPath:  container.ContainerCopilotDir + "/config.json",
			defaultContent: []byte("{}\n"),
		},
	}

	for _, f := range configFiles {
		hostPath := filepath.Join(configDir, f.name)
		if err := ensureConfigFile(hostPath, f.defaultContent); err != nil {
			return nil, fmt.Errorf("ensuring config file %s: %w", f.name, err)
		}
		specs = append(specs, container.MountSpec{
			HostPath:      hostPath,
			ContainerPath: f.containerPath,
		})
	}

	// Per-project session store and session state — both isolated by project.
	projectDataDir, err := deps.EnsureSharedDataDir("projects/" + project)
	if err != nil {
		return nil, fmt.Errorf("ensuring project data dir for %s: %w", project, err)
	}
	sessionStorePath := filepath.Join(projectDataDir, "session-store.db")
	if err := ensureConfigFile(sessionStorePath, []byte{}); err != nil {
		return nil, fmt.Errorf("ensuring session-store.db: %w", err)
	}
	specs = append(specs, container.MountSpec{
		HostPath:      sessionStorePath,
		ContainerPath: container.ContainerCopilotDir + "/session-store.db",
	})

	sessionStateDir, err := deps.EnsureSharedDataDir("projects/" + project + "/session-state")
	if err != nil {
		return nil, fmt.Errorf("ensuring session-state dir for project %s: %w", project, err)
	}
	specs = append(specs, container.MountSpec{
		HostPath:      sessionStateDir,
		ContainerPath: container.ContainerCopilotDir + "/session-state",
	})

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
