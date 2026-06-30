// SPDX-License-Identifier: AGPL-3.0-or-later
// Package customisations manages the on-host user defaults directory tree
// that marshal scaffolds on every container-assembling invocation.
package customisations

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rob-broadley/ai-airbase/marshal/internal/container"
)

// MountDir pairs a host-side subdirectory under the project root with its
// corresponding container-side mount path. When ReadOnly is true, the mount
// is flagged as read-only inside the container.
type MountDir struct {
	HostSubdir    string
	ContainerPath string
	ReadOnly      bool
}

// MountDirs returns all bind-mounted subdirectories that marshal scaffolds
// under $XDG_DATA_HOME/marshal/defaults. Returns a copy to prevent mutation.
func MountDirs() []MountDir {
	return []MountDir{
		{HostSubdir: "opencode/config", ContainerPath: container.ContainerOpencodeConfigDir},
		{HostSubdir: "opencode/share", ContainerPath: container.ContainerOpencodeDataDir},
		{HostSubdir: "opencode/state", ContainerPath: container.ContainerOpencodeStateDir},
	}
}

// Ensure creates the user defaults directory tree at
// $XDG_DATA_HOME/marshal/defaults for every entry in MountDirs with
// 0o700 permissions. Returns an error if the base is unavailable,
// a symlink exists at any subdir path, or a non-directory file
// exists at any subdir path. Pre-existing directories are left alone.
func Ensure(xdgDataHome func() string) error {
	base := xdgDataHome()
	if base == "" {
		return fmt.Errorf("data directory unavailable: set HOME or XDG_DATA_HOME")
	}
	defaultsDir := DefaultsDir(base)
	for _, sub := range MountDirs() {
		path := filepath.Join(defaultsDir, sub.HostSubdir)
		if err := ensureDefaultsSubdir(path); err != nil {
			return err
		}
	}
	return nil
}

// DefaultsDir returns $XDG_DATA_HOME/marshal/defaults. Performs no I/O.
func DefaultsDir(base string) string {
	return filepath.Join(base, "marshal", "defaults")
}

func ensureDefaultsSubdir(path string) error {
	info, err := os.Lstat(path)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to follow symlink at %s", path)
		}
		if !info.IsDir() {
			return fmt.Errorf("refusing to clobber non-directory at %s", path)
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("checking user defaults subdir %s: %w", path, err)
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return fmt.Errorf("creating user defaults subdir %s: %w", path, err)
	}
	return nil
}
