// SPDX-License-Identifier: AGPL-3.0-or-later
package hostinfo

import (
	"os"
	"testing"
)

// setenv sets an env var for the duration of a test and restores it on cleanup.
func setenv(t *testing.T, key, value string) {
	t.Helper()
	original, existed := os.LookupEnv(key)
	os.Setenv(key, value)
	t.Cleanup(func() {
		if existed {
			os.Setenv(key, original)
		} else {
			os.Unsetenv(key)
		}
	})
}

// unsetenv unsets an env var for the duration of a test and restores it on cleanup.
func unsetenv(t *testing.T, key string) {
	t.Helper()
	original, existed := os.LookupEnv(key)
	os.Unsetenv(key)
	t.Cleanup(func() {
		if existed {
			os.Setenv(key, original)
		}
	})
}

// ---------------------------------------------------------------------------
// xdgBaseDir — home directory unavailable
// ---------------------------------------------------------------------------

// TestXdgConfigHome_BothEnvAndHomeMissing_ReturnsEmptyString verifies that
// XDGConfigHome returns "" when XDG_CONFIG_HOME is unset and HOME is unset,
// so that os.UserHomeDir cannot resolve a home directory from the environment.
// When the process runs in an environment where UserHomeDir falls back to the
// passwd database and still succeeds, the test is skipped.
func TestXdgConfigHome_BothEnvAndHomeMissing_ReturnsEmptyString(t *testing.T) {
	// Given XDG_CONFIG_HOME is unset and HOME is unset
	unsetenv(t, "XDG_CONFIG_HOME")
	unsetenv(t, "HOME")

	// If UserHomeDir succeeds despite HOME being unset (e.g. via passwd lookup),
	// the test environment cannot exercise the error path — skip gracefully.
	if _, err := os.UserHomeDir(); err == nil {
		t.Skip("UserHomeDir succeeded without HOME set (passwd fallback active) — skipping on this host")
	}

	// When XDGConfigHome is called
	got := XDGConfigHome()

	// Then the result is an empty string
	if got != "" {
		t.Errorf("expected empty string when home is unavailable, got %q", got)
	}
}
