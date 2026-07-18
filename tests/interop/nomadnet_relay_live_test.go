// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// Live NomadNet link test through a Go transport relay. Python peers over UDP
// while the relay maintains TCP mesh uplinks. RUN_LIVE_INTEROP=1.

package interop

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/interfaces"
	"quad4/reticulum-go/pkg/transport"
	"quad4/reticulum-go/tests/interop/harness"
)

type directoryPeer struct {
	Name string
	Host string
	Port int
}

func defaultMeshPeers() []directoryPeer {
	return []directoryPeer{
		{Name: "Beleth", Host: "rns.beleth.net", Port: 4242},
		{Name: "MichMesh", Host: "rns.michmesh.net", Port: 7822},
		{Name: "Quortal", Host: "reticulum.qortal.link", Port: 4242},
		{Name: "StoppedCold", Host: "rns.stoppedcold.com", Port: 4242},
		{Name: "Sydney", Host: "sydney.reticulum.au", Port: 4242},
	}
}

func meshPeersFromEnv(t *testing.T) []directoryPeer {
	t.Helper()
	raw := strings.TrimSpace(os.Getenv("INTEROP_NOMADNET_MESH_PEERS"))
	if raw == "" {
		return defaultMeshPeers()
	}
	var peers []directoryPeer
	for part := range strings.SplitSeq(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		fields := strings.Split(part, ":")
		if len(fields) != 3 {
			t.Fatalf("invalid INTEROP_NOMADNET_MESH_PEERS entry %q, want name:host:port", part)
		}
		port, err := strconv.Atoi(strings.TrimSpace(fields[2]))
		if err != nil {
			t.Fatalf("invalid port in %q: %v", part, err)
		}
		peers = append(peers, directoryPeer{
			Name: strings.TrimSpace(fields[0]),
			Host: strings.TrimSpace(fields[1]),
			Port: port,
		})
	}
	if len(peers) == 0 {
		return defaultMeshPeers()
	}
	return peers
}

func setupGoTransportRelay(t *testing.T, pyListen, pyForward int, peers []directoryPeer) (*transport.Transport, func()) {
	t.Helper()
	cfg := &common.ReticulumConfig{EnableTransport: true}
	tr := transport.NewTransport(cfg)

	relayAddr := "127.0.0.1:" + strconv.Itoa(pyForward)
	targetAddr := "127.0.0.1:" + strconv.Itoa(pyListen)
	udpIface, err := interfaces.NewUDPInterface("relay_udp", relayAddr, targetAddr, true)
	if err != nil {
		t.Fatalf("udp relay iface: %v", err)
	}
	if err := tr.RegisterInterface("relay_udp", udpIface); err != nil {
		t.Fatalf("register relay_udp: %v", err)
	}
	if err := udpIface.Start(); err != nil {
		t.Fatalf("start relay_udp: %v", err)
	}

	var meshIfaces []common.NetworkInterface
	for _, peer := range peers {
		iface, err := interfaces.NewTCPClientInterface(peer.Name, peer.Host, peer.Port, false, false, true)
		if err != nil {
			t.Logf("skip mesh peer %s (%s:%d): %v", peer.Name, peer.Host, peer.Port, err)
			continue
		}
		if err := tr.RegisterInterface(peer.Name, iface); err != nil {
			t.Logf("skip mesh peer %s register: %v", peer.Name, err)
			continue
		}
		meshIfaces = append(meshIfaces, iface)
		t.Logf("dialing mesh peer %s (%s:%d)", peer.Name, peer.Host, peer.Port)
	}
	if len(meshIfaces) == 0 {
		t.Fatal("no mesh TCP peers registered")
	}

	onlineDeadline := time.Now().Add(45 * time.Second)
	online := 0
	for time.Now().Before(onlineDeadline) {
		online = 0
		for _, iface := range meshIfaces {
			if iface.IsOnline() {
				online++
			}
		}
		if online > 0 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if online == 0 {
		t.Fatal("no mesh TCP peers came online")
	}
	t.Logf("mesh TCP peers online=%d/%d", online, len(meshIfaces))

	if err := tr.InitializePathRequestHandler(); err != nil {
		t.Fatalf("path handler: %v", err)
	}

	return tr, func() { tr.Close() }
}

// TestLiveNomadNetLinkThroughGoRelay exercises the meshchatx-style topology:
// Python client over UDP to a Go transport relay with multiple TCP mesh uplinks.
//
// Required: RUN_LIVE_INTEROP=1
// Optional:
//   - INTEROP_NOMADNET_MESH_PEERS=name:host:port,... (default: 5 public nodes)
//   - INTEROP_NOMADNET_ANNOUNCE_WAIT_SEC (default 120)
//   - INTEROP_NOMADNET_LINK_TIMEOUT_SEC (default 120)
func TestLiveNomadNetLinkThroughGoRelay(t *testing.T) {
	liveOrSkip(t)

	peers := meshPeersFromEnv(t)
	preflightPeers := make([]harness.MeshPeer, 0, len(peers))
	for _, p := range peers {
		preflightPeers = append(preflightPeers, harness.MeshPeer{Name: p.Name, Host: p.Host, Port: p.Port})
	}
	if online, err := harness.MeshPreflight(preflightPeers, 5*time.Second); err != nil {
		t.Skipf("mesh preflight failed. no TCP peers reachable. %v", err)
	} else {
		t.Logf("mesh preflight online=%d", online)
	}

	sess := harness.Begin(t)
	announceWait := envDurationSeconds("INTEROP_NOMADNET_ANNOUNCE_WAIT_SEC", 120*time.Second)
	linkTimeout := envDurationSeconds("INTEROP_NOMADNET_LINK_TIMEOUT_SEC", 120*time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), announceWait+linkTimeout+2*time.Minute)
	defer cancel()

	pyListen := freeUDPPort(t)
	pyForward := freeUDPPort(t)

	sess.Emit("announce", harness.KindAnnounce, "waiting for nomadnet announces on Go relay")
	tr, cleanup := setupGoTransportRelay(t, pyListen, pyForward, peers)
	defer cleanup()

	collector := newNomadnetAnnounceCollector()
	tr.RegisterAnnounceHandler(collector)
	defer tr.UnregisterAnnounceHandler(collector)

	deadline := time.Now().Add(announceWait)
	minNodes := max(envInt("INTEROP_NOMADNET_NODE_TARGET", 3), 1)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			sess.Emit("fail", harness.KindAnnounce, "context while waiting for announces")
			t.Fatalf("context while waiting for announces: %v", ctx.Err())
		default:
		}
		if len(collector.snapshot()) >= minNodes {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	nodes := collector.snapshot()
	if len(nodes) == 0 {
		sess.Emit("fail", harness.KindAnnounce, "no nomadnet announces observed")
		t.Fatalf("no nomadnet announces observed on Go relay within %s", announceWait)
	}

	maxAttempts := max(envInt("INTEROP_NOMADNET_NODE_ATTEMPTS", 3), 1)
	if maxAttempts > len(nodes) {
		maxAttempts = len(nodes)
	}

	var lastErr error
	for i := 0; i < maxAttempts; i++ {
		nodeHash := nodes[i].destHash
		t.Logf("attempt %d/%d nomadnet node %x hops=%d", i+1, maxAttempts, nodeHash, nodes[i].hops)
		sess.Emit("node", harness.KindAnnounce, hex.EncodeToString(nodeHash))

		probeCtx, probeCancel := context.WithTimeout(ctx, linkTimeout+2*time.Minute)
		probe := harness.StartPython(t, harness.ProbeOpts{
			Ctx:          probeCtx,
			Script:       filepath.Join(scriptDir(t), "py", "nomadnet_relay_probe.py"),
			Events:       sess.Events,
			ArtifactsDir: sess.Dir,
			Env: []string{
				"INTEROP_LISTEN_PORT=" + strconv.Itoa(pyListen),
				"INTEROP_FORWARD_PORT=" + strconv.Itoa(pyForward),
				"INTEROP_NOMADNET_DEST_HASH=" + hex.EncodeToString(nodeHash),
				"INTEROP_NOMADNET_ANNOUNCE_WAIT_SEC=" + strconv.Itoa(int(announceWait.Seconds())),
				"INTEROP_NOMADNET_LINK_TIMEOUT_SEC=" + strconv.Itoa(int(linkTimeout.Seconds())),
			},
		})

		probe.WaitExact(t, probeCtx, "READY", 30*time.Second, harness.KindReady)

		ok := false
		deadline = time.Now().Add(linkTimeout + time.Minute)
	readLoop:
		for time.Now().Before(deadline) {
			select {
			case <-probeCtx.Done():
				lastErr = probeCtx.Err()
				break readLoop
			case err := <-probe.Done():
				if err != nil {
					lastErr = err
				} else {
					lastErr = fmt.Errorf("python probe exited before NOMADNET_LINK_OK")
				}
				break readLoop
			default:
			}
			line, err := probe.ReadLine(probeCtx, 5*time.Second)
			if err != nil {
				if probeCtx.Err() != nil {
					lastErr = probeCtx.Err()
					break readLoop
				}
				select {
				case err := <-probe.Done():
					if err != nil {
						lastErr = err
					} else {
						lastErr = fmt.Errorf("python probe exited before NOMADNET_LINK_OK")
					}
					break readLoop
				default:
				}
				continue
			}
			line = strings.TrimSpace(line)
			switch {
			case strings.HasPrefix(line, "NODE "):
				t.Logf("python selected nomadnet node %s", strings.TrimPrefix(line, "NODE "))
			case line == "PATH_OK":
				t.Log("python learned path to nomadnet node through Go relay")
				sess.Emit("path_ok", harness.KindPath, "")
			case line == "NOMADNET_LINK_OK":
				t.Log("python established nomadnet link and fetched a page through Go relay")
				sess.Emit("link_ok", harness.KindLink, "")
				ok = true
				probeCancel()
				return
			default:
				t.Logf("python: %s", line)
			}
		}
		probeCancel()
		probe.Kill(3 * time.Second)
		if ok {
			return
		}
		t.Logf("node %x failed (%v), trying next candidate", nodeHash, lastErr)
	}
	sess.Emit("fail", harness.KindTimeout, "timed out waiting for NOMADNET_LINK_OK")
	if lastErr != nil {
		t.Fatalf("timed out waiting for NOMADNET_LINK_OK after %d nodes: %v", maxAttempts, lastErr)
	}
	t.Fatal("timed out waiting for NOMADNET_LINK_OK from python probe")
}

// TestDirectoryMeshPeersOnline is a lightweight sanity check that the default
// directory peers used by the relay test are reachable over TCP.
func TestDirectoryMeshPeersOnline(t *testing.T) {
	if os.Getenv("RUN_LIVE_INTEROP") == "" {
		t.Skip("set RUN_LIVE_INTEROP=1 to query directory and probe mesh peers")
	}

	resp, err := http.Get("https://directory.rns.recipes/api/directory/submitted?search=&type=&status=online")
	if err != nil {
		t.Fatalf("directory fetch: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("directory status = %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read directory body: %v", err)
	}

	var parsed struct {
		Data []struct {
			Name   string `json:"name"`
			Host   string `json:"host"`
			Port   int    `json:"port"`
			Status string `json:"status"`
			Type   string `json:"type"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("decode directory json: %v", err)
	}
	if len(parsed.Data) == 0 {
		t.Fatal("directory returned no online peers")
	}

	onlineTCP := 0
	for _, entry := range parsed.Data {
		if entry.Type != "tcp" && entry.Type != "backbone" {
			continue
		}
		if strings.ToLower(entry.Status) != "online" {
			continue
		}
		onlineTCP++
	}
	t.Logf("directory reports %d online tcp/backbone peers", onlineTCP)
	if onlineTCP < 3 {
		t.Fatalf("expected at least 3 online tcp/backbone peers, got %d", onlineTCP)
	}
}
