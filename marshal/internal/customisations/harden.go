// SPDX-License-Identifier: AGPL-3.0-or-later
package customisations

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rob-broadley/ai-airbase/marshal/internal/pathutil"
)

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
