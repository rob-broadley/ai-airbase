// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/rob-broadley/ai-airbase/marshal/internal/config"
	"github.com/rob-broadley/ai-airbase/marshal/internal/container"
	"github.com/rob-broadley/ai-airbase/marshal/internal/customisations"
	"github.com/rob-broadley/ai-airbase/marshal/internal/pathutil"
	"github.com/rob-broadley/ai-airbase/marshal/internal/textutil"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// containerWorkspaceDir is the container-side root where project directories
// are bind-mounted. It mirrors the unexported workspaceDir constant in the
// container package and is used throughout this package for path construction.
const containerWorkspaceDir = "/workspace"

// projectDirPaths holds the per-project host directory paths under
// XDG_DATA_HOME/marshal/projects/<project>/, keyed by MountDirs entry
// (e.g. "opencode/config"). It is a pure data type; no I/O happens
// during construction.
type projectDirPaths map[string]string

// projectRootDir creates the per-project root directory under the shared
// data dir and returns its resolved path. The project root is
// projects/<project> under XDG_DATA_HOME/marshal/. This is the single
// call that ensures the parent exists; mount dirs are created as children
// of this root by the caller.
func projectRootDir(deps Deps, project string) (string, error) {
	root, err := deps.ensureSharedDataDir()("projects/" + project)
	if err != nil {
		return "", err
	}
	return root, nil
}

// provisionProjectDir ensures the per-project host dir tree exists under root
// with 0o700 permissions. The root must already exist (created by
// projectRootDir). Callers (each subcommand) must invoke ensureHostState
// before resolveContainerParams so that buildContainerMounts can use the
// paths directly.
//
// Returns the resolved per-project host directory paths (keyed by MountDirs
// entry) and an error if any ensure or harden step fails. Error wrapping is
// preserved verbatim so existing tests and operators see the same diagnostics.
func provisionProjectDir(deps Deps, project, root string) (projectDirPaths, error) {
	paths := make(projectDirPaths, len(customisations.MountDirs()))
	for _, md := range customisations.MountDirs() {
		resolved := filepath.Join(root, md.HostSubdir)
		if err := os.MkdirAll(resolved, 0o700); err != nil {
			return nil, fmt.Errorf("ensuring project %s dir for project %s: %w", md.HostSubdir, project, err)
		}
		if err := hardenProjectDir(resolved); err != nil {
			return nil, fmt.Errorf("enforcing permissions on %s dir: %w", md.HostSubdir, err)
		}
		paths[md.HostSubdir] = resolved
	}

	return paths, nil
}

// ensureHostState ensures all host-side state needed before the container is
// built: the per-project root directory (projectRootDir), the per-project host
// dir tree (provisionProjectDir), the on-host user defaults directory tree
// (customisations.Ensure), and the copy of defaults files into the per-project
// directory (customisations.CopyDefaults).
//
// This function is the single entry point for host-state setup; it is called
// by each subcommand (runCreate, runRecreate, ensureContainerAndStart,
// ensureContainerAndExec) before resolveContainerParams.
//
// Returns the per-project host directory paths from provisionProjectDir so
// callers can thread them into resolveContainerParams and avoid redundant
// I/O in buildContainerMounts.
func ensureHostState(deps Deps, project string) (projectDirPaths, error) {
	root, err := projectRootDir(deps, project)
	if err != nil {
		if os.IsPermission(err) {
			return nil, fmt.Errorf("permission denied: %w", err)
		}
		return nil, fmt.Errorf("ensuring project root for project %s: %w", project, err)
	}
	paths, err := provisionProjectDir(deps, project, root)
	if err != nil {
		return paths, err
	}
	xdgDataHomeFn := deps.xdgDataHome()
	defaultsDir := customisations.DefaultsDir(xdgDataHomeFn())
	if err := customisations.Ensure(xdgDataHomeFn); err != nil {
		return paths, fmt.Errorf("ensuring user defaults tree: %w", err)
	}
	if err := customisations.SeedGitConfigDefaults(defaultsDir, deps.lookupGitConfigFn()); err != nil {
		return paths, fmt.Errorf("seeding git config defaults: %w", err)
	}
	if err := customisations.CopyDefaults(defaultsDir, root); err != nil {
		return paths, fmt.Errorf("copying defaults to project dir: %w", err)
	}
	return paths, nil
}

// buildContainerMounts returns MountSpec values that bind host credentials
// and per-project opencode directories into the container.
//
// Git config comes from XDG_CONFIG_HOME so backup tools and dotfile managers
// handle it. The opencode configuration directory (settings.json,
// mcp-config.json) is per-project under XDG_DATA_HOME at
// projects/<project>/opencode/config/.
//
// The share/ (containing opencode.db) and state/ directories are both
// per-project under XDG_DATA_HOME so conversation history and checkpoints
// survive container recreates.
//
// The agents/ and skills/ directories baked into the container image are
// left untouched — no whole-directory /opt/cadre mount is used.
//
// The per-project host dir paths are passed in as the 'paths' parameter
// (already ensured, hardened, and write-checked by ensureHostState, which
// is called by each subcommand before resolveContainerParams).
// buildContainerMounts performs no I/O for the per-project dirs. The only
// I/O is the git config setup under XDG_CONFIG_HOME, which writes the file
// if missing on first run.
func buildContainerMounts(deps Deps, paths projectDirPaths) ([]container.MountSpec, error) {
	// User git config — overrides /etc/gitconfig baked into the image.
	// This is the only I/O in this function: setupGitConfigMount writes the
	// git config file to disk if it does not yet exist on first run.
	gitSpec, err := setupGitConfigMount(deps.ensureSharedConfigDirFn(), deps.lookupGitConfigFn())
	if err != nil {
		return nil, err
	}

	// Per-project host dirs — paths passed in by ensureHostState.
	mounts := []container.MountSpec{gitSpec}
	for _, md := range customisations.MountDirs() {
		mounts = append(mounts, container.MountSpec{
			HostPath:      paths[md.HostSubdir],
			ContainerPath: md.ContainerPath,
		})
	}
	return mounts, nil
}

// setupGitConfigMount ensures the git config file exists with host user info and returns its mount spec.
func setupGitConfigMount(ensureConfigDir func(string) (string, error), lookupGit func(string) string) (container.MountSpec, error) {
	gitConfigDir, err := ensureConfigDir("git")
	if err != nil {
		return container.MountSpec{}, fmt.Errorf("ensuring config dir git: %w", err)
	}
	gitConfigPath := filepath.Join(gitConfigDir, "config")
	if err := ensureConfigFile(gitConfigPath, customisations.BuildGitConfigContent(lookupGit)); err != nil {
		return container.MountSpec{}, fmt.Errorf("ensuring git config file: %w", err)
	}
	return container.MountSpec{
		HostPath:      gitConfigPath,
		ContainerPath: container.ContainerGitConfigFile,
	}, nil
}

// ensureConfigFile creates the file at path with defaultContent if it does not
// already exist. Uses O_EXCL so a concurrent create wins cleanly — the file is
// left with whatever content the other writer put there. Returns an error if
// path exists as a directory, since Podman cannot bind-mount a file over a
// directory. The file is created with 0o600 (owner-read/write) permissions.
// A best-effort removal is attempted on write failure to avoid leaving a
// partial file; if the removal itself fails the partial file may persist and
// will be treated as a valid (though possibly corrupt) file on the next run.
func ensureConfigFile(path string, defaultContent []byte) error {
	fi, err := os.Lstat(path)
	if err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("config path is a symlink, refusing to follow: %s", path)
		}
		if fi.IsDir() {
			return fmt.Errorf("config path exists but is a directory: %s", path)
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return nil // another process created it concurrently
		}
		return err
	}
	if _, err = f.Write(defaultContent); err != nil {
		_ = f.Close()
		_ = os.Remove(path) // best-effort: remove partial file so next run may retry
		return err
	}
	return f.Close() // surface any flush/close error
}

// buildUserConfig constructs the container.UserConfig for the calling user.
// HomeDir is set to ContainerUserHome, which matches the container image convention.
// Returns an error if UID is 0 to prevent the container from running as root.
// GID 0 is intentionally permitted: on some distributions (e.g. Fedora) regular
// users may have the root group as their primary GID.
func buildUserConfig(deps Deps) (container.UserConfig, error) {
	uid := deps.Getuid()
	gid := deps.Getgid()
	if uid == 0 {
		return container.UserConfig{}, fmt.Errorf("marshal must not run as root: UID 0")
	}
	return container.UserConfig{
		UID:     uid,
		GID:     gid,
		HomeDir: container.ContainerUserHome,
	}, nil
}

// resolveContainer resolves the project name from projectFlag and deps,
// validates it, and returns both the project name and the derived container
// name. Returns an "invalid project" error when the resolved name fails
// validation.
func resolveContainer(deps Deps, projectFlag string) (project, containerName string, err error) {
	project, err = config.ResolveProject(projectFlag, deps.Getwd)
	if err != nil {
		return "", "", fmt.Errorf("resolving project: %w", err)
	}
	if err = config.ValidateProjectName(project); err != nil {
		return "", "", fmt.Errorf("invalid project: %w", err)
	}
	return project, containerNameForProject(project), nil
}

// statusEntry pairs a status symbol with a display string for a single line
// in the Mounts or Masks section of the status output.
type statusEntry struct {
	symbol  string
	display string
}

// writeStatusSection writes one section of the mounts/masks status block to w.
// When entries is non-empty the header is printed on its own line followed by
// each entry as "  <symbol> <display>". When entries is empty noneLabel is
// printed as a single line (e.g. "Mounts:       none").
func writeStatusSection(w io.Writer, header, noneLabel string, entries []statusEntry) {
	if len(entries) > 0 {
		fmt.Fprintf(w, "%s:\n", header)
		for _, e := range entries {
			fmt.Fprintf(w, "  %s %s\n", e.symbol, e.display)
		}
	} else {
		fmt.Fprintf(w, "%s\n", noneLabel)
	}
}

// renderMountsAndMasks writes the Mounts and Masks sections for an existing
// container (running or stopped) to w, reconciling the configured entries with
// the actual mounts reported by podman inspect.
//
// Reconciliation rules:
//   - Configured mount is active (✓) when a bind mount exists whose Destination
//     equals /workspace/<basename(hostPath)> AND whose Source equals the configured
//     host path.
//   - Configured mount is missing (✗) when no such bind mount is present.
//   - Configured mask is active (✓) when a volume mount exists whose Destination
//     matches the computed container path for that mask.
//   - Configured mask is missing (✗) when no such volume mount is present.
//   - Untracked bind (?) when a bind mount's Destination starts with /workspace/
//     and its Source is not a configured mount.
//   - Untracked volume (?) when a volume mount's Destination starts with /workspace/
//     and is not accounted for by any configured mask.
//
// When there are no entries at all, "none" placeholders are used.
func renderMountsAndMasks(logger *slog.Logger, w io.Writer, configuredMounts, configuredMasks []string, actualMounts []container.ContainerMount) {
	// Index configured mount host paths and compute their expected container
	// destinations in a single pass.
	mountContainerDest := make(map[string]string, len(configuredMounts))
	for _, m := range configuredMounts {
		mountContainerDest[m] = filepath.Join(containerWorkspaceDir, filepath.Base(m))
	}

	// Index actual bind and volume mount destinations.
	// activeBind maps container destination → host source for bind mounts.
	activeBind := make(map[string]string)
	activeVolume := make(map[string]bool)
	for _, am := range actualMounts {
		switch am.Type {
		case "bind":
			activeBind[am.Destination] = am.Source
		case "volume":
			activeVolume[am.Destination] = true
		}
	}

	// Reconcile configured mounts and collect untracked bind mounts.
	var mountEntries []statusEntry
	for _, m := range configuredMounts {
		sym := "✗"
		if activeBind[mountContainerDest[m]] == m {
			sym = "✓"
		}
		mountEntries = append(mountEntries, statusEntry{sym, textutil.SanitiseForTerminal(m)})
	}
	for _, am := range actualMounts {
		if am.Type != "bind" || !strings.HasPrefix(am.Destination, containerWorkspaceDir+"/") {
			continue
		}
		if _, ok := mountContainerDest[am.Source]; ok {
			continue // tracked
		}
		mountEntries = append(mountEntries, statusEntry{"?", textutil.SanitiseForTerminal(am.Source)})
	}

	// Compute container destination for each configured mask and reconcile.
	accountedVolumes := make(map[string]bool)
	var maskEntries []statusEntry
	for _, mask := range configuredMasks {
		containerDest := maskContainerPath(mask, configuredMounts, mountContainerDest)
		if containerDest == "" {
			logger.Debug("configured mask has no parent bind mount", "mask", mask)
		}
		sym := "✗"
		if containerDest != "" && activeVolume[containerDest] {
			sym = "✓"
			accountedVolumes[containerDest] = true
		}
		maskEntries = append(maskEntries, statusEntry{sym, textutil.SanitiseForTerminal(mask)})
	}

	// Collect untracked volume mounts.
	for _, am := range actualMounts {
		if am.Type != "volume" || !strings.HasPrefix(am.Destination, containerWorkspaceDir+"/") {
			continue
		}
		if accountedVolumes[am.Destination] {
			continue // tracked
		}
		hostPath := hostEquivalentPath(am.Destination, mountContainerDest, activeBind)
		if hostPath == am.Destination {
			logger.Debug("untracked mask path not resolved to host path", "path", am.Destination)
		}
		maskEntries = append(maskEntries, statusEntry{"?", textutil.SanitiseForTerminal(hostPath)})
	}

	writeStatusSection(w, "Mounts", "Mounts:       none", mountEntries)
	writeStatusSection(w, "Masks", "Masks:        none", maskEntries)
}

// hostEquivalentPath derives the host-side path equivalent to containerPath by
// reversing the mountContainerDest mapping (hostPath → containerDest). It finds
// the entry whose containerDest value is a prefix of containerPath, computes the
// relative suffix, and joins it onto the host path. If no match is found in
// mountContainerDest, it falls back to actualBind (containerDest → hostSource)
// using the same logic, covering untracked volumes under untracked bind mounts.
// Returns containerPath unchanged when no matching bind mount is found in either map.
func hostEquivalentPath(containerPath string, mountContainerDest, actualBind map[string]string) string {
	for hostPath, containerDest := range mountContainerDest {
		prefix := containerDest + string(filepath.Separator)
		if strings.HasPrefix(containerPath, prefix) {
			rel := containerPath[len(containerDest):]
			return hostPath + rel
		}
		if containerPath == containerDest {
			return hostPath
		}
	}
	// Fallback: check actual bind mounts (containerDest → hostSource).
	for containerDest, hostSource := range actualBind {
		prefix := containerDest + string(filepath.Separator)
		if strings.HasPrefix(containerPath, prefix) {
			rel := containerPath[len(containerDest):]
			return hostSource + rel
		}
		if containerPath == containerDest {
			return hostSource
		}
	}
	return containerPath
}

// maskContainerPath returns the container-side destination for mask given the
// configured mount host paths and their pre-computed container destinations.
// It walks configuredMounts looking for the mount that contains mask, then
// computes the relative path from that mount root to the mask and joins it
// onto the mount's container destination. Returns an empty string when mask
// does not fall inside any configured mount.
func maskContainerPath(mask string, configuredMounts []string, mountContainerDest map[string]string) string {
	for _, m := range configuredMounts {
		prefix := m + string(filepath.Separator)
		if strings.HasPrefix(mask, prefix) || mask == m {
			rel, err := filepath.Rel(m, mask)
			if err != nil {
				// Both m and mask are guaranteed to be absolute paths, so filepath.Rel
				// cannot fail here. Panic to surface any future invariant violation.
				panic(fmt.Sprintf("maskContainerPath: unexpected filepath.Rel error: %v", err))
			}
			return filepath.Join(mountContainerDest[m], rel)
		}
	}
	return ""
}

// hardenProjectDir enforces strict permissions on a project directory and its
// contents. It ensures directories have 0o700 and files have 0o600 permissions,
// verifies that the root directory is owner-writable (the 0o200 bit is set),
// and aborts if the root directory itself is a symlink (which would make the
// bind mount target a symlink, and os.RemoveAll would follow it). It returns
// nil when the directory does not exist.
//
// Nested symlinks inside the directory are checked for sandbox escape: a
// symlink whose target resolves to a path outside the bind mount directory is
// a security violation (the container could follow it to read or write host
// files outside the sandbox). Symlinks that stay inside the bind mount
// (e.g. relative links created by package managers) are allowed. Symlinks
// are skipped during the permission walk (chmod on a symlink is meaningless).
//
// This function is intended for hardening pre-existing directories that may
// have had their permissions loosened by external processes. For freshly
// created directories from ensureSharedDataDir, the 0o700 permissions are
// already set by MkdirAll — hardenProjectDir is still called to perform the
// root write-permission verification. Callers may also perform an additional
// stateless write-permission check via syscall.Access.
func hardenProjectDir(dir string) error {
	fi, err := os.Lstat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return errors.New("security violation: directory is a symlink")
	}
	if fi.Mode().Perm()&0o200 == 0 {
		return fmt.Errorf("permission denied: %s", dir)
	}

	// Resolve the bind mount root to detect sandbox escapes. filepath.EvalSymlinks
	// returns the real path with all symlinks resolved.
	rootResolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return fmt.Errorf("resolving bind mount root %s: %w", dir, err)
	}

	return filepath.WalkDir(dir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		// Check symlinks for sandbox escape: resolve the target and verify it
		// stays under the bind mount root. The container is untrusted AI code
		// that could follow a symlink to read/write host files outside the
		// sandbox. A broken symlink (target does not exist) is not an escape
		// — the container would get ENOENT, not access outside the sandbox.
		if d.Type()&os.ModeSymlink != 0 {
			targetResolved, err := filepath.EvalSymlinks(path)
			if err != nil {
				if os.IsNotExist(err) {
					return nil
				}
				return fmt.Errorf("resolving symlink %s: %w", path, err)
			}
			if !pathutil.IsPathUnder(targetResolved, rootResolved) {
				return fmt.Errorf("security violation: symlink %s escapes the bind mount (resolves to %s)", path, targetResolved)
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		var targetMode os.FileMode
		if d.IsDir() {
			targetMode = 0o700
		} else {
			targetMode = 0o600
		}
		if info.Mode().Perm() != targetMode {
			if err := os.Chmod(path, targetMode); err != nil {
				return err
			}
		}
		return nil
	})
}
