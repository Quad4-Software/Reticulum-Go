// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/Reticulum-Go/pkg/rnsutil"
)

// serveCommitsPage renders the commit log for a ref with pagination.
func (n *Node) serveCommitsPage(_ string, data []byte, _ []byte, _ []byte, remote *identity.Identity, _ int64) any {
	st := time.Now()
	vars := pageVars(data)
	group, repo, repoPath, errBody := n.accessibleRepo(vars, remote)
	if errBody != nil {
		return n.renderTemplate(string(errBody), "", "commits", st)
	}
	resolved, displayRef, ok := n.pageRef(repoPath, vars["ref"])
	if !ok {
		return n.renderTemplate(pageError("Unknown ref"), "", "commits", st)
	}
	filterPath := vars["path"]
	if decoded, err := url.QueryUnescape(filterPath); err == nil {
		filterPath = decoded
	}
	page := varInt(vars, "page", 0)
	if page < 0 {
		page = 0
	}
	icons := n.icons()

	nav := n.repoNav(group, repo) + " / " + mLink("Commits", pagePathCommits,
		map[string]string{"g": group, "r": repo})

	body := func(b *strings.Builder) {
		b.WriteString(mHeading(3) + " Commits " + mDim(displayRef+" ("+resolved[:8]+")") + "\n\n")
		entries, err := n.git.Log(repoPath, resolved, filterPath, page*commitsPerPage, commitsPerPage)
		if err != nil {
			b.WriteString("Could not list commits\n")
			return
		}
		if len(entries) == 0 {
			b.WriteString("No commits\n")
			return
		}
		for _, e := range entries {
			b.WriteString("`a  ")
			b.WriteString(mLink(e.SHA[:7], pagePathCommit, map[string]string{
				"g": group, "r": repo, "ref": displayRef, "h": e.SHA,
			}))
			b.WriteString("  " + mEscape(e.Subject) + "\n")
			b.WriteString("`a      " + mDim(mEscape(e.Author)+" - "+formatRelativeTime(e.UnixTime)+" ("+prettyDate(e.UnixTime)+")") + "\n")
		}
		pf := map[string]string{"g": group, "r": repo, "ref": displayRef}
		if filterPath != "" {
			pf["path"] = filterPath
		}
		var pageLinks []string
		if page > 0 {
			pf["page"] = fmtInt(page - 1)
			pageLinks = append(pageLinks, mLinkR("« Newer", pagePathCommits, pf))
		}
		if len(entries) == commitsPerPage {
			pf["page"] = fmtInt(page + 1)
			pageLinks = append(pageLinks, mLinkR("Older »", pagePathCommits, pf))
		}
		if len(pageLinks) > 0 {
			b.WriteString("\n" + strings.Join(pageLinks, "   "+icons.Sep+"   ") + "\n")
		}
	}
	return n.renderPage("commits", nav, st, body)
}

// serveCommitPage renders a single commit: metadata, signature status,
// changed files and the formatted diff.
func (n *Node) serveCommitPage(_ string, data []byte, _ []byte, _ []byte, remote *identity.Identity, _ int64) any {
	st := time.Now()
	vars := pageVars(data)
	group, repo, repoPath, errBody := n.accessibleRepo(vars, remote)
	if errBody != nil {
		return n.renderTemplate(string(errBody), "", "commit", st)
	}
	displayRef := vars["ref"]
	if displayRef == "" {
		displayRef = "HEAD"
	}
	commitish := vars["h"]
	if len(commitish) < 7 {
		return n.renderTemplate(pageError("No commit specified"), "", "commit", st)
	}
	out, err := n.git.RevParseVerify(repoPath, commitish)
	if err != nil || SanSHA(strings.TrimSpace(out)) == "" {
		return n.renderTemplate(pageError("Commit not found"), "", "commit", st)
	}
	sha := SanSHA(strings.TrimSpace(out))
	if n.git.CatFileType(repoPath, sha) != "commit" {
		return n.renderTemplate(pageError("Object is not a commit"), "", "commit", st)
	}
	detail, err := n.git.CommitDetail(repoPath, sha)
	if err != nil {
		return n.renderTemplate(pageError("Could not read commit"), "", "commit", st)
	}
	icons := n.icons()
	nav := n.repoNav(group, repo) + " / " + mLink("Commits", pagePathCommits,
		map[string]string{"g": group, "r": repo}) + " / " + mEscape(sha[:7])

	body := func(b *strings.Builder) {
		b.WriteString(mHeading(2) + " Commit " + mEscape(sha[:8]) + "\n\n")
		b.WriteString(mLinkR(icons.Folder+" Browse tree at this commit", pagePathTree,
			map[string]string{"g": group, "r": repo, "ref": sha}) + "\n\n")

		b.WriteString("Author:    " + mEscape(detail.Author) + " " + mDim("<"+mEscape(detail.Email)+">") + "\n")
		b.WriteString("           " + mDim(prettyDate(detail.AuthorTS)+" ("+formatRelativeTime(detail.AuthorTS)+")") + "\n")
		if detail.Committer != "" && detail.Committer != detail.Author {
			b.WriteString("Committer: " + mEscape(detail.Committer) + " " +
				mDim(prettyDate(detail.CommitTS)) + "\n")
		}
		for _, p := range detail.Parents {
			b.WriteString("Parent:    " + mLink(p[:8], pagePathCommit, map[string]string{
				"g": group, "r": repo, "ref": displayRef, "h": p,
			}) + "\n")
		}

		if sig := n.commitSignature(repoPath, sha); sig.signed {
			b.WriteString("\nSignature: " + sig.render() + "\n")
		}

		if detail.Body != "" {
			b.WriteString("\n" + mEscape(detail.Subject) + "\n")
			b.WriteString(mEscape(firstLines(detail.Body, 40)) + "\n")
		} else if detail.Subject != "" {
			b.WriteString("\n" + mEscape(detail.Subject) + "\n")
		}

		stats, derr := n.git.DiffNumstat(repoPath, sha)
		if derr == nil && len(stats) > 0 {
			b.WriteString("\n" + mHeading(4) + " Changed files\n\n")
			for _, fs := range stats {
				status := "M"
				if len(detail.Parents) > 0 {
					existsBefore := n.git.CatFileExists(repoPath, detail.Parents[0]+":"+fs.Path)
					existsNow := n.git.CatFileExists(repoPath, sha+":"+fs.Path)
					switch {
					case !existsBefore && existsNow:
						status = "A"
					case existsBefore && !existsNow:
						status = "D"
					case fs.OldPath != "":
						status = "R"
					}
				}
				delta := ""
				if fs.Binary {
					delta = "binary"
				} else {
					delta = fmt.Sprintf("+%d -%d", fs.Adds, fs.Dels)
				}
				nameField := fs.Path
				if fs.OldPath != "" {
					nameField = fs.OldPath + " -> " + fs.Path
				}
				b.WriteString("`a  " + statusColor(status) + " " +
					mLink(mEscape(nameField), pagePathBlob, map[string]string{
						"g": group, "r": repo, "ref": sha, "path": fs.Path,
					}) + "  " + mDim(delta) + "`a\n")
			}
		}

		if diff, derr := n.git.ShowDiff(repoPath, sha); derr == nil && strings.TrimSpace(diff) != "" {
			b.WriteString("\n" + mDivider() + "\n\n")
			b.WriteString(formatDiff(diff))
		}
	}
	return n.renderPage("commit", nav, st, body)
}

// statusColor renders the change-status letter colored.
func statusColor(status string) string {
	switch status {
	case "A":
		return mFg("A", "0a0")
	case "D":
		return mFg("D", "f00")
	case "R":
		return mFg("R", "0aa")
	default:
		return mFg("M", "999")
	}
}

// formatDiff renders a unified diff with micron colors, matching the
// reference format_diff semantics: additions green, deletions red, hunks
// purple, diff metadata dimmed, all content escaped.
func formatDiff(diff string) string {
	var b strings.Builder
	for line := range strings.SplitSeq(strings.TrimRight(diff, "\n"), "\n") {
		esc := mEscape(line)
		switch {
		case strings.HasPrefix(line, "diff --git"), strings.HasPrefix(line, "index "),
			strings.HasPrefix(line, "new file"), strings.HasPrefix(line, "deleted file"),
			strings.HasPrefix(line, "similarity"), strings.HasPrefix(line, "rename"):
			b.WriteString("`F666" + esc + "`f\n")
		case strings.HasPrefix(line, "@@"):
			b.WriteString("`F0aa" + esc + "`f\n")
		case strings.HasPrefix(line, "+"):
			b.WriteString("`F0a0" + esc + "`f\n")
		case strings.HasPrefix(line, "-"):
			b.WriteString("`F900" + esc + "`f\n")
		default:
			b.WriteString(esc + "\n")
		}
	}
	return b.String()
}

// commitSigResult is the parsed signature state for display.
type commitSigResult struct {
	signed      bool
	valid       bool
	authorMatch bool
	signerHash  string
	message     string
}

func (s commitSigResult) render() string {
	if !s.signed {
		return mDim("Not signed")
	}
	if s.valid && s.authorMatch {
		return mFg("Valid, signed by <"+s.signerHash+">", "0a0")
	}
	if s.valid {
		return mFg("Invalid signer <"+s.signerHash+">", "fa0")
	}
	return mFg("Invalid signature", "f00")
}

// sshSigMagic is the SSHSIG blob magic prefix.
var sshSigMagic = []byte("SSHSIG")

// unarmorSSHSig decodes a PEM-style SSH SIGNATURE block to raw bytes.
func unarmorSSHSig(armored string) ([]byte, error) {
	var b64 strings.Builder
	inSig := false
	for line := range strings.SplitSeq(strings.TrimSpace(armored), "\n") {
		if strings.Contains(line, "BEGIN SSH SIGNATURE") {
			inSig = true
			continue
		}
		if strings.Contains(line, "END SSH SIGNATURE") {
			break
		}
		if inSig {
			b64.WriteString(strings.TrimSpace(line))
		}
	}
	if b64.Len() == 0 {
		return nil, fmt.Errorf("no signature data in armored input")
	}
	return base64.StdEncoding.DecodeString(b64.String())
}

// readSSHString reads a uint32-length-prefixed string from data at offset.
func readSSHString(data []byte, offset int) ([]byte, int, error) {
	if offset+4 > len(data) {
		return nil, 0, fmt.Errorf("truncated ssh string length")
	}
	l := int(binary.BigEndian.Uint32(data[offset : offset+4]))
	offset += 4
	if offset+l > len(data) {
		return nil, 0, fmt.Errorf("truncated ssh string")
	}
	return data[offset : offset+l], offset + l, nil
}

// sshSigRSGData extracts the signature_data field from an SSHSIG blob, which
// for rngit commits carries the RSG structure.
func sshSigRSGData(sigData []byte) ([]byte, error) {
	if len(sigData) < len(sshSigMagic)+4 || !strings.HasPrefix(string(sigData[:len(sshSigMagic)]), string(sshSigMagic)) {
		return nil, fmt.Errorf("invalid SSH signature magic")
	}
	offset := len(sshSigMagic)
	version := binary.BigEndian.Uint32(sigData[offset : offset+4])
	if version != 1 {
		return nil, fmt.Errorf("unsupported SSH signature version %d", version)
	}
	offset += 4
	var err error
	for i := 0; i < 4; i++ { // public_key, namespace, reserved, hash_algorithm
		_, offset, err = readSSHString(sigData, offset)
		if err != nil {
			return nil, err
		}
	}
	sig, _, err := readSSHString(sigData, offset)
	if err != nil {
		return nil, err
	}
	return sig, nil
}

// extractCommitAuthor returns the address field of the commit author line,
// which for rngit-signed commits is the identity hash.
func extractCommitAuthor(content []byte) string {
	for line := range strings.Lines(string(content)) {
		if strings.TrimSpace(line) == "" {
			break
		}
		if !strings.HasPrefix(line, "author ") {
			continue
		}
		lt := strings.IndexByte(line, '<')
		gt := strings.IndexByte(line, '>')
		if lt > len("author ") && gt > lt && gt < len(line)-1 {
			return line[lt+1 : gt]
		}
	}
	return ""
}

// commitSignature verifies an RSG commit signature, matching the reference
// get_commit_signature flow: extract gpgsig header, unarmor the SSH
// signature, validate the RSG over the commit content minus signature lines,
// and compare the signer to the commit author.
func (n *Node) commitSignature(repoPath, sha string) commitSigResult {
	res := commitSigResult{message: "Not signed"}
	out, _, err := n.git.runPage(repoPath, "cat-file", "-p", sha)
	if err != nil {
		res.message = "Could not read commit object"
		return res
	}
	var sigLines, signedLines []string
	inSig := false
	for line := range strings.SplitSeq(string(out), "\n") {
		if strings.HasPrefix(line, "gpgsig ") || strings.HasPrefix(line, "gpgsig-sha256 ") {
			inSig = true
			sigLines = append(sigLines, line[strings.IndexByte(line, ' ')+1:])
			continue
		}
		if inSig && strings.HasPrefix(line, " ") {
			sigLines = append(sigLines, line[1:])
			continue
		}
		inSig = false
		signedLines = append(signedLines, line)
	}
	if len(sigLines) == 0 {
		return res
	}
	res.signed = true
	sigData, err := unarmorSSHSig(strings.Join(sigLines, "\n"))
	if err != nil {
		res.message = "Malformed SSH signature armor"
		return res
	}
	rsg, err := sshSigRSGData(sigData)
	if err != nil {
		res.message = "Malformed SSH wrapping for RSG data"
		return res
	}
	signedContent := []byte(strings.Join(signedLines, "\n"))
	result, err := rnsutil.ValidateRSG(rsg, signedContent, nil)
	if err != nil || !result.Valid {
		res.message = "Invalid signature"
		return res
	}
	res.valid = true
	if result.Signer != nil {
		res.signerHash = hex.EncodeToString(result.Signer.Hash())
	}
	author := extractCommitAuthor(signedContent)
	if author == "" {
		res.message = "Could not verify author"
		return res
	}
	res.authorMatch = author == res.signerHash
	if res.authorMatch {
		res.message = "Valid, signed by <" + res.signerHash + ">"
	} else {
		res.message = "Invalid signer <" + res.signerHash + ">, author is <" + author + ">"
	}
	return res
}

// formatRelativeTime renders a timestamp as a relative phrase, matching the
// reference format_relative_time output.
func formatRelativeTime(ts int64) string {
	diff := time.Now().Unix() - ts
	if diff < 0 {
		diff = 0
	}
	plural := func(n int64, unit string) string {
		if n == 1 {
			return fmt.Sprintf("%d %s ago", n, unit)
		}
		return fmt.Sprintf("%d %ss ago", n, unit)
	}
	switch {
	case diff < 60:
		return "just now"
	case diff < 3600:
		return plural(diff/60, "minute")
	case diff < 86400:
		return plural(diff/3600, "hour")
	case diff < 604800:
		return plural(diff/86400, "day")
	case diff < 2592000:
		return plural(diff/604800, "week")
	case diff < 31536000:
		return plural(diff/2592000, "month")
	default:
		return plural(diff/31536000, "year")
	}
}

// firstLines returns at most n lines of text.
func firstLines(s string, n int) string {
	var b strings.Builder
	i := 0
	for line := range strings.SplitSeq(s, "\n") {
		if i >= n {
			b.WriteString("...")
			break
		}
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(line)
		i++
	}
	return b.String()
}
