// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interop

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"git.quad4.io/Networks/Reticulum-Go/pkg/common"
	"git.quad4.io/Networks/Reticulum-Go/pkg/destination"
	"git.quad4.io/Networks/Reticulum-Go/pkg/identity"
	"git.quad4.io/Networks/Reticulum-Go/pkg/interfaces"
	rlink "git.quad4.io/Networks/Reticulum-Go/pkg/link"
	"git.quad4.io/Networks/Reticulum-Go/pkg/transport"
)

type nomadnetAnnounceCollector struct {
	mu        sync.Mutex
	announces map[string]announcedNode
}

type announcedNode struct {
	destHash []byte
	id       *identity.Identity
	appData  []byte
	hops     uint8
}

func newNomadnetAnnounceCollector() *nomadnetAnnounceCollector {
	return &nomadnetAnnounceCollector{
		announces: make(map[string]announcedNode),
	}
}

func (c *nomadnetAnnounceCollector) AspectFilter() []string {
	return []string{"nomadnetwork.node"}
}

func (c *nomadnetAnnounceCollector) ReceivePathResponses() bool {
	return true
}

func (c *nomadnetAnnounceCollector) ReceivedAnnounce(destHash []byte, ident any, appData []byte, hops uint8) error {
	id, ok := ident.(*identity.Identity)
	if !ok || id == nil {
		return nil
	}

	if len(destHash) != 16 {
		return nil
	}
	if !isNomadNetNodeDestination(id, destHash) {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	key := hex.EncodeToString(destHash)
	if _, exists := c.announces[key]; exists {
		return nil
	}

	c.announces[key] = announcedNode{
		destHash: append([]byte(nil), destHash...),
		id:       id,
		appData:  append([]byte(nil), appData...),
		hops:     hops,
	}
	return nil
}

func (c *nomadnetAnnounceCollector) snapshot() []announcedNode {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]announcedNode, 0, len(c.announces))
	for _, node := range c.announces {
		out = append(out, node)
	}
	return out
}

func isNomadNetNodeDestination(id *identity.Identity, destHash []byte) bool {
	nameHashFull := sha256.Sum256([]byte("nomadnetwork.node"))
	nameHash10 := nameHashFull[:10]
	identityHash := identity.TruncatedHash(id.GetPublicKey())
	combined := append(append([]byte(nil), nameHash10...), identityHash...)
	expectedFull := sha256.Sum256(combined)
	expected := expectedFull[:16]
	return string(expected) == string(destHash)
}

func envInt(key string, fallback int) int {
	if raw := strings.TrimSpace(os.Getenv(key)); raw != "" {
		v, err := strconv.Atoi(raw)
		if err == nil {
			return v
		}
	}
	return fallback
}

func envDurationSeconds(key string, fallback time.Duration) time.Duration {
	if raw := strings.TrimSpace(os.Getenv(key)); raw != "" {
		v, err := strconv.Atoi(raw)
		if err == nil && v > 0 {
			return time.Duration(v) * time.Second
		}
	}
	return fallback
}

func envPagePaths() []string {
	raw := strings.TrimSpace(os.Getenv("INTEROP_NOMADNET_PAGE_PATHS"))
	if raw == "" {
		return []string{"/page/index.mu", "/page/default.mu", "/page/home.mu"}
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return []string{"/page/index.mu", "/page/default.mu", "/page/home.mu"}
	}
	return out
}

func waitForRequestReceipt(receipt *rlink.RequestReceipt, timeout time.Duration) ([]byte, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if receipt.Concluded() {
			resp := receipt.GetResponse()
			if len(resp) == 0 {
				return nil, fmt.Errorf("request concluded without response")
			}
			return resp, nil
		}
		time.Sleep(80 * time.Millisecond)
	}
	return nil, context.DeadlineExceeded
}

// TestLiveNomadNetCrawlFetchMU listens for NomadNet node announces over TCP and fetches .mu pages.
// Required: RUN_LIVE_INTEROP=1.
// Optional env:
//   - INTEROP_NOMADNET_TCP_HOST (default public mesh host; see test)
//   - INTEROP_NOMADNET_TCP_PORT (default 7822)
//   - INTEROP_NOMADNET_TCP_NAME (default Beleth Clearnet TCP)
//   - INTEROP_NOMADNET_ANNOUNCE_WAIT_SEC (default 45)
//   - INTEROP_NOMADNET_NODE_TARGET (default 3)
//   - INTEROP_NOMADNET_PAGE_PATHS (comma-separated paths)
//   - INTEROP_NOMADNET_SAVE_DIR (if set, writes each received page body to
//     {dir}/{node16hex}_{path_slashes_as_underscores}.mu)
func TestLiveNomadNetCrawlFetchMU(t *testing.T) {
	liveOrSkip(t)

	tcpHost := strings.TrimSpace(os.Getenv("INTEROP_NOMADNET_TCP_HOST"))
	if tcpHost == "" {
		tcpHost = "rns.michmesh.net"
	}
	tcpPort := envInt("INTEROP_NOMADNET_TCP_PORT", 7822)
	tcpName := strings.TrimSpace(os.Getenv("INTEROP_NOMADNET_TCP_NAME"))
	if tcpName == "" {
		tcpName = "Beleth Clearnet TCP"
	}
	announceWait := envDurationSeconds("INTEROP_NOMADNET_ANNOUNCE_WAIT_SEC", 45*time.Second)
	nodeTarget := envInt("INTEROP_NOMADNET_NODE_TARGET", 3)
	pagePaths := envPagePaths()
	saveDir := strings.TrimSpace(os.Getenv("INTEROP_NOMADNET_SAVE_DIR"))

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()

	tr := transport.NewTransport(common.DefaultConfig())
	iface, err := interfaces.NewTCPClientInterface(tcpName, tcpHost, tcpPort, false, false, true)
	if err != nil {
		t.Fatalf("tcp interface connect: %v", err)
	}
	if err := tr.RegisterInterface(tcpName, iface); err != nil {
		t.Fatalf("register tcp interface: %v", err)
	}
	if err := tr.InitializePathRequestHandler(); err != nil {
		t.Fatalf("path handler: %v", err)
	}
	defer tr.Close()

	collector := newNomadnetAnnounceCollector()
	tr.RegisterAnnounceHandler(collector)
	defer tr.UnregisterAnnounceHandler(collector)

	deadline := time.Now().Add(announceWait)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			t.Fatalf("context while waiting for announces: %v", ctx.Err())
		default:
		}
		if len(collector.snapshot()) >= nodeTarget {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}

	nodes := collector.snapshot()
	if len(nodes) == 0 {
		t.Fatalf("no nomadnet announces observed within %s", announceWait)
	}

	if len(nodes) > nodeTarget {
		nodes = nodes[:nodeTarget]
	}
	t.Logf("crawl candidates=%d host=%s:%d", len(nodes), tcpHost, tcpPort)

	fetches := 0
	successes := 0
	for _, node := range nodes {
		nodeHashHex := hex.EncodeToString(node.destHash)
		if err := waitPath(ctx, tr, node.destHash, 35*time.Second); err != nil {
			t.Logf("skip %s: no path (%v)", nodeHashHex, err)
			continue
		}
		destOut, err := destination.FromHash(node.destHash, node.id, destination.Single, tr)
		if err != nil {
			t.Logf("skip %s: destination from hash failed (%v)", nodeHashHex, err)
			continue
		}

		established := make(chan struct{}, 1)
		lnk := rlink.NewLink(destOut, tr, nil, func(_ *rlink.Link) {
			select {
			case established <- struct{}{}:
			default:
			}
		}, nil)

		if err := lnk.Establish(); err != nil {
			t.Logf("skip %s: link establish failed (%v)", nodeHashHex, err)
			continue
		}
		select {
		case <-established:
		case <-time.After(45 * time.Second):
			t.Logf("skip %s: link establish timeout", nodeHashHex)
			lnk.Teardown()
			continue
		}
		lnk.Start()

		for _, path := range pagePaths {
			fetches++
			receipt, err := lnk.Request(path, []byte("crawl"), 20*time.Second)
			if err != nil {
				t.Logf("node=%s path=%s request err=%v", nodeHashHex, path, err)
				continue
			}
			resp, err := waitForRequestReceipt(receipt, 22*time.Second)
			if err != nil {
				t.Logf("node=%s path=%s receipt timeout=%v", nodeHashHex, path, err)
				continue
			}
			if saveDir != "" && len(resp) > 0 {
				if err := saveNomadnetPageFile(saveDir, nodeHashHex, path, resp); err != nil {
					t.Logf("save page: %v", err)
				} else {
					t.Logf("saved %s", nomadnetSavedName(nodeHashHex, path))
				}
			}
			if bytesContains(resp, ".mu") || bytesContains(resp, "Nomad") || bytesContains(resp, "<html") {
				successes++
				t.Logf("node=%s path=%s ok bytes=%d", nodeHashHex, path, len(resp))
			} else {
				// Accept non-empty responses as success; page formats vary by node.
				successes++
				t.Logf("node=%s path=%s response bytes=%d", nodeHashHex, path, len(resp))
			}
		}

		lnk.Teardown()
	}

	if fetches == 0 {
		t.Fatalf("no page fetches attempted")
	}
	if successes == 0 {
		t.Fatalf("all %d page fetches failed", fetches)
	}
	t.Logf("nomadnet crawl result: %d/%d fetches successful", successes, fetches)
}

func bytesContains(b []byte, sub string) bool {
	return strings.Contains(string(b), sub)
}

func nomadnetSavedName(nodeHex, pagePath string) string {
	safe := strings.TrimPrefix(strings.TrimSpace(pagePath), "/")
	safe = strings.ReplaceAll(safe, "/", "_")
	if safe == "" {
		safe = "page"
	}
	return fmt.Sprintf("%s_%s.mu", nodeHex, safe)
}

func saveNomadnetPageFile(dir, nodeHex, pagePath string, body []byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	name := nomadnetSavedName(nodeHex, pagePath)
	return os.WriteFile(filepath.Join(dir, name), body, 0o644)
}
