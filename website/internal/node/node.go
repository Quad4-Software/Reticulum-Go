// Package node wraps Reticulum-Go's pageserver as an embeddable node.
package node

import (
	"fmt"
	"io"
	"os"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/debug"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/pageserver"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/reticulumconfig"
)

// Node is a running Reticulum page server.
type Node struct {
	r *pageserver.Reticulum
}

// Params configures the node.
type Params struct {
	// ReticulumConfigPath is the Reticulum-Go config file. Empty resolves
	// the default ~/.reticulum-go/config.
	ReticulumConfigPath string

	// IdentityPath overrides the identity file.
	IdentityPath string

	// NodeName is the announce display name.
	NodeName string

	// AnnounceInterval is the periodic announce period. Zero announces once.
	AnnounceInterval time.Duration

	// PagesDir is served under /page/.
	PagesDir string

	// FilesDir is served under /file/.
	FilesDir string

	// PageRefreshInterval rescans PagesDir on an interval. Zero disables.
	PageRefreshInterval time.Duration

	// FileRefreshInterval rescans FilesDir on an interval. Zero disables.
	FileRefreshInterval time.Duration

	// DisablePageStats turns off the built-in page view counter.
	DisablePageStats bool

	// LogLevel overrides the debug level. Negative keeps the config value.
	LogLevel int
}

// Hash returns the destination hash as lowercase hex.
func (n *Node) Hash() string {
	return fmt.Sprintf("%x", n.r.GetDestination().GetHash())
}

// Start loads the Reticulum config, creates the pageserver node, registers
// the announce handler, and starts it.
func Start(p Params) (*Node, error) {
	cfg, err := loadConfig(p.ReticulumConfigPath)
	if err != nil {
		return nil, err
	}
	if p.LogLevel >= 0 {
		debug.SetDebugLevel(p.LogLevel)
	}

	r, err := pageserver.NewReticulum(cfg, buildOptions(p))
	if err != nil {
		return nil, fmt.Errorf("create reticulum node: %w", err)
	}

	go r.MonitorInterfaces()

	handler := pageserver.NewAnnounceHandler(r, []string{"*"})
	r.Transport().RegisterAnnounceHandler(handler)

	if err := r.Start(); err != nil {
		return nil, fmt.Errorf("start reticulum node: %w", err)
	}

	return &Node{r: r}, nil
}

// buildOptions maps Params onto pageserver.Options.
func buildOptions(p Params) pageserver.Options {
	nodeName := p.NodeName
	if nodeName == "" {
		nodeName = "hugo-reticulum"
	}
	return pageserver.Options{
		PagesDir:                p.PagesDir,
		FilesDir:                p.FilesDir,
		PageRefreshInterval:     p.PageRefreshInterval,
		FileRefreshInterval:     p.FileRefreshInterval,
		AnnounceIntervalMinutes: int(p.AnnounceInterval / time.Minute),
		IdentityFileOverride:    p.IdentityPath,
		NodeDisplayName:         pageserver.ClampDisplayName(nodeName, "hugo-reticulum"),
		DisablePageStats:        p.DisablePageStats,
	}
}

// Summary writes the startup banner, including the destination hash, to w.
func (n *Node) Summary(w io.Writer) {
	pageserver.PrintStartupSummaryTo(w, n.r)
}

// Stop shuts the node down.
func (n *Node) Stop() error {
	return n.r.Stop()
}

// loadConfig mirrors the pageserver CLI: use the override path, else the
// default config location, creating a default file when missing.
func loadConfig(override string) (*common.ReticulumConfig, error) {
	path := override
	if path == "" {
		p, err := reticulumconfig.GetConfigPath()
		if err != nil {
			return nil, fmt.Errorf("resolve default config path: %w", err)
		}
		path = p
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := reticulumconfig.CreateDefaultConfig(path); err != nil {
			return nil, fmt.Errorf("create default config %q: %w", path, err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("stat config %q: %w", path, err)
	}
	cfg, err := reticulumconfig.LoadConfig(path)
	if err != nil {
		return nil, fmt.Errorf("load config %q: %w", path, err)
	}
	return cfg, nil
}
