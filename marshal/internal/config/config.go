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
	"strings"

	"github.com/BurntSushi/toml"
)

// Config holds per-project marshal configuration.
type Config struct {
	Mounts []string `toml:"mounts"`
	Masks  []string `toml:"masks"`
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
//
// An error is returned only when the CWD path is taken and getwd fails.
// The flag and env-var paths never fail.
func ResolveProject(flagValue string, getwd func() (string, error)) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}
	if v := os.Getenv("MARSHAL_PROJECT"); v != "" {
		return v, nil
	}
	cwd, err := getwd()
	if err != nil {
		return "", fmt.Errorf("resolving project from working directory: %w", err)
	}
	return filepath.Base(cwd), nil
}

// validProjectName matches project names that are safe to use as path components.
// Names must start with an alphanumeric character and contain only alphanumerics,
// hyphens, underscores, and dots — no slashes, backslashes, or dot-dot sequences
// that could cause path traversal when constructing config file paths.
var validProjectName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.\-]{0,126}[a-zA-Z0-9]$|^[a-zA-Z0-9]$`)

// stagingProjectName matches names that end with either staging-container
// suffix used by removeAndRecreateContainer. The format is:
//
//	<name>-pending-<PID>-<nanosecond>   (staging container)
//	<name>-retiring-<PID>-<nanosecond>  (aside container awaiting cleanup)
//
// Both suffixes are reserved to prevent a project's canonical container name
// from colliding with another project's internal staging or retiring container.
var stagingProjectName = regexp.MustCompile(`(-pending-\d+-\d+|-retiring-\d+-\d+)$`)

// ValidateProjectName returns an error if name is empty or contains characters
// that would allow it to escape the projects/ config subdirectory.
func ValidateProjectName(name string) error {
	if name == "" {
		return fmt.Errorf("project name must not be empty")
	}
	if !validProjectName.MatchString(name) {
		return fmt.Errorf("invalid project name %q: must contain only letters, digits, hyphens, underscores, or dots and must not start with a dot", name)
	}
	if stagingProjectName.MatchString(name) {
		return fmt.Errorf("invalid project name %q: must not end with \"-pending-<pid>-<nano>\" or \"-retiring-<pid>-<nano>\" (reserved for internal staging containers)", name)
	}
	return nil
}

// projectsDir returns the filesystem path for the projects configuration directory.
func projectsDir() string {
	return filepath.Join(xdgConfigHome(), "marshal", "projects")
}

// ListProjects returns the names of all registered projects by scanning the
// projects configuration directory for *.toml files. Names are returned in
// lexicographic ascending order (os.ReadDir guarantees entries are sorted by
// filename, yielding case-sensitive byte-order sort). Returns an empty slice
// (not an error) when the directory does not exist. Warnings holds a
// human-readable message for each file whose stem is not a valid project name;
// those files are excluded from names.
func ListProjects() (names, warnings []string, err error) {
	dir := projectsDir()
	entries, readErr := os.ReadDir(dir)
	if readErr != nil {
		if errors.Is(readErr, os.ErrNotExist) {
			return []string{}, nil, nil
		}
		return nil, nil, fmt.Errorf("reading projects directory: %w", readErr)
	}
	names = make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		filename := e.Name()
		if !strings.HasSuffix(filename, ".toml") {
			continue
		}
		project := strings.TrimSuffix(filename, ".toml")
		if validateErr := ValidateProjectName(project); validateErr != nil {
			warnings = append(warnings, fmt.Sprintf("skipping %q: %v", filename, validateErr))
			continue
		}
		names = append(names, project)
	}
	return names, warnings, nil
}

// configPath returns the filesystem path for a given project's config file.
func configPath(projectName string) string {
	return filepath.Join(projectsDir(), projectName+".toml")
}

// Load reads the config for projectName from disk.
// If the file does not exist, an empty Config is returned (not an error).
func Load(projectName string) (*Config, error) {
	if err := ValidateProjectName(projectName); err != nil {
		return nil, err
	}
	path := configPath(projectName)
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("config directory unavailable: set HOME or XDG_CONFIG_HOME")
	}
	cfg := &Config{}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		cfg.Masks = []string{}
		return cfg, nil
	}
	meta, err := toml.DecodeFile(path, cfg)
	if err != nil {
		return nil, fmt.Errorf("decoding config %s: %w", path, err)
	}
	if undecoded := meta.Undecoded(); len(undecoded) > 0 {
		keys := make([]string, len(undecoded))
		for i, k := range undecoded {
			keys[i] = k.String()
		}
		return nil, fmt.Errorf("config %s contains unrecognised fields: %s", path, strings.Join(keys, ", "))
	}
	if cfg.Masks == nil {
		cfg.Masks = []string{}
	}
	return cfg, nil
}

// Delete removes the config file for projectName.
// If the file does not exist, Delete returns nil (idempotent).
func Delete(projectName string) error {
	if err := ValidateProjectName(projectName); err != nil {
		return err
	}
	path := configPath(projectName)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing project config: %w", err)
	}
	return nil
}

// Save writes cfg for projectName to disk atomically via a temp-file-then-rename
// pattern, so concurrent readers never observe a zero-byte or partial-write state.
// Parent directories are created as needed. Close errors are always surfaced.
func Save(projectName string, cfg *Config) error {
	if err := ValidateProjectName(projectName); err != nil {
		return err
	}
	path := configPath(projectName)
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
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("syncing temp config file: %w", err)
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

// ensureSharedDir creates the shared directory returned by pathFn(subdir).
// kind and envVar are used only in error messages ("config", "XDG_CONFIG_HOME").
func ensureSharedDir(pathFn func(string) string, kind, envVar, subdir string) (string, error) {
	path := pathFn(subdir)
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("%s directory unavailable: set HOME or %s", kind, envVar)
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return "", fmt.Errorf("creating shared %s directory %s: %w", kind, path, err)
	}
	return path, nil
}

// ---------------------------------------------------------------------------
// Shared config
// ---------------------------------------------------------------------------

// sharedConfigPath returns the filesystem path $XDG_CONFIG_HOME/marshal/<subdir>.
// subdir may contain path separators (e.g. "opencode/settings.json").
func sharedConfigPath(subdir string) string {
	return filepath.Join(xdgConfigHome(), "marshal", subdir)
}

// EnsureSharedConfigDir resolves sharedConfigPath(subdir), creates the directory
// with permissions 0o700 (owner-only, suitable for credentials), and returns
// the path.
func EnsureSharedConfigDir(subdir string) (string, error) {
	return ensureSharedDir(sharedConfigPath, "config", "XDG_CONFIG_HOME", subdir)
}

// ---------------------------------------------------------------------------
// Shared data
// ---------------------------------------------------------------------------

// sharedDataPath returns the filesystem path $XDG_DATA_HOME/marshal/<subdir>.
// subdir may contain path separators (e.g. "projects/myapp/session-state").
func sharedDataPath(subdir string) string {
	return filepath.Join(xdgDataHome(), "marshal", subdir)
}

// EnsureSharedDataDir resolves sharedDataPath(subdir), creates the directory
// with permissions 0o700 (owner-only, suitable for credentials), and returns
// the path.
func EnsureSharedDataDir(subdir string) (string, error) {
	return ensureSharedDir(sharedDataPath, "data", "XDG_DATA_HOME", subdir)
}
