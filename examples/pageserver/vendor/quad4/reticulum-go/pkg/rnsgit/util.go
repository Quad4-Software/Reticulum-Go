// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import (
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// ParseRNSURL parses rns://hash/group/repo or rns://hash/group.
func ParseRNSURL(raw string) (destHex, group, repo string, err error) {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(strings.ToLower(raw), ProtoRNS) {
		return "", "", "", fmt.Errorf("invalid protocol, want rns://")
	}
	rest := raw[len(ProtoRNS):]
	parts := strings.SplitN(rest, "/", 3)
	if len(parts) < 2 {
		return "", "", "", fmt.Errorf("invalid URL format, use rns://<hash>/<group>/<repo>")
	}
	destHex = parts[0]
	group = parts[1]
	if len(parts) == 3 {
		repo = parts[2]
	}
	if len(destHex) != 32 {
		return "", "", "", fmt.Errorf("invalid destination hash length")
	}
	if _, err := hex.DecodeString(destHex); err != nil {
		return "", "", "", fmt.Errorf("invalid destination hash: %w", err)
	}
	return destHex, group, repo, nil
}

// RepoPath returns group/repo.
func RepoPath(group, repo string) string {
	return group + "/" + repo
}

// SanRef validates a git ref name.
func SanRef(ref string) string {
	if ref == "" {
		return ""
	}
	if strings.HasPrefix(ref, "-") || strings.HasPrefix(ref, "/") {
		return ""
	}
	if strings.HasSuffix(ref, "/") || strings.HasSuffix(ref, ".") {
		return ""
	}
	if strings.Contains(ref, " ") || !strings.Contains(ref, "/") {
		return ""
	}
	if strings.Contains(ref, "..") || strings.Contains(ref, "/.") || strings.Contains(ref, "//") || strings.Contains(ref, `\`) {
		return ""
	}
	for comp := range strings.SplitSeq(ref, "/") {
		if strings.HasSuffix(comp, ".lock") {
			return ""
		}
	}
	for _, c := range ref {
		if c < 40 {
			return ""
		}
	}
	if strings.ContainsAny(ref, "~^:?*[@") || ref == "@" {
		return ""
	}
	if strings.Contains(ref, "@{") {
		return ""
	}
	return ref
}

// ExpandSymbolicRef resolves HEAD and other symbolic refs to a full ref name
// suitable for SanRef and git bundle create (e.g. HEAD -> refs/heads/dev).
func ExpandSymbolicRef(ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("empty ref")
	}
	out, err := localGitCmd("rev-parse", "--symbolic-full-name", ref).Output()
	if err != nil {
		return "", err
	}
	full := strings.TrimSpace(string(out))
	if full == "" {
		return "", fmt.Errorf("empty symbolic-full-name for %q", ref)
	}
	return full, nil
}

// SanRefs validates a list of refs.
func SanRefs(refs []string) []string {
	if len(refs) == 0 {
		return nil
	}
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		if SanRef(r) == "" {
			return nil
		}
		out = append(out, r)
	}
	return out
}

// SanSHA validates a 40-char git object id.
func SanSHA(sha string) string {
	if len(sha) < 40 {
		return ""
	}
	if _, err := hex.DecodeString(sha); err != nil {
		return ""
	}
	return sha
}

// ParseRepoPath splits group/repo request paths.
func ParseRepoPath(path string) (group, repo string, ok bool) {
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	if len(parts[0]) > 256 || len(parts[1]) > 256 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// EscapeGitStdout escapes a value for git remote-helper output.
func EscapeGitStdout(value string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, c := range value {
		switch c {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		case '\r':
			b.WriteString(`\r`)
		default:
			if c < 32 || c > 126 {
				b.WriteString(fmt.Sprintf(`\x%02x`, c))
			} else {
				b.WriteRune(c)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

func isGitBundle(data []byte) bool {
	return strings.HasPrefix(string(data), "# v2 git bundle")
}

func localGitCmd(args ...string) *exec.Cmd {
	cmd := exec.Command("git", args...) // #nosec G204 -- fixed git binary
	env := envWithoutGitDir()
	if dir := os.Getenv("GIT_DIR"); dir != "" {
		env = append(env, "GIT_DIR="+dir)
	}
	if wt := os.Getenv("GIT_WORK_TREE"); wt != "" {
		env = append(env, "GIT_WORK_TREE="+wt)
		cmd.Dir = wt
	}
	cmd.Env = env
	return cmd
}

func flushWriter(w io.Writer) {
	if f, ok := w.(interface{ Flush() error }); ok {
		_ = f.Flush()
	}
}

// WriteGitListPayload writes a remote-helper list response and the required
// terminating blank line (matches Python git-remote-rns).
func WriteGitListPayload(w io.Writer, text string) {
	fmt.Fprint(w, text)
	if !strings.HasSuffix(text, "\n") {
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w)
	flushWriter(w)
}

// GitCleanEnv returns a copy of the environment without GIT_DIR or GIT_WORK_TREE.
func GitCleanEnv() []string {
	return envWithoutGitDir()
}

func envWithoutGitDir() []string {
	env := os.Environ()
	out := make([]string, 0, len(env))
	for _, e := range env {
		if strings.HasPrefix(e, "GIT_DIR=") || strings.HasPrefix(e, "GIT_WORK_TREE=") {
			continue
		}
		out = append(out, e)
	}
	return out
}
