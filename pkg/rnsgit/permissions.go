// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import (
	"bytes"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	permRead     = 0x01
	permWrite    = 0x02
	permCreate   = 0x04
	permStats    = 0x05
	permRelease  = 0x06
	permInteract = 0x07
	permPropose  = 0x08
	permAdmin    = 0xFE

	tgtAll  = "__all__"
	tgtNone = "__none__"
)

// PermissionSet stores allowed identity hashes and special targets.
type PermissionSet struct {
	All    bool
	None   bool
	IDs    map[string]struct{}
	Admins map[string]struct{}
}

// AccessTable resolves repository and group permissions.
type AccessTable struct {
	Groups map[string]*GroupAccess
}

// GroupAccess holds group-level and per-repo permissions.
type GroupAccess struct {
	Name         string
	Path         string
	Read         PermissionSet
	Write        PermissionSet
	Create       PermissionSet
	Stats        PermissionSet
	Release      PermissionSet
	Interact     PermissionSet
	Propose      PermissionSet
	Admin        PermissionSet
	Repositories map[string]*RepoAccess
}

// RepoAccess holds repository-level permissions.
type RepoAccess struct {
	Name     string
	Path     string
	Read     PermissionSet
	Write    PermissionSet
	Create   PermissionSet
	Stats    PermissionSet
	Release  PermissionSet
	Interact PermissionSet
	Propose  PermissionSet
	Admin    PermissionSet
}

// NewAccessTable builds permissions from config and on-disk .allowed files.
func NewAccessTable(cfg *ServerConfig) (*AccessTable, error) {
	t := &AccessTable{Groups: map[string]*GroupAccess{}}
	for group, root := range cfg.RepositoryGroups {
		abs, err := filepath.Abs(root)
		if err != nil {
			return nil, err
		}
		ga := &GroupAccess{
			Name:         group,
			Path:         abs,
			Repositories: map[string]*RepoAccess{},
		}
		if rule, ok := cfg.AccessRules[group]; ok {
			applyRules(&ga.Read, &ga.Write, &ga.Create, &ga.Stats, &ga.Release, &ga.Interact, &ga.Propose, &ga.Admin, rule)
		}
		groupAllowed := filepath.Join(filepath.Dir(abs), group+".allowed")
		applyAllowedFile(ga, groupAllowed)
		if err := os.MkdirAll(abs, 0o755); err != nil {
			return nil, err
		}
		entries, _ := os.ReadDir(abs)
		for _, ent := range entries {
			if !ent.IsDir() || strings.HasPrefix(ent.Name(), ".") {
				continue
			}
			repoPath := filepath.Join(abs, ent.Name())
			ra := &RepoAccess{Name: ent.Name(), Path: repoPath}
			inheritRepoFromGroup(ra, ga)
			repoAllowed := filepath.Join(abs, ent.Name()+".allowed")
			applyRepoAllowedFile(ra, repoAllowed)
			ga.Repositories[ent.Name()] = ra
		}
		t.Groups[group] = ga
	}
	return t, nil
}

func inheritRepoFromGroup(ra *RepoAccess, ga *GroupAccess) {
	ra.Read = ga.Read
	ra.Write = ga.Write
	ra.Create = ga.Create
	ra.Stats = ga.Stats
	ra.Release = ga.Release
	ra.Interact = ga.Interact
	ra.Propose = ga.Propose
	ra.Admin = ga.Admin
}

func applyRepoAllowedFile(ra *RepoAccess, path string) {
	lines := readAllowedLines(path)
	if len(lines) == 0 {
		return
	}
	ra.Read = emptyPerm()
	ra.Write = emptyPerm()
	ra.Create = emptyPerm()
	ra.Stats = emptyPerm()
	ra.Release = emptyPerm()
	ra.Interact = emptyPerm()
	ra.Propose = emptyPerm()
	ra.Admin = emptyPerm()
	applyRules(&ra.Read, &ra.Write, &ra.Create, &ra.Stats, &ra.Release, &ra.Interact, &ra.Propose, &ra.Admin, strings.Join(lines, "\n"))
}

func applyAllowedFile(ga *GroupAccess, path string) {
	lines := readAllowedLines(path)
	if len(lines) == 0 {
		return
	}
	applyRules(&ga.Read, &ga.Write, &ga.Create, &ga.Stats, &ga.Release, &ga.Interact, &ga.Propose, &ga.Admin, strings.Join(lines, "\n"))
}

func readAllowedLines(path string) []string {
	st, err := os.Stat(path)
	if err != nil {
		return nil
	}
	if st.Mode()&0o111 != 0 {
		out, err := exec.Command(path).Output() // #nosec G204 -- executable permission file
		if err != nil {
			return nil
		}
		return splitAllowed(string(out))
	}
	b, err := os.ReadFile(path) // #nosec G304 -- permission config
	if err != nil {
		return nil
	}
	return splitAllowed(string(b))
}

func splitAllowed(s string) []string {
	var lines []string
	for line := range strings.SplitSeq(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

func applyRules(read, write, create, stats, release, interact, propose, admin *PermissionSet, rule string) {
	for part := range strings.SplitSeq(rule, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		perm, target, ok := strings.Cut(part, ":")
		if !ok {
			continue
		}
		perm = strings.ToLower(strings.TrimSpace(perm))
		target = strings.TrimSpace(target)
		sets := permSets(perm, read, write, create, stats, release, interact, propose, admin)
		for _, ps := range sets {
			addTarget(ps, target)
		}
	}
}

func permSets(perm string, read, write, create, stats, release, interact, propose, admin *PermissionSet) []*PermissionSet {
	switch perm {
	case "r", "read":
		return []*PermissionSet{read}
	case "w", "write":
		return []*PermissionSet{write}
	case "rw", "readwrite":
		return []*PermissionSet{read, write}
	case "c", "create":
		return []*PermissionSet{create}
	case "s", "stats":
		return []*PermissionSet{stats}
	case "rel", "release":
		return []*PermissionSet{release}
	case "i", "interact":
		return []*PermissionSet{interact}
	case "p", "propose":
		return []*PermissionSet{propose}
	case "adm", "admin":
		return []*PermissionSet{admin}
	default:
		return nil
	}
}

func addTarget(ps *PermissionSet, target string) {
	target = strings.ToLower(target)
	switch target {
	case "all", "a", "everyone":
		ps.All = true
	case "none", "n", "nobody":
		ps.None = true
	default:
		if len(target) == 32 {
			if _, err := hex.DecodeString(target); err == nil {
				if ps.IDs == nil {
					ps.IDs = map[string]struct{}{}
				}
				ps.IDs[target] = struct{}{}
			}
		}
	}
}

func emptyPerm() PermissionSet {
	return PermissionSet{IDs: map[string]struct{}{}}
}

func (t *AccessTable) Resolve(group, repo string, remoteHash []byte, perm int) bool {
	if t == nil {
		return false
	}
	ga, ok := t.Groups[group]
	if !ok {
		return false
	}
	hexID := hex.EncodeToString(remoteHash)
	if repo != "" {
		ra, ok := ga.Repositories[repo]
		if !ok {
			return false
		}
		return resolveRepo(ra, ga, hexID, perm)
	}
	return resolveSet(ga.permSet(perm), ga.Admin, hexID)
}

func resolveRepo(ra *RepoAccess, ga *GroupAccess, hexID string, perm int) bool {
	ps := ra.permSet(perm)
	if hasExplicit(ps) {
		return resolveSet(ps, ra.Admin, hexID)
	}
	return resolveSet(ga.permSet(perm), ga.Admin, hexID) || inSet(ra.Admin, hexID)
}

func hasExplicit(ps PermissionSet) bool {
	return ps.All || ps.None || len(ps.IDs) > 0
}

func resolveSet(ps PermissionSet, admins PermissionSet, hexID string) bool {
	if ps.None {
		return false
	}
	if ps.All {
		return true
	}
	if inSet(ps, hexID) || inSet(admins, hexID) {
		return true
	}
	return false
}

func inSet(ps PermissionSet, hexID string) bool {
	if ps.All {
		return true
	}
	_, ok := ps.IDs[hexID]
	return ok
}

func (ga *GroupAccess) permSet(perm int) PermissionSet {
	switch perm {
	case permRead:
		return ga.Read
	case permWrite:
		return ga.Write
	case permCreate:
		return ga.Create
	case permStats:
		return ga.Stats
	case permRelease:
		return ga.Release
	case permInteract:
		return ga.Interact
	case permPropose:
		return ga.Propose
	case permAdmin:
		return ga.Admin
	default:
		return emptyPerm()
	}
}

func (ra *RepoAccess) permSet(perm int) PermissionSet {
	switch perm {
	case permRead:
		return ra.Read
	case permWrite:
		return ra.Write
	case permCreate:
		return ra.Create
	case permStats:
		return ra.Stats
	case permRelease:
		return ra.Release
	case permInteract:
		return ra.Interact
	case permPropose:
		return ra.Propose
	case permAdmin:
		return ra.Admin
	default:
		return emptyPerm()
	}
}

// WriteAllowedFile saves permission lines to path.
func WriteAllowedFile(path string, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var buf bytes.Buffer
	for line := range strings.SplitSeq(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if _, _, ok := strings.Cut(line, ":"); !ok {
			return os.ErrInvalid
		}
		buf.WriteString(line)
		buf.WriteByte('\n')
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// ReadAllowedFile returns the contents of an allowed file.
func ReadAllowedFile(path string) (string, error) {
	lines := readAllowedLines(path)
	return strings.Join(lines, "\n"), nil
}

// GrantCreatorAdmin writes admin permissions for a new repository creator.
func GrantCreatorAdmin(repoAllowedPath, creatorHex string) error {
	content := "adm:" + creatorHex + "\nrw:" + creatorHex
	return WriteAllowedFile(repoAllowedPath, content)
}
