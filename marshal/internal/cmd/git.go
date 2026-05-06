// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd

import (
	"os"
	"os/exec"
	"strings"
)

// lookupHostGitConfig reads a git config key from the host using git(1) with
// --global scope and returns the trimmed value, or an empty string if the key
// is unset or git is unavailable. Git-specific environment variables that could
// redirect the lookup to a project-local config are stripped:
//   - GIT_DIR, GIT_CONFIG, GIT_CONFIG_GLOBAL: redirect which files are read
//   - GIT_CONFIG_NOSYSTEM: would suppress system config but not the issue here
//
// GIT_CONFIG_COUNT/KEY/VALUE are intentionally not stripped — they cannot
// override --global scope reads so they are safe to leave in place.
// Note: users who rely solely on GIT_CONFIG_GLOBAL (e.g. dotfile managers) will
// get empty results here; they can populate the file manually after first run.
func lookupHostGitConfig(key string) string {
	cmd := exec.Command("git", "config", "--global", "--", key)
	cmd.Env = filterEnv(os.Environ(), "GIT_DIR", "GIT_CONFIG", "GIT_CONFIG_GLOBAL", "GIT_CONFIG_NOSYSTEM")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// filterEnv returns a copy of env with any entry whose name matches one of the
// given keys removed.
func filterEnv(env []string, keys ...string) []string {
	drop := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		drop[k] = struct{}{}
	}
	filtered := make([]string, 0, len(env))
	for _, e := range env {
		name, _, _ := strings.Cut(e, "=")
		if _, blocked := drop[name]; !blocked {
			filtered = append(filtered, e)
		}
	}
	return filtered
}
