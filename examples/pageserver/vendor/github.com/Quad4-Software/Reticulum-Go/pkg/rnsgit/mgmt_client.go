// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Quad4-Software/msgpack/v5/pkg/msgpack"
)

// MgmtClient performs remote rngit management commands.
type MgmtClient struct {
	*Client
	status io.Writer
}

// NewMgmtClient creates a management client.
func NewMgmtClient(configDir, rnsConfig string) (*MgmtClient, error) {
	if err := EnsureClientConfig(configDir); err != nil {
		return nil, err
	}
	cfg, err := LoadClientConfig(configDir)
	if err != nil {
		return nil, err
	}
	if rnsConfig != "" {
		cfg.RNSConfigDir = rnsConfig
	}
	id, err := PrepareGitIdentity(cfg.IdentityPath)
	if err != nil {
		return nil, err
	}
	tmp, err := osMkdirTemp()
	if err != nil {
		return nil, err
	}
	batch := cfg.RefBatchSize
	if batch <= 0 {
		batch = DefaultRefBatchSize
	}
	return &MgmtClient{Client: &Client{
		cfg:          cfg,
		identity:     id,
		refBatchSize: batch,
		tmpDir:       tmp,
	}}, nil
}

func osMkdirTemp() (string, error) {
	return os.MkdirTemp("", "rnsgit-mgmt-")
}

// SetStatusWriter sets optional progress lines for connect and mgmt commands.
func (c *MgmtClient) SetStatusWriter(w io.Writer) {
	c.status = w
}

// Connect opens a link to the remote node given an rns:// URL.
func (c *MgmtClient) Connect(ctx context.Context, remote string) error {
	destHex, group, repo, err := ParseRNSURL(strings.TrimSuffix(remote, "/"))
	if err != nil {
		return err
	}
	c.destHex = destHex
	if repo != "" {
		c.repoPath = RepoPath(group, repo)
	} else {
		c.repoPath = group
	}
	c.progressWriter = c.status
	return c.Client.Connect(ctx)
}

// CreateRepo requests repository creation on the remote node.
func (c *MgmtClient) CreateRepo(ctx context.Context, remote string) error {
	_, group, repo, err := ParseRNSURL(remote)
	if err != nil {
		return err
	}
	req, err := EncodeRequest(map[int]any{IdxRepository: RepoPath(group, repo)})
	if err != nil {
		return err
	}
	body, err := c.sendRequest(ctx, PathCreate, req, 120*time.Second)
	if err != nil {
		return err
	}
	return checkStatus(body, "create")
}

// SyncRepo requests upstream sync for a fork or mirror.
func (c *MgmtClient) SyncRepo(ctx context.Context, remote string) error {
	_, group, repo, err := ParseRNSURL(remote)
	if err != nil {
		return err
	}
	req, err := EncodeRequest(map[int]any{IdxRepository: RepoPath(group, repo)})
	if err != nil {
		return err
	}
	body, err := c.sendRequest(ctx, PathSync, req, 2*time.Hour)
	if err != nil {
		return err
	}
	return checkStatus(body, "sync")
}

// CloneRemote forks or mirrors a repository on the remote node.
func (c *MgmtClient) CloneRemote(ctx context.Context, source, target, kind string) error {
	_, group, repo, err := ParseRNSURL(target)
	if err != nil {
		return err
	}
	path := PathFork
	if kind == "mirror" {
		path = PathMirror
	}
	req, err := EncodeMixedRequest(map[any]any{
		IdxRepository: RepoPath(group, repo),
		"source":      source,
	})
	if err != nil {
		return err
	}
	body, err := c.sendRequest(ctx, path, req, 2*time.Hour)
	if err != nil {
		return err
	}
	return checkStatus(body, kind)
}

// ManagePermissions reads and optionally writes group or repository permissions.
func (c *MgmtClient) ManagePermissions(ctx context.Context, remote, contentPath string, useEditor bool) error {
	remote = strings.TrimSuffix(remote, "/")
	_, group, repo, err := ParseRNSURL(remote)
	if err != nil {
		return err
	}
	op := "gperms"
	getReq := map[any]any{"operation": op, "step": "get", IdxGroup: group}
	setReqKey := any(IdxGroup)
	setReqVal := any(group)
	if repo != "" {
		op = "rperms"
		repoPath := RepoPath(group, repo)
		getReq = map[any]any{"operation": op, "step": "get", IdxRepository: repoPath}
		setReqKey = IdxRepository
		setReqVal = repoPath
	}
	body, err := c.sendRequest(ctx, PathPerms, getReq, 120*time.Second)
	if err != nil {
		return err
	}
	current, err := ParsePermsGetBody(body)
	if err != nil {
		return err
	}
	var content string
	switch {
	case contentPath != "":
		b, err := os.ReadFile(contentPath) // #nosec G304 -- operator-chosen permissions file
		if err != nil {
			return err
		}
		content = string(b)
	case useEditor:
		edited, ok, err := editWithEditor(current)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("edit cancelled")
		}
		content = edited
	default:
		return fmt.Errorf("provide -content or set EDITOR to modify permissions")
	}
	setReq := map[any]any{"operation": op, "step": "set", "content": content, setReqKey: setReqVal}
	body, err = c.sendRequest(ctx, PathPerms, setReq, 120*time.Second)
	if err != nil {
		return err
	}
	return checkStatus(body, "permissions")
}

// ReleaseList returns a formatted release table from the remote
// repository, decoding the Python msgpack release listing.
func (c *MgmtClient) ReleaseList(ctx context.Context, remote string) (string, error) {
	_, group, repo, err := ParseRNSURL(remote)
	if err != nil {
		return "", err
	}
	req, err := EncodeMixedRequest(map[any]any{
		IdxRepository: RepoPath(group, repo),
		"operation":   "list",
	})
	if err != nil {
		return "", err
	}
	body, err := c.sendRequest(ctx, PathRelease, req, 120*time.Second)
	if err != nil {
		return "", err
	}
	if len(body) == 0 || body[0] != ResOK {
		return "", fmt.Errorf("list releases failed: %s", string(body[1:]))
	}
	if len(body) == 1 {
		return "", nil
	}
	var decoded any
	if err := msgpack.Unmarshal(body[1:], &decoded); err != nil {
		return "", err
	}
	releases, latest := parseReleaseList(decoded)
	var b strings.Builder
	for _, r := range releases {
		tag := anyString(r["tag"])
		status := anyString(r["status"])
		marker := ""
		if latest == tag {
			marker = " (latest)"
		}
		created := anyInt(r["created"])
		when := ""
		if created > 0 {
			when = time.Unix(created, 0).Format("2006-01-02 15:04:05")
		}
		arts := anyInt(r["artifacts"])
		fmt.Fprintf(&b, "%s\t%s\t%s\t%d artifacts%s\n", tag, status, when, arts, marker)
	}
	return b.String(), nil
}

// parseReleaseList extracts the release dicts and latest tag from a
// decoded release list response.
func parseReleaseList(decoded any) ([]map[string]any, string) {
	m, ok := decoded.(map[string]any)
	if !ok {
		if am, ok := decoded.(map[any]any); ok {
			m = map[string]any{}
			for k, v := range am {
				m[fmt.Sprint(k)] = v
			}
		} else {
			return nil, ""
		}
	}
	latest := anyString(m["latest"])
	var releases []map[string]any
	switch l := m["releases"].(type) {
	case []any:
		for _, e := range l {
			switch rm := e.(type) {
			case map[string]any:
				releases = append(releases, rm)
			case map[any]any:
				nm := map[string]any{}
				for k, v := range rm {
					nm[fmt.Sprint(k)] = v
				}
				releases = append(releases, nm)
			}
		}
	}
	return releases, latest
}

func anyString(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case []byte:
		return string(s)
	default:
		return fmt.Sprint(v)
	}
}

func anyInt(v any) int64 {
	switch n := v.(type) {
	case int:
		return int64(n)
	case int64:
		return n
	case uint64:
		return int64(n) // #nosec G115 -- timestamps fit int64
	default:
		return 0
	}
}

// ReleaseView returns release details decoded from the remote msgpack
// release info response.
func (c *MgmtClient) ReleaseView(ctx context.Context, remote, tag string) (string, error) {
	_, group, repo, err := ParseRNSURL(remote)
	if err != nil {
		return "", err
	}
	req, err := EncodeMixedRequest(map[any]any{
		IdxRepository: RepoPath(group, repo),
		"operation":   "view",
		"tag":         tag,
	})
	if err != nil {
		return "", err
	}
	body, err := c.sendRequest(ctx, PathRelease, req, 120*time.Second)
	if err != nil {
		return "", err
	}
	if len(body) == 0 || body[0] != ResOK {
		return "", fmt.Errorf("view release failed: %s", string(body[1:]))
	}
	var decoded any
	if err := msgpack.Unmarshal(body[1:], &decoded); err != nil {
		return "", err
	}
	var b strings.Builder
	if m, ok := decoded.(map[string]any); ok {
		fmt.Fprintf(&b, "Tag: %s\nStatus: %s\n", anyString(m["tag"]), anyString(m["status"]))
		if h := anyString(m["hash"]); h != "" {
			fmt.Fprintf(&b, "Commit: %s\n", h)
		}
		if notes := anyString(m["notes"]); notes != "" {
			fmt.Fprintf(&b, "Notes (%s):\n%s\n", anyString(m["notes_format"]), notes)
		}
		if arts, ok := m["artifacts"].([]any); ok && len(arts) > 0 {
			b.WriteString("Artifacts:\n")
			for _, a := range arts {
				if am, ok := a.(map[string]any); ok {
					fmt.Fprintf(&b, "  %s (%d bytes)\n", anyString(am["name"]), anyInt(am["size"]))
				}
			}
		}
	}
	return b.String(), nil
}

// ReleaseFetch downloads a release artifact to outPath.
func (c *MgmtClient) ReleaseFetch(ctx context.Context, remote, tag, artifact, outPath string) error {
	_, group, repo, err := ParseRNSURL(remote)
	if err != nil {
		return err
	}
	req, err := EncodeMixedRequest(map[any]any{
		IdxRepository: RepoPath(group, repo),
		"operation":   "fetch",
		"tag":         tag,
		"artifact":    artifact,
	})
	if err != nil {
		return err
	}
	body, meta, err := c.sendRequestWithMeta(ctx, PathRelease, req, 2*time.Hour)
	if err != nil {
		return err
	}
	if meta != nil {
		if code, ok := MetadataResultCode(meta); ok && code != ResOK {
			return fmt.Errorf("fetch release failed")
		}
	} else if len(body) == 0 || body[0] != ResOK {
		return fmt.Errorf("fetch release failed: %s", string(body[1:]))
	}
	if len(body) == 0 {
		return fmt.Errorf("empty release artifact")
	}
	if outPath == "" {
		outPath = artifact
	}
	if err := os.WriteFile(outPath, body, 0o644); err != nil { // #nosec G306 -- downloaded artifact
		return err
	}
	return nil
}

// ReleaseArtifact pairs an artifact name with its file contents for
// ReleaseCreate.
type ReleaseArtifact struct {
	Name string
	Data []byte
}

// ReleaseCreate publishes a release tag on the remote repository using
// the Python init/artifact/finalize step protocol.
func (c *MgmtClient) ReleaseCreate(ctx context.Context, remote, tag, notes, notesFormat, commitHash string, artifacts []ReleaseArtifact) error {
	_, group, repo, err := ParseRNSURL(remote)
	if err != nil {
		return err
	}
	repoPath := RepoPath(group, repo)
	init := map[any]any{
		IdxRepository: repoPath,
		"operation":   "create",
		"step":        "init",
		"tag":         tag,
	}
	if notes != "" {
		init["notes"] = notes
		if notesFormat != "" {
			init["notes_format"] = notesFormat
		}
	}
	if commitHash != "" {
		init["hash"] = commitHash
	}
	if err := c.releaseStep(ctx, init, "init release"); err != nil {
		return err
	}
	for _, a := range artifacts {
		step := map[any]any{
			IdxRepository:   repoPath,
			"operation":     "create",
			"step":          "artifact",
			"tag":           tag,
			"artifact_name": a.Name,
			"artifact_data": a.Data,
		}
		if err := c.releaseStep(ctx, step, "upload artifact "+a.Name); err != nil {
			return err
		}
	}
	finalize := map[any]any{
		IdxRepository: repoPath,
		"operation":   "create",
		"step":        "finalize",
		"tag":         tag,
	}
	return c.releaseStep(ctx, finalize, "finalize release")
}

func (c *MgmtClient) releaseStep(ctx context.Context, fields map[any]any, desc string) error {
	req, err := EncodeMixedRequest(fields)
	if err != nil {
		return err
	}
	body, err := c.sendRequest(ctx, PathRelease, req, 120*time.Second)
	if err != nil {
		return err
	}
	return checkStatus(body, desc)
}

// ReleaseDelete removes a release from the remote repository.
func (c *MgmtClient) ReleaseDelete(ctx context.Context, remote, tag string) error {
	return c.releaseTagOp(ctx, remote, "delete", tag, "delete release")
}

// ReleaseLatest marks a release tag as the latest release.
func (c *MgmtClient) ReleaseLatest(ctx context.Context, remote, tag string) error {
	return c.releaseTagOp(ctx, remote, "latest", tag, "set latest release")
}

func (c *MgmtClient) releaseTagOp(ctx context.Context, remote, op, tag, desc string) error {
	_, group, repo, err := ParseRNSURL(remote)
	if err != nil {
		return err
	}
	req, err := EncodeMixedRequest(map[any]any{
		IdxRepository: RepoPath(group, repo),
		"operation":   op,
		"tag":         tag,
	})
	if err != nil {
		return err
	}
	body, err := c.sendRequest(ctx, PathRelease, req, 120*time.Second)
	if err != nil {
		return err
	}
	return checkStatus(body, desc)
}

// WorkList returns formatted work document listings from the remote
// repository, matching the Python rngit client work list output.
func (c *MgmtClient) WorkList(ctx context.Context, remote, scope string) (string, error) {
	_, group, repo, err := ParseRNSURL(remote)
	if err != nil {
		return "", err
	}
	if scope == "" {
		scope = "active"
	}
	req, err := EncodeMixedRequest(map[any]any{
		IdxRepository: RepoPath(group, repo),
		"operation":   "list",
		"scope":       scope,
	})
	if err != nil {
		return "", err
	}
	body, err := c.sendRequest(ctx, PathWork, req, 120*time.Second)
	if err != nil {
		return "", err
	}
	if len(body) == 0 || body[0] != ResOK {
		return "", fmt.Errorf("list work failed: %s", string(body[1:]))
	}
	if len(body) == 1 {
		return "", nil
	}
	var result map[string]any
	if err := msgpack.Unmarshal(body[1:], &result); err != nil {
		return "", err
	}
	var b strings.Builder
	for _, s := range []string{"active", "completed", "proposed"} {
		docs, _ := result[s].([]any)
		if len(docs) == 0 {
			continue
		}
		fmt.Fprintf(&b, "%s documents:\n", strings.ToUpper(s[:1])+s[1:])
		for _, item := range docs {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			fmt.Fprintf(&b, "  #%v %s (%s, %v comments)\n",
				m["id"], m["title"], m["author"], m["comments"])
		}
	}
	if b.Len() == 0 {
		return "No work documents\n", nil
	}
	return b.String(), nil
}

// WorkView returns a rendered work document with comments.
func (c *MgmtClient) WorkView(ctx context.Context, remote, docID, scope string) (string, error) {
	_, group, repo, err := ParseRNSURL(remote)
	if err != nil {
		return "", err
	}
	id, err := strconv.Atoi(docID)
	if err != nil {
		return "", fmt.Errorf("invalid document ID")
	}
	if scope == "" {
		scope = "all"
	}
	req, err := EncodeMixedRequest(map[any]any{
		IdxRepository: RepoPath(group, repo),
		"operation":   "view",
		"doc_id":      id,
		"scope":       scope,
	})
	if err != nil {
		return "", err
	}
	body, err := c.sendRequest(ctx, PathWork, req, 600*time.Second)
	if err != nil {
		return "", err
	}
	if len(body) == 0 || body[0] != ResOK {
		return "", fmt.Errorf("view work failed: %s", string(body[1:]))
	}
	var result map[string]any
	if err := msgpack.Unmarshal(body[1:], &result); err != nil {
		return "", err
	}
	meta, _ := result["meta"].(map[string]any)
	var b strings.Builder
	fmt.Fprintf(&b, "%s #%v: %s\n\n", reqStringVal(result["scope"]), result["id"], meta["title"])
	b.WriteString(fmt.Sprint(result["content"]))
	b.WriteString("\n")
	if comments, ok := result["comments"].([]any); ok && len(comments) > 0 {
		b.WriteString("\nComments:\n")
		for _, item := range comments {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			fmt.Fprintf(&b, "\n#%v by %s:\n%s\n", m["id"], m["author"], m["content"])
		}
	}
	return b.String(), nil
}

// WorkCreate opens an editor and creates a work document on the remote repository.
func (c *MgmtClient) WorkCreate(ctx context.Context, remote, title, contentPath string) (string, error) {
	content, err := c.workContent(title, contentPath)
	if err != nil {
		return "", err
	}
	return c.workSubmit(ctx, remote, "create", 0, "", title, content)
}

// WorkPropose opens an editor and proposes a work document on the remote repository.
func (c *MgmtClient) WorkPropose(ctx context.Context, remote, title, contentPath string) (string, error) {
	content, err := c.workContent(title, contentPath)
	if err != nil {
		return "", err
	}
	return c.workSubmit(ctx, remote, "propose", 0, "", title, content)
}

// WorkEdit submits signed edits to a work document.
func (c *MgmtClient) WorkEdit(ctx context.Context, remote, docID, scope, title, contentPath string) (string, error) {
	content, err := c.workContent(title, contentPath)
	if err != nil {
		return "", err
	}
	id, err := strconv.Atoi(docID)
	if err != nil {
		return "", fmt.Errorf("invalid document ID")
	}
	return c.workSubmit(ctx, remote, "edit", id, scope, title, content)
}

// WorkDelete deletes a work document on the remote repository.
func (c *MgmtClient) WorkDelete(ctx context.Context, remote, docID, scope string) error {
	return c.workSimpleOp(ctx, remote, "delete", docID, scope, 120*time.Second)
}

// WorkComplete moves an active document to completed.
func (c *MgmtClient) WorkComplete(ctx context.Context, remote, docID string) error {
	return c.workSimpleOp(ctx, remote, "complete", docID, "", 120*time.Second)
}

// WorkActivate moves a completed or proposed document to active.
func (c *MgmtClient) WorkActivate(ctx context.Context, remote, docID string) error {
	return c.workSimpleOp(ctx, remote, "activate", docID, "", 120*time.Second)
}

// WorkComment adds a comment to a work document.
func (c *MgmtClient) WorkComment(ctx context.Context, remote, docID, scope, contentPath string) error {
	content, err := c.workContent("Update on document #"+docID, contentPath)
	if err != nil {
		return err
	}
	id, err := strconv.Atoi(docID)
	if err != nil {
		return fmt.Errorf("invalid document ID")
	}
	req, err := EncodeMixedRequest(map[any]any{
		IdxRepository: c.repoPathFor(remote),
		"operation":   "comment",
		"doc_id":      id,
		"scope":       scopeOrDefault(scope),
		"content":     content,
		"format":      "markdown",
	})
	if err != nil {
		return err
	}
	body, err := c.sendRequest(ctx, PathWork, req, 600*time.Second)
	if err != nil {
		return err
	}
	return checkStatus(body, "comment")
}

// WorkPermissions edits a document's permission file remotely.
func (c *MgmtClient) WorkPermissions(ctx context.Context, remote, docID, contentPath string, useEditor bool) error {
	id, err := strconv.Atoi(docID)
	if err != nil {
		return fmt.Errorf("invalid document ID")
	}
	repoPath := c.repoPathFor(remote)
	getReq, err := EncodeMixedRequest(map[any]any{
		IdxRepository: repoPath,
		"operation":   "perms",
		"doc_id":      id,
		"step":        "get",
	})
	if err != nil {
		return err
	}
	body, err := c.sendRequest(ctx, PathWork, getReq, 120*time.Second)
	if err != nil {
		return err
	}
	current, err := ParsePermsGetBody(body)
	if err != nil {
		return err
	}
	var content string
	switch {
	case contentPath != "":
		b, err := os.ReadFile(contentPath) // #nosec G304 -- operator-chosen permissions file
		if err != nil {
			return err
		}
		content = string(b)
	case useEditor:
		edited, ok, err := editWithEditor(current)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("edit cancelled")
		}
		content = edited
	default:
		return fmt.Errorf("provide -content or set EDITOR to modify permissions")
	}
	setReq, err := EncodeMixedRequest(map[any]any{
		IdxRepository: repoPath,
		"operation":   "perms",
		"doc_id":      id,
		"step":        "set",
		"content":     content,
	})
	if err != nil {
		return err
	}
	body, err = c.sendRequest(ctx, PathWork, setReq, 120*time.Second)
	if err != nil {
		return err
	}
	return checkStatus(body, "permissions")
}

func (c *MgmtClient) repoPathFor(remote string) string {
	_, group, repo, err := ParseRNSURL(strings.TrimSuffix(remote, "/"))
	if err != nil {
		return c.repoPath
	}
	return RepoPath(group, repo)
}

// scopeOrDefault applies the Python client default scope for requests that
// always send a scope field.
func scopeOrDefault(scope string) string {
	if scope == "" {
		return "active"
	}
	return scope
}

func (c *MgmtClient) workSimpleOp(ctx context.Context, remote, operation, docID, scope string, timeout time.Duration) error {
	id, err := strconv.Atoi(docID)
	if err != nil {
		return fmt.Errorf("invalid document ID")
	}
	fields := map[any]any{
		IdxRepository: c.repoPathFor(remote),
		"operation":   operation,
		"doc_id":      id,
	}
	if scope != "" {
		fields["scope"] = scope
	}
	req, err := EncodeMixedRequest(fields)
	if err != nil {
		return err
	}
	body, err := c.sendRequest(ctx, PathWork, req, timeout)
	if err != nil {
		return err
	}
	return checkStatus(body, operation)
}

func (c *MgmtClient) workContent(title, contentPath string) (string, error) {
	if contentPath != "" {
		b, err := os.ReadFile(contentPath) // #nosec G304 -- operator-chosen work doc
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
	initial := title + "\n\n"
	edited, ok, err := editWithEditor(initial)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("edit cancelled")
	}
	return edited, nil
}

// workSubmit signs content and sends create, propose or edit operations,
// matching the Python rngit client request format.
func (c *MgmtClient) workSubmit(ctx context.Context, remote, operation string, docID int, scope, title, content string) (string, error) {
	_, group, repo, err := ParseRNSURL(remote)
	if err != nil {
		return "", err
	}
	signature, err := c.identity.Sign([]byte(content))
	if err != nil {
		return "", fmt.Errorf("could not sign work document: %w", err)
	}
	fields := map[any]any{
		IdxRepository: RepoPath(group, repo),
		"operation":   operation,
		"title":       title,
		"content":     content,
		"format":      "markdown",
		"signature":   signature,
	}
	if docID > 0 {
		fields["doc_id"] = docID
	}
	if scope != "" {
		fields["scope"] = scope
	}
	req, err := EncodeMixedRequest(fields)
	if err != nil {
		return "", err
	}
	body, err := c.sendRequest(ctx, PathWork, req, 600*time.Second)
	if err != nil {
		return "", err
	}
	if len(body) == 0 || body[0] != ResOK {
		return "", fmt.Errorf("%s failed: %s", operation, string(body[1:]))
	}
	if len(body) > 1 {
		var result map[string]any
		if err := msgpack.Unmarshal(body[1:], &result); err == nil {
			if id, ok := result["id"]; ok {
				return fmt.Sprintf("Work document %v %v", result["scope"], id), nil
			}
		}
	}
	return "", nil
}

func checkStatus(body []byte, op string) error {
	if len(body) == 0 || body[0] != ResOK {
		msg := string(body[1:])
		if msg == "" {
			msg = op + " failed"
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

func stringBody(body []byte, op string) (string, error) {
	if len(body) == 0 || body[0] != ResOK {
		msg := string(body[1:])
		if msg == "" {
			msg = op + " failed"
		}
		return "", fmt.Errorf("%s", msg)
	}
	return string(body[1:]), nil
}
