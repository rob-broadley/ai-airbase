// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rob-broadley/ai-airbase/marshal/internal/config"
	"github.com/rob-broadley/ai-airbase/marshal/internal/container"
)

// resolveMountPaths is a pure helper that resolves flag mount paths to absolute
// values. When no flags are supplied the stored mounts are returned unchanged.
// When flags are supplied, cfg.Mounts is updated in place. An empty return
// value is valid — the caller's container.ResolveMounts will then default to
// CWD→/workspace.
func resolveMountPaths(cwd string, mountFlagValues []string, cfg *config.Config) (mounts []string, err error) {
	if len(mountFlagValues) == 0 {
		return cfg.Mounts, nil
	}
	if mounts, err = resolveAbsolutePaths(cwd, mountFlagValues); err != nil {
		return nil, err
	}
	if err := checkBasenameConflicts(mounts); err != nil {
		return nil, err
	}
	cfg.Mounts = mounts
	return mounts, nil
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
// directory basename (which would cause a collision under /workspace/ in the
// container) or if one path is a strict subdirectory of another (which would
// defeat the --mask security guarantee by exposing the inner tree via the
// outer mount).
func checkBasenameConflicts(paths []string) error {
	// Basename collision check.
	seen := map[string]string{}
	for _, p := range paths {
		base := filepath.Base(p)
		if existing, ok := seen[base]; ok {
			return fmt.Errorf("mount conflict: %s and %s both map to /workspace/%s", existing, p, base)
		}
		seen[base] = p
	}
	// Nesting check: reject any pair where one path is an ancestor of another.
	for i, a := range paths {
		for j, b := range paths {
			if i == j {
				continue
			}
			if strings.HasPrefix(b, a+string(filepath.Separator)) {
				return fmt.Errorf("mount conflict: %s is nested inside %s", b, a)
			}
		}
	}
	return nil
}

// resolveMaskPaths validates and resolves --mask flag values against the
// configured project mounts. When no flags are supplied the stored masks are
// returned unchanged. When flags are supplied, cfg.Masks is updated in place.
// mountPaths are the resolved absolute project mount paths (from cfg.Mounts
// after mount resolution). Relative mask paths are resolved against cwd;
// absolute paths are used as-is. The resolved path must fall under exactly
// one configured mount.
func resolveMaskPaths(cwd string, mountPaths, maskFlagValues []string, cfg *config.Config) (masks []string, err error) {
	if len(maskFlagValues) == 0 {
		return cfg.Masks, nil
	}

	seen := make(map[string]struct{})
	resolved := make([]string, 0, len(maskFlagValues))

	for _, raw := range maskFlagValues {
		abs, err := resolveOneMask(raw, cwd, mountPaths)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[abs]; exists {
			return nil, fmt.Errorf("duplicate mask path: %q", raw)
		}
		seen[abs] = struct{}{}
		resolved = append(resolved, abs)
	}

	// Reject nested masks: if one resolved mask is a subdirectory of another,
	// the outer volume silently shadows the inner one in the container, making
	// the inner mask volume inaccessible. Fail early with a clear error.
	for i, a := range resolved {
		for j, b := range resolved {
			if i != j && strings.HasPrefix(b, a+string(filepath.Separator)) {
				return nil, fmt.Errorf("mask conflict: %q is nested inside %q — mask only the outermost directory", b, a)
			}
		}
	}

	cfg.Masks = resolved
	return resolved, nil
}

// resolveOneMask resolves and validates a single raw mask flag value against
// the configured mount paths. It returns the cleaned absolute path, or an
// error if the value contains ':', resolves to a mount root, falls outside
// all configured mounts, or exists as a regular file on the host.
func resolveOneMask(raw, cwd string, mountPaths []string) (string, error) {
	if strings.ContainsRune(raw, ':') {
		return "", fmt.Errorf("mask path %q must not contain ':'", raw)
	}

	var abs string
	if filepath.IsAbs(raw) {
		abs = filepath.Clean(raw)
	} else {
		abs = filepath.Clean(filepath.Join(cwd, raw))
	}

	if err := validateMaskPosition(abs, raw, mountPaths); err != nil {
		return "", err
	}

	info, statErr := os.Stat(abs)
	if statErr != nil && !os.IsNotExist(statErr) {
		return "", fmt.Errorf("stat mask path %q (%s): %w", raw, abs, statErr)
	}
	if statErr == nil && !info.IsDir() {
		return "", fmt.Errorf("mask path %q exists as a regular file on the host", raw)
	}

	return abs, nil
}

// validateMaskPosition checks that abs is a strict subdirectory of one of the
// configured mount paths. It distinguishes two failure modes with separate
// error messages: abs equals a mount root (user likely forgot to append a
// subdirectory name), or abs lies entirely outside all configured mounts.
// raw is the original user-supplied value and is included in error messages.
func validateMaskPosition(abs, raw string, mountPaths []string) error {
	for _, mp := range mountPaths {
		if abs == mp {
			return fmt.Errorf("mask path %q resolves to the mount root %s", raw, abs)
		}
		if strings.HasPrefix(abs, mp+string(filepath.Separator)) {
			return nil
		}
	}
	return fmt.Errorf("no configured mount contains mask path %q (resolved: %s)", raw, abs)
}

// validateSavedMounts checks that the saved mount list is free of basename
// conflicts and nesting. It is called on every command that loads an existing
// config so that stale manual edits fail closed rather than silently
// misapplying the mount/mask layout.
func validateSavedMounts(mounts []string) error {
	if err := checkBasenameConflicts(mounts); err != nil {
		return fmt.Errorf("saved mount config is invalid: %w", err)
	}
	return nil
}

// containerParams holds the resolved parameters needed to create or interact
// with a managed container.
type containerParams struct {
	containerName string
	image         string
	workdir       string
	mountSpecs    []container.MountSpec
	maskVolumes   []container.NamedVolumeMount
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
	if err := validateSavedMounts(cfg.Mounts); err != nil {
		return containerParams{}, err
	}

	image := deps.resolveImage()
	mountSpecs := container.ResolveMounts(cwd, cfg.Mounts)
	maskVolumes, err := container.BuildMaskVolumes(containerName, cfg.Masks, cfg.Mounts, mountSpecs)
	if err != nil {
		return containerParams{}, fmt.Errorf("resolving mask volumes: %w", err)
	}
	if len(cfg.Masks) > 0 {
		deps.logger().Debug("mask volumes resolved", "count", len(maskVolumes), "masks", cfg.Masks)
	}

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
		maskVolumes:   maskVolumes,
		userConfig:    uc,
		workdir:       workdir,
		cmd:           container.DefaultContainerCmd,
	}, nil
}
