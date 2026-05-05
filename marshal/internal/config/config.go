// SPDX-License-Identifier: AGPL-3.0-or-later
// Package config manages per-project marshal configuration, persisted as TOML
// files under the XDG base directory.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/BurntSushi/toml"
)

// Config holds per-project marshal configuration.
type Config struct {
	Mounts []string `toml:"mounts"`
}

// ---------------------------------------------------------------------------
// Per-project configuration
// ---------------------------------------------------------------------------

// ResolveProject returns the project name by consulting, in priority order:
//  1. flagValue — if non-empty, use it directly
//  2. MARSHAL_PROJECT env var — if set, use its value
//  3. basename of the current working directory (via getwd)
//
// getwd is injected so callers can supply deps.Getwd in tests and os.Getwd
// in production — keeping the function fully testable.
func ResolveProject(flagValue string, getwd func() (string, error)) string {
	if flagValue != "" {
		return flagValue
	}
	if v := os.Getenv("MARSHAL_PROJECT"); v != "" {
		return v
	}
	cwd, err := getwd()
	if err != nil {
		return ""
	}
	return filepath.Base(cwd)
}

// validProjectName matches project names that are safe to use as path components.
// Names must start with an alphanumeric character and contain only alphanumerics,
// hyphens, underscores, and dots — no slashes, backslashes, or dot-dot sequences
// that could cause path traversal when constructing config file paths.
var validProjectName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.\-]{0,126}[a-zA-Z0-9]$|^[a-zA-Z0-9]$`)

// ValidateProjectName returns an error if name is empty or contains characters
// that would allow it to escape the projects/ config subdirectory.
func ValidateProjectName(name string) error {
	if name == "" {
		return fmt.Errorf("project name must not be empty")
	}
	if !validProjectName.MatchString(name) {
		return fmt.Errorf("invalid project name %q: must contain only letters, digits, hyphens, underscores, or dots and must not start with a dot", name)
	}
	return nil
}

// ConfigPath returns the filesystem path for a given project's config file.
func ConfigPath(projectName string) string {
	return filepath.Join(xdgConfigHome(), "marshal", "projects", projectName+".toml")
}

// Load reads the config for projectName from disk.
// If the file does not exist, an empty Config is returned (not an error).
func Load(projectName string) (*Config, error) {
	if err := ValidateProjectName(projectName); err != nil {
		return nil, err
	}
	path := ConfigPath(projectName)
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("config directory unavailable: set HOME or XDG_CONFIG_HOME")
	}
	cfg := &Config{}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return nil, fmt.Errorf("decoding config %s: %w", path, err)
	}
	return cfg, nil
}

// Save writes cfg for projectName to disk atomically via a temp-file-then-rename
// pattern, so concurrent readers never observe a zero-byte or partial-write state.
// Parent directories are created as needed. Close errors are always surfaced.
func Save(projectName string, cfg *Config) error {
	if err := ValidateProjectName(projectName); err != nil {
		return err
	}
	path := ConfigPath(projectName)
	if !filepath.IsAbs(path) {
		return fmt.Errorf("config directory unavailable: set HOME or XDG_CONFIG_HOME")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".marshal-config-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp config file: %w", err)
	}
	tmpName := tmp.Name()
	committed := false
	defer func() {
		if !committed {
			os.Remove(tmpName) //nolint:errcheck
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("setting config file permissions: %w", err)
	}
	if err := toml.NewEncoder(tmp).Encode(cfg); err != nil {
		tmp.Close()
		return fmt.Errorf("encoding config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temp config file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("committing config file: %w", err)
	}
	committed = true
	return nil
}

// ---------------------------------------------------------------------------
// Shared data
// ---------------------------------------------------------------------------

// SharedDataPath returns the filesystem path $XDG_DATA_HOME/marshal/<subdir>.
// subdir may contain path separators (e.g. "config/github-copilot").
func SharedDataPath(subdir string) string {
	return filepath.Join(xdgDataHome(), "marshal", subdir)
}

// EnsureSharedDataDir resolves SharedDataPath(subdir), creates the directory
// with permissions 0o700 (owner-only, suitable for credentials), and returns
// the path.
func EnsureSharedDataDir(subdir string) (string, error) {
	path := SharedDataPath(subdir)
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("data directory unavailable: set HOME or XDG_DATA_HOME")
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return "", fmt.Errorf("creating shared data directory %s: %w", path, err)
	}
	return path, nil
}
