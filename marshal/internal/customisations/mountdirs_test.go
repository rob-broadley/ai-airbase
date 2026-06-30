// SPDX-License-Identifier: AGPL-3.0-or-later
package customisations_test

import (
	"testing"

	"github.com/rob-broadley/ai-airbase/marshal/internal/container"
	"github.com/rob-broadley/ai-airbase/marshal/internal/customisations"
)

func TestMountDirs_ContainsExpectedEntries(t *testing.T) {
	want := []customisations.MountDir{
		{HostSubdir: "opencode/config", ContainerPath: container.ContainerOpencodeConfigDir},
		{HostSubdir: "opencode/share", ContainerPath: container.ContainerOpencodeDataDir},
		{HostSubdir: "opencode/state", ContainerPath: container.ContainerOpencodeStateDir},
		{HostSubdir: "git/config", ContainerPath: container.ContainerGitConfigDir, ReadOnly: true},
	}
	got := customisations.MountDirs()
	if len(got) != len(want) {
		t.Fatalf("MountDirs has %d entries, want %d: %v", len(got), len(want), got)
	}
	for i, v := range got {
		if v != want[i] {
			t.Errorf("MountDirs[%d] = %+v, want %+v", i, v, want[i])
		}
	}
}
