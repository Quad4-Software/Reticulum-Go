package main

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Quad4-Software/hugo-reticulum/internal/markdown"
)

func TestMarkdownFlagsOptions(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	var mf markdownFlags
	mf.register(fs)
	if err := fs.Parse([]string{
		"-max-width", "42",
		"-link-style", "underline",
		"-link-color", "ff0000",
		"-image-policy", "alt",
		"-table-align", "right",
		"-scope", ":/custom/",
		"-node-hash", "abc123",
	}); err != nil {
		t.Fatal(err)
	}
	opts := mf.options()
	if opts.MaxWidth != 42 {
		t.Errorf("MaxWidth = %d", opts.MaxWidth)
	}
	if opts.LinkStyle != markdown.LinkUnderline {
		t.Errorf("LinkStyle = %v", opts.LinkStyle)
	}
	if opts.LinkColor != "ff0000" {
		t.Errorf("LinkColor = %q", opts.LinkColor)
	}
	if opts.ImagePolicy != markdown.ImageAlt {
		t.Errorf("ImagePolicy = %v", opts.ImagePolicy)
	}
	if opts.TableAlign != "right" {
		t.Errorf("TableAlign = %q", opts.TableAlign)
	}
	if opts.RewriteLink == nil || opts.RewriteImageLink == nil {
		t.Error("rewrite funcs should be set")
	}
	if got := opts.RewriteLink("page.md"); got != "abc123:/custom/page" {
		t.Errorf("RewriteLink = %q", got)
	}
}

func TestLinkStyleValue(t *testing.T) {
	cases := map[string]markdown.LinkStyle{
		"":          markdown.LinkPlain,
		"plain":     markdown.LinkPlain,
		"underline": markdown.LinkUnderline,
		"bold":      markdown.LinkBold,
		"both":      markdown.LinkBoth,
		"color":     markdown.LinkColor,
		"BOTH":      markdown.LinkBoth,
	}
	for in, want := range cases {
		if got := linkStyleValue(in); got != want {
			t.Errorf("linkStyleValue(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestImagePolicyValue(t *testing.T) {
	cases := map[string]markdown.ImagePolicy{
		"":      markdown.ImageLink,
		"link":  markdown.ImageLink,
		"alt":   markdown.ImageAlt,
		"skip":  markdown.ImageSkip,
		"ALT":   markdown.ImageAlt,
		"other": markdown.ImageLink,
	}
	for in, want := range cases {
		if got := imagePolicyValue(in); got != want {
			t.Errorf("imagePolicyValue(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestBuildFlagsLoad(t *testing.T) {
	dir := t.TempDir()
	cfgData := `
[site]
public_dir = "pub"
output_dir = "out"

[markdown]
link_style = "bold"
`
	if err := os.WriteFile(filepath.Join(dir, "hugo-reticulum.toml"), []byte(cfgData), 0o644); err != nil {
		t.Fatal(err)
	}
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	var bf buildFlags
	bf.register(fs)
	if err := fs.Parse([]string{"-source", dir}); err != nil {
		t.Fatal(err)
	}
	cfg, err := bf.load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Site.PublicDir != "pub" {
		t.Errorf("PublicDir = %q", cfg.Site.PublicDir)
	}
	if cfg.InPlace() {
		t.Error("output_dir set should make InPlace false")
	}
	if cfg.Markdown.LinkStyle != "bold" {
		t.Errorf("LinkStyle = %q", cfg.Markdown.LinkStyle)
	}
	if !strings.HasSuffix(cfg.OutputPath(), filepath.Join(dir, "out")) {
		t.Errorf("OutputPath = %q", cfg.OutputPath())
	}
}

func TestRunConvertFile(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.md")
	out := filepath.Join(dir, "out.mu")
	if err := os.WriteFile(in, []byte("# Title\n\nSome **bold** text.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code := runConvert([]string{"-o", out, in})
	if code != 0 {
		t.Fatalf("runConvert = %d", code)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "`!bold`!") {
		t.Fatalf("output missing bold markup: %q", data)
	}
	if !strings.HasPrefix(string(data), "> Title") {
		t.Fatalf("output missing heading: %q", data)
	}
}
