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
	"syscall"

	"github.com/rob-broadley/ai-airbase/marshal/internal/config"
	"github.com/rob-broadley/ai-airbase/marshal/internal/container"
	"github.com/rob-broadley/ai-airbase/marshal/internal/customisations"
	"github.com/rob-broadley/ai-airbase/marshal/internal/hostinfo"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// containerWorkspaceDir is the container-side root where project directories
// are bind-mounted. It mirrors the unexported workspaceDir constant in the
// container package and is used throughout this package for path construction.
const containerWorkspaceDir = "/workspace"

// writePermissionBit is the mode bit for write permission in syscall.Access
// (W_OK from the access(2) syscall).
const writePermissionBit = 0x2

// projectDirPaths holds the per-project host directory paths under
// XDG_DATA_HOME/marshal/projects/<project>/opencode/. It is a pure data
// struct; no I/O happens during construction.
type projectDirPaths struct {
	config string
	share  string
	state  string
}

// provisionProjectDir ensures the per-project host dir tree exists with 0o700
// permissions and that the config dir is writeable. This is the host-side
// state needed before the container is built. Callers (each subcommand) must
// invoke ensureHostState before resolveContainerParams so that
// buildContainerMounts can use the paths directly.
//
// Returns the resolved per-project host directory paths (config, share,
// state) and an error if any ensure, harden, or access step fails. Error
// wrapping is preserved verbatim from the original extraction so existing
// tests and operators see the same diagnostics.
func provisionProjectDir(deps Deps, project string) (projectDirPaths, error) {
	// User-editable config files — project-specific config directory.
	configDir, err := deps.ensureSharedDataDir()("projects/" + project + "/opencode/config")
	if err != nil {
		if os.IsPermission(err) {
			return projectDirPaths{}, fmt.Errorf("permission denied: %w", err)
		}
		return projectDirPaths{}, fmt.Errorf("ensuring project config dir for project %s: %w", project, err)
	}
	if err := hardenProjectDir(configDir); err != nil {
		return projectDirPaths{}, fmt.Errorf("enforcing permissions on config dir: %w", err)
	}

	// Verify write permissions on the config directory. syscall.Access is a
	// stateless check that doesn't create any files in the target directory.
	if err := syscall.Access(configDir, writePermissionBit); err != nil {
		if os.IsPermission(err) {
			return projectDirPaths{}, fmt.Errorf("permission denied: %s", configDir)
		}
		return projectDirPaths{}, fmt.Errorf("verifying write permissions on config dir: %w", err)
	}

	// Per-project session store — both isolated by project.
	projectShareDir, err := deps.ensureSharedDataDir()("projects/" + project + "/opencode/share")
	if err != nil {
		return projectDirPaths{}, fmt.Errorf("ensuring project share dir for %s: %w", project, err)
	}
	if err := hardenProjectDir(projectShareDir); err != nil {
		return projectDirPaths{}, fmt.Errorf("enforcing permissions on share dir: %w", err)
	}

	// Per-project session state — isolated by project.
	projectStateDir, err := deps.ensureSharedDataDir()("projects/" + project + "/opencode/state")
	if err != nil {
		return projectDirPaths{}, fmt.Errorf("ensuring project state dir for project %s: %w", project, err)
	}
	if err := hardenProjectDir(projectStateDir); err != nil {
		return projectDirPaths{}, fmt.Errorf("enforcing permissions on state dir: %w", err)
	}

	return projectDirPaths{config: configDir, share: projectShareDir, state: projectStateDir}, nil
}

// ensureHostState ensures all host-side state needed before the container is
// built: the per-project host dir tree (provisionProjectDir) and the
// on-host user defaults directory tree (customisations.Ensure).
//
// This function is the single entry point for host-state setup; it is called
// by each subcommand (runCreate, runRecreate, ensureContainerAndStart,
// ensureContainerAndExec) before resolveContainerParams.
//
// Returns the per-project host directory paths from provisionProjectDir so
// callers can thread them into resolveContainerParams and avoid redundant
// I/O in buildContainerMounts.
func ensureHostState(deps Deps, project string) (projectDirPaths, error) {
	paths, err := provisionProjectDir(deps, project)
	if err != nil {
		return paths, err
	}
	if err := customisations.Ensure(hostinfo.XDGDataHome); err != nil {
		return paths, err
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
	return []container.MountSpec{
		gitSpec,
		{HostPath: paths.config, ContainerPath: container.ContainerOpencodeConfigDir},
		{HostPath: paths.share, ContainerPath: container.ContainerOpencodeDataDir},
		{HostPath: paths.state, ContainerPath: container.ContainerOpencodeStateDir},
	}, nil
}

// setupGitConfigMount ensures the git config file exists with host user info and returns its mount spec.
func setupGitConfigMount(ensureConfigDir func(string) (string, error), lookupGit func(string) string) (container.MountSpec, error) {
	gitConfigDir, err := ensureConfigDir("git")
	if err != nil {
		return container.MountSpec{}, fmt.Errorf("ensuring config dir git: %w", err)
	}
	gitConfigPath := filepath.Join(gitConfigDir, "config")
	if err := ensureConfigFile(gitConfigPath, buildGitConfigContent(lookupGit)); err != nil {
		return container.MountSpec{}, fmt.Errorf("ensuring git config file: %w", err)
	}
	return container.MountSpec{
		HostPath:      gitConfigPath,
		ContainerPath: container.ContainerGitConfigFile,
	}, nil
}

// buildGitConfigContent generates a minimal git [user] section from the host
// git configuration. Values are double-quoted per the gitconfig spec to handle
// backslashes, semicolons, and hash characters safely. Control characters are
// stripped before quoting. Returns an empty byte slice when neither value is
// set so the file is still created, allowing the user to populate it manually.
func buildGitConfigContent(lookup func(string) string) []byte {
	name := sanitizeForTerminal(lookup("user.name"))
	email := sanitizeForTerminal(lookup("user.email"))
	if name == "" && email == "" {
		return []byte{}
	}
	var b strings.Builder
	b.WriteString("[user]\n")
	if name != "" {
		fmt.Fprintf(&b, "\tname = %s\n", gitQuote(name))
	}
	if email != "" {
		fmt.Fprintf(&b, "\temail = %s\n", gitQuote(email))
	}
	return []byte(b.String())
}

// gitQuote wraps a git config value in double quotes and escapes the four
// sequences the gitconfig spec recognises inside double-quoted strings: \\ \" \n \t.
func gitQuote(v string) string {
	v = strings.ReplaceAll(v, `\`, `\\`)
	v = strings.ReplaceAll(v, `"`, `\"`)
	v = strings.ReplaceAll(v, "\n", `\n`)
	v = strings.ReplaceAll(v, "\t", `\t`)
	return `"` + v + `"`
}

// sanitizeForTerminal strips C0 control characters (< 0x20), DEL (0x7f), C1
// control characters (0x80–0x9F, including the 8-bit CSI U+009B), Unicode
// bidirectional formatting characters (U+200E–U+200F, U+202A–U+202E,
// U+2066–U+2069), line/paragraph separators (U+2028–U+2029), zero-width
// characters (U+200B Zero Width Space, U+200C Zero Width Non-Joiner,
// U+200D Zero Width Joiner, U+FEFF BOM/Zero Width No-Break Space), and the
// Arabic Letter Mark (U+061C) from a string before writing it to terminal
// output, guarding against escape sequence injection and Trojan-source
// visual-spoof attacks from untrusted sources such as OCI image labels.
func sanitizeForTerminal(v string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) ||
			(r >= 0x200e && r <= 0x200f) || // LRM, RLM
			(r >= 0x202a && r <= 0x202e) || // bidi embedding/override
			(r >= 0x2066 && r <= 0x2069) || // bidi isolates
			r == 0x2028 || r == 0x2029 || // line/paragraph separator
			r == 0x200b || r == 0x200c || r == 0x200d || r == 0xfeff || // zero-width chars
			r == 0x061c { // Arabic Letter Mark
			return -1 // drop control and bidi formatting characters
		}
		return r
	}, v)
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
		mountEntries = append(mountEntries, statusEntry{sym, sanitizeForTerminal(m)})
	}
	for _, am := range actualMounts {
		if am.Type != "bind" || !strings.HasPrefix(am.Destination, containerWorkspaceDir+"/") {
			continue
		}
		if _, ok := mountContainerDest[am.Source]; ok {
			continue // tracked
		}
		mountEntries = append(mountEntries, statusEntry{"?", sanitizeForTerminal(am.Source)})
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
		maskEntries = append(maskEntries, statusEntry{sym, sanitizeForTerminal(mask)})
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
		maskEntries = append(maskEntries, statusEntry{"?", sanitizeForTerminal(hostPath)})
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
			if !isPathUnder(targetResolved, rootResolved) {
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

// isPathUnder reports whether child is the same as parent or sits inside it.
// Both paths must be cleaned and absolute.
func isPathUnder(child, parent string) bool {
	// Ensure parent has a trailing separator so prefix matching is safe
	// (e.g. /a/b should not match /a/bc).
	parentWithSep := parent
	if !strings.HasSuffix(parentWithSep, string(filepath.Separator)) {
		parentWithSep += string(filepath.Separator)
	}
	if child == parent {
		return true
	}
	return strings.HasPrefix(child, parentWithSep)
}
