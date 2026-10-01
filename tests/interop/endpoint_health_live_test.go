// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

// Live check for endpoint-health quarantine and announce reject-reason
// counters through the shared-instance RPC stats path. Set
// RUN_LIVE_INTEROP=1.

package interop

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/interfaces"
	"github.com/Quad4-Software/Reticulum-Go/pkg/node"
	"github.com/Quad4-Software/Reticulum-Go/pkg/rnsutil"
)

// TestLiveEndpointQuarantineAndAnnounceStats runs a Go shared instance with a
// UDP iface plus a TCP client against a dead endpoint, seeds the endpoint
// tracker near the quarantine threshold, then verifies the RPC stats surface
// the quarantine and the announce reject-reason counters.
func TestLiveEndpointQuarantineAndAnnounceStats(t *testing.T) {
	liveOrSkip(t)

	port := freeUDPPort(t)
	ctrlPort := port + 1
	ifacePort := freeUDPPort(t)
	deadPort := freeUDPPort(t)
	cfgDir := t.TempDir()
	config := strings.Join([]string{
		"[reticulum]",
		"enable_transport = false",
		"share_instance = yes",
		"shared_instance_port = " + strconv.Itoa(port),
		"instance_control_port = " + strconv.Itoa(ctrlPort),
		"shared_instance_type = tcp",
		"",
		"[interfaces]",
		"  [[live_udp]]",
		"    type = UDPInterface",
		"    enabled = yes",
		"    listen_ip = 127.0.0.1",
		"    listen_port = " + strconv.Itoa(ifacePort),
		"",
		"  [[dead_tcp]]",
		"    type = TCPClientInterface",
		"    enabled = yes",
		"    target_host = 127.0.0.1",
		"    target_port = " + strconv.Itoa(deadPort),
		"",
	}, "\n")
	cfgPath := filepath.Join(cfgDir, "config")
	if err := os.WriteFile(cfgPath, []byte(config), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	goCfg, err := rnsutil.LoadConfigDir(cfgDir)
	if err != nil {
		t.Fatalf("load config dir: %v", err)
	}
	n, err := node.New(goCfg)
	if err != nil {
		t.Fatalf("node new: %v", err)
	}
	if err := n.Start(); err != nil {
		t.Fatalf("node start: %v", err)
	}
	defer n.Stop()

	deadEp := fmt.Sprintf("127.0.0.1:%d", deadPort)
	// Seed the shared tracker just below the quarantine threshold so the next
	// real driver dial failure engages it.
	for i := 0; i < interfaces.EndpointQuarantineConsecutiveFails-1; i++ {
		interfaces.DefaultEndpointTracker.RecordFailure(deadEp)
	}

	// Wait for a real dial failure to push the endpoint into quarantine.
	deadline := time.Now().Add(15 * time.Second)
	var st interfaces.EndpointStatus
	for time.Now().Before(deadline) {
		st = interfaces.DefaultEndpointTracker.Status(deadEp)
		if st.Quarantined {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !st.Quarantined {
		t.Fatalf("endpoint %s never quarantined; status=%+v", deadEp, st)
	}
	if st.DialFailures < uint64(interfaces.EndpointQuarantineConsecutiveFails) {
		t.Fatalf("dial failures=%d want >=%d", st.DialFailures, interfaces.EndpointQuarantineConsecutiveFails)
	}

	// Feed a malformed announce through the UDP interface to exercise the
	// reject-reason counter path end to end.
	iface, err := n.Transport().GetInterface("live_udp")
	if err != nil {
		t.Fatalf("live_udp missing: %v", err)
	}
	_ = n.Transport().HandleAnnounce([]byte{0x00}, iface)

	// Poll the RPC stats until quarantine and announce counters surface.
	client, err := rnsutil.DialRPC(goCfg, nil)
	if err != nil {
		t.Fatalf("rpc dial: %v", err)
	}

	found := false
	deadline = time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		stats, err := client.GetInterfaceStats()
		if err != nil {
			time.Sleep(200 * time.Millisecond)
			continue
		}
		for _, is := range stats.Interfaces {
			switch is.Name {
			case "dead_tcp":
				if !is.EndpointQuarantined {
					t.Fatalf("dead_tcp not quarantined in stats: %+v", is)
				}
				if is.EndpointDialFailures < uint64(interfaces.EndpointQuarantineConsecutiveFails) {
					t.Fatalf("endpoint_dial_failures=%d", is.EndpointDialFailures)
				}
				if is.EndpointQuarantineS <= 0 {
					t.Fatalf("endpoint_quarantine_s=%f want >0", is.EndpointQuarantineS)
				}
			case "live_udp":
				if is.AnnounceMalformed == 0 {
					continue
				}
				found = true
			}
		}
		if found {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !found {
		t.Fatal("announce_malformed never surfaced for live_udp")
	}
}
