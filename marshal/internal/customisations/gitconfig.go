// SPDX-License-Identifier: AGPL-3.0-or-later
package customisations

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rob-broadley/ai-airbase/marshal/internal/textutil"
)

// GitQuote wraps a git config value in double quotes and escapes the four
// sequences the gitconfig spec recognises inside double-quoted strings: \\ \" \n \t.
func GitQuote(v string) string {
	v = strings.ReplaceAll(v, `\`, `\\`)
	v = strings.ReplaceAll(v, `"`, `\"`)
	v = strings.ReplaceAll(v, "\n", `\n`)
	v = strings.ReplaceAll(v, "\t", `\t`)
	return `"` + v + `"`
}

// BuildGitConfigContent generates a minimal git [user] section from the host
// git configuration. Values are double-quoted per the gitconfig spec to handle
// backslashes, semicolons, and hash characters safely. Control characters are
// stripped before quoting. Returns an empty byte slice when neither name nor
// email is set.
func BuildGitConfigContent(lookup func(string) string) []byte {
	name := textutil.SanitiseForTerminal(lookup("user.name"))
	email := textutil.SanitiseForTerminal(lookup("user.email"))
	if name == "" && email == "" {
		return []byte{}
	}
	var b strings.Builder
	b.WriteString("[user]\n")
	if name != "" {
		fmt.Fprintf(&b, "\tname = %s\n", GitQuote(name))
	}
	if email != "" {
		fmt.Fprintf(&b, "\temail = %s\n", GitQuote(email))
	}
	return []byte(b.String())
}

// SeedGitConfigDefaults writes a git [user] config section into the defaults tree at
// defaultsDir/git/config/config when the file does not yet exist. Content is
// generated via BuildGitConfigContent. Returns nil when the file already
// exists, when a concurrent invocation creates the file between the existence
// check and the write (O_EXCL race), or when the lookup returns no values
// (nothing to seed).
func SeedGitConfigDefaults(defaultsDir string, lookupUserInfo func(string) string) error {
	content := BuildGitConfigContent(lookupUserInfo)
	if len(content) == 0 {
		return nil
	}

	targetPath := filepath.Join(defaultsDir, "git", "config", "config")

	if _, err := os.Lstat(targetPath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(targetPath), 0o700); err != nil {
		return err
	}

	f, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return nil // concurrent create: another process won the O_EXCL race; file now exists
		}
		return err
	}
	if _, err = f.Write(content); err != nil {
		_ = f.Close()
		_ = os.Remove(targetPath)
		return err
	}
	return f.Close()
}
