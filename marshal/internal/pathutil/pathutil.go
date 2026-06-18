// SPDX-License-Identifier: AGPL-3.0-or-later
// Package pathutil provides path-related utilities for security checks.
package pathutil

import (
	"path/filepath"
	"strings"
)

// IsPathUnder reports whether child is the same as parent or sits inside it.
// Both paths must be cleaned and absolute.
func IsPathUnder(child, parent string) bool {
	child = filepath.Clean(child)
	parent = filepath.Clean(parent)
	// Ensure parent has a trailing separator so prefix matching is safe
	// (e.g. /a/b should not match /a/bc).
	parentWithSep := parent
	if !strings.HasSuffix(parentWithSep, string(filepath.Separator)) {
		parentWithSep += string(filepath.Separator)
	}
	if child == parent {
		return true
	}
	return strings.HasPrefix(child, parentWithSep)
}
