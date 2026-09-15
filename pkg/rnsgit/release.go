// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"quad4/msgpack/v5/pkg/msgpack"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/link"
)

// releaseMeta is a ConfigObj-style META file for a release directory.
type releaseMeta map[string]string

// releaseMetaOrder defines the write order for META keys.
var releaseMetaOrder = []string{"tag", "hash", "created", "status", "created_by", "published_at"}

func readReleaseMeta(path string) (releaseMeta, error) {
	b, err := os.ReadFile(path) // #nosec G304 -- release META
	if err != nil {
		return nil, err
	}
	m := releaseMeta{}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		m[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return m, nil
}

func writeReleaseMeta(path string, m releaseMeta) error {
	var b strings.Builder
	seen := map[string]bool{}
	for _, k := range releaseMetaOrder {
		if v, ok := m[k]; ok {
			b.WriteString(k + " = " + v + "\n")
			seen[k] = true
		}
	}
	var extra []string
	for k := range m {
		if !seen[k] {
			extra = append(extra, k)
		}
	}
	sort.Strings(extra)
	for _, k := range extra {
		b.WriteString(k + " = " + m[k] + "\n")
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0o644); err != nil { // #nosec G306 -- release META
		return err
	}
	return os.Rename(tmp, path)
}

func metaInt(m releaseMeta, key string) int64 {
	v, err := strconv.ParseInt(m[key], 10, 64)
	if err != nil {
		return 0
	}
	return v
}

// releasesListData mirrors Python releases_list_data. It returns release
// info dicts and the latest published tag, or an empty latest tag.
func releasesListData(releasesPath string) ([]map[string]any, string) {
	releases := []map[string]any{}
	latest := ""
	entries, err := os.ReadDir(releasesPath)
	if err != nil {
		return releases, ""
	}
	published := map[string]bool{}
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		releaseDir := filepath.Join(releasesPath, ent.Name())
		meta, err := readReleaseMeta(filepath.Join(releaseDir, "META"))
		if err != nil {
			continue
		}
		tag := meta["tag"]
		if tag == "" {
			tag = ent.Name()
		}
		status := meta["status"]
		if status == "" {
			status = "unknown"
		}
		info := map[string]any{
			"tag":        tag,
			"hash":       meta["hash"],
			"created":    metaInt(meta, "created"),
			"status":     status,
			"created_by": meta["created_by"],
		}
		preview := ""
		format := "markdown"
		for _, nf := range []struct{ name, format string }{
			{"RELEASE.md", "markdown"},
			{"RELEASE.mu", "micron"},
			{"RELEASE.txt", "text"},
		} {
			notesPath := filepath.Join(releaseDir, nf.name)
			b, err := os.ReadFile(notesPath) // #nosec G304 -- release notes
			if err != nil {
				continue
			}
			var lines []string
			for _, line := range strings.Split(string(b), "\n") {
				if strings.HasPrefix(line, "#") || strings.HasPrefix(line, ">") {
					continue
				}
				lines = append(lines, line)
			}
			preview = strings.TrimSpace(strings.Join(lines, "\n"))
			format = nf.format
			break
		}
		info["preview"] = preview
		info["format"] = format
		artifacts := 0
		if arts, err := os.ReadDir(filepath.Join(releaseDir, "artifacts")); err == nil {
			for _, a := range arts {
				if !a.IsDir() {
					artifacts++
				}
			}
		}
		info["artifacts"] = artifacts
		releases = append(releases, info)
		published[tag] = status == "published"
	}
	if b, err := os.ReadFile(filepath.Join(releasesPath, "latest")); err == nil { // #nosec G304 -- latest marker
		latestTag := strings.TrimSpace(string(b))
		if published[latestTag] {
			latest = latestTag
		}
	}
	sort.SliceStable(releases, func(i, j int) bool {
		return releaseCreated(releases[i]) > releaseCreated(releases[j])
	})
	return releases, latest
}

func releaseCreated(info map[string]any) int64 {
	switch v := info["created"].(type) {
	case int64:
		return v
	case int:
		return int64(v)
	default:
		return 0
	}
}

// releaseData mirrors Python release_data for a single release directory.
func releaseData(releaseDir, tag string) map[string]any {
	meta, err := readReleaseMeta(filepath.Join(releaseDir, "META"))
	if err != nil {
		return nil
	}
	status := meta["status"]
	if status == "" {
		status = "unknown"
	}
	info := map[string]any{
		"tag":        firstNonEmpty(meta["tag"], tag),
		"hash":       meta["hash"],
		"created":    metaInt(meta, "created"),
		"status":     status,
		"created_by": meta["created_by"],
	}
	notes := ""
	format := "text"
	for _, nf := range []struct{ name, format string }{
		{"RELEASE.md", "markdown"},
		{"RELEASE.mu", "micron"},
	} {
		notesPath := filepath.Join(releaseDir, nf.name)
		b, err := os.ReadFile(notesPath) // #nosec G304 -- release notes
		if err != nil {
			continue
		}
		notes = string(b)
		format = nf.format
		break
	}
	info["notes"] = notes
	info["notes_format"] = format
	artifacts := []map[string]any{}
	if arts, err := os.ReadDir(filepath.Join(releaseDir, "artifacts")); err == nil {
		for _, a := range arts {
			if a.IsDir() {
				continue
			}
			fi, err := a.Info()
			if err != nil {
				continue
			}
			artifacts = append(artifacts, map[string]any{"name": a.Name(), "size": fi.Size()})
		}
	}
	info["artifacts"] = artifacts
	thanks := int64(0)
	if b, err := os.ReadFile(filepath.Join(releaseDir, "THANKS")); err == nil { // #nosec G304 -- thanks count
		var m map[string]any
		if msgpack.Unmarshal(b, &m) == nil {
			switch v := m["count"].(type) {
			case int64:
				thanks = v
			case uint64:
				thanks = int64(v) // #nosec G115 -- count fits int64
			}
		}
	}
	info["thanks"] = thanks
	return info
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// sanReleaseTag validates a release tag and returns its basename.
func sanReleaseTag(tag string) (string, []byte) {
	if tag == "" || strings.Contains(tag, "/") {
		return "", StatusResponse(ResInvalidReq, "Invalid tag specified")
	}
	return path.Base(tag), nil
}

// resolveReleaseTag handles the special "latest" tag.
func resolveReleaseTag(releasesPath, tag string) (string, []byte) {
	if tag != "latest" {
		return tag, nil
	}
	b, err := os.ReadFile(filepath.Join(releasesPath, "latest")) // #nosec G304 -- latest marker
	if err != nil {
		return "", StatusResponse(ResNotFound, "No latest release found")
	}
	latest := strings.TrimSpace(string(b))
	if latest == "" {
		return "", StatusResponse(ResNotFound, "No latest release found")
	}
	return latest, nil
}

func packOK(v any) []byte {
	packed, err := msgpack.Marshal(v)
	if err != nil {
		return StatusResponse(ResRemoteFail, "Remote error")
	}
	return append([]byte{ResOK}, packed...)
}

func (n *Node) releaseList(releasesPath string) any {
	if fi, err := os.Stat(releasesPath); err != nil || !fi.IsDir() {
		packed, _ := msgpack.Marshal([]any{})
		return append([]byte{ResOK}, packed...)
	}
	releases, latest := releasesListData(releasesPath)
	data := map[string]any{"releases": releases, "latest": nil}
	if latest != "" {
		data["latest"] = latest
	}
	return packOK(data)
}

func (n *Node) releaseView(releasesPath string, req map[any]any) any {
	tag, resp := sanReleaseTag(reqString(req, "tag"))
	if resp != nil {
		return resp
	}
	tag, resp = resolveReleaseTag(releasesPath, tag)
	if resp != nil {
		return resp
	}
	releaseDir := filepath.Join(releasesPath, tag)
	if fi, err := os.Stat(releaseDir); err != nil || !fi.IsDir() {
		return StatusResponse(ResNotFound, "Release not found")
	}
	info := releaseData(releaseDir, tag)
	if info == nil {
		return StatusResponse(ResRemoteFail, "Error getting release data")
	}
	return packOK(info)
}

func (n *Node) releaseFetch(releasesPath string, req map[any]any) any {
	tag, resp := sanReleaseTag(reqString(req, "tag"))
	if resp != nil {
		return resp
	}
	artifact := reqString(req, "artifact")
	if artifact == "" || strings.Contains(artifact, "/") {
		return StatusResponse(ResInvalidReq, "Invalid artifact specified")
	}
	artifact = path.Base(artifact)
	tag, resp = resolveReleaseTag(releasesPath, tag)
	if resp != nil {
		return resp
	}
	releaseDir := filepath.Join(releasesPath, tag)
	if fi, err := os.Stat(releaseDir); err != nil || !fi.IsDir() {
		return StatusResponse(ResNotFound, "Release not found")
	}
	artifactPath := filepath.Join(releaseDir, "artifacts", artifact)
	fi, err := os.Stat(artifactPath)
	if err != nil || !fi.Mode().IsRegular() {
		return StatusResponse(ResNotFound, "Artifact not found")
	}
	data, err := os.ReadFile(artifactPath) // #nosec G304 -- release artifact
	if err != nil {
		return StatusResponse(ResNotFound, "Artifact not found")
	}
	meta, _ := msgpack.Marshal(map[string]any{"name": []byte(artifact)})
	return link.FileResponse{Data: data, MetadataPacked: meta, AutoCompress: true}
}

func (n *Node) releaseCreate(releasesPath, repoPath string, req map[any]any, remote *identity.Identity) any {
	switch step := reqString(req, "step"); step {
	case "":
		return StatusResponse(ResInvalidReq, "Invalid request")
	case "init":
		return n.releaseCreateInit(releasesPath, repoPath, req, remote)
	case "artifact":
		return n.releaseCreateArtifact(releasesPath, req)
	case "finalize":
		return n.releaseCreateFinalize(releasesPath, req)
	default:
		return StatusResponse(ResInvalidReq, "Invalid request")
	}
}

func (n *Node) releaseCreateInit(releasesPath, repoPath string, req map[any]any, remote *identity.Identity) any {
	tag, resp := sanReleaseTag(reqString(req, "tag"))
	if resp != nil {
		return resp
	}
	if tag == "" || tag == "." || tag == ".." {
		return StatusResponse(ResInvalidReq, "Invalid tag name")
	}
	commitHash := reqString(req, "hash")
	notes := reqString(req, "notes")
	notesFormat := reqString(req, "notes_format")
	if notesFormat == "" {
		notesFormat = "markdown"
	}
	if _, err := n.git.RevParseVerify(repoPath, "refs/tags/"+tag); err != nil {
		return StatusResponse(ResInvalidReq, fmt.Sprintf("Tag '%s' does not exist in repository", tag))
	}
	if err := os.MkdirAll(releasesPath, 0o755); err != nil { // #nosec G301 -- releases root
		return StatusResponse(ResRemoteFail, "Remote error")
	}
	releaseDir := filepath.Join(releasesPath, tag)
	if fi, err := os.Stat(releaseDir); err == nil && fi.IsDir() {
		return StatusResponse(ResDisallowed, "Release already exists")
	}
	if err := os.MkdirAll(filepath.Join(releaseDir, "artifacts"), 0o755); err != nil { // #nosec G301 -- release dir
		return StatusResponse(ResRemoteFail, "Remote error")
	}
	meta := releaseMeta{
		"tag":        tag,
		"created":    strconv.FormatInt(time.Now().Unix(), 10),
		"status":     "draft",
		"created_by": fmt.Sprintf("%x", remote.Hash()),
	}
	if commitHash != "" {
		meta["hash"] = commitHash
	}
	if err := writeReleaseMeta(filepath.Join(releaseDir, "META"), meta); err != nil {
		return StatusResponse(ResRemoteFail, "Remote error")
	}
	if notes != "" {
		notesFile := "RELEASE.md"
		if notesFormat == "micron" {
			notesFile = "RELEASE.mu"
		}
		if err := os.WriteFile(filepath.Join(releaseDir, notesFile), []byte(notes), 0o644); err != nil { // #nosec G306 -- release notes
			return StatusResponse(ResRemoteFail, "Remote error")
		}
	}
	thanks, _ := msgpack.Marshal(map[string]any{"count": 0})
	if err := os.WriteFile(filepath.Join(releaseDir, "THANKS"), thanks, 0o644); err != nil { // #nosec G306 -- thanks marker
		return StatusResponse(ResRemoteFail, "Remote error")
	}
	return []byte{ResOK}
}

func (n *Node) releaseCreateArtifact(releasesPath string, req map[any]any) any {
	tag := reqString(req, "tag")
	artifactName := reqString(req, "artifact_name")
	artifactData, hasData := req["artifact_data"]
	if tag == "" || artifactName == "" {
		return StatusResponse(ResInvalidReq, "Missing tag or artifact name")
	}
	if strings.Contains(tag, "/") {
		return StatusResponse(ResInvalidReq, "Invalid tag specified")
	}
	if artifactData == nil || !hasData {
		return StatusResponse(ResInvalidReq, "No artifact data")
	}
	tag = path.Base(tag)
	artifactName = path.Base(artifactName)
	releaseDir := filepath.Join(releasesPath, tag)
	if fi, err := os.Stat(releaseDir); err != nil || !fi.IsDir() {
		return StatusResponse(ResNotFound, "Release not found")
	}
	meta, err := readReleaseMeta(filepath.Join(releaseDir, "META"))
	if err != nil {
		return StatusResponse(ResRemoteFail, "Remote error")
	}
	if meta["status"] != "draft" {
		return StatusResponse(ResDisallowed, "Release was finalized and is not writable")
	}
	var data []byte
	switch v := artifactData.(type) {
	case string:
		data = []byte(v)
	case []byte:
		data = v
	default:
		return StatusResponse(ResInvalidReq, "No artifact data")
	}
	artifactsDir := filepath.Join(releaseDir, "artifacts")
	if err := os.MkdirAll(artifactsDir, 0o755); err != nil { // #nosec G301 -- artifacts dir
		return StatusResponse(ResRemoteFail, "Remote error")
	}
	if err := os.WriteFile(filepath.Join(artifactsDir, artifactName), data, 0o644); err != nil { // #nosec G306 -- artifact
		return StatusResponse(ResRemoteFail, "Remote error")
	}
	return []byte{ResOK}
}

func (n *Node) releaseCreateFinalize(releasesPath string, req map[any]any) any {
	tag := reqString(req, "tag")
	if tag == "" {
		return StatusResponse(ResInvalidReq, "No tag specified")
	}
	if strings.Contains(tag, "/") {
		return StatusResponse(ResInvalidReq, "Invalid tag specified")
	}
	tag = path.Base(tag)
	releaseDir := filepath.Join(releasesPath, tag)
	if fi, err := os.Stat(releaseDir); err != nil || !fi.IsDir() {
		return StatusResponse(ResNotFound, "Release not found")
	}
	metaPath := filepath.Join(releaseDir, "META")
	meta, err := readReleaseMeta(metaPath)
	if err != nil {
		return StatusResponse(ResRemoteFail, "Remote error")
	}
	if meta["status"] != "draft" {
		return StatusResponse(ResDisallowed, "Release was finalized and is not writable")
	}
	meta["status"] = "published"
	meta["published_at"] = strconv.FormatInt(time.Now().Unix(), 10)
	if err := writeReleaseMeta(metaPath, meta); err != nil {
		return StatusResponse(ResRemoteFail, "Remote error")
	}
	_ = n.writeLatestRelease(releasesPath, tag) // #nosec G104 - best effort latest marker; release is already published
	return []byte{ResOK}
}

func (n *Node) releaseDelete(releasesPath string, req map[any]any) any {
	tag := reqString(req, "tag")
	if tag == "" {
		return StatusResponse(ResInvalidReq, "No tag specified")
	}
	if strings.Contains(tag, "/") {
		return StatusResponse(ResInvalidReq, "Invalid tag specified")
	}
	tag = path.Base(tag)
	releaseDir := filepath.Join(releasesPath, tag)
	if fi, err := os.Stat(releaseDir); err != nil || !fi.IsDir() {
		return StatusResponse(ResNotFound, "Release not found")
	}
	if err := os.RemoveAll(releaseDir); err != nil {
		return StatusResponse(ResRemoteFail, "Remote error")
	}
	return []byte{ResOK}
}

func (n *Node) releaseLatest(releasesPath string, req map[any]any) any {
	tag := reqString(req, "tag")
	if tag == "" {
		return StatusResponse(ResInvalidReq, "No tag specified")
	}
	if strings.Contains(tag, "/") {
		return StatusResponse(ResInvalidReq, "Invalid tag specified")
	}
	tag = path.Base(tag)
	releaseDir := filepath.Join(releasesPath, tag)
	if fi, err := os.Stat(releaseDir); err != nil || !fi.IsDir() {
		return StatusResponse(ResNotFound, "Release not found")
	}
	if err := n.writeLatestRelease(releasesPath, tag); err != nil {
		return StatusResponse(ResRemoteFail, "Remote error")
	}
	return []byte{ResOK}
}

// writeLatestRelease updates the latest marker via tmp+rename, matching
// Python's atomic latest-pointer writes.
func (n *Node) writeLatestRelease(releasesPath, tag string) error {
	latestPath := filepath.Join(releasesPath, "latest")
	tmp := latestPath + ".tmp"
	if err := os.WriteFile(tmp, []byte(tag), 0o644); err != nil { // #nosec G306 -- latest marker
		return err
	}
	return os.Rename(tmp, latestPath)
}
