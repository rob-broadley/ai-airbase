// SPDX-License-Identifier: AGPL-3.0-or-later
package customisations

import (
	"io"
	"os"
	"path/filepath"
)

// CopyDefaults iterates MountDirs and copies each source subdirectory from
// defaultsDir into dst. For each entry in MountDirs (e.g. "opencode/config"),
// the source is defaultsDir/<entry> and the destination is dst/<entry>.
// Source subdirectories that don't exist are silently skipped. Files that
// already exist at the destination are never overwritten. Each source
// subdirectory is hardened before copying. Destination files are created
// with 0600 permissions (owner read+write only).
func CopyDefaults(defaultsDir, dst string) error {
	for _, entry := range MountDirs() {
		src := filepath.Join(defaultsDir, entry.HostSubdir)
		if _, err := os.Stat(src); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if err := HardenSourceTree(src); err != nil {
			return err
		}
		dstDir := filepath.Join(dst, entry.HostSubdir)
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

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
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
		return nil
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
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o700); err != nil {
		return err
	}
	if filepath.IsAbs(target) {
		targetRel, err := filepath.Rel(filepath.Dir(srcPath), target)
		if err != nil {
			return err
		}
		target = targetRel
	}
	return os.Symlink(target, dstPath)
}
