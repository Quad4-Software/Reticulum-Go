// Command hugo-reticulum builds and serves Hugo sites as Micron pages over
// Reticulum.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/Quad4-Software/hugo-reticulum/internal/config"
	"github.com/Quad4-Software/hugo-reticulum/internal/markdown"
	"github.com/Quad4-Software/hugo-reticulum/internal/node"
	"github.com/Quad4-Software/hugo-reticulum/internal/site"
)

const usageText = `hugo-reticulum - serve a Hugo site as Micron pages over Reticulum

Usage:
  hugo-reticulum convert [flags] <file.md | ->
  hugo-reticulum build [flags]
  hugo-reticulum serve [flags]

Run "hugo-reticulum <command> -h" for command flags.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usageText)
		os.Exit(2)
	}
	var code int
	switch os.Args[1] {
	case "convert":
		code = runConvert(os.Args[2:])
	case "build":
		code = runBuild(os.Args[2:])
	case "serve":
		code = runServe(os.Args[2:])
	case "-h", "--help", "help":
		fmt.Fprint(os.Stdout, usageText)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", os.Args[1], usageText)
		code = 2
	}
	os.Exit(code)
}

// markdownFlags collects the shared conversion flags.
type markdownFlags struct {
	maxWidth    int
	linkStyle   string
	linkColor   string
	imagePolicy string
	tableAlign  string
	scope       string
	nodeHash    string
}

func (f *markdownFlags) register(fs *flag.FlagSet) {
	fs.IntVar(&f.maxWidth, "max-width", 0, "table and quote width budget in characters (0 = 100)")
	fs.StringVar(&f.linkStyle, "link-style", "", "link decoration: plain, underline, bold, both, color")
	fs.StringVar(&f.linkColor, "link-color", "", "hex color for link-style=color (3 or 6 digits)")
	fs.StringVar(&f.imagePolicy, "image-policy", "", "image handling: link, alt, skip")
	fs.StringVar(&f.tableAlign, "table-align", "", "table block alignment: left, center, right, or empty")
	fs.StringVar(&f.scope, "scope", ":/page/", "scope prefix for relative links")
	fs.StringVar(&f.nodeHash, "node-hash", "", "prefix scoped links with this node hash")
}

func (f *markdownFlags) options() markdown.Options {
	return markdown.Options{
		MaxWidth:         f.maxWidth,
		LinkStyle:        linkStyleValue(f.linkStyle),
		LinkColor:        f.linkColor,
		ImagePolicy:      imagePolicyValue(f.imagePolicy),
		TableAlign:       f.tableAlign,
		RewriteLink:      markdown.DefaultRewriteLink(f.scope, f.nodeHash),
		RewriteImageLink: markdown.DefaultRewriteLink(":/file/", f.nodeHash),
	}
}

func linkStyleValue(s string) markdown.LinkStyle {
	switch strings.ToLower(s) {
	case "underline":
		return markdown.LinkUnderline
	case "bold":
		return markdown.LinkBold
	case "both":
		return markdown.LinkBoth
	case "color":
		return markdown.LinkColor
	default:
		return markdown.LinkPlain
	}
}

func imagePolicyValue(s string) markdown.ImagePolicy {
	switch strings.ToLower(s) {
	case "alt":
		return markdown.ImageAlt
	case "skip":
		return markdown.ImageSkip
	default:
		return markdown.ImageLink
	}
}

func runConvert(args []string) int {
	fs := flag.NewFlagSet("convert", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var mf markdownFlags
	mf.register(fs)
	var outPath string
	fs.StringVar(&outPath, "o", "", "write output to file instead of stdout")
	fs.StringVar(&outPath, "output", "", "same as -o")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	var input []byte
	var err error
	switch {
	case fs.NArg() == 0 || fs.Arg(0) == "-":
		input, err = io.ReadAll(os.Stdin)
	default:
		input, err = os.ReadFile(fs.Arg(0)) // #nosec G304 -- user supplied path
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "read input: %v\n", err)
		return 1
	}

	out, err := markdown.Convert(string(input), mf.options())
	if err != nil {
		fmt.Fprintf(os.Stderr, "convert: %v\n", err)
		return 1
	}
	if outPath != "" {
		if err := os.WriteFile(outPath, []byte(out), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "write output: %v\n", err)
			return 1
		}
		return 0
	}
	fmt.Print(out)
	return 0
}

// buildFlags collects shared build/serve flags.
type buildFlags struct {
	sourceDir string
	config    string
	hugoPath  string
	outputDir string
	staticDir string
	inPlace   bool
}

func (f *buildFlags) register(fs *flag.FlagSet) {
	fs.StringVar(&f.sourceDir, "source", "", "Hugo site directory (default: current directory)")
	fs.StringVar(&f.config, "config", "", "hugo-reticulum.toml path (default: hugo-reticulum.toml found upward)")
	fs.StringVar(&f.hugoPath, "hugo", "", "hugo binary path (default: hugo on PATH)")
	fs.StringVar(&f.outputDir, "output", "", "output directory for Micron files (overrides config)")
	fs.StringVar(&f.staticDir, "static", "", "static files directory (overrides config)")
	fs.BoolVar(&f.inPlace, "in-place", false, "rewrite .mu files inside the publish dir")
}

func (f *buildFlags) load() (*config.Config, error) {
	cfgPath := f.config
	if cfgPath == "" {
		start := f.sourceDir
		if start == "" {
			start = "."
		}
		abs, err := filepath.Abs(start)
		if err != nil {
			return nil, err
		}
		cfgPath = config.Find(abs)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, err
	}
	if f.sourceDir != "" {
		cfg.Site.SourceDir = f.sourceDir
	}
	if f.outputDir != "" {
		cfg.Site.OutputDir = f.outputDir
		if !f.inPlace && cfg.Site.InPlace == nil {
			v := false
			cfg.Site.InPlace = &v
		}
	}
	if f.staticDir != "" {
		cfg.Site.StaticDir = f.staticDir
	}
	if f.inPlace {
		v := true
		cfg.Site.InPlace = &v
	}
	if err := cfg.Normalize(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// siteOptions maps config markdown settings to site.Options.
func siteOptions(cfg *config.Config) site.Options {
	return site.Options{
		Resolver: &site.Resolver{
			PublicDir:   cfg.PublicPath(),
			NodeHash:    cfg.Reticulum.NodeHash,
			RepoBaseURL: cfg.Markdown.RepoBaseURL,
		},
		MaxWidth:    cfg.Markdown.MaxWidth,
		LinkStyle:   linkStyleValue(cfg.Markdown.LinkStyle),
		LinkColor:   cfg.Markdown.LinkColor,
		ImagePolicy: imagePolicyValue(cfg.Markdown.ImagePolicy),
		TableAlign:  cfg.Markdown.TableAlign,
	}
}

// runHugo executes the hugo binary against the site.
func runHugo(cfg *config.Config, hugoPath string) error {
	if hugoPath == "" {
		hugoPath = cfg.Hugo.Path
	}
	if hugoPath == "" {
		hugoPath = "hugo"
	}
	args := append([]string{"--source", cfg.SourceDir(), "--destination", cfg.PublicPath()}, cfg.Hugo.Args...)
	cmd := exec.Command(hugoPath, args...) // #nosec G204 -- operator supplied binary and args
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = cfg.SourceDir()
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("hugo failed: %w", err)
	}
	return nil
}

// buildSite runs hugo and converts sentinel regions into the output tree.
func buildSite(cfg *config.Config, hugoPath string) error {
	if err := runHugo(cfg, hugoPath); err != nil {
		return err
	}
	opts := siteOptions(cfg)
	if cfg.InPlace() {
		n, err := site.WalkFiles(cfg.PublicPath(), opts)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "processed %d .mu files in %s\n", n, cfg.PublicPath())
		return nil
	}
	// Copy mode: render .mu files into the output directory.
	dst := cfg.OutputPath()
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	n, err := site.RenderTree(cfg.PublicPath(), dst, opts)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "rendered %d .mu files to %s\n", n, dst)
	return nil
}

func runBuild(args []string) int {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var bf buildFlags
	bf.register(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	cfg, err := bf.load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		return 1
	}
	if err := buildSite(cfg, bf.hugoPath); err != nil {
		fmt.Fprintf(os.Stderr, "build: %v\n", err)
		return 1
	}
	return 0
}

// serveFlags adds node and watch flags on top of buildFlags.
type serveFlags struct {
	buildFlags
	watch         bool
	watchInterval time.Duration
	identityPath  string
	configPath    string
	pageRefresh   int
	fileRefresh   int
	disableStats  bool
}

func (f *serveFlags) register(fs *flag.FlagSet) {
	f.buildFlags.register(fs)
	fs.BoolVar(&f.watch, "watch", false, "rebuild when site files change")
	fs.DurationVar(&f.watchInterval, "watch-interval", 2*time.Second, "poll interval for --watch")
	fs.StringVar(&f.identityPath, "identity", "", "Reticulum identity file path")
	fs.StringVar(&f.configPath, "reticulum-config", "", "Reticulum-Go config file path")
	fs.IntVar(&f.pageRefresh, "page-refresh", 0, "rescan pages every N seconds (0 = off)")
	fs.IntVar(&f.fileRefresh, "file-refresh", 0, "rescan files every N seconds (0 = off)")
	fs.BoolVar(&f.disableStats, "no-page-stats", false, "disable page view stats")
}

func runServe(args []string) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var sf serveFlags
	sf.register(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	cfg, err := sf.load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		return 1
	}
	if err := buildSite(cfg, sf.hugoPath); err != nil {
		fmt.Fprintf(os.Stderr, "build: %v\n", err)
		return 1
	}

	pagesDir := cfg.PublicPath()
	if !cfg.InPlace() {
		pagesDir = cfg.OutputPath()
	}
	filesDir := cfg.StaticPath()
	if _, err := os.Stat(filesDir); err != nil {
		filesDir = cfg.PublicPath()
	}

	pageRefresh := time.Duration(sf.pageRefresh) * time.Second
	fileRefresh := time.Duration(sf.fileRefresh) * time.Second
	if sf.watch && pageRefresh == 0 {
		pageRefresh = sf.watchInterval
		fileRefresh = sf.watchInterval
	}

	n, err := node.Start(node.Params{
		ReticulumConfigPath: firstNonEmpty(sf.configPath, cfg.Reticulum.ConfigPath),
		IdentityPath:        firstNonEmpty(sf.identityPath, cfg.Reticulum.IdentityPath),
		NodeName:            cfg.Reticulum.NodeName,
		AnnounceInterval:    cfg.AnnounceInterval(),
		PagesDir:            pagesDir,
		FilesDir:            filesDir,
		PageRefreshInterval: pageRefresh,
		FileRefreshInterval: fileRefresh,
		DisablePageStats:    sf.disableStats || cfg.Reticulum.DisablePageStats,
		LogLevel:            -1,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "reticulum: %v\n", err)
		return 1
	}
	defer n.Stop()

	n.Summary(os.Stderr)
	fmt.Fprintf(os.Stderr, "serving pages from %s\nserving files from %s\n", pagesDir, filesDir)

	if sf.watch {
		go watchLoop(cfg, sf.watchInterval, func() {
			fmt.Fprintln(os.Stderr, "change detected, rebuilding")
			if err := buildSite(cfg, sf.hugoPath); err != nil {
				fmt.Fprintf(os.Stderr, "rebuild: %v\n", err)
			}
		})
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan
	fmt.Fprintln(os.Stderr, "shutting down")
	return 0
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// watchLoop polls the site signature and calls rebuild when it changes.
func watchLoop(cfg *config.Config, interval time.Duration, rebuild func()) {
	sig := siteSignature(cfg.SourceDir())
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		cur := siteSignature(cfg.SourceDir())
		if cur != sig {
			sig = cur
			rebuild()
		}
	}
}

// siteSignature hashes file names, sizes, and mtimes under the site source
// directories, skipping build output.
func siteSignature(root string) string {
	var b strings.Builder
	for _, dir := range []string{"content", "layouts", "static", "assets", "i18n", "data"} {
		base := filepath.Join(root, dir)
		_ = filepath.WalkDir(base, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			fmt.Fprintf(&b, "%s|%d|%d\n", p, info.Size(), info.ModTime().UnixNano())
			return nil
		})
	}
	for _, f := range []string{"hugo.toml", "hugo.yaml", "hugo.json", "hugo-reticulum.toml"} {
		p := filepath.Join(root, f)
		if info, err := os.Stat(p); err == nil {
			fmt.Fprintf(&b, "%s|%d|%d\n", p, info.Size(), info.ModTime().UnixNano())
		}
	}
	return b.String()
}
