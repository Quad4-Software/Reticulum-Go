// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import (
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
)

// PermissionSet stores allowed identity hashes and special targets.
type PermissionSet struct {
	All  bool
	None bool
	IDs  map[string]struct{}
}

// PermSets bundles the eight rngit permission lists.
type PermSets struct {
	Read     PermissionSet
	Write    PermissionSet
	Create   PermissionSet
	Stats    PermissionSet
	Release  PermissionSet
	Interact PermissionSet
	Propose  PermissionSet
	Admin    PermissionSet
}

// AccessTable resolves repository and group permissions.
type AccessTable struct {
	Groups  map[string]*GroupAccess
	Blocked map[string]bool
	Aliases map[string]string
	Rules   map[string]string
}

// GroupAccess holds group-level and per-repo permissions.
type GroupAccess struct {
	Name         string
	Path         string
	DynamicPerms bool
	PermSets
	Repositories map[string]*RepoAccess
}

// RepoAccess holds repository-level permissions.
type RepoAccess struct {
	Name string
	Path string
	PermSets
}

// NewAccessTable builds permissions from config and on-disk .allowed files.
func NewAccessTable(cfg *ServerConfig) (*AccessTable, error) {
	t := &AccessTable{
		Groups:  map[string]*GroupAccess{},
		Blocked: cfg.BlockedIdentities,
		Aliases: cfg.IdentityAliases,
		Rules:   cfg.AccessRules,
	}
	if t.Blocked == nil {
		t.Blocked = map[string]bool{}
	}
	if t.Aliases == nil {
		t.Aliases = map[string]string{}
	}
	if t.Rules == nil {
		t.Rules = map[string]string{}
	}
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
		t.updateGroupPermissions(ga)
		if err := os.MkdirAll(abs, 0o755); err != nil { // #nosec G301 -- git group directory
			return nil, err
		}
		entries, _ := os.ReadDir(abs)
		for _, ent := range entries {
			if !ent.IsDir() || strings.HasPrefix(ent.Name(), ".") {
				continue
			}
			name := ent.Name()
			if strings.HasSuffix(name, ".work") || strings.HasSuffix(name, ".releases") {
				continue
			}
			repoPath := filepath.Join(abs, name)
			if !isBareRepoDir(repoPath) {
				continue
			}
			ra := &RepoAccess{Name: name, Path: repoPath}
			t.updateRepoPermissions(ga, ra)
			ga.Repositories[name] = ra
		}
		t.Groups[group] = ga
	}
	return t, nil
}

// isBareRepoDir reports whether path has the filesystem shape of a bare git
// repository, matching the Python is_git_repository/is_bare_repository check.
func isBareRepoDir(path string) bool {
	head, err := os.Stat(filepath.Join(path, "HEAD"))
	if err != nil || !head.Mode().IsRegular() {
		return false
	}
	objects, err := os.Stat(filepath.Join(path, "objects"))
	if err != nil || !objects.IsDir() {
		return false
	}
	refs, err := os.Stat(filepath.Join(path, "refs"))
	if err != nil || !refs.IsDir() {
		return false
	}
	return true
}

// updateGroupPermissions reloads a group's .allowed file and config rules.
// Equivalent to Python update_group_permissions.
func (t *AccessTable) updateGroupPermissions(ga *GroupAccess) {
	ga.PermSets = PermSets{}
	sets, dynamic := loadAllowedPermissions(ga.Path+".allowed", t.Aliases)
	ga.PermSets = sets
	ga.DynamicPerms = dynamic
	applyConfigRules(&ga.PermSets, t.configRules(ga.Name), t.Aliases)
}

func (t *AccessTable) configRules(group string) []string {
	if rule, ok := t.Rules[group]; ok && strings.TrimSpace(rule) != "" {
		return []string{rule}
	}
	return nil
}

// updateRepoPermissions reloads a repository's .allowed file only.
// Equivalent to Python update_repository_permissions.
func (t *AccessTable) updateRepoPermissions(ga *GroupAccess, ra *RepoAccess) {
	sets, _ := loadAllowedPermissions(ra.Path+".allowed", t.Aliases)
	ra.PermSets = sets
}

// loadAllowedPermissions reads a static or executable .allowed file.
// Executable resolvers are run and their stdout parsed, matching Python
// load_allowed_permissions.
func loadAllowedPermissions(path string, aliases map[string]string) (PermSets, bool) {
	var input string
	dynamic := false
	if st, err := os.Stat(path); err == nil && st.Mode().IsRegular() {
		if st.Mode()&0o111 != 0 {
			out, err := exec.Command(path).Output() // #nosec G204 -- executable permission resolver
			if err != nil {
				return PermSets{}, true
			}
			input = string(out)
			dynamic = true
		} else if b, err := os.ReadFile(path); err == nil { // #nosec G304 -- permission config
			input = string(b)
		}
	}
	return permissionsFromAllowedInput(input, aliases), dynamic
}

// permissionsFromAllowedInput parses .allowed file content into permission
// lists, matching Python permissions_from_allowed_input.
func permissionsFromAllowedInput(input string, aliases map[string]string) PermSets {
	var sets PermSets
	for line := range strings.SplitSeq(input, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		perms, target, ok := parsePermissionEntry(line, aliases)
		if !ok {
			continue
		}
		for _, perm := range perms {
			addTarget(sets.set(perm), target)
		}
	}
	return sets
}

func (b *PermSets) set(perm int) *PermissionSet {
	switch perm {
	case permRead:
		return &b.Read
	case permWrite:
		return &b.Write
	case permCreate:
		return &b.Create
	case permStats:
		return &b.Stats
	case permRelease:
		return &b.Release
	case permInteract:
		return &b.Interact
	case permPropose:
		return &b.Propose
	case permAdmin:
		return &b.Admin
	default:
		return nil
	}
}

func (b PermSets) permSet(perm int) PermissionSet {
	if ps := b.set(perm); ps != nil {
		return *ps
	}
	return PermissionSet{}
}

// applyConfigRules applies [access] section entries for a group. Each entry is
// a permission line, comma or newline separated values are also split for
// convenience.
func applyConfigRules(sets *PermSets, rules []string, aliases map[string]string) {
	for _, rule := range rules {
		for part := range strings.FieldsFuncSeq(rule, func(r rune) bool { return r == ',' || r == '\n' }) {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			perms, target, ok := parsePermissionEntry(part, aliases)
			if !ok {
				continue
			}
			for _, perm := range perms {
				addTarget(sets.set(perm), target)
			}
		}
	}
}

// permTarget is a parsed permission target: a marker or identity hash.
type permTarget struct {
	none  bool
	all   bool
	hexID string
}

// resolveAlias expands a configured identity alias, matching Python
// __resolve_identity_alias.
func resolveAlias(alias string, aliases map[string]string) string {
	switch strings.ToLower(alias) {
	case "n", "none", "nobody", "a", "all", "everyone":
		return alias
	}
	if len(alias) == 32 {
		if _, err := hex.DecodeString(alias); err == nil {
			return alias
		}
	}
	if resolved, ok := aliases[alias]; ok {
		return resolved
	}
	return alias
}

// parsePermissionEntry parses a "perm:target" line, matching Python
// parse_permission. The rw permission expands to read and write.
func parsePermissionEntry(line string, aliases map[string]string) ([]int, permTarget, bool) {
	var none []int
	permPart, targetPart, found := strings.Cut(line, ":")
	if !found || strings.Contains(targetPart, ":") {
		return none, permTarget{}, false
	}
	var perms []int
	switch strings.ToLower(permPart) {
	case "r", "read":
		perms = []int{permRead}
	case "w", "write":
		perms = []int{permWrite}
	case "rw", "readwrite":
		perms = []int{permRead, permWrite}
	case "c", "create":
		perms = []int{permCreate}
	case "s", "stats":
		perms = []int{permStats}
	case "rel", "release":
		perms = []int{permRelease}
	case "i", "interact":
		perms = []int{permInteract}
	case "p", "propose":
		perms = []int{permPropose}
	case "adm", "admin":
		perms = []int{permAdmin}
	default:
		return none, permTarget{}, false
	}
	target := resolveAlias(targetPart, aliases)
	switch target {
	case "n", "none", "nobody":
		return perms, permTarget{none: true}, true
	case "a", "all", "everyone":
		return perms, permTarget{all: true}, true
	}
	if len(target) == 32 {
		if decoded, err := hex.DecodeString(target); err == nil {
			return perms, permTarget{hexID: hex.EncodeToString(decoded)}, true
		}
	}
	return none, permTarget{}, false
}

func addTarget(ps *PermissionSet, target permTarget) {
	switch {
	case target.all:
		ps.All = true
	case target.none:
		ps.None = true
	case target.hexID != "":
		if ps.IDs == nil {
			ps.IDs = map[string]struct{}{}
		}
		ps.IDs[target.hexID] = struct{}{}
	}
}

func hasExplicit(ps PermissionSet) bool {
	return ps.All || ps.None || len(ps.IDs) > 0
}

// resolveLevel evaluates one permission level. Admin membership only counts
// explicit identity hashes, matching Python remote_hash in admins checks.
func resolveLevel(ps, admins PermissionSet, hexID string) int {
	if ps.None {
		return -1
	}
	if ps.All {
		return 1
	}
	if _, ok := ps.IDs[hexID]; ok {
		return 1
	}
	if _, ok := admins.IDs[hexID]; ok {
		return 1
	}
	return 0
}

// Resolve checks group or repository permission for a remote identity hash.
func (t *AccessTable) Resolve(group, repo string, remoteHash []byte, perm int) bool {
	if t == nil {
		return false
	}
	ga, ok := t.Groups[group]
	if !ok {
		return false
	}
	hexID := hex.EncodeToString(remoteHash)
	if t.Blocked[hexID] {
		return false
	}
	if repo == "" {
		return resolveLevel(ga.permSet(perm), ga.Admin, hexID) > 0
	}
	ra, ok := ga.Repositories[repo]
	if !ok {
		return false
	}
	return resolveRepo(ra, ga, hexID, perm)
}

func resolveRepo(ra *RepoAccess, ga *GroupAccess, hexID string, perm int) bool {
	ps := ra.permSet(perm)
	if r := resolveLevel(ps, ra.Admin, hexID); r != 0 {
		return r > 0
	}
	if hasExplicit(ps) {
		return false
	}
	return resolveLevel(ga.permSet(perm), ga.Admin, hexID) > 0
}

// ResolveDoc checks a document-scoped permission, matching Python
// resolve_doc_permission. Document .allowed files grant additional access but
// cannot revoke base access except through an explicit none target.
func (t *AccessTable) ResolveDoc(group, repo string, docID int, remoteHash []byte, perm int) bool {
	if t == nil {
		return false
	}
	ga, ok := t.Groups[group]
	if !ok {
		return false
	}
	ra, ok := ga.Repositories[repo]
	if !ok {
		return false
	}
	hexID := hex.EncodeToString(remoteHash)
	if t.Blocked[hexID] {
		return false
	}
	workPath := ra.Path + ".work"
	allowedPath := filepath.Join(workPath, strconv.Itoa(docID)+".allowed")
	if st, err := os.Stat(workPath); err == nil && st.IsDir() {
		if b, err := os.ReadFile(allowedPath); err == nil { // #nosec G304 -- document permission file
			doc := permissionsFromAllowedInput(string(b), t.Aliases)
			if r := resolveLevel(doc.permSet(perm), doc.Admin, hexID); r != 0 {
				return r > 0
			}
		}
	}
	return resolveRepo(ra, ga, hexID, perm)
}

// ValidateAllowedContent checks permission file lines, matching the remote
// permission validation in Python rngit.
func ValidateAllowedContent(content string, aliases map[string]string) (string, int, bool) {
	for i, line := range strings.Split(content, "\n") {
		stripped := strings.TrimSpace(line)
		if stripped == "" || strings.HasPrefix(stripped, "#") {
			continue
		}
		if _, _, ok := parsePermissionEntry(stripped, aliases); !ok {
			return stripped, i + 1, false
		}
	}
	return "", 0, true
}

// IsExecutableResolver reports whether an existing .allowed file is an
// executable permission resolver. Remote updates to resolvers are refused.
func IsExecutableResolver(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.Mode().IsRegular() && st.Mode()&0o111 != 0
}

// WriteAllowedFile saves permission lines to path verbatim via tmp+rename.
func WriteAllowedFile(path string, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { // #nosec G301 -- allowed file parent dir
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil { // #nosec G306 -- allowed file
		return err
	}
	return os.Rename(tmp, path)
}

// ReadAllowedFile returns the verbatim contents of an allowed file.
func ReadAllowedFile(path string) (string, error) {
	b, err := os.ReadFile(path) // #nosec G304 -- permission config
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// GrantCreatorAdmin writes the creator permission file for a new repository,
// matching the Python REPO_CREATE_PERMS_TEMPLATE of adm:{IDENTITY_HASH}.
// Admin membership implies all other permissions during resolution.
func GrantCreatorAdmin(repoAllowedPath, creatorHex string) error {
	return WriteAllowedFile(repoAllowedPath, "adm:"+creatorHex)
}
