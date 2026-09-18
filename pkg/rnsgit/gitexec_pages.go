// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// gitPageTimeout bounds git subprocesses spawned by page handlers, matching
// the reference GIT_COMMAND_TIMEOUT of 8 seconds.
const gitPageTimeout = 8 * time.Second

func (g *GitRunner) runPage(dir string, args ...string) ([]byte, []byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitPageTimeout)
	defer cancel()
	return g.runCtx(ctx, dir, args...)
}

// TreeEntry is one ls-tree row.
type TreeEntry struct {
	Mode string
	Type string // blob, tree, commit, link
	SHA  string
	Size int64
	Name string
	Link string // symlink target when mode 120000
}

// LsTree lists the entries of ref:path sorted directories-first then by
// case-insensitive name, matching the Python tree page ordering.
func (g *GitRunner) LsTree(repoPath, ref, dirPath string) ([]TreeEntry, error) {
	spec := ref
	if dirPath != "" {
		spec = ref + ":" + dirPath
	}
	out, _, err := g.runPage(repoPath, "ls-tree", "-l", spec)
	if err != nil {
		return nil, err
	}
	var entries []TreeEntry
	for line := range strings.SplitSeq(strings.TrimRight(string(out), "\n"), "\n") {
		if line == "" {
			continue
		}
		// format: "<mode> <type> <sha> <size|- >\t<name>"
		tab := strings.IndexByte(line, '\t')
		if tab < 0 {
			continue
		}
		meta := strings.Fields(line[:tab])
		if len(meta) < 3 {
			continue
		}
		e := TreeEntry{
			Mode: meta[0],
			Type: meta[1],
			SHA:  meta[2],
			Name: line[tab+1:],
		}
		if len(meta) >= 4 && meta[3] != "-" {
			e.Size, _ = strconv.ParseInt(meta[3], 10, 64)
		}
		if e.Mode == "120000" {
			e.Type = "link"
			if target, _, serr := g.runPage(repoPath, "show", spec+"/"+e.Name); serr == nil {
				e.Link = strings.TrimSpace(string(target))
			}
		}
		entries = append(entries, e)
	}
	sort.SliceStable(entries, func(i, j int) bool {
		iDir := entries[i].Type == "tree" || entries[i].Type == "commit"
		jDir := entries[j].Type == "tree" || entries[j].Type == "commit"
		if iDir != jDir {
			return iDir
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
	return entries, nil
}

// LogEntry is one commit summary row for the commits page.
type LogEntry struct {
	SHA      string
	Subject  string
	Author   string
	Email    string
	UnixTime int64
}

// commitsPerPage matches the reference COMMITS_PER_PAGE window.
const commitsPerPage = 100

// Log returns commit rows for ref, optionally filtered by path.
func (g *GitRunner) Log(repoPath, ref, path string, skip, count int) ([]LogEntry, error) {
	args := []string{"log", "--format=%H|%s|%an|%ae|%at",
		"--skip", strconv.Itoa(skip), "-n", strconv.Itoa(count), ref}
	if path != "" {
		args = append(args, "--", path)
	}
	out, _, err := g.runPage(repoPath, args...)
	if err != nil {
		return nil, err
	}
	var entries []LogEntry
	for line := range strings.SplitSeq(strings.TrimRight(string(out), "\n"), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 5)
		if len(parts) < 5 {
			continue
		}
		ts, _ := strconv.ParseInt(strings.TrimSpace(parts[4]), 10, 64)
		entries = append(entries, LogEntry{
			SHA: parts[0], Subject: parts[1], Author: parts[2],
			Email: parts[3], UnixTime: ts,
		})
	}
	return entries, nil
}

// RefInfo is one refs page row.
type RefInfo struct {
	ObjectName string
	RefName    string
	Short      string
	Subject    string
	ObjectType string
	TagSubject string
}

// ForEachRef returns all refs with subjects; tags additionally carry the
// tag message subject used to detect annotated tags.
func (g *GitRunner) ForEachRef(repoPath string) ([]RefInfo, error) {
	out, _, err := g.runPage(repoPath, "for-each-ref",
		"--format=%(objectname)|%(refname)|%(refname:short)|%(subject)|%(objecttype)|%(contents:subject)")
	if err != nil {
		return nil, err
	}
	var refs []RefInfo
	for line := range strings.SplitSeq(strings.TrimRight(string(out), "\n"), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 6)
		for len(parts) < 6 {
			parts = append(parts, "")
		}
		refs = append(refs, RefInfo{
			ObjectName: parts[0], RefName: parts[1], Short: parts[2],
			Subject: parts[3], ObjectType: parts[4], TagSubject: parts[5],
		})
	}
	return refs, nil
}

// SymbolicRef resolves a symbolic ref to its target name.
func (g *GitRunner) SymbolicRef(repoPath, ref string) string {
	out, _, err := g.runPage(repoPath, "symbolic-ref", ref)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// CatFileType returns the object type of a revision, or "".
func (g *GitRunner) CatFileType(repoPath, obj string) string {
	out, _, err := g.runPage(repoPath, "cat-file", "-t", obj)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// CatFileExists reports whether an object spec resolves to an object.
func (g *GitRunner) CatFileExists(repoPath, spec string) bool {
	_, _, err := g.runPage(repoPath, "cat-file", "-e", spec)
	return err == nil
}

// CommitDetail carries the parsed header fields the commit page renders.
type CommitDetail struct {
	SHA       string
	Parents   []string
	Author    string
	Email     string
	AuthorTS  int64
	Committer string
	CommitTS  int64
	Subject   string
	Body      string
	Signature string // raw armored gpgsig/gpgsig-sha256 block, if present
	SigHeader string // which header carried it
}

// CommitDetail loads the commit object and parses its headers.
func (g *GitRunner) CommitDetail(repoPath, sha string) (*CommitDetail, error) {
	out, _, err := g.runPage(repoPath, "cat-file", "-p", sha)
	if err != nil {
		return nil, err
	}
	text := string(out)
	head, body, _ := strings.Cut(text, "\n\n")
	d := &CommitDetail{SHA: sha}
	var sig strings.Builder
	sigHeader := ""
	inSig := false
	for line := range strings.SplitSeq(head, "\n") {
		if inSig {
			if strings.HasPrefix(line, " ") {
				sig.WriteString(strings.TrimPrefix(line, " "))
				sig.WriteString("\n")
				continue
			}
			inSig = false
		}
		key, val, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		switch key {
		case "parent":
			d.Parents = append(d.Parents, val)
		case "author":
			d.Author, d.Email, d.AuthorTS = parseIdentLine(val)
		case "committer":
			d.Committer, _, d.CommitTS = parseIdentLine(val)
		case "gpgsig", "gpgsig-sha256":
			inSig = true
			sigHeader = key
			sig.WriteString(val)
			sig.WriteString("\n")
		}
	}
	d.Signature = strings.TrimRight(sig.String(), "\n")
	d.SigHeader = sigHeader
	if subj, rest, ok := strings.Cut(strings.TrimSpace(body), "\n\n"); ok {
		d.Subject = subj
		d.Body = rest
	} else {
		d.Subject = strings.TrimSpace(body)
	}
	return d, nil
}

// parseIdentLine splits "Name <email> ts tz" into parts.
func parseIdentLine(line string) (name, email string, ts int64) {
	if lt := strings.IndexByte(line, '<'); lt >= 0 {
		name = strings.TrimSpace(line[:lt])
		if gt := strings.IndexByte(line[lt:], '>'); gt >= 0 {
			email = line[lt+1 : lt+gt]
			rest := strings.Fields(line[lt+gt+1:])
			if len(rest) > 0 {
				ts, _ = strconv.ParseInt(rest[0], 10, 64)
			}
			return name, email, ts
		}
	}
	return strings.TrimSpace(line), "", 0
}

// DiffFileStat is one numstat row for the commit page.
type DiffFileStat struct {
	Path    string
	Adds    int64
	Dels    int64
	Binary  bool
	OldPath string // rename source when detected
}

// DiffNumstat runs git diff-tree --numstat -r for a commit against its first
// parent (or the empty tree for a root commit).
func (g *GitRunner) DiffNumstat(repoPath, sha string) ([]DiffFileStat, error) {
	out, _, err := g.runPage(repoPath, "diff-tree", "--numstat", "-r", "--no-commit-id", sha)
	if err != nil {
		return nil, err
	}
	var stats []DiffFileStat
	for line := range strings.SplitSeq(strings.TrimRight(string(out), "\n"), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 3 {
			continue
		}
		fs := DiffFileStat{}
		if parts[0] == "-" {
			fs.Binary = true
		} else {
			fs.Adds, _ = strconv.ParseInt(parts[0], 10, 64)
			fs.Dels, _ = strconv.ParseInt(parts[1], 10, 64)
		}
		// rename rows look like "old => new" or "dir/{a => b}/f"
		pathField := parts[2]
		if old, newp, ok := parseRename(pathField); ok {
			fs.OldPath = old
			fs.Path = newp
		} else {
			fs.Path = pathField
		}
		stats = append(stats, fs)
	}
	return stats, nil
}

// parseRename splits the numstat "old => new" rename notation, including the
// compacted "dir/{old => new}/rest" form git emits for shared prefixes.
func parseRename(field string) (old, newp string, ok bool) {
	i := strings.Index(field, " => ")
	if i < 0 {
		return "", "", false
	}
	old, newp = field[:i], field[i+4:]
	l := strings.Index(old, "{")
	r := strings.Index(newp, "}")
	if l >= 0 && r >= 0 {
		prefix := old[:l]
		suffix := newp[r+1:]
		old = prefix + old[l+1:]
		newp = prefix + newp[:r] + suffix
	}
	return old, newp, true
}

// ShowDiff returns the patch body of a commit: git show --format=.
func (g *GitRunner) ShowDiff(repoPath, sha string) (string, error) {
	out, _, err := g.runPage(repoPath, "show", "--format=", sha)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// BlobInfo describes a path at a ref for the blob page.
type BlobInfo struct {
	Exists  bool
	IsTree  bool
	Size    int64
	Mode    string
	Symlink string
	Binary  bool
}

// BlobInfo inspects ref:path without reading the blob body.
func (g *GitRunner) BlobInfo(repoPath, ref, filePath string) (*BlobInfo, error) {
	spec := ref + ":" + filePath
	typ := g.CatFileType(repoPath, spec)
	info := &BlobInfo{}
	switch typ {
	case "tree":
		info.Exists = true
		info.IsTree = true
		return info, nil
	case "blob":
		info.Exists = true
	default:
		return info, nil
	}
	out, _, err := g.runPage(repoPath, "cat-file", "-s", spec)
	if err == nil {
		info.Size, _ = strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	}
	// symlink detection via parent ls-tree mode 120000
	parent, base := filepath.Split(strings.TrimSuffix(filePath, "/"))
	parent = strings.TrimSuffix(parent, "/")
	lspec := ref + ":" + parent
	if ls, _, lerr := g.runPage(repoPath, "ls-tree", lspec, "--", base); lerr == nil {
		fields := strings.Fields(string(ls))
		if len(fields) >= 1 {
			info.Mode = fields[0]
			if info.Mode == "120000" {
				if target, _, terr := g.runPage(repoPath, "show", spec); terr == nil {
					info.Symlink = strings.TrimSpace(string(target))
				}
			}
		}
	}
	// binary detection: NUL in the first chunk of the blob
	if head, herr := g.showHead(repoPath, spec, 8192); herr == nil {
		info.Binary = bytes.IndexByte(head, 0) >= 0
	}
	return info, nil
}

// showHead reads at most limit bytes of a blob without materializing it all.
func (g *GitRunner) showHead(repoPath, spec string, limit int64) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitPageTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, g.Git, "show", spec) // #nosec G204 -- fixed git binary
	cmd.Dir = repoPath
	cmd.Env = envWithoutGitDir()
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	buf := make([]byte, limit)
	n, rerr := io.ReadAtLeast(pipe, buf, 1)
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	if rerr != nil && n == 0 {
		return nil, rerr
	}
	return buf[:n], nil
}

// ShowBlob returns blob bytes at ref:path.
func (g *GitRunner) ShowBlob(repoPath, ref, filePath string) ([]byte, error) {
	out, _, err := g.runPage(repoPath, "show", ref+":"+filePath)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Description returns the repository description from git config or the
// sibling .description file, matching get_repository_description.
func (g *GitRunner) Description(repoPath string) string {
	if v, err := g.ConfigValue(repoPath, "repository.description"); err == nil && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	if b, err := os.ReadFile(repoPath + ".description"); err == nil { // #nosec G304 -- repo description file
		return strings.TrimSpace(string(b))
	}
	return ""
}

// readmeCandidates lists the filenames probed for a rendered readme, matching
// the reference order.
var readmeCandidates = []string{
	"README.mu", "Readme.mu", "readme.mu",
	"README", "readme",
	"README.md", "readme.md",
	"README.rst", "README.txt", "readme.rst", "readme.txt",
}

// Readme returns the readme blob content and whether it is markdown.
func (g *GitRunner) Readme(repoPath, ref string) (content string, markdown bool, ok bool) {
	for _, name := range readmeCandidates {
		b, err := g.ShowBlob(repoPath, ref, name)
		if err != nil {
			continue
		}
		lower := strings.ToLower(name)
		return string(b), strings.HasSuffix(lower, ".md"), true
	}
	return "", false, false
}

// CommitCount returns the number of commits reachable from ref for the nav
// counter. Uses rev-list --count, matching commits page totals.
func (g *GitRunner) CommitCount(repoPath, ref string) int64 {
	out, _, err := g.runPage(repoPath, "rev-list", "--count", ref)
	if err != nil {
		return 0
	}
	n, _ := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	return n
}

// RefCount returns the number of refs under a prefix.
func (g *GitRunner) RefCount(repoPath, prefix string) int64 {
	out, _, err := g.runPage(repoPath, "for-each-ref", "--format=%(refname)", prefix)
	if err != nil {
		return 0
	}
	return int64(len(strings.Fields(strings.TrimSpace(string(out)))))
}
