// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package interop

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/destination"
	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/Reticulum-Go/pkg/interfaces"
	rlink "github.com/Quad4-Software/Reticulum-Go/pkg/link"
	"github.com/Quad4-Software/Reticulum-Go/pkg/transport"
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
	sort.Slice(out, func(i, j int) bool {
		if out[i].hops != out[j].hops {
			return out[i].hops < out[j].hops
		}
		return bytes.Compare(out[i].destHash, out[j].destHash) < 0
	})
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
//   - INTEROP_NOMADNET_TCP_HOST/PORT/NAME pin a single uplink; unset, the test
//     walks the same public peer list as the relay test until one forwards
//     announces
//   - INTEROP_NOMADNET_ANNOUNCE_WAIT_SEC (default 45, per uplink)
//   - INTEROP_NOMADNET_NODE_TARGET (default 3)
//   - INTEROP_NOMADNET_PAGE_PATHS (comma-separated paths)
//   - INTEROP_NOMADNET_SAVE_DIR (if set, writes each received page body to
//     {dir}/{node16hex}_{path_slashes_as_underscores}.mu)
func TestLiveNomadNetCrawlFetchMU(t *testing.T) {
	liveOrSkip(t)

	var peers []directoryPeer
	if tcpHost := strings.TrimSpace(os.Getenv("INTEROP_NOMADNET_TCP_HOST")); tcpHost != "" {
		tcpName := strings.TrimSpace(os.Getenv("INTEROP_NOMADNET_TCP_NAME"))
		if tcpName == "" {
			tcpName = "custom"
		}
		peers = []directoryPeer{{Name: tcpName, Host: tcpHost, Port: envInt("INTEROP_NOMADNET_TCP_PORT", 7822)}}
	} else {
		peers = meshPeersFromEnv(t)
	}
	announceWait := envDurationSeconds("INTEROP_NOMADNET_ANNOUNCE_WAIT_SEC", 45*time.Second)
	nodeTarget := envInt("INTEROP_NOMADNET_NODE_TARGET", 3)
	// Collect a wider candidate pool than the fetch target: public nodes
	// announce freely but many never answer page requests or even link, so
	// the crawl needs spare candidates before it gives up on an uplink.
	poolTarget := nodeTarget * 4
	if poolTarget < 12 {
		poolTarget = 12
	}
	pagePaths := envPagePaths()
	saveDir := strings.TrimSpace(os.Getenv("INTEROP_NOMADNET_SAVE_DIR"))

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	var tr *transport.Transport
	var collector *nomadnetAnnounceCollector
	var nodes []announcedNode
	var usedPeer directoryPeer
	// Walk every uplink and keep the richest announce pool. An uplink that
	// forwards only a trickle leaves too few candidates to survive
	// link-establish failures, so the first uplink that answers is not
	// automatically the crawl source.
	for _, peer := range peers {
		t.Logf("uplink %s %s:%d: waiting up to %s for announces", peer.Name, peer.Host, peer.Port, announceWait)
		tr2 := transport.NewTransport(common.DefaultConfig())
		iface, err := interfaces.NewTCPClientInterface(peer.Name, peer.Host, peer.Port, false, false, true)
		if err != nil {
			t.Logf("uplink %s connect failed: %v", peer.Name, err)
			tr2.Close()
			continue
		}
		if err := tr2.RegisterInterface(peer.Name, iface); err != nil {
			tr2.Close()
			t.Fatalf("register tcp interface: %v", err)
		}
		if err := tr2.InitializePathRequestHandler(); err != nil {
			tr2.Close()
			t.Fatalf("path handler: %v", err)
		}
		col := newNomadnetAnnounceCollector()
		tr2.RegisterAnnounceHandler(col)

		deadline := time.Now().Add(announceWait)
		for time.Now().Before(deadline) {
			if ctx.Err() != nil {
				break
			}
			if len(col.snapshot()) >= poolTarget {
				break
			}
			time.Sleep(250 * time.Millisecond)
		}
		got := col.snapshot()
		if len(got) == 0 {
			t.Logf("uplink %s delivered no announces in %s", peer.Name, announceWait)
			tr2.Close()
			continue
		}
		t.Logf("uplink %s delivered %d announce(s)", peer.Name, len(got))
		if len(got) > len(nodes) {
			if tr != nil {
				tr.Close()
			}
			tr = tr2
			collector = col
			nodes = got
			usedPeer = peer
		} else {
			tr2.Close()
		}
		if len(nodes) >= nodeTarget {
			break
		}
	}
	if tr == nil {
		t.Fatalf("no nomadnet announces observed via any of %d uplink(s), %s each", len(peers), announceWait)
	}
	defer tr.Close()
	defer tr.UnregisterAnnounceHandler(collector)

	if len(nodes) > poolTarget {
		nodes = nodes[:poolTarget]
	}
	t.Logf("crawl candidates=%d host=%s:%d", len(nodes), usedPeer.Host, usedPeer.Port)

	fetches := 0
	successes := 0
	for _, node := range nodes {
		if successes > 0 {
			break
		}
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
				// Accept non-empty responses as success. Page formats vary by node.

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
