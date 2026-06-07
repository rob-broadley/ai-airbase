// SPDX-License-Identifier: AGPL-3.0-or-later
package hostinfo

import (
	"fmt"
	"os"
	"path/filepath"
)

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

// SharedConfigPath returns the filesystem path $XDG_CONFIG_HOME/marshal/<subdir>.
// subdir may contain path separators (e.g. "opencode/settings.json").
func SharedConfigPath(subdir string) string {
	return filepath.Join(XDGConfigHome(), "marshal", subdir)
}

// EnsureSharedConfigDir resolves SharedConfigPath(subdir), creates the directory
// with permissions 0o700 (owner-only, suitable for credentials), and returns
// the path.
func EnsureSharedConfigDir(subdir string) (string, error) {
	return ensureSharedDir(SharedConfigPath, "config", "XDG_CONFIG_HOME", subdir)
}

// ---------------------------------------------------------------------------
// Shared data
// ---------------------------------------------------------------------------

// SharedDataPath returns the filesystem path $XDG_DATA_HOME/marshal/<subdir>.
// subdir may contain path separators (e.g. "projects/myapp/state").
func SharedDataPath(subdir string) string {
	return filepath.Join(XDGDataHome(), "marshal", subdir)
}

// EnsureSharedDataDir resolves SharedDataPath(subdir), creates the directory
// with permissions 0o700 (owner-only, suitable for credentials), and returns
// the path.
func EnsureSharedDataDir(subdir string) (string, error) {
	return ensureSharedDir(SharedDataPath, "data", "XDG_DATA_HOME", subdir)
}
