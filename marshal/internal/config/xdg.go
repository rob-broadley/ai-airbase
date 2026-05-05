// SPDX-License-Identifier: AGPL-3.0-or-later
package config

import (
	"os"
	"path/filepath"
)

// xdgBaseDir resolves an XDG base directory.
// It returns the value of envVar when set; otherwise it joins the user home
// directory with relativeFallback. Returns an empty string when the home
// directory cannot be determined.
func xdgBaseDir(envVar, relativeFallback string) string {
	if v := os.Getenv(envVar); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, relativeFallback)
}

// xdgConfigHome returns $XDG_CONFIG_HOME if set, otherwise ~/.config.
// Returns an empty string when the home directory cannot be determined.
func xdgConfigHome() string {
	return xdgBaseDir("XDG_CONFIG_HOME", ".config")
}

// xdgDataHome returns $XDG_DATA_HOME if set, otherwise ~/.local/share.
// Returns an empty string when the home directory cannot be determined.
func xdgDataHome() string {
	return xdgBaseDir("XDG_DATA_HOME", filepath.Join(".local", "share"))
}
