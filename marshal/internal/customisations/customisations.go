// SPDX-License-Identifier: AGPL-3.0-or-later
// Package customisations manages the on-host user defaults directory tree
// that marshal scaffolds on every container-assembling invocation.
package customisations

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Ensure creates the user defaults directory tree at
// $XDG_DATA_HOME/marshal/defaults/opencode/{config,share,state} with
// 0o700 permissions. Returns an error if the base is unavailable,
// a symlink exists at any subdir path, or a non-directory file
// exists at any subdir path. Pre-existing directories are left alone.
func Ensure(xdgDataHome func() string) error {
	base := xdgDataHome()
	if base == "" {
		return fmt.Errorf("data directory unavailable: set HOME or XDG_DATA_HOME")
	}
	defaultsDir := DefaultsDir(base)
	for _, sub := range []string{"config", "share", "state"} {
		path := filepath.Join(defaultsDir, "opencode", sub)
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
// to a path outside dir. Broken symlinks are not treated as escapes.
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
					return nil
				}
				return fmt.Errorf("resolving symlink %s: %w", path, err)
			}
			if !isPathUnder(targetResolved, rootResolved) {
				return fmt.Errorf("security violation: symlink %s escapes the source root (resolves to %s)", path, targetResolved)
			}
		}
		return nil
	})
}

// isPathUnder reports whether child is the same as parent or sits inside it.
// Both paths must be cleaned and absolute.
func isPathUnder(child, parent string) bool {
	parentWithSep := parent
	if !strings.HasSuffix(parentWithSep, string(filepath.Separator)) {
		parentWithSep += string(filepath.Separator)
	}
	if child == parent {
		return true
	}
	return strings.HasPrefix(child, parentWithSep)
}

// CopyDefaults copies every file in the source tree into the destination tree,
// creating intermediate directories as needed. Files that already exist at the
// destination are never overwritten. The source tree is hardened before copying.
// Destination file permissions are not modified — hardenProjectDir handles that.
func CopyDefaults(src, dst string) error {
	if err := HardenSourceTree(src); err != nil {
		return err
	}
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if d.IsDir() {
			rel, err := filepath.Rel(src, path)
			if err != nil {
				return err
			}
			if rel == "." {
				return nil
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o700)
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dst, rel)
		if _, err := os.Stat(dstPath); err == nil {
			return nil // never overwrite
		}
		if err := os.MkdirAll(filepath.Dir(dstPath), 0o700); err != nil {
			return err
		}
		return copyFile(path, dstPath)
	})
}

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
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
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
