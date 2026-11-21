package site

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProcessBytesSentinel(t *testing.T) {
	in := "#!bg=112233\n> Title\n\n" + SentinelOpen + "\n# Hello *World*\n\nSome **bold** text.\n" + SentinelClose + "\n\n`<name`value>\n"
	out, err := ProcessBytes([]byte(in), Options{}, "index")
	if err != nil {
		t.Fatal(err)
	}
	want := "#!bg=112233\n> Title\n\n> Hello `*World`*\n\nSome `!bold`! text.\n\n`<name`value>\n"
	if string(out) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out, want)
	}
}

func TestProcessBytesNoSentinel(t *testing.T) {
	in := "plain micron\n> heading\n"
	out, err := ProcessBytes([]byte(in), Options{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != in {
		t.Fatalf("got %q want %q", out, in)
	}
}

func TestProcessBytesUnclosed(t *testing.T) {
	_, err := ProcessBytes([]byte(SentinelOpen+"\nbody\n"), Options{}, "")
	if err == nil {
		t.Fatal("expected error for unclosed sentinel")
	}
}

func TestProcessBytesMultipleRegions(t *testing.T) {
	in := SentinelOpen + "\n# A\n" + SentinelClose + "\nmiddle\n" + SentinelOpen + "\n# B\n" + SentinelClose + "\n"
	out, err := ProcessBytes([]byte(in), Options{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "> A\n") || !strings.Contains(string(out), "> B\n") {
		t.Fatalf("got:\n%s", out)
	}
}

func TestResolverLink(t *testing.T) {
	r := &Resolver{}
	cases := []struct {
		href     string
		fromPage string
		want     string
	}{
		{"/about/", "index", ":/page/about/index.mu"},
		{"about.md", "index", ":/page/about/index.mu"},
		{"../other.md", "blog/post/index", ":/page/blog/other/index.mu"},
		{"other.md", "blog/post", ":/page/blog/other/index.mu"},
		{"#section", "index", "#section"},
		{"page.md#deep", "index", ":/page/page/index.mu|anchor=deep"},
		{"https://example.com", "index", "https://example.com"},
		{":/page/x.mu", "index", ":/page/x.mu"},
		{"/docs/guide.pdf", "index", ":/file/docs/guide.pdf"},
		{"images/pic.webp", "index", ":/file/images/pic.webp"},
	}
	for _, tc := range cases {
		got := r.Link(tc.href, tc.fromPage)
		if got != tc.want {
			t.Errorf("Link(%q, %q) = %q, want %q", tc.href, tc.fromPage, got, tc.want)
		}
	}
}

func TestResolverNodeHash(t *testing.T) {
	r := &Resolver{NodeHash: "0123456789abcdef0123456789abcdef"}
	got := r.Link("/about/", "index")
	want := "0123456789abcdef0123456789abcdef:/page/about/index.mu"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestResolverUsesIndex(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "flat"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "flat", "page.mu"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Resolver{PublicDir: dir}
	// flat/page.mu exists, so /flat/page resolves to it directly.
	got := r.Link("/flat/page", "index")
	if got != ":/page/flat/page.mu" {
		t.Fatalf("got %q", got)
	}
	// Unknown path falls back to index layout.
	got = r.Link("/nope/", "index")
	if got != ":/page/nope/index.mu" {
		t.Fatalf("got %q", got)
	}
}

func TestWalkFiles(t *testing.T) {
	dir := t.TempDir()
	body := SentinelOpen + "\n# Hi\n" + SentinelClose + "\n"
	if err := os.WriteFile(filepath.Join(dir, "index.mu"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "skip.mu"), []byte("no sentinels\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	n, err := WalkFiles(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	// Every .mu file is processed now: files without sentinels still get
	// chrome link rewriting.
	if n != 2 {
		t.Fatalf("processed %d files, want 2", n)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "index.mu"))
	if !strings.Contains(string(data), "> Hi\n") {
		t.Fatalf("got %q", data)
	}
	skip, _ := os.ReadFile(filepath.Join(dir, "skip.mu"))
	if string(skip) != "no sentinels\n" {
		t.Fatalf("skip.mu changed: %q", skip)
	}
}
