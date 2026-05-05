// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/rob-broadley/ai-airbase/marshal/internal/config"
	"github.com/rob-broadley/ai-airbase/marshal/internal/container"
)

// resolveMountPaths is a pure helper that resolves flag mount paths to absolute
// values and reports whether they differ from the paths already stored in cfg.
// When no flags are supplied the stored mounts are returned unchanged and
// changed is false. When flags are supplied, cfg.Mounts is updated in place
// and changed reports whether the resulting set differs from what was stored.
// An empty return value is valid — the caller's container.ResolveMounts will
// then default to CWD→/workspace.
func resolveMountPaths(cwd string, mountFlagValues []string, cfg *config.Config) (mounts []string, changed bool, err error) {
	if len(mountFlagValues) == 0 {
		return cfg.Mounts, false, nil
	}
	if mounts, err = resolveAbsolutePaths(cwd, mountFlagValues); err != nil {
		return nil, false, err
	}
	if err := checkBasenameConflicts(mounts); err != nil {
		return nil, false, err
	}
	changed = !mountsEqual(cfg.Mounts, mounts)
	cfg.Mounts = mounts
	return
}

// resolveAbsolutePaths validates that no path contains ':' (which would be
// misinterpreted as a mount-spec separator) and converts relative paths to
// absolute using cwd as the base.
func resolveAbsolutePaths(cwd string, rawPaths []string) ([]string, error) {
	resolved := make([]string, 0, len(rawPaths))
	for _, p := range rawPaths {
		if strings.ContainsRune(p, ':') {
			return nil, fmt.Errorf("mount path %q must not contain ':'", p)
		}
		if !filepath.IsAbs(p) {
			p = filepath.Clean(filepath.Join(cwd, p))
		}
		resolved = append(resolved, p)
	}
	return resolved, nil
}

// checkBasenameConflicts returns an error if any two paths share the same
// directory basename, which would cause a collision under /workspace/ in the
// container (each mount lands at /workspace/<basename>).
func checkBasenameConflicts(paths []string) error {
	seen := map[string]string{}
	for _, p := range paths {
		base := filepath.Base(p)
		if existing, ok := seen[base]; ok {
			return fmt.Errorf("mount conflict: %s and %s both map to /workspace/%s", existing, p, base)
		}
		seen[base] = p
	}
	return nil
}

// mountsEqual reports whether two mount-path slices contain the same paths
// regardless of order. Both slices are copied before sorting so the originals
// are not mutated.
func mountsEqual(a, b []string) bool {
	aSorted := slices.Clone(a)
	bSorted := slices.Clone(b)
	slices.Sort(aSorted)
	slices.Sort(bSorted)
	return slices.Equal(aSorted, bSorted)
}

// containerParams holds the resolved parameters needed to create or interact
// with a managed container.
type containerParams struct {
	containerName string
	image         string
	workdir       string
	mountSpecs    []container.MountSpec
	userConfig    container.UserConfig
}

// resolveContainerParams resolves the project name, working directory, config,
// mounts, credentials, and user identity into a single containerParams value.
// It returns the resolved params together with a mountsChanged bool that is
// true when the --mount flags produced a different mount set than what was
// previously stored in config (used by the caller to decide whether the
// container must be recreated). When mountsChanged is true the updated config
// is explicitly persisted here before the params are returned.
// It is the single source of truth for how a container is configured and is
// called by both ensureContainerAndStart/Exec and runRecreate.
func resolveContainerParams(deps Deps, projectFlag string, mountFlagValues []string) (params containerParams, mountsChanged bool, err error) {
	project := config.ResolveProject(projectFlag, deps.Getwd)
	if err := config.ValidateProjectName(project); err != nil {
		return containerParams{}, false, err
	}
	containerName := containerNameForProject(project)

	cwd, err := deps.Getwd()
	if err != nil {
		return containerParams{}, false, fmt.Errorf("getting working directory: %w", err)
	}

	cfg, err := config.Load(project)
	if err != nil {
		return containerParams{}, false, fmt.Errorf("loading config: %w", err)
	}

	mounts, mountsChanged, err := resolveMountPaths(cwd, mountFlagValues, cfg)
	if err != nil {
		return containerParams{}, false, err
	}

	// resolveMountPaths is a pure helper; persist the updated mount list here
	// when it reports that the flags produced a different set than was saved.
	if mountsChanged {
		if err := deps.saveConfig()(project, cfg); err != nil {
			return containerParams{}, false, fmt.Errorf("saving config: %w", err)
		}
	}

	image := deps.resolveImage()
	mountSpecs := container.ResolveMounts(cwd, mounts)

	workdir := container.WorkdirFromMounts(mountSpecs)

	credMounts, err := buildCredentialMounts(deps)
	if err != nil {
		return containerParams{}, false, err
	}
	mountSpecs = append(mountSpecs, credMounts...)

	uc := buildUserConfig(deps)

	return containerParams{
		containerName: containerName,
		image:         image,
		mountSpecs:    mountSpecs,
		userConfig:    uc,
		workdir:       workdir,
	}, mountsChanged, nil
}
