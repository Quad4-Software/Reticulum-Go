// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Quad4-Software/Reticulum-Go/pkg/link"
	"github.com/Quad4-Software/msgpack/v5/pkg/msgpack"
)

func riffWebP(fourcc string, body []byte) []byte {
	out := make([]byte, 30)
	copy(out[0:4], "RIFF")
	copy(out[8:12], "WEBP")
	copy(out[12:16], fourcc)
	copy(out[16:], body)
	return out
}

func TestWebPInfo(t *testing.T) {
	// VP8X: 24-bit LE width-1 at 24, height-1 at 27.
	vp8x := riffWebP("VP8X", make([]byte, 14))
	vp8x[24], vp8x[25], vp8x[26] = 0x3F, 0x01, 0x00 // 319+1 = 320
	vp8x[27], vp8x[28], vp8x[29] = 0x7F, 0x00, 0x00 // 127+1 = 128
	if w, h, ok := webpInfo(vp8x); !ok || w != 320 || h != 128 {
		t.Fatalf("VP8X: %dx%d ok=%v", w, h, ok)
	}

	// VP8 lossy: 14-bit LE width at 26, height at 28.
	vp8 := riffWebP("VP8 ", make([]byte, 14))
	vp8[26], vp8[27] = 0x40, 0x01 // 320
	vp8[28], vp8[29] = 0xE0, 0x00 // 224
	if w, h, ok := webpInfo(vp8); !ok || w != 320 || h != 224 {
		t.Fatalf("VP8: %dx%d ok=%v", w, h, ok)
	}

	// VP8L: signature byte at 20, packed bits at 21.
	vp8l := riffWebP("VP8L", make([]byte, 14))
	vp8l[20] = 0x2F
	bits := uint32(99) | (uint32(49) << 14) // w=100, h=50
	vp8l[21] = byte(bits)
	vp8l[22] = byte(bits >> 8)
	vp8l[23] = byte(bits >> 16)
	vp8l[24] = byte(bits >> 24)
	if w, h, ok := webpInfo(vp8l); !ok || w != 100 || h != 50 {
		t.Fatalf("VP8L: %dx%d ok=%v", w, h, ok)
	}

	if _, _, ok := webpInfo([]byte("not a webp")); ok {
		t.Fatal("accepted invalid data")
	}
	if _, _, ok := webpInfo(riffWebP("JUNK", make([]byte, 14))); ok {
		t.Fatal("accepted unknown fourcc")
	}
}

func TestFileStem(t *testing.T) {
	for in, want := range map[string]string{
		"photo.png": "photo",
		"a.b.webp":  "a.b",
		"noext":     "noext",
		".hidden":   ".hidden",
		"name.jpeg": "name",
	} {
		if got := fileStem(in); got != want {
			t.Fatalf("fileStem(%q) = %q want %q", in, got, want)
		}
	}
}

func TestSelectedMediaBackendEnv(t *testing.T) {
	dir := t.TempDir()
	// Fake backend executable on PATH.
	fake := filepath.Join(dir, "zzfakeenc")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\ncat >/dev/null\n"), 0o755); err != nil { // #nosec G306 -- test helper
		t.Fatal(err)
	}
	mediaBackends = append(mediaBackends, struct {
		name string
		argv []string
	}{"zzfakeenc", []string{fake}})
	defer func() { mediaBackends = mediaBackends[:len(mediaBackends)-1] }()

	t.Setenv("RNGIT_MEDIA_BACKEND", "zzfakeenc")
	name, argv, ok := selectedMediaBackend()
	if !ok || name != "zzfakeenc" || argv[0] != fake {
		t.Fatalf("env backend: %v %v %v", name, argv, ok)
	}

	t.Setenv("RNGIT_MEDIA_BACKEND", "nonexistent")
	if _, _, ok := selectedMediaBackend(); ok {
		t.Fatal("unknown env backend should fail")
	}
}

func TestServeMedia(t *testing.T) {
	node, id, _ := newWorkTestNode(t, "r:all")
	repoPath := filepath.Join(node.accessTable().Groups["public"].Path, "demo")

	testGitEnv := append(GitCleanEnv(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repoPath}, args...)...) // #nosec G204 -- test git
		cmd.Env = testGitEnv
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}

	blobCmd := exec.Command("git", "-C", repoPath, "hash-object", "-w", "--stdin") // #nosec G204 -- test git
	blobCmd.Env = testGitEnv
	blobCmd.Stdin = strings.NewReader("hello blob")
	blobOut, err := blobCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("hash-object: %v %s", err, blobOut)
	}
	blob := strings.TrimSpace(string(blobOut))
	mk := exec.Command("git", "-C", repoPath, "mktree") // #nosec G204 -- test git
	mk.Env = testGitEnv
	mk.Stdin = strings.NewReader("100644 blob " + blob + "\tfile.txt\n")
	treeOut, err := mk.CombinedOutput()
	if err != nil {
		t.Fatalf("mktree: %v %s", err, treeOut)
	}
	tree := strings.TrimSpace(string(treeOut))
	commit := git("commit-tree", tree, "-m", "init")
	git("update-ref", "refs/heads/master", commit)

	mediaReq := func(p string) []byte {
		t.Helper()
		b, err := EncodeMixedRequest(map[any]any{"key": "k", "path": p})
		if err != nil {
			t.Fatal(err)
		}
		return b
	}

	// Happy path serves the blob with file name metadata.
	resp := node.serveMedia(mediaReq("/media/public/demo/master/file.txt"), id)
	fr, ok := resp.(link.FileResponse)
	if !ok {
		t.Fatalf("response type %T", resp)
	}
	if string(fr.Data) != "hello blob" {
		t.Fatalf("data: %q", fr.Data)
	}
	if fr.AutoCompress {
		t.Fatal("media responses must not auto-compress")
	}
	var meta map[string]any
	if err := msgpack.Unmarshal(fr.MetadataPacked, &meta); err != nil {
		t.Fatal(err)
	}
	if string(meta["name"].([]byte)) != "file.txt" {
		t.Fatalf("name: %v", meta["name"])
	}

	// Missing key, short path, unknown repo, bad ref, missing blob.
	for _, p := range []string{
		"", "/media", "/media/public/demo", "/media/public/demo/master",
		"/media/public/demo/master/missing.txt",
		"/media/nope/demo/master/file.txt",
		"/media/public/nope/master/file.txt",
		"/media/public/demo/badref/file.txt",
	} {
		if r := node.serveMedia(mediaReq(p), id); r != nil {
			t.Fatalf("path %q: got %T", p, r)
		}
	}

	// No request key at all.
	bad, _ := EncodeMixedRequest(map[any]any{"path": "/media/public/demo/master/file.txt"})
	if r := node.serveMedia(bad, id); r != nil {
		t.Fatalf("missing key: %T", r)
	}

	// Unidentified remote maps to the null identity. With r:all it can read.
	if r := node.serveMedia(mediaReq("/media/public/demo/master/file.txt"), nil); r == nil {
		t.Fatal("unidentified remote denied despite r:all")
	}

	// Blocking the null identity cuts off unidentified access.
	node.cfg.BlockedIdentities[nullIdentHash] = true
	if err := node.reloadAccess(); err != nil {
		t.Fatal(err)
	}
	if r := node.serveMedia(mediaReq("/media/public/demo/master/file.txt"), nil); r != nil {
		t.Fatalf("blocked null remote: %T", r)
	}
}
