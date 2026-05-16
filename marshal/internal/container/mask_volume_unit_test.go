// SPDX-License-Identifier: AGPL-3.0-or-later
package container

import (
	"testing"
)

// TestMaskVolumeSpec_DotVenv verifies that a .venv mask under the mount root
// produces the correct volume name and container path.
func TestMaskVolumeSpec_DotVenv(t *testing.T) {
	// Given containerName=marshal-myapp, mask=/home/user/myapp/.venv,
	// mountRoot=/home/user/myapp, containerMountRoot=/workspace/myapp
	spec := maskVolumeSpec(
		"marshal-myapp",
		"/home/user/myapp/.venv",
		"/home/user/myapp",
		"/workspace/myapp",
	)

	// Then the volume name is marshal-myapp-mask-myapp-.venv
	// (mount basename "myapp" is included to prevent collisions in multi-mount projects)
	if spec.Name != "marshal-myapp-mask-myapp-.venv" {
		t.Errorf("expected Name=%q, got %q", "marshal-myapp-mask-myapp-.venv", spec.Name)
	}
	// And the container path is /workspace/myapp/.venv
	if spec.ContainerPath != "/workspace/myapp/.venv" {
		t.Errorf("expected ContainerPath=%q, got %q", "/workspace/myapp/.venv", spec.ContainerPath)
	}
}

// TestMaskVolumeSpec_CollisionPrevented verifies that two distinct relative paths
// that would collide under a naïve slash→dash replacement produce different
// volume names. Specifically, src/vendor and src-vendor must NOT share a name.
func TestMaskVolumeSpec_CollisionPrevented(t *testing.T) {
	// Given containerName=marshal-myapp, mountRoot=/home/user/myapp,
	// containerMountRoot=/workspace/myapp
	specSlash := maskVolumeSpec(
		"marshal-myapp",
		"/home/user/myapp/src/vendor",
		"/home/user/myapp",
		"/workspace/myapp",
	)
	specHyphen := maskVolumeSpec(
		"marshal-myapp",
		"/home/user/myapp/src-vendor",
		"/home/user/myapp",
		"/workspace/myapp",
	)

	// Then the two paths must produce distinct and injectively encoded volume names
	if specSlash.Name == specHyphen.Name {
		t.Errorf("collision: src/vendor and src-vendor both produced volume name %q", specSlash.Name)
	}
	if specSlash.Name != "marshal-myapp-mask-myapp-src-vendor" {
		t.Errorf("expected specSlash.Name=%q, got %q", "marshal-myapp-mask-myapp-src-vendor", specSlash.Name)
	}
	if specHyphen.Name != "marshal-myapp-mask-myapp-src--vendor" {
		t.Errorf("expected specHyphen.Name=%q, got %q", "marshal-myapp-mask-myapp-src--vendor", specHyphen.Name)
	}
}

// TestMaskVolumeSpec_SrcVendor verifies that a nested path src/vendor produces
// a volume name with slashes replaced by dashes.
func TestMaskVolumeSpec_SrcVendor(t *testing.T) {
	// Given containerName=marshal-myapp, mask=/home/user/myapp/src/vendor,
	// mountRoot=/home/user/myapp, containerMountRoot=/workspace/myapp
	spec := maskVolumeSpec(
		"marshal-myapp",
		"/home/user/myapp/src/vendor",
		"/home/user/myapp",
		"/workspace/myapp",
	)

	// Then the volume name includes mount basename "myapp" and replaces '/' with '-' in the rel path
	if spec.Name != "marshal-myapp-mask-myapp-src-vendor" {
		t.Errorf("expected Name=%q, got %q", "marshal-myapp-mask-myapp-src-vendor", spec.Name)
	}
	// And the container path preserves the directory separator
	if spec.ContainerPath != "/workspace/myapp/src/vendor" {
		t.Errorf("expected ContainerPath=%q, got %q", "/workspace/myapp/src/vendor", spec.ContainerPath)
	}
}

// TestMaskVolumeSpec_MultiMountNoCollision verifies that two mounts with the
// same relative subpath (e.g. ".venv") produce distinct volume names.
// This is the B1 regression test: without the mount-basename component both
// masks would produce the same name, silently sharing one backing store.
func TestMaskVolumeSpec_MultiMountNoCollision(t *testing.T) {
	// Given two mounts foo and bar, each containing a .venv mask
	specFoo := maskVolumeSpec(
		"marshal-myapp",
		"/home/user/foo/.venv",
		"/home/user/foo",
		"/workspace/foo",
	)
	specBar := maskVolumeSpec(
		"marshal-myapp",
		"/home/user/bar/.venv",
		"/home/user/bar",
		"/workspace/bar",
	)

	// Then the volume names must be distinct
	if specFoo.Name == specBar.Name {
		t.Errorf("collision: foo/.venv and bar/.venv both produced volume name %q", specFoo.Name)
	}
	// And each name must embed its mount basename
	if specFoo.Name != "marshal-myapp-mask-foo-.venv" {
		t.Errorf("expected specFoo.Name=%q, got %q", "marshal-myapp-mask-foo-.venv", specFoo.Name)
	}
	if specBar.Name != "marshal-myapp-mask-bar-.venv" {
		t.Errorf("expected specBar.Name=%q, got %q", "marshal-myapp-mask-bar-.venv", specBar.Name)
	}
}
