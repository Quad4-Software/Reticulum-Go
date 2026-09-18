// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
)

// accessibleGroups returns the groups where remote can read at least one
// repository, matching get_accessible_groups.
func (n *Node) accessibleGroups(remote *identity.Identity) map[string][]string {
	hash := remotePageHash(remote)
	access := n.accessTable()
	out := map[string][]string{}
	for group, ga := range access.Groups {
		for name := range ga.Repositories {
			if access.Resolve(group, name, hash, permRead) {
				out[group] = append(out[group], name)
			}
		}
		if len(out[group]) == 0 {
			delete(out, group)
		}
	}
	return out
}

// serveFrontPage renders the index page listing accessible groups.
func (n *Node) serveFrontPage(_ string, data []byte, _ []byte, _ []byte, remote *identity.Identity, _ int64) any {
	st := time.Now()
	if remote == nil && n.accessTable().Blocked[nullIdentHash] {
		return n.pageNoIdent()
	}
	nav := mLinkStrong("Node", pagePathIndex, nil) + " /"
	body := func(b *strings.Builder) {
		groups := n.accessibleGroups(remote)
		names := make([]string, 0, len(groups))
		for g := range groups {
			names = append(names, g)
		}
		sort.Strings(names)
		if len(names) == 0 {
			b.WriteString("No accessible repositories\n")
		}
		for _, g := range names {
			b.WriteString("`a  ")
			b.WriteString(mLink(g, pagePathGroup, map[string]string{"g": g}))
			b.WriteString("`a\n")
		}
	}
	n.viewSucceeded("", "", remote)
	return n.renderPage("front", nav, st, body)
}

// serveGroupPage renders one group's repository list.
func (n *Node) serveGroupPage(_ string, data []byte, _ []byte, _ []byte, remote *identity.Identity, _ int64) any {
	st := time.Now()
	if remote == nil && n.accessTable().Blocked[nullIdentHash] {
		return n.pageNoIdent()
	}
	vars := pageVars(data)
	group := vars["g"]
	if group == "" {
		return n.renderTemplate(pageError("No group specified"), "", "group", st)
	}
	access := n.accessTable()
	if _, ok := access.Groups[group]; !ok {
		return n.renderTemplate(pageError("Group not found"), "", "group", st)
	}
	hash := remotePageHash(remote)
	nav := mLinkStrong("Node", pagePathIndex, nil) + " / " + mEscape(group)
	body := func(b *strings.Builder) {
		var repos []string
		for name := range access.Groups[group].Repositories {
			if access.Resolve(group, name, hash, permRead) {
				repos = append(repos, name)
			}
		}
		sort.Strings(repos)
		if len(repos) == 0 {
			b.WriteString("No accessible repositories\n")
		}
		for _, name := range repos {
			b.WriteString("`a  ")
			b.WriteString(mLink(name, pagePathRepo, map[string]string{"g": group, "r": name}))
			if desc := n.git.Description(n.repoPathMust(group, name)); desc != "" {
				b.WriteString(" - ")
				b.WriteString(mEscape(desc))
			}
			b.WriteString("`a\n")
		}
	}
	n.viewSucceeded(group, "", remote)
	return n.renderPage("group", nav, st, body)
}

func (n *Node) repoPathMust(group, repo string) string {
	p, _ := n.repoPath(group, repo)
	return p
}

// serveRepoPage renders the repository landing page: clone URL, fork/mirror
// provenance, nav bar, readme and thanks counter.
func (n *Node) serveRepoPage(_ string, data []byte, _ []byte, linkID []byte, remote *identity.Identity, _ int64) any {
	st := time.Now()
	vars := pageVars(data)
	group, repo, repoPath, errBody := n.accessibleRepo(vars, remote)
	if errBody != nil {
		return n.renderTemplate(string(errBody), "", "repo", st)
	}
	icons := n.icons()
	fields := map[string]string{"g": group, "r": repo}
	nav := mLinkStrong("Node", pagePathIndex, nil) + " / " +
		mLink(group, pagePathGroup, map[string]string{"g": group}) + " / " +
		mEscape(repo)

	body := func(b *strings.Builder) {
		b.WriteString(mHeading(2) + " " + mEscape(repo) + "\n\n")
		clone := fmt.Sprintf("rns://%s/%s/%s", n.ReposDestHash(), group, repo)
		b.WriteString("`=`=Clone: " + mEscape(clone) + "\n`=``\n\n")

		if typ, _ := n.git.ConfigValue(repoPath, "repository.rngit.type"); typ == "fork" || typ == "mirror" {
			source, _ := n.git.ConfigValue(repoPath, "repository.rngit.upstream.source")
			verb := "Forked"
			if typ == "mirror" {
				verb = "Mirrored"
			}
			if source != "" {
				if strings.HasPrefix(source, ProtoRNS) {
					if dest, sg, sr, perr := ParseRNSURL(source); perr == nil && sr != "" {
						b.WriteString(verb + " from " +
							mLinkE(mEscape(source), dest, pagePathRepo,
								map[string]string{"g": sg, "r": sr}) + "\n")
					} else {
						b.WriteString(verb + " from " + mEscape(source) + "\n")
					}
				} else {
					b.WriteString(verb + " from " + mEscape(source) + "\n")
				}
				if syncTs, _ := n.git.ConfigValue(repoPath, "repository.rngit.upstream.sync"); syncTs != "" {
					if ts, perr := parseUnix(syncTs); perr == nil {
						b.WriteString(mDim("Last synced "+prettyTimeAgo(ts)) + "\n")
					}
				}
				b.WriteString("\n")
			}
		}

		if desc := n.git.Description(repoPath); desc != "" {
			b.WriteString(mEscape(desc) + "\n\n")
		}

		// nav bar
		var links []string
		links = append(links, mLinkR(icons.Folder+" Files", pagePathTree, fields))
		if rels, _ := releasesListData(repoPath + ".releases"); len(rels) > 0 {
			links = append(links, mLinkR(icons.Package+" Releases("+fmtInt(len(rels))+")", pagePathReleases, fields))
		}
		if wc := n.workDocCount(repoPath, "active"); wc > 0 {
			links = append(links, mLinkR(icons.Work+" Work("+fmtInt(wc)+")", pagePathWork, fields))
		}
		links = append(links, mLinkR(icons.Commits+" Commits", pagePathCommits, fields))
		links = append(links, mLinkR(icons.Branch+" Branches", pagePathRefs,
			map[string]string{"g": group, "r": repo, "type": "heads"}))
		links = append(links, mLinkR(icons.Tag+" Tags", pagePathRefs,
			map[string]string{"g": group, "r": repo, "type": "tags"}))
		thanks := n.repositoryThanks(repoPath, vars["thanks"] == "y", linkID)
		links = append(links, mLinkR(icons.Heart+" Thanks("+fmtInt(int(thanks))+")", pagePathRepo,
			map[string]string{"g": group, "r": repo, "thanks": "y"}))
		if n.accessTable().Resolve(group, repo, remotePageHash(remote), permStats) {
			links = append(links, mLinkR(icons.Stats+" Stats", pagePathStats, fields))
		}
		b.WriteString(strings.Join(links, "  "+icons.Sep+"  ") + "\n\n")
		b.WriteString(mDivider() + "\n\n")

		if content, markdown, ok := n.git.Readme(repoPath, "HEAD"); ok {
			if markdown {
				scope := pagePathBlob + "`g=" + mFieldValue(group) +
					"|r=" + mFieldValue(repo) + "|ref=HEAD|path="
				b.WriteString(n.mdc.formatBlock(content, scope))
			} else {
				b.WriteString(content)
			}
		}
	}
	n.viewSucceeded(group, repo, remote)
	return n.renderPage("repo", nav, st, body)
}

// parseUnix parses a unix timestamp string.
func parseUnix(s string) (int64, error) {
	var ts int64
	_, err := fmt.Sscanf(strings.TrimSpace(s), "%d", &ts)
	return ts, err
}
