// SPDX-License-Identifier: AGPL-3.0-or-later
// Package customisations manages the on-host user defaults directory tree
// that marshal scaffolds on every container-assembling invocation.
package customisations

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/rob-broadley/ai-airbase/marshal/internal/pathutil"
)

// MountDirs returns all bind-mounted subdirectories that marshal scaffolds
// under $XDG_DATA_HOME/marshal/defaults. Returns a copy to prevent mutation.
func MountDirs() []string {
	return []string{"opencode/config", "opencode/share", "opencode/state"}
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
		path := filepath.Join(defaultsDir, sub)
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

// HardenSourceTree walks dir and returns an error if any symlink resolves
// to a path outside dir. Broken relative symlinks are skipped. Broken
// absolute symlinks whose raw target is outside dir are also rejected as
// security violations.
func HardenSourceTree(dir string) error {
	rootResolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return fmt.Errorf("resolving source tree root %s: %w", dir, err)
	}
	return filepath.WalkDir(dir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.Type()&os.ModeSymlink != 0 {
			targetResolved, err := filepath.EvalSymlinks(path)
			if err != nil {
				if os.IsNotExist(err) {
					// Broken symlink — check raw target for absolute escape
					target, readErr := os.Readlink(path)
					if readErr == nil && filepath.IsAbs(target) {
						if !pathutil.IsPathUnder(filepath.Clean(target), rootResolved) {
							return fmt.Errorf("security violation: broken symlink %s has absolute target outside source root (target: %s)", path, target)
						}
					}
					return nil
				}
				return fmt.Errorf("resolving symlink %s: %w", path, err)
			}
			if !pathutil.IsPathUnder(targetResolved, rootResolved) {
				return fmt.Errorf("security violation: symlink %s escapes the source root (resolves to %s)", path, targetResolved)
			}
		}
		return nil
	})
}

// CopyDefaults iterates MountDirs and copies each source subdirectory from
// defaultsDir into dst. For each entry in MountDirs (e.g. "opencode/config"),
// the source is defaultsDir/<entry> and the destination is dst/<entry>.
// Source subdirectories that don't exist are silently skipped. Files that
// already exist at the destination are never overwritten. Each source
// subdirectory is hardened before copying. Destination file permissions are
// not modified — hardenProjectDir handles that.
func CopyDefaults(defaultsDir, dst string) error {
	for _, entry := range MountDirs() {
		src := filepath.Join(defaultsDir, entry)
		if _, err := os.Stat(src); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if err := HardenSourceTree(src); err != nil {
			return err
		}
		dstDir := filepath.Join(dst, entry)
		if err := copyTree(src, dstDir); err != nil {
			return err
		}
	}
	return nil
}

// copyTree copies the entire source tree into dst, preserving symlinks,
// never overwriting existing entries, and creating intermediate directories
// as needed.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dst, rel)
		if d.Type()&os.ModeSymlink != 0 {
			return copySymlinkToDest(path, dstPath)
		}
		if d.IsDir() {
			return copyDirToDest(dstPath)
		}
		return copyFileToDest(path, dstPath)
	})
}

// copyFile copies src to dst, overwriting dst.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}

	_, err = io.Copy(out, in)
	if err != nil {
		_ = out.Close()
		_ = os.Remove(dst)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(dst)
		return err
	}
	return nil
}

// copyFileToDest copies srcPath to dstPath. Files that already exist at the
// destination are skipped.
func copyFileToDest(srcPath, dstPath string) error {
	if _, err := os.Lstat(dstPath); err == nil {
		return nil // never overwrite
	}
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o700); err != nil {
		return err
	}
	return copyFile(srcPath, dstPath)
}

// copyDirToDest creates a directory at dstPath.
func copyDirToDest(dstPath string) error {
	return os.MkdirAll(dstPath, 0o700)
}

// copySymlinkToDest reads the symlink at srcPath and recreates it at
// dstPath, preserving the relative target path. Absolute targets are
// converted to relative paths from the destination symlink's location.
// Broken symlinks and symlinked directories are preserved as-is. Existing
// entries at the destination are never overwritten.
func copySymlinkToDest(srcPath, dstPath string) error {
	target, err := os.Readlink(srcPath)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(dstPath); err == nil {
		return nil // never overwrite
	}
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o700); err != nil {
		return err
	}
	if filepath.IsAbs(target) {
		// The absolute target is within the source tree. Compute its
		// relative path from the symlink's directory, which is the same
		// relative path needed from the destination symlink's directory.
		targetRel, err := filepath.Rel(filepath.Dir(srcPath), target)
		if err != nil {
			return err
		}
		target = targetRel
	}
	return os.Symlink(target, dstPath)
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
