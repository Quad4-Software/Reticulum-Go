// SPDX-License-Identifier: LicenseRef-Reticulum
package integration_test

import (
	"bufio"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/Quad4-Software/reticulum-go-protocols/internal/testconfig"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/destination"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/interfaces"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
	"github.com/Quad4-Software/reticulum-go-protocols/internal/lxsttest"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/call"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/proto"
)

type peerEvent struct {
	Event       string `json:"event"`
	Identity    string `json:"identity"`
	Destination string `json:"destination"`
	Error       string `json:"error"`
	N           int    `json:"n"`
	Frames      int    `json:"frames"`
}

func TestPythonLXSTGoDialsPython(t *testing.T) {
	lxsttest.SkipLive(t)
	py := lxsttest.Python(t)
	goPort := freeUDP(t)
	pyPort := freeUDP(t)
	cfgDir := t.TempDir()
	peer := filepath.Join(lxsttest.RepoRoot(t), "testdata", "lxst", "lxst_peer.py")
	cmd := exec.Command(py, peer,
		"--configdir", cfgDir,
		"--listen-port", fmt.Sprintf("%d", pyPort),
		"--target-port", fmt.Sprintf("%d", goPort),
		"--mode", "listen",
		"--auto-answer", "0.3",
		"--frames", "10",
		"--name", "py_listen",
	)
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	events := bufio.NewScanner(stdout)
	ready := waitEvent(t, events, "ready", 10*time.Second)
	if ready.Identity == "" {
		t.Fatal("python peer missing identity")
	}

	tr, id := startGoUDP(t, goPort, pyPort)
	remoteHash, err := hex.DecodeString(ready.Identity)
	if err != nil {
		t.Fatal(err)
	}
	destHash := proto.TelephonyHash(remoteHash)
	if err := waitPath(tr, destHash, 15*time.Second); err != nil {
		t.Fatal(err)
	}
	remote, err := identity.Recall(destHash)
	if err != nil {
		t.Fatal(err)
	}

	established := make(chan struct{}, 1)
	var frames uint64
	caller := call.NewCall(tr, call.Config{
		Identity:    id,
		UseAudio:    false,
		ConnectTime: 15 * time.Second,
		WaitTime:    20 * time.Second,
		Events: call.Events{
			OnAnswered: func(*call.Call) { established <- struct{}{} },
			OnFrame: func(pcm []int16) {
				if len(pcm) > 0 {
					frames++
				}
			},
		},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	if err := caller.Dial(ctx, remote); err != nil {
		t.Fatalf("dial python: %v", err)
	}
	select {
	case <-established:
	case <-time.After(10 * time.Second):
		t.Fatal("python did not establish")
	}
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) && caller.RecvFrames() == 0 {
		time.Sleep(50 * time.Millisecond)
	}
	got := caller.RecvFrames()
	_ = caller.Hangup("done")
	if got == 0 {
		t.Fatal("go received no opus frames from python LXST")
	}
}

func TestPythonLXSTPythonDialsGo(t *testing.T) {
	lxsttest.SkipLive(t)
	py := lxsttest.Python(t)
	goPort := freeUDP(t)
	pyPort := freeUDP(t)
	tr, id := startGoUDP(t, goPort, pyPort)
	dest, err := destination.New(id, destination.In, destination.Single, proto.AppName, tr, proto.AspectName)
	if err != nil {
		t.Fatal(err)
	}
	dest.AcceptsLinks(true)

	established := make(chan struct{}, 1)
	var recv uint64
	sb := call.NewSwitchboard(tr, call.Config{
		Identity: id,
		UseAudio: false,
		Events: call.Events{
			OnRinging: func(c *call.Call) {
				if !c.Incoming() {
					return
				}
				go func() {
					time.Sleep(200 * time.Millisecond)
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					_ = c.Answer(ctx)
				}()
			},
			OnAnswered: func(*call.Call) { established <- struct{}{} },
			OnFrame: func(pcm []int16) {
				if len(pcm) > 0 {
					recv++
				}
			},
		},
	}, nil)
	sb.Bind(dest)

	cfgDir := t.TempDir()
	peer := filepath.Join(lxsttest.RepoRoot(t), "testdata", "lxst", "lxst_peer.py")
	cmd := exec.Command(py, peer,
		"--configdir", cfgDir,
		"--listen-port", fmt.Sprintf("%d", pyPort),
		"--target-port", fmt.Sprintf("%d", goPort),
		"--mode", "dial",
		"--dial", fmt.Sprintf("%x", id.Hash()),
		"--frames", "10",
		"--name", "py_dial",
	)
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	events := bufio.NewScanner(stdout)
	_ = waitEvent(t, events, "ready", 10*time.Second)
	for range 8 {
		_ = dest.Announce(false, nil, nil)
		time.Sleep(150 * time.Millisecond)
	}
	go func() {
		for events.Scan() {
			t.Logf("python: %s", events.Text())
		}
	}()
	select {
	case <-established:
	case <-time.After(20 * time.Second):
		t.Fatal("go did not receive established call from python")
	}
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if c := sb.Active(); c != nil && c.RecvFrames() > 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("go received no opus frames from python caller")
}

func TestPythonTelephoneGoDialsPython(t *testing.T) {
	lxsttest.SkipLive(t)
	py := lxsttest.Python(t)
	goPort := freeUDP(t)
	pyPort := freeUDP(t)
	cfgDir := t.TempDir()
	peer := filepath.Join(lxsttest.RepoRoot(t), "testdata", "lxst", "lxst_telephone.py")
	cmd := exec.Command(py, peer,
		"--configdir", cfgDir,
		"--listen-port", fmt.Sprintf("%d", pyPort),
		"--target-port", fmt.Sprintf("%d", goPort),
		"--mode", "listen",
		"--auto-answer", "0.3",
		"--name", "tel_listen",
	)
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	events := bufio.NewScanner(stdout)
	ready := waitEvent(t, events, "ready", 15*time.Second)
	go func() {
		for events.Scan() {
			t.Logf("python: %s", events.Text())
		}
	}()

	tr, id := startGoUDP(t, goPort, pyPort)
	remoteHash, err := hex.DecodeString(ready.Identity)
	if err != nil {
		t.Fatal(err)
	}
	destHash := proto.TelephonyHash(remoteHash)
	if err := waitPath(tr, destHash, 15*time.Second); err != nil {
		t.Fatal(err)
	}
	remote, err := identity.Recall(destHash)
	if err != nil {
		t.Fatal(err)
	}

	established := make(chan struct{}, 1)
	caller := call.NewCall(tr, call.Config{
		Identity:    id,
		UseAudio:    false,
		ConnectTime: 15 * time.Second,
		WaitTime:    25 * time.Second,
		Events: call.Events{
			OnAnswered: func(*call.Call) { established <- struct{}{} },
		},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := caller.Dial(ctx, remote); err != nil {
		t.Fatalf("dial python telephone: %v", err)
	}
	select {
	case <-established:
	case <-time.After(10 * time.Second):
		t.Fatal("python telephone did not establish")
	}
	_ = caller.Hangup("done")
}

func TestPythonTelephonePythonDialsGo(t *testing.T) {
	lxsttest.SkipLive(t)
	py := lxsttest.Python(t)
	goPort := freeUDP(t)
	pyPort := freeUDP(t)
	tr, id := startGoUDP(t, goPort, pyPort)
	dest, err := destination.New(id, destination.In, destination.Single, proto.AppName, tr, proto.AspectName)
	if err != nil {
		t.Fatal(err)
	}
	dest.AcceptsLinks(true)

	established := make(chan struct{}, 1)
	sb := call.NewSwitchboard(tr, call.Config{
		Identity: id,
		UseAudio: false,
		Events: call.Events{
			OnRinging: func(c *call.Call) {
				if !c.Incoming() {
					return
				}
				go func() {
					time.Sleep(200 * time.Millisecond)
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					_ = c.Answer(ctx)
				}()
			},
			OnAnswered: func(*call.Call) { established <- struct{}{} },
		},
	}, nil)
	sb.Bind(dest)

	cfgDir := t.TempDir()
	peer := filepath.Join(lxsttest.RepoRoot(t), "testdata", "lxst", "lxst_telephone.py")
	cmd := exec.Command(py, peer,
		"--configdir", cfgDir,
		"--listen-port", fmt.Sprintf("%d", pyPort),
		"--target-port", fmt.Sprintf("%d", goPort),
		"--mode", "dial",
		"--dial", fmt.Sprintf("%x", id.Hash()),
		"--name", "tel_dial",
	)
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	events := bufio.NewScanner(stdout)
	_ = waitEvent(t, events, "ready", 15*time.Second)
	for range 8 {
		_ = dest.Announce(false, nil, nil)
		time.Sleep(150 * time.Millisecond)
	}
	go func() {
		for events.Scan() {
			t.Logf("python: %s", events.Text())
		}
	}()
	select {
	case <-established:
	case <-time.After(25 * time.Second):
		t.Fatal("go did not receive established call from python telephone")
	}
	if c := sb.Active(); c != nil {
		_ = c.Hangup("done")
	}
}

func startGoUDP(t *testing.T, listenPort, targetPort int) (*transport.Transport, *identity.Identity) {
	t.Helper()
	cfg := testconfig.New()
	cfg.ShareInstance = false
	cfg.InMemoryPathTable = true
	cfg.InMemoryKnownDestinations = true
	cfg.ConfigPath = t.TempDir() + "/config"
	tr := transport.NewTransport(cfg)
	if err := tr.Start(); err != nil {
		t.Fatal(err)
	}
	addr := fmt.Sprintf("127.0.0.1:%d", listenPort)
	target := fmt.Sprintf("127.0.0.1:%d", targetPort)
	iface, err := interfaces.NewUDPInterface("UDP", addr, target, true)
	if err != nil {
		t.Fatal(err)
	}
	iface.In = true
	iface.Out = true
	if err := tr.RegisterInterface("UDP", iface); err != nil {
		t.Fatal(err)
	}
	if err := iface.Start(); err != nil {
		t.Fatal(err)
	}
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	return tr, id
}

func waitPath(tr *transport.Transport, hash []byte, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	_ = tr.RequestPath(hash, "", nil, false)
	for time.Now().Before(deadline) {
		if tr.HasPath(hash) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
		_ = tr.RequestPath(hash, "", nil, false)
	}
	return fmt.Errorf("no path to %x", hash)
}

func waitEvent(t *testing.T, sc *bufio.Scanner, name string, timeout time.Duration) peerEvent {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !sc.Scan() {
			if err := sc.Err(); err != nil {
				t.Fatal(err)
			}
			t.Fatalf("python stdout closed waiting for %s", name)
		}
		var ev peerEvent
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
			continue
		}
		if ev.Event == "error" {
			t.Fatalf("python error: %s", ev.Error)
		}
		if ev.Event == name {
			return ev
		}
	}
	t.Fatalf("timeout waiting for python event %s", name)
	return peerEvent{}
}

func freeUDP(t *testing.T) int {
	t.Helper()
	c, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	return c.LocalAddr().(*net.UDPAddr).Port
}

func TestPythonCodec2GoDialsPython(t *testing.T) {
	lxsttest.SkipLive(t)
	py := lxsttest.Python(t)
	goPort := freeUDP(t)
	pyPort := freeUDP(t)
	cfgDir := t.TempDir()
	peer := filepath.Join(lxsttest.RepoRoot(t), "testdata", "lxst", "lxst_peer.py")
	cmd := exec.Command(py, peer,
		"--configdir", cfgDir,
		"--listen-port", fmt.Sprintf("%d", pyPort),
		"--target-port", fmt.Sprintf("%d", goPort),
		"--mode", "listen",
		"--auto-answer", "0.3",
		"--profile", "lbw",
		"--frames", "8",
		"--name", "py_c2",
	)
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()
	events := bufio.NewScanner(stdout)
	ready := waitEvent(t, events, "ready", 15*time.Second)
	errCh := make(chan string, 8)
	go func() {
		for events.Scan() {
			var ev peerEvent
			if err := json.Unmarshal(events.Bytes(), &ev); err != nil {
				continue
			}
			if ev.Event == "error" {
				errCh <- ev.Error
				return
			}
			t.Logf("python: %s", events.Text())
		}
	}()
	tr, id := startGoUDP(t, goPort, pyPort)
	remoteHash, err := hex.DecodeString(ready.Identity)
	if err != nil {
		t.Fatal(err)
	}
	destHash := proto.TelephonyHash(remoteHash)
	if err := waitPath(tr, destHash, 15*time.Second); err != nil {
		t.Fatal(err)
	}
	remote, err := identity.Recall(destHash)
	if err != nil {
		t.Fatal(err)
	}
	established := make(chan struct{}, 1)
	var recvFrames uint64
	caller := call.NewCall(tr, call.Config{
		Identity:    id,
		UseAudio:    false,
		Profile:     proto.ProfileBandwidthLow,
		ConnectTime: 15 * time.Second,
		WaitTime:    25 * time.Second,
		Events: call.Events{
			OnAnswered: func(*call.Call) { established <- struct{}{} },
			OnFrame: func(pcm []int16) {
				if len(pcm) > 0 {
					recvFrames++
				}
			},
		},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := caller.Dial(ctx, remote); err != nil {
		t.Fatalf("dial python codec2: %v", err)
	}
	select {
	case <-established:
	case <-time.After(10 * time.Second):
		t.Fatal("python codec2 call did not establish")
	}
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) && caller.SentFrames() == 0 {
		time.Sleep(50 * time.Millisecond)
	}
	sent := caller.SentFrames()
	recv := caller.RecvFrames()
	if recv == 0 {
		recv = recvFrames
	}
	_ = caller.Hangup("done")
	select {
	case msg := <-errCh:
		t.Fatalf("python codec2 peer error: %s", msg)
	default:
	}
	if sent == 0 {
		t.Fatal("go sent no codec2 frames to python LXST")
	}
	if recv == 0 {
		t.Fatal("go received no codec2 frames from python LXST")
	}
}

func TestPythonCodec2PythonDialsGo(t *testing.T) {
	lxsttest.SkipLive(t)
	py := lxsttest.Python(t)
	goPort := freeUDP(t)
	pyPort := freeUDP(t)
	tr, id := startGoUDP(t, goPort, pyPort)
	dest, err := destination.New(id, destination.In, destination.Single, proto.AppName, tr, proto.AspectName)
	if err != nil {
		t.Fatal(err)
	}
	dest.AcceptsLinks(true)

	established := make(chan struct{}, 1)
	var recv uint64
	sb := call.NewSwitchboard(tr, call.Config{
		Identity: id,
		UseAudio: false,
		Profile:  proto.ProfileBandwidthLow,
		Events: call.Events{
			OnRinging: func(c *call.Call) {
				if !c.Incoming() {
					return
				}
				go func() {
					time.Sleep(200 * time.Millisecond)
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					_ = c.Answer(ctx)
				}()
			},
			OnAnswered: func(*call.Call) { established <- struct{}{} },
			OnFrame: func(pcm []int16) {
				if len(pcm) > 0 {
					recv++
				}
			},
		},
	}, nil)
	sb.Bind(dest)

	cfgDir := t.TempDir()
	peer := filepath.Join(lxsttest.RepoRoot(t), "testdata", "lxst", "lxst_peer.py")
	cmd := exec.Command(py, peer,
		"--configdir", cfgDir,
		"--listen-port", fmt.Sprintf("%d", pyPort),
		"--target-port", fmt.Sprintf("%d", goPort),
		"--mode", "dial",
		"--dial", fmt.Sprintf("%x", id.Hash()),
		"--profile", "lbw",
		"--frames", "10",
		"--name", "py_c2_dial",
	)
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	events := bufio.NewScanner(stdout)
	_ = waitEvent(t, events, "ready", 15*time.Second)
	for range 8 {
		_ = dest.Announce(false, nil, nil)
		time.Sleep(150 * time.Millisecond)
	}
	go func() {
		for events.Scan() {
			var ev peerEvent
			if err := json.Unmarshal(events.Bytes(), &ev); err != nil {
				continue
			}
			if ev.Event == "error" {
				t.Errorf("python codec2 dial error: %s", ev.Error)
				return
			}
			t.Logf("python: %s", events.Text())
		}
	}()
	select {
	case <-established:
	case <-time.After(20 * time.Second):
		t.Fatal("go did not receive established codec2 call from python")
	}
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) && recv == 0 {
		time.Sleep(50 * time.Millisecond)
	}
	if c := sb.Active(); c != nil {
		sent := c.SentFrames()
		_ = c.Hangup("done")
		if sent == 0 {
			t.Fatal("go sent no codec2 frames to python caller")
		}
	}
	if recv == 0 {
		t.Fatal("go received no codec2 frames from python caller")
	}
}

func TestPythonBusyAllowNone(t *testing.T) {
	lxsttest.SkipLive(t)
	py := lxsttest.Python(t)
	goPort := freeUDP(t)
	pyPort := freeUDP(t)
	cfgDir := t.TempDir()
	peer := filepath.Join(lxsttest.RepoRoot(t), "testdata", "lxst", "lxst_telephone.py")
	cmd := exec.Command(py, peer,
		"--configdir", cfgDir,
		"--listen-port", fmt.Sprintf("%d", pyPort),
		"--target-port", fmt.Sprintf("%d", goPort),
		"--mode", "listen",
		"--allowed", "none",
		"--name", "tel_none",
	)
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()
	events := bufio.NewScanner(stdout)
	ready := waitEvent(t, events, "ready", 15*time.Second)
	go func() {
		for events.Scan() {
			t.Logf("python: %s", events.Text())
		}
	}()
	tr, id := startGoUDP(t, goPort, pyPort)
	remoteHash, err := hex.DecodeString(ready.Identity)
	if err != nil {
		t.Fatal(err)
	}
	destHash := proto.TelephonyHash(remoteHash)
	if err := waitPath(tr, destHash, 15*time.Second); err != nil {
		t.Fatal(err)
	}
	remote, err := identity.Recall(destHash)
	if err != nil {
		t.Fatal(err)
	}
	busy := make(chan struct{}, 1)
	caller := call.NewCall(tr, call.Config{
		Identity:    id,
		UseAudio:    false,
		ConnectTime: 12 * time.Second,
		WaitTime:    15 * time.Second,
		Events: call.Events{
			OnBusy: func(*call.Call) { busy <- struct{}{} },
		},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	err = caller.Dial(ctx, remote)
	select {
	case <-busy:
	case <-time.After(12 * time.Second):
		t.Fatalf("expected busy from python ALLOW_NONE, dial err=%v", err)
	}
}

func TestPythonRejectIncoming(t *testing.T) {
	lxsttest.SkipLive(t)
	py := lxsttest.Python(t)
	goPort := freeUDP(t)
	pyPort := freeUDP(t)
	cfgDir := t.TempDir()
	peer := filepath.Join(lxsttest.RepoRoot(t), "testdata", "lxst", "lxst_telephone.py")
	cmd := exec.Command(py, peer,
		"--configdir", cfgDir,
		"--listen-port", fmt.Sprintf("%d", pyPort),
		"--target-port", fmt.Sprintf("%d", goPort),
		"--mode", "listen",
		"--reject",
		"--name", "tel_rej",
	)
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()
	events := bufio.NewScanner(stdout)
	ready := waitEvent(t, events, "ready", 15*time.Second)
	go func() {
		for events.Scan() {
			t.Logf("python: %s", events.Text())
		}
	}()
	tr, id := startGoUDP(t, goPort, pyPort)
	remoteHash, err := hex.DecodeString(ready.Identity)
	if err != nil {
		t.Fatal(err)
	}
	destHash := proto.TelephonyHash(remoteHash)
	if err := waitPath(tr, destHash, 15*time.Second); err != nil {
		t.Fatal(err)
	}
	remote, err := identity.Recall(destHash)
	if err != nil {
		t.Fatal(err)
	}
	rejected := make(chan struct{}, 1)
	caller := call.NewCall(tr, call.Config{
		Identity:    id,
		UseAudio:    false,
		ConnectTime: 12 * time.Second,
		WaitTime:    15 * time.Second,
		Events: call.Events{
			OnRejected: func(*call.Call) { rejected <- struct{}{} },
		},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	err = caller.Dial(ctx, remote)
	select {
	case <-rejected:
	case <-time.After(12 * time.Second):
		t.Fatalf("expected reject from python hangup-at-ring, dial err=%v", err)
	}
}

func TestPythonHalfDuplexGoDialsPython(t *testing.T) {
	lxsttest.SkipLive(t)
	py := lxsttest.Python(t)
	goPort := freeUDP(t)
	pyPort := freeUDP(t)
	cfgDir := t.TempDir()
	peer := filepath.Join(lxsttest.RepoRoot(t), "testdata", "lxst", "lxst_peer.py")
	cmd := exec.Command(py, peer,
		"--configdir", cfgDir,
		"--listen-port", fmt.Sprintf("%d", pyPort),
		"--target-port", fmt.Sprintf("%d", goPort),
		"--mode", "listen",
		"--call-mode", "half",
		"--auto-answer", "0.3",
		"--frames", "8",
		"--name", "py_hdx",
	)
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()
	events := bufio.NewScanner(stdout)
	ready := waitEvent(t, events, "ready", 15*time.Second)
	errCh := make(chan string, 4)
	go func() {
		for events.Scan() {
			var ev peerEvent
			if err := json.Unmarshal(events.Bytes(), &ev); err != nil {
				continue
			}
			if ev.Event == "error" {
				errCh <- ev.Error
				return
			}
			t.Logf("python: %s", events.Text())
		}
	}()
	tr, id := startGoUDP(t, goPort, pyPort)
	remoteHash, err := hex.DecodeString(ready.Identity)
	if err != nil {
		t.Fatal(err)
	}
	destHash := proto.TelephonyHash(remoteHash)
	if err := waitPath(tr, destHash, 15*time.Second); err != nil {
		t.Fatal(err)
	}
	remote, err := identity.Recall(destHash)
	if err != nil {
		t.Fatal(err)
	}
	established := make(chan struct{}, 1)
	caller := call.NewCall(tr, call.Config{
		Identity:    id,
		UseAudio:    false,
		Mode:        proto.ModeHalfDuplex,
		ConnectTime: 15 * time.Second,
		WaitTime:    25 * time.Second,
		Events: call.Events{
			OnAnswered: func(*call.Call) { established <- struct{}{} },
		},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := caller.Dial(ctx, remote); err != nil {
		t.Fatalf("dial: %v", err)
	}
	select {
	case <-established:
	case <-time.After(10 * time.Second):
		t.Fatal("half duplex call did not establish")
	}
	if caller.Mode() != proto.ModeHalfDuplex {
		t.Fatalf("mode %d want half duplex", caller.Mode())
	}
	time.Sleep(900 * time.Millisecond)
	if caller.SentFrames() != 0 {
		t.Fatalf("expected no frames while squelched, got %d", caller.SentFrames())
	}
	caller.PTT(true)
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) && caller.SentFrames() == 0 {
		time.Sleep(50 * time.Millisecond)
	}
	sent := caller.SentFrames()
	recv := caller.RecvFrames()
	_ = caller.Hangup("done")
	select {
	case msg := <-errCh:
		t.Fatalf("python peer error: %s", msg)
	default:
	}
	if sent == 0 {
		t.Fatal("go sent no frames after PTT in half duplex")
	}
	if recv == 0 {
		t.Fatal("go received no frames from python in half duplex")
	}
}

func TestPythonHalfDuplexPythonDialsGo(t *testing.T) {
	lxsttest.SkipLive(t)
	py := lxsttest.Python(t)
	goPort := freeUDP(t)
	pyPort := freeUDP(t)
	tr, id := startGoUDP(t, goPort, pyPort)
	dest, err := destination.New(id, destination.In, destination.Single, proto.AppName, tr, proto.AspectName)
	if err != nil {
		t.Fatal(err)
	}
	dest.AcceptsLinks(true)

	established := make(chan struct{}, 1)
	var recv uint64
	sb := call.NewSwitchboard(tr, call.Config{
		Identity: id,
		UseAudio: false,
		Mode:     proto.ModeHalfDuplex,
		Events: call.Events{
			OnRinging: func(c *call.Call) {
				if !c.Incoming() {
					return
				}
				go func() {
					time.Sleep(200 * time.Millisecond)
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					_ = c.Answer(ctx)
				}()
			},
			OnAnswered: func(c *call.Call) {
				if c.Mode() != proto.ModeHalfDuplex {
					t.Errorf("incoming mode %d want half duplex", c.Mode())
				}
				if !c.Squelched() {
					t.Error("incoming half duplex should start squelched")
				}
				established <- struct{}{}
				go func() {
					time.Sleep(350 * time.Millisecond)
					c.PTT(true)
				}()
			},
			OnFrame: func(pcm []int16) {
				if len(pcm) > 0 {
					recv++
				}
			},
		},
	}, nil)
	sb.Bind(dest)

	cfgDir := t.TempDir()
	peer := filepath.Join(lxsttest.RepoRoot(t), "testdata", "lxst", "lxst_peer.py")
	cmd := exec.Command(py, peer,
		"--configdir", cfgDir,
		"--listen-port", fmt.Sprintf("%d", pyPort),
		"--target-port", fmt.Sprintf("%d", goPort),
		"--mode", "dial",
		"--call-mode", "half",
		"--dial", fmt.Sprintf("%x", id.Hash()),
		"--frames", "14",
		"--name", "py_hdx_dial",
	)
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	events := bufio.NewScanner(stdout)
	_ = waitEvent(t, events, "ready", 15*time.Second)
	for range 8 {
		_ = dest.Announce(false, nil, nil)
		time.Sleep(150 * time.Millisecond)
	}
	go func() {
		for events.Scan() {
			var ev peerEvent
			if err := json.Unmarshal(events.Bytes(), &ev); err != nil {
				continue
			}
			if ev.Event == "error" {
				t.Errorf("python half duplex dial error: %s", ev.Error)
				return
			}
			t.Logf("python: %s", events.Text())
		}
	}()
	select {
	case <-established:
	case <-time.After(20 * time.Second):
		t.Fatal("go did not receive established half duplex call")
	}
	c := sb.Active()
	if c == nil {
		t.Fatal("no active call on switchboard")
	}
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) && c.SentFrames() == 0 {
		time.Sleep(50 * time.Millisecond)
	}
	sent := c.SentFrames()
	_ = c.Hangup("done")
	if sent == 0 {
		t.Fatal("go sent no frames after PTT in half duplex incoming call")
	}
	if recv == 0 {
		t.Fatal("go received no frames from python half duplex caller")
	}
}
