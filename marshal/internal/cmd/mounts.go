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
			p = filepath.Join(cwd, p)
		}
		resolved = append(resolved, filepath.Clean(p))
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
	cmd           []string
}

// resolveContainerParams resolves the project name, working directory, config,
// mounts, credentials, and user identity into a single containerParams value.
// Mounts are always read from the saved project config; callers that need to
// update the mount list (e.g. the create subcommand) must persist the config
// before calling this function.
// It is the single source of truth for how a container is configured and is
// called by ensureContainerAndStart/Exec and runRecreate.
func resolveContainerParams(deps Deps, projectFlag string) (params containerParams, err error) {
	project, containerName, err := resolveContainer(deps, projectFlag)
	if err != nil {
		return containerParams{}, err
	}

	cwd, err := deps.Getwd()
	if err != nil {
		return containerParams{}, fmt.Errorf("getting working directory: %w", err)
	}

	cfg, err := config.Load(project)
	if err != nil {
		return containerParams{}, fmt.Errorf("loading config: %w", err)
	}

	image := deps.resolveImage()
	mountSpecs := container.ResolveMounts(cwd, cfg.Mounts)

	workdir := container.WorkdirFromMounts(mountSpecs)

	credMounts, err := buildCredentialMounts(deps, project)
	if err != nil {
		return containerParams{}, err
	}
	mountSpecs = append(mountSpecs, credMounts...)

	uc, err := buildUserConfig(deps)
	if err != nil {
		return containerParams{}, err
	}

	return containerParams{
		containerName: containerName,
		image:         image,
		mountSpecs:    mountSpecs,
		userConfig:    uc,
		workdir:       workdir,
		cmd:           container.DefaultContainerCmd,
	}, nil
}
