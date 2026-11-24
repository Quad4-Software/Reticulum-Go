// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// minimalWebP returns a 30-byte VP8 WebP header webpInfo accepts.
func minimalWebP() []byte {
	b := make([]byte, 30)
	copy(b[0:4], "RIFF")
	copy(b[8:12], "WEBP")
	copy(b[12:16], "VP8 ")
	b[26] = 1
	b[28] = 1
	return b
}

// fakeMagick installs a stub "magick" binary on PATH. It records its argv
// into marker and emits a fixed valid WebP on stdout, so conversion tests can
// observe whether and how the encoder was invoked.
func fakeMagick(t *testing.T) (dir, marker string) {
	t.Helper()
	dir = t.TempDir()
	marker = filepath.Join(dir, "argv")
	script := "#!/bin/sh\nprintf '%s\n' \"$@\" > " + marker + "\ncat " + filepath.Join(dir, "out.webp") + "\n"
	if err := os.WriteFile(filepath.Join(dir, "magick"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "out.webp"), minimalWebP(), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("RNGIT_MEDIA_BACKEND", "magick")
	return dir, marker
}

func TestDetectImageFormat(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want string
	}{
		{"png", []byte("\x89PNG\r\n\x1a\n" + "rest"), "png"},
		{"jpeg", []byte("\xff\xd8\xff\xe0rest"), "jpeg"},
		{"gif87", []byte("GIF87a" + "rest"), "gif"},
		{"gif89", []byte("GIF89a" + "rest"), "gif"},
		{"tiff_ii", []byte("II*\x00rest"), "tiff"},
		{"tiff_mm", []byte("MM\x00*rest"), "tiff"},
		{"webp", minimalWebP(), "webp"},
		{"bmp", append([]byte("BM"+string(make([]byte, 12))), []byte{40, 0, 0, 0}...), "bmp"},
		{"bmp_bad_dib", append([]byte("BM"+string(make([]byte, 12))), []byte{7, 0, 0, 0}...), ""},
		{"mvg", []byte("push graphic-context\nviewbox 0 0 640 480\n"), ""},
		{"msl", []byte("<?xml version=\"1.0\"?><image><read filename=\"/etc/passwd\"/></image>"), ""},
		{"text", []byte("root:x:0:0:root:/root:/bin/sh\n"), ""},
		{"empty", nil, ""},
		{"short", []byte("BM"), ""},
	}
	for _, c := range cases {
		if got := DetectImageFormat(c.data); got != c.want {
			t.Errorf("%s: DetectImageFormat = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestConvertToWebPRejectsNonImageInput(t *testing.T) {
	_, marker := fakeMagick(t)
	out := filepath.Join(t.TempDir(), "o.webp")
	// Input produces MVG bytes: a classic ImageMagick arbitrary-read payload.
	if ConvertToWebP([]string{"printf", "push graphic-context\nviewbox 0 0 640 480\n"}, out, "", 0, 0, 0) {
		t.Fatal("non-image input converted")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("encoder was invoked for rejected input")
	}
}

func TestConvertToWebPPassesDetectedFormat(t *testing.T) {
	_, marker := fakeMagick(t)
	out := filepath.Join(t.TempDir(), "o.webp")
	if !ConvertToWebP([]string{"printf", "\x89PNG\r\n\x1a\n" + strings.Repeat("x", 200)}, out, "", 0, 0, 0) {
		t.Fatal("valid PNG input not converted")
	}
	argv, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("encoder argv not recorded: %v", err)
	}
	if !strings.Contains(string(argv), "png:-") {
		t.Fatalf("encoder argv %q missing explicit png:- input spec", argv)
	}
	if strings.Contains(string(argv), "png:-\n-") || !strings.Contains(string(argv), "webp:-") {
		t.Fatalf("encoder argv %q malformed", argv)
	}
}

func TestConvertFileToWebPRejectsNonImage(t *testing.T) {
	_, marker := fakeMagick(t)
	src := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(src, []byte("root:x:0:0:root:/root:/bin/sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := ConvertFileToWebP(src, 85, 0, 0); ok {
		t.Fatal("non-image file converted")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("encoder was invoked for rejected input")
	}
}

func TestConvertFileToWebPValidImage(t *testing.T) {
	_, marker := fakeMagick(t)
	src := filepath.Join(t.TempDir(), "img.jpg")
	if err := os.WriteFile(src, append([]byte("\xff\xd8\xff\xe0"), make([]byte, 100)...), 0o644); err != nil {
		t.Fatal(err)
	}
	out, ok := ConvertFileToWebP(src, 85, 0, 0)
	if !ok {
		t.Fatal("valid jpeg file rejected")
	}
	defer func() { _ = os.Remove(out) }()
	argv, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("encoder argv not recorded: %v", err)
	}
	if !strings.Contains(string(argv), "jpeg:-") {
		t.Fatalf("encoder argv %q missing explicit jpeg:- input spec", argv)
	}
}

func TestConvertFileToWebPSizeCap(t *testing.T) {
	fakeMagick(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "big.png")
	f, err := os.Create(src)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("\x89PNG\r\n\x1a\n")); err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(convSizeMax + 1); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	if _, ok := ConvertFileToWebP(src, 85, 0, 0); ok {
		t.Fatal("oversized file converted")
	}
}
