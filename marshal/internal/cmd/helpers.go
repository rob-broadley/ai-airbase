// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/rob-broadley/ai-airbase/marshal/internal/config"
	"github.com/rob-broadley/ai-airbase/marshal/internal/container"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// containerWorkspaceDir is the container-side root where project directories
// are bind-mounted. It mirrors the unexported workspaceDir constant in the
// container package and is used throughout this package for path construction.
const containerWorkspaceDir = "/workspace"

// buildCredentialMounts returns MountSpec values that bind host credential files
// into the container. These mounts are shared across all projects unless noted.
//
// Config files (settings, mcp-config, git config, etc.) come from XDG_CONFIG_HOME
// so backup tools and dotfile managers handle them.
//
// The share/ (containing opencode.db) and state/ directories are both per-project under XDG_DATA_HOME
// so conversation history and checkpoints survive container recreates.
//
// The agents/ and skills/ directories baked into the container image are left
// untouched — no whole-directory /opt/cadre mount is used.
func buildCredentialMounts(deps Deps, project string) ([]container.MountSpec, error) {
	specs := make([]container.MountSpec, 0, 8)
	ensureConfigDir := deps.ensureSharedConfigDirFn()
	lookupGit := deps.lookupGitConfigFn()

	// User git config — overrides /etc/gitconfig baked into the image.
	gitConfigDir, err := ensureConfigDir("git")
	if err != nil {
		return nil, fmt.Errorf("ensuring config dir git: %w", err)
	}
	gitConfigPath := filepath.Join(gitConfigDir, "config")
	if err := ensureConfigFile(gitConfigPath, buildGitConfigContent(lookupGit)); err != nil {
		return nil, fmt.Errorf("ensuring git config file: %w", err)
	}
	specs = append(specs, container.MountSpec{
		HostPath:      gitConfigPath,
		ContainerPath: container.ContainerGitConfigFile,
	})

	// User-editable config files — individual file mounts from XDG_CONFIG.
	configDir, err := ensureConfigDir("opencode")
	if err != nil {
		return nil, fmt.Errorf("ensuring config dir opencode: %w", err)
	}
	specs = append(specs, container.MountSpec{
		HostPath:      configDir,
		ContainerPath: container.ContainerOpencodeConfigDir,
	})

	// Per-project session store and session state — both isolated by project.
	projectShareDir, err := deps.ensureSharedDataDir()("projects/" + project + "/share")
	if err != nil {
		return nil, fmt.Errorf("ensuring project share dir for %s: %w", project, err)
	}
	specs = append(specs, container.MountSpec{
		HostPath:      projectShareDir,
		ContainerPath: container.ContainerOpencodeDataDir,
	})

	projectStateDir, err := deps.ensureSharedDataDir()("projects/" + project + "/state")
	if err != nil {
		return nil, fmt.Errorf("ensuring project state dir for project %s: %w", project, err)
	}
	specs = append(specs, container.MountSpec{
		HostPath:      projectStateDir,
		ContainerPath: container.ContainerOpencodeStateDir,
	})

	return specs, nil
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
