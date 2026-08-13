// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsutil

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/link"
	"quad4/reticulum-go/pkg/resource"
	"quad4/reticulum-go/pkg/term"
	"quad4/reticulum-go/pkg/transport"
)

const (
	// RNCPAppName is the destination app name for file transfer.
	RNCPAppName = "rncp"
	// RNCPAspect is the receive aspect.
	RNCPAspect = "receive"
	// RNCPFetchPath is the link request path for fetch mode.
	RNCPFetchPath = "fetch_file"
	// RNCPFetchNotAllowed is the REQ_FETCH_NOT_ALLOWED status byte.
	RNCPFetchNotAllowed = 0xF0
	// DefaultRNCPTimeout is the default path/link wait.
	DefaultRNCPTimeout = 15 * time.Second
)

// RNCPIdentityPath returns the default identity path under storage.
func RNCPIdentityPath(cfgStorage string) string {
	if cfgStorage == "" {
		return ""
	}
	return filepath.Join(cfgStorage, "identities", RNCPAppName)
}

// PrepareRNCPIdentity loads or creates the rncp identity file.
func PrepareRNCPIdentity(path string) (*identity.Identity, error) {
	if path == "" {
		return nil, fmt.Errorf("empty identity path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if st, err := os.Stat(path); err == nil && !st.IsDir() {
		return identity.FromFile(path)
	}
	id, err := identity.New()
	if err != nil {
		return nil, err
	}
	if err := id.ToFile(path); err != nil {
		return nil, err
	}
	return id, nil
}

// LoadAllowedIdentities reads 32-hex identity hashes from allowed files and CLI args.
func LoadAllowedIdentities(extra []string) ([][]byte, error) {
	var out [][]byte
	seen := map[string]struct{}{}
	add := func(h []byte) {
		k := hex.EncodeToString(h)
		if _, ok := seen[k]; ok {
			return
		}
		seen[k] = struct{}{}
		out = append(out, h)
	}
	candidates := []string{
		"/etc/rncp/allowed_identities",
		filepath.Join(os.Getenv("HOME"), ".config", "rncp", "allowed_identities"),
		filepath.Join(os.Getenv("HOME"), ".rncp", "allowed_identities"),
		filepath.Join(os.Getenv("HOME"), ".config", "rgocp", "allowed_identities"),
		filepath.Join(os.Getenv("HOME"), ".rgocp", "allowed_identities"),
	}
	for _, path := range candidates {
		hashes, err := readAllowedFile(path)
		if err != nil {
			continue
		}
		for _, h := range hashes {
			add(h)
		}
	}
	for _, a := range extra {
		h, err := ParseDestHash(strings.TrimSpace(a))
		if err != nil {
			return nil, fmt.Errorf("allowed identity %q: %w", a, err)
		}
		add(h)
	}
	return out, nil
}

func readAllowedFile(path string) ([][]byte, error) {
	f, err := os.Open(path) // #nosec G304,G703 -- operator-configured allow list path
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out [][]byte
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		h, err := ParseDestHash(line)
		if err != nil {
			continue
		}
		out = append(out, h)
	}
	return out, sc.Err()
}

// AllowedContains reports whether hash is in the allow list.
func AllowedContains(list [][]byte, hash []byte) bool {
	for _, a := range list {
		if bytes.Equal(a, hash) {
			return true
		}
	}
	return false
}

// UniqueSavePath returns a writable path, appending .N when the file exists
// unless overwrite is set.
func UniqueSavePath(dir, name string, overwrite bool) (string, error) {
	base := filepath.Base(name)
	if base == "." || base == ".." || base == "" {
		return "", fmt.Errorf("invalid file name")
	}
	var full string
	if dir != "" {
		full = filepath.Join(dir, base)
		absDir, err := filepath.Abs(dir)
		if err != nil {
			return "", err
		}
		absFull, err := filepath.Abs(full)
		if err != nil {
			return "", err
		}
		if !strings.HasPrefix(absFull, absDir+string(os.PathSeparator)) && absFull != absDir {
			return "", fmt.Errorf("invalid save path")
		}
	} else {
		full = base
	}
	if overwrite {
		_ = os.Remove(full)
		return full, nil
	}
	if _, err := os.Stat(full); os.IsNotExist(err) {
		return full, nil
	}
	for i := 1; ; i++ {
		cand := full + "." + fmt.Sprintf("%d", i)
		if _, err := os.Stat(cand); os.IsNotExist(err) {
			return cand, nil
		}
	}
}

// WriteReceivedFile writes payload to a unique path under saveDir.
func WriteReceivedFile(saveDir, name string, data []byte, overwrite bool) (string, error) {
	path, err := UniqueSavePath(saveDir, name, overwrite)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// FilenameFromMetadata extracts the UTF-8 basename from rncp metadata.
func FilenameFromMetadata(meta map[string]any) string {
	if meta == nil {
		return "received.bin"
	}
	raw, ok := meta["name"]
	if !ok {
		return "received.bin"
	}
	var name string
	switch v := raw.(type) {
	case []byte:
		name = string(v)
	case string:
		name = v
	default:
		return "received.bin"
	}
	name = filepath.Base(name)
	if name == "" || name == "." || name == ".." {
		return "received.bin"
	}
	return name
}

// WaitLinkActive waits until the link is active or ctx is done.
func WaitLinkActive(ctx context.Context, l *link.Link) error {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		if l.IsActive() {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// EstablishRNCPLink waits for a path and opens an outbound link to rncp.receive.
func EstablishRNCPLink(ctx context.Context, tr *transport.Transport, destHash []byte) (*link.Link, error) {
	if err := WaitPathWindow(ctx, tr, destHash); err != nil {
		return nil, fmt.Errorf("path: %w", err)
	}
	remote, err := identity.Recall(destHash)
	if err != nil {
		return nil, fmt.Errorf("recall: %w", err)
	}
	outDest, err := destination.New(remote, destination.Out, destination.Single, RNCPAppName, tr, RNCPAspect)
	if err != nil {
		return nil, err
	}
	l := link.NewLink(outDest, tr, nil, nil, nil)
	if err := activateOutboundLink(ctx, l); err != nil {
		return nil, fmt.Errorf("link: %w", err)
	}
	return l, nil
}

// SendFileOverLink pushes a file resource with rncp metadata over an active link.
func SendFileOverLink(l *link.Link, filePath string, autoCompress bool, progress func(float64, int64, int64)) error {
	data, err := os.ReadFile(filePath) // #nosec G304 -- operator-selected file path
	if err != nil {
		return err
	}
	res, err := resource.New(data, autoCompress)
	if err != nil {
		return err
	}
	meta := map[string]any{"name": []byte(filepath.Base(filePath))}
	if err := res.SetMetadata(meta); err != nil {
		return err
	}
	if progress != nil {
		res.SetProgressCallback(func(r *resource.Resource) {
			progress(r.GetProgress(), int64(float64(r.GetDataSize())*r.GetProgress()), r.GetDataSize())
		})
	}
	return l.SendResource(res)
}

// FetchFileStatus classifies a fetch_file response value.
type FetchFileStatus int

const (
	FetchFound FetchFileStatus = iota
	FetchNotFound
	FetchNotAllowed
	FetchRemoteError
	FetchUnknown
)

// ClassifyFetchResponse maps fetch_file response values to FetchFileStatus.
func ClassifyFetchResponse(v any) FetchFileStatus {
	switch x := v.(type) {
	case bool:
		if x {
			return FetchFound
		}
		return FetchNotFound
	case nil:
		return FetchRemoteError
	case int:
		if x == RNCPFetchNotAllowed {
			return FetchNotAllowed
		}
		if x == 0 {
			return FetchNotFound
		}
	case int64:
		if x == RNCPFetchNotAllowed {
			return FetchNotAllowed
		}
		if x == 0 {
			return FetchNotFound
		}
	case uint8:
		if x == RNCPFetchNotAllowed {
			return FetchNotAllowed
		}
	case []byte:
		if len(x) == 1 && x[0] == RNCPFetchNotAllowed {
			return FetchNotAllowed
		}
	}
	return FetchUnknown
}

// WaitRequest waits until a request receipt concludes or ctx is done.
func WaitRequest(ctx context.Context, r *link.RequestReceipt) error {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		if r.Concluded() {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// ProgressPrinter prints transfer progress to stderr (Go uniqueness: clean line updates).
type ProgressPrinter struct {
	mu      sync.Mutex
	last    string
	enabled bool
	out     *os.File
}

// NewProgressPrinter returns a progress printer writing to stderr.
// Silent disables output.
func NewProgressPrinter(silent bool) *ProgressPrinter {
	return NewProgressPrinterTo(silent, os.Stderr)
}

// NewProgressPrinterTo returns a progress printer writing to out.
func NewProgressPrinterTo(silent bool, out *os.File) *ProgressPrinter {
	if out == nil {
		out = os.Stderr
	}
	return &ProgressPrinter{enabled: !silent, out: out}
}

// Update prints a progress line.
func (p *ProgressPrinter) Update(label string, pct float64, got, total int64, bps float64) {
	if p == nil || !p.enabled {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	clearSeq := term.ProgressClear(p.out)
	line := fmt.Sprintf("%s%s %.1f%% - %s of %s - %s/s",
		clearSeq, term.Cyan(p.out, label), pct*100, SizeString(float64(got), "B"), SizeString(float64(total), "B"), SizeString(bps, "b"))
	fmt.Fprint(p.out, line)
	p.last = line
}

// Done finishes the progress line.
func (p *ProgressPrinter) Done(msg string) {
	if p == nil || !p.enabled {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	fmt.Fprint(p.out, term.ProgressClear(p.out))
	if msg != "" {
		fmt.Fprintln(p.out, msg)
	}
}
