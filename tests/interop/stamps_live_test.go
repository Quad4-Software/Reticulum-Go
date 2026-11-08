// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

// Live stamp interop: stamped announces + stamped link requests vs Python
// RNS 1.6.0. Set RUN_LIVE_INTEROP=1 and PYTHON_INTEROP=<venv python>.

package interop

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/debug"
	"github.com/Quad4-Software/Reticulum-Go/pkg/destination"
	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/Reticulum-Go/pkg/interfaces"
	rlink "github.com/Quad4-Software/Reticulum-Go/pkg/link"
	"github.com/Quad4-Software/Reticulum-Go/pkg/packet"
	"github.com/Quad4-Software/Reticulum-Go/pkg/transport"
)

func setupGoStampPeer(t *testing.T, stampsCfg common.StampsConfig, pyListen, pyForward int) (*transport.Transport, *interfaces.UDPInterface, func()) {
	t.Helper()
	tr := transport.NewTransport(&common.ReticulumConfig{Stamps: stampsCfg})
	addrGo := "127.0.0.1:" + strconv.Itoa(pyForward)
	targetGo := "127.0.0.1:" + strconv.Itoa(pyListen)
	iface, err := interfaces.NewUDPInterface("interop_udp", addrGo, targetGo, true)
	if err != nil {
		t.Fatalf("udp iface: %v", err)
	}
	if err := tr.RegisterInterface("interop_udp", iface); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := iface.Start(); err != nil {
		t.Fatalf("start iface: %v", err)
	}
	if err := tr.InitializePathRequestHandler(); err != nil {
		t.Fatalf("path handler: %v", err)
	}
	return tr, iface, func() { tr.Close() }
}

// pyStampPeer starts stamp_peer.py, reads "DEST <hex>" then "READY", and
// returns the peer dest hash plus a channel of all stdout lines.
func pyStampPeer(t *testing.T, ctx context.Context, mode string, pyListen, pyForward int, env map[string]string) ([]byte, <-chan string, *exec.Cmd) {
	t.Helper()
	cmd := exec.CommandContext(ctx, pythonExe(), "-u", pyScript(t, "stamp_peer.py"))
	cmd.Env = append(os.Environ(),
		"INTEROP_MODE="+mode,
		"INTEROP_LISTEN_PORT="+strconv.Itoa(pyListen),
		"INTEROP_FORWARD_PORT="+strconv.Itoa(pyForward),
	)
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	cmd.Stderr = os.Stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start python: %v", err)
	}
	lines := make(chan string, 64)
	br := bufio.NewReader(out)
	go func() {
		for {
			l, err := br.ReadString('\n')
			if err != nil {
				close(lines)
				return
			}
			lines <- strings.TrimSpace(l)
		}
	}()
	var destHash []byte
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		l, ok := <-lines
		if !ok {
			t.Fatal("python exited early")
		}
		if strings.HasPrefix(l, "DEST ") {
			destHash, _ = hex.DecodeString(strings.Fields(l)[1])
		}
		if l == "READY" {
			break
		}
	}
	if len(destHash) == 0 {
		t.Fatal("no DEST line")
	}
	return destHash, lines, cmd
}

// (a) Go client -> stamped Python server; sub-cases per destination_cost.
func TestLiveInteropGoLinkToStampedPython(t *testing.T) {
	liveOrSkip(t)
	for _, tc := range []struct {
		name      string
		cfg       common.StampsConfig
		wantStrip bool
	}{
		{"cost8", common.StampsConfig{DestinationCost: 8, DestinationCostSet: true}, true},
		{"default", common.StampsConfig{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
			defer cancel()
			pyListen := freeUDPPort(t)
			pyForward := freeUDPPort(t)
			tr, iface, cleanup := setupGoStampPeer(t, tc.cfg, pyListen, pyForward)
			defer cleanup()

			pyDest, lines, cmd := pyStampPeer(t, ctx, "server", pyListen, pyForward, map[string]string{
				"INTEROP_LINK_COST": "9", "DEST_VALUE": "10",
			})
			defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()

			var srvID *identity.Identity
			deadline := time.Now().Add(60 * time.Second)
			for time.Now().Before(deadline) {
				if id, err := identity.Recall(pyDest); err == nil && id != nil && tr.HasPath(pyDest) {
					srvID = id
					break
				}
				time.Sleep(100 * time.Millisecond)
			}
			if srvID == nil {
				t.Fatal("announce never received")
			}
			sv, svOK := identity.RecallStampValue(pyDest)
			lc, lcOK := identity.RecallLinkCost(pyDest)
			if tc.wantStrip {
				if !svOK || sv < 10 {
					t.Fatalf("stamp value %v ok=%v", sv, svOK)
				}
				if !lcOK || lc != 9 {
					t.Fatalf("lr cost %v ok=%v", lc, lcOK)
				}
			} else if lcOK {
				t.Fatalf("default-cost config should not record lr_cost, got %v", lc)
			}

			destOut, err := destination.FromHash(pyDest, srvID, destination.Single, tr)
			if err != nil {
				t.Fatalf("fromhash: %v", err)
			}
			estCh := make(chan *rlink.Link, 1)
			lnk := rlink.NewLink(destOut, tr, iface, func(l *rlink.Link) { estCh <- l }, nil)
			if err := lnk.Establish(); err != nil {
				t.Fatalf("establish: %v", err)
			}
			select {
			case l := <-estCh:
				if tc.wantStrip {
					if c, ok := l.StampCost(); !ok || c < 9 {
						t.Fatalf("initiator stamp cost %v ok=%v", c, ok)
					}
				}
			case <-time.After(90 * time.Second):
				t.Fatal("link never established")
			}
			// Python prints LINK <stamp_cost> on establish.
			select {
			case l := <-lines:
				if !strings.HasPrefix(l, "LINK") {
					t.Fatalf("unexpected line %q", l)
				}
				if tc.wantStrip {
					fields := strings.Fields(l)
					if len(fields) < 2 {
						t.Fatalf("bad LINK line %q", l)
					}
					if c, err := strconv.ParseFloat(fields[1], 64); err != nil || c < 9 {
						t.Fatalf("python stamp_cost %q want >=9", fields[1])
					}
				}
			case <-time.After(30 * time.Second):
				t.Fatal("no LINK line from python")
			}
			// Echo round-trip.
			got := make(chan []byte, 1)
			lnk.SetPacketCallback(func(d []byte, _ *packet.Packet) { got <- d })
			lnk.Start()
			if err := lnk.SendPacket([]byte("stamp-echo")); err != nil {
				t.Fatalf("send: %v", err)
			}
			select {
			case d := <-got:
				if !bytes.Equal(d, []byte("stamp-echo")) {
					t.Fatalf("echo %q", d)
				}
			case <-time.After(30 * time.Second):
				t.Fatal("no echo")
			}
		})
	}
}

// (b) Python client -> Go stamped server.
func TestLiveInteropPythonLinkToStampedGo(t *testing.T) {
	liveOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	pyListen := freeUDPPort(t)
	pyForward := freeUDPPort(t)
	tr, _, cleanup := setupGoStampPeer(t, common.StampsConfig{
		StampDestinations: true,
		DestinationValue:  10, DestinationValueSet: true,
	}, pyListen, pyForward)
	defer cleanup()

	idGo, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	destGo, err := destination.New(idGo, destination.In, destination.Single, interopApp, tr, interopAspect)
	if err != nil {
		t.Fatal(err)
	}
	destGo.AcceptsLinks(true)
	estCh := make(chan *rlink.Link, 1)
	destGo.SetLinkEstablishedCallback(func(v any) {
		if l, ok := v.(*rlink.Link); ok {
			estCh <- l
		}
	})
	destGo.SetValue(10)
	destGo.SetLinkCost(9)

	cmd := exec.CommandContext(ctx, pythonExe(), "-u", pyScript(t, "stamp_peer.py"))
	cmd.Env = append(os.Environ(),
		"INTEROP_MODE=client", "DEST_COST=8", "DEST_VALUE=10",
		"INTEROP_LISTEN_PORT="+strconv.Itoa(pyListen),
		"INTEROP_FORWARD_PORT="+strconv.Itoa(pyForward),
		"INTEROP_GO_DEST_HASH="+hex.EncodeToString(destGo.GetHash()),
		"PYLOG_STDOUT=1", "PYLOG_LEVEL=6",
	)
	cmd.Stderr = os.Stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	lines := make(chan string, 32)
	go func() {
		br := bufio.NewReader(out)
		for {
			l, err := br.ReadString('\n')
			if err != nil {
				close(lines)
				return
			}
			lines <- strings.TrimSpace(l)
		}
	}()
	// Give python a moment, then announce.
	time.Sleep(2 * time.Second)
	if err := destGo.Announce(false, nil, nil); err != nil {
		t.Fatalf("announce: %v", err)
	}
	var lrLine string
	var allLines []string
	deadline := time.Now().Add(150 * time.Second)
	for time.Now().Before(deadline) {
		l, ok := <-lines
		if !ok {
			break
		}
		allLines = append(allLines, l)
		if strings.HasPrefix(l, "LR ") {
			lrLine = l
		}
		if l == "OK" {
			break
		}
	}
	t.Logf("python output: %v", allLines)
	if !strings.HasPrefix(lrLine, "LR 9.0") && !strings.HasPrefix(lrLine, "LR 9 ") {
		t.Fatalf("python recall_link_cost line %q want lr_cost 9", lrLine)
	}
	select {
	case l := <-estCh:
		if c, ok := l.StampCost(); !ok || c < 9 {
			t.Fatalf("go responder stamp cost %v ok=%v", c, ok)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("no inbound link")
	}
}

// (c) Python stamped announce -> Go strips app_data correctly.
func TestLiveInteropPythonStampedAnnounceToGo(t *testing.T) {
	liveOrSkip(t)
	if v := os.Getenv("RNS_DEBUG_LEVEL"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			debug.SetDebugLevel(n)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	pyListen := freeUDPPort(t)
	pyForward := freeUDPPort(t)
	tr, _, cleanup := setupGoStampPeer(t, common.StampsConfig{
		DestinationCost: 8, DestinationCostSet: true,
	}, pyListen, pyForward)
	defer cleanup()

	var gotApp []byte
	got := make(chan []byte, 1)
	tr.RegisterAnnounceHandler(stampAnnounceSink{func(a []byte) { got <- a }})

	pyDest, _, cmd := pyStampPeer(t, ctx, "announce", pyListen, pyForward, map[string]string{"DEST_VALUE": "10", "INTEROP_LINK_COST": "9"})
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()

	select {
	case gotApp = <-got:
	case <-time.After(90 * time.Second):
		t.Fatal("announce never arrived")
	}
	if len(gotApp) != 0 {
		t.Fatalf("app_data %x, want stripped/empty", gotApp)
	}
	if sv, ok := identity.RecallStampValue(pyDest); !ok || sv < 10 {
		t.Fatalf("stamp value %v ok=%v", sv, ok)
	}
	if lc, ok := identity.RecallLinkCost(pyDest); !ok || lc != 9 {
		t.Fatalf("lr cost %v ok=%v", lc, ok)
	}
}

type stampAnnounceSink struct{ f func([]byte) }

func (s stampAnnounceSink) AspectFilter() []string     { return nil }
func (s stampAnnounceSink) ReceivePathResponses() bool { return true }
func (s stampAnnounceSink) ReceivedAnnounce(_ []byte, _ any, appData []byte, _ uint8) error {
	s.f(appData)
	return nil
}

// --- RNS 1.6.0 enforcement interop ---------------------------------------

// waitPyLine reads until a line matching prefix or channel close/timeout.
func waitPyLine(t *testing.T, lines <-chan string, prefix string, d time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		select {
		case l, ok := <-lines:
			if !ok {
				return ""
			}
			if strings.HasPrefix(l, prefix) {
				return l
			}
		case <-time.After(deadline.Sub(time.Now())):
			return ""
		}
	}
	return ""
}

// (1)+(2): enforcing Python accepts Go stamped announce, drops unstamped.
func TestLiveInteropEnforcedPythonAnnounceFilter(t *testing.T) {
	liveOrSkip(t)
	for _, tc := range []struct {
		name     string
		stamped  bool
		wantPath bool
	}{
		{"stamped_accepted", true, true},
		{"unstamped_dropped", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()
			pyListen := freeUDPPort(t)
			pyForward := freeUDPPort(t)
			cfg := common.StampsConfig{}
			if tc.stamped {
				cfg.StampDestinations = true
				cfg.DestinationValue = 10
				cfg.DestinationValueSet = true
			}
			tr, _, cleanup := setupGoStampPeer(t, cfg, pyListen, pyForward)
			defer cleanup()

			idGo, err := identity.New()
			if err != nil {
				t.Fatal(err)
			}
			destGo, err := destination.New(idGo, destination.In, destination.Single, interopApp, tr, interopAspect)
			if err != nil {
				t.Fatal(err)
			}
			if tc.stamped {
				destGo.SetValue(10)
			}

			_, lines, cmd := pyStampPeer(t, ctx, "sink", pyListen, pyForward, map[string]string{
				"INTEROP_ENFORCE":      "1",
				"INTEROP_WAIT":         "20",
				"INTEROP_GO_DEST_HASH": hex.EncodeToString(destGo.GetHash()),
			})
			defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()

			// Python sink monitors a Go dest hash; point it at ours.
			// (sink mode reads INTEROP_GO_DEST_HASH at start — set before launch;
			//  we rely on the stamped announce reaching it.)
			if err := destGo.Announce(false, nil, nil); err != nil {
				t.Fatalf("announce: %v", err)
			}
			// Re-announce a few times for robustness.
			go func() {
				for i := 0; i < 4; i++ {
					time.Sleep(3 * time.Second)
					_ = destGo.Announce(false, nil, nil)
				}
			}()

			l := waitPyLine(t, lines, "PATH", 25*time.Second)
			if tc.wantPath && l == "" {
				t.Fatal("enforcing python did not accept stamped announce")
			}
			if !tc.wantPath && l != "" {
				t.Fatalf("enforcing python accepted unstamped announce: %q", l)
			}
		})
	}
}

// (3): enforcing Python answers a stamped Go path request, ignores unstamped.
func TestLiveInteropEnforcedPythonPathRequest(t *testing.T) {
	liveOrSkip(t)
	for _, tc := range []struct {
		name     string
		stampPRs bool
		wantPath bool
	}{
		{"stamped_pr_answered", true, true},
		{"unstamped_pr_dropped", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()
			pyListen := freeUDPPort(t)
			pyForward := freeUDPPort(t)
			tr, _, cleanup := setupGoStampPeer(t, common.StampsConfig{
				StampPRs: tc.stampPRs,
				PRValue:  10, PRValueSet: true,
				PRStampTimeout: 30, PRStampTimeoutSet: true,
			}, pyListen, pyForward)
			defer cleanup()

			pyDest, lines, cmd := pyStampPeer(t, ctx, "server", pyListen, pyForward, map[string]string{
				"INTEROP_ENFORCE": "1", "DEST_VALUE": "10", "NO_ANNOUNCE": "1",
			})
			defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
			_ = lines

			if err := tr.RequestPath(pyDest, "interop_udp", nil, false); err != nil {
				t.Fatalf("request path: %v", err)
			}
			deadline := time.Now().Add(20 * time.Second)
			for time.Now().Before(deadline) {
				if tr.HasPath(pyDest) {
					break
				}
				time.Sleep(150 * time.Millisecond)
			}
			if tc.wantPath && !tr.HasPath(pyDest) {
				t.Fatal("stamped PR not answered by enforcing python")
			}
			if !tc.wantPath && tr.HasPath(pyDest) {
				t.Fatal("unstamped PR answered by enforcing python")
			}
		})
	}
}

// (4): enforcing Python link request stamps — Go stamped works, unstamped rejected.
func TestLiveInteropEnforcedPythonLinkRequest(t *testing.T) {
	liveOrSkip(t)
	for _, tc := range []struct {
		name       string
		stampLRs   bool
		wantActive bool
	}{
		{"stamped_lr_accepted", true, true},
		{"unstamped_lr_rejected", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()
			pyListen := freeUDPPort(t)
			pyForward := freeUDPPort(t)
			tr, iface, cleanup := setupGoStampPeer(t, common.StampsConfig{
				DestinationCost: 8, DestinationCostSet: true,
				StampLRs: tc.stampLRs, StampLRsSet: true,
			}, pyListen, pyForward)
			defer cleanup()

			pyDest, lines, cmd := pyStampPeer(t, ctx, "server", pyListen, pyForward, map[string]string{
				"INTEROP_ENFORCE": "1", "INTEROP_LINK_COST": "9", "DEST_VALUE": "10",
			})
			defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()

			var srvID *identity.Identity
			deadline := time.Now().Add(60 * time.Second)
			for time.Now().Before(deadline) {
				if id, err := identity.Recall(pyDest); err == nil && id != nil && tr.HasPath(pyDest) {
					srvID = id
					break
				}
				time.Sleep(100 * time.Millisecond)
			}
			if srvID == nil {
				t.Fatal("announce never received")
			}
			if lc, ok := identity.RecallLinkCost(pyDest); !ok || lc != 9 {
				t.Fatalf("lr cost %v ok=%v", lc, ok)
			}

			destOut, err := destination.FromHash(pyDest, srvID, destination.Single, tr)
			if err != nil {
				t.Fatal(err)
			}
			estCh := make(chan *rlink.Link, 1)
			lnk := rlink.NewLink(destOut, tr, iface, func(l *rlink.Link) { estCh <- l }, nil)
			if err := lnk.Establish(); err != nil {
				t.Fatalf("establish: %v", err)
			}
			if tc.wantActive {
				select {
				case <-estCh:
				case <-time.After(60 * time.Second):
					t.Fatal("stamped link never established")
				}
				if l := waitPyLine(t, lines, "LINK", 15*time.Second); l == "" {
					t.Fatal("python did not record stamped link")
				}
			} else {
				select {
				case <-estCh:
					t.Fatal("unstamped link request was accepted by enforcing python")
				case <-time.After(10 * time.Second):
				}
				if l := waitPyLine(t, lines, "LINK", 5*time.Second); l != "" {
					t.Fatalf("enforcing python accepted unstamped link request: %q", l)
				}
			}
		})
	}
}

// (5): Go enforcing — unstamped Python announce dropped, stamped accepted,
// unstamped Python path request ignored.
func TestLiveInteropGoEnforcedAnnounceAndPR(t *testing.T) {
	liveOrSkip(t)
	for _, tc := range []struct {
		name     string
		stamped  bool
		wantPath bool
	}{
		{"stamped_accepted", true, true},
		{"unstamped_dropped", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()
			pyListen := freeUDPPort(t)
			pyForward := freeUDPPort(t)
			_, _, cleanup := setupGoStampPeer(t, common.StampsConfig{
				EnforceStamps:   true,
				DestinationCost: 8, DestinationCostSet: true,
				PRCost: 8, PRCostSet: true,
			}, pyListen, pyForward)
			defer cleanup()

			env := map[string]string{
				"DEST_VALUE":      "10",
				"INTEROP_ENFORCE": "",
			}
			if !tc.stamped {
				env["STAMP_DESTS"] = "no"
			}
			pyDest, _, cmd := pyStampPeer(t, ctx, "announce", pyListen, pyForward, env)
			defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()

			deadline := time.Now().Add(30 * time.Second)
			got := false
			for time.Now().Before(deadline) {
				if id, err := identity.Recall(pyDest); err == nil && id != nil {
					got = true
					break
				}
				time.Sleep(150 * time.Millisecond)
			}
			if tc.wantPath && !got {
				t.Fatal("go enforcing dropped stamped python announce")
			}
			if !tc.wantPath && got {
				t.Fatal("go enforcing accepted unstamped python announce")
			}
		})
	}
}

// (5b): Go enforcing ignores an unstamped Python path request.
func TestLiveInteropGoEnforcedPathRequest(t *testing.T) {
	liveOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pyListen := freeUDPPort(t)
	pyForward := freeUDPPort(t)
	tr, _, cleanup := setupGoStampPeer(t, common.StampsConfig{
		EnforceStamps:   true,
		DestinationCost: 8, DestinationCostSet: true,
		PRCost: 8, PRCostSet: true,
	}, pyListen, pyForward)
	defer cleanup()

	// Go destination the Python requestor will ask for.
	idGo, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	destGo, err := destination.New(idGo, destination.In, destination.Single, interopApp, tr, interopAspect)
	if err != nil {
		t.Fatal(err)
	}
	_ = destGo

	_, lines, cmd := pyStampPeer(t, ctx, "requestor", pyListen, pyForward, map[string]string{
		"INTEROP_WAIT":         "12",
		"STAMP_PRS":            "no",
		"INTEROP_GO_DEST_HASH": hex.EncodeToString(destGo.GetHash()),
	})
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()

	if l := waitPyLine(t, lines, "PATH", 20*time.Second); l != "" {
		t.Fatal("unstamped python path request answered by enforcing go")
	}
}
