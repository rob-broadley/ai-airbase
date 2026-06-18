// SPDX-License-Identifier: AGPL-3.0-or-later
package customisations_test

import (
	"testing"

	"github.com/rob-broadley/ai-airbase/marshal/internal/customisations"
)

func TestMountDirs_ContainsExpectedEntries(t *testing.T) {
	want := []string{"opencode/config", "opencode/share", "opencode/state"}
	got := customisations.MountDirs()
	if len(got) != len(want) {
		t.Fatalf("MountDirs has %d entries, want %d: %v", len(got), len(want), got)
	}
	for i, v := range got {
		if v != want[i] {
			t.Errorf("MountDirs[%d] = %q, want %q", i, v, want[i])
		}
	}
}
