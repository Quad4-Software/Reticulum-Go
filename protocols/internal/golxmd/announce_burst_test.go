// SPDX-License-Identifier: LicenseRef-Reticulum
package golxmd

import (
	"bytes"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/debug"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxmf"
)

// The startup announce pattern fires the initial delivery and propagation
// announces plus the staggered re-announce copies and the control announces
// inside the same 10 second burst window. Every one of them must reach the
// wire: the per-destination burst limiter exists for runaway loops, and the
// staggered schedule is sized to fit under it.
func TestStartupStaggeredAnnouncesNotDropped(t *testing.T) {
	if testing.Short() {
		t.Skip("real-time announce schedule test skipped in -short mode")
	}

	home := t.TempDir()
	storageDir := filepath.Join(home, "storage")
	messagesDir := filepath.Join(storageDir, "messages")
	if err := os.MkdirAll(messagesDir, 0o700); err != nil {
		t.Fatal(err)
	}
	identPath := filepath.Join(home, "identity")
	id, err := identity.NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	if err := id.ToFile(identPath); err != nil {
		t.Fatal(err)
	}

	// UDP sink counting every datagram the service emits.
	sink, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	sinkPort := sink.LocalAddr().(*net.UDPAddr).Port
	var datagrams atomic.Int64
	sinkDone := make(chan struct{})
	go func() {
		defer close(sinkDone)
		buf := make([]byte, 4096)
		for {
			_ = sink.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
			n, _, err := sink.ReadFromUDP(buf)
			if err != nil {
				if ne, ok := err.(net.Error); ok && ne.Timeout() {
					continue
				}
				return
			}
			if n > 0 {
				datagrams.Add(1)
			}
		}
	}()
	t.Cleanup(func() {
		_ = sink.Close()
		<-sinkDone
	})

	listen, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	listenPort := listen.LocalAddr().(*net.UDPAddr).Port
	_ = listen.Close()

	var dbg bytes.Buffer
	debug.Init()
	debug.SetExtraWriter(&dbg)
	t.Cleanup(func() { debug.SetExtraWriter(nil) })

	lxmfCfg := lxmf.DefaultConfig()
	lxmfCfg.LXMF.AnnounceAtStart = true
	lxmfCfg.Propagation.EnableNode = true
	lxmfCfg.Propagation.AnnounceAtStart = true
	// Two control identities trigger the startup control announce, the
	// same trick the interop harness uses upstream.
	dummy := strings.Repeat("cd", 16)
	peer := strings.Repeat("ab", 16)
	lxmfCfg.Propagation.ControlAllowed = []string{peer, dummy}

	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	svc := NewService(Config{
		HomeDir:      home,
		IdentityPath: identPath,
		StorageDir:   storageDir,
		MessagesDir:  messagesDir,
		ReadyPath:    filepath.Join(home, "ready.json"),
		UDPListen:    "127.0.0.1:" + strconv.Itoa(listenPort),
		UDPForward:   "127.0.0.1:" + strconv.Itoa(sinkPort),
	}, lxmfCfg, log)
	if err := svc.Start(); err != nil {
		t.Fatalf("service start: %v", err)
	}
	t.Cleanup(svc.Stop)

	// The first-window pattern completes once the t=2s and t=6s staggered
	// copies are out. Nine seconds covers it with margin for stamp
	// generation on loaded machines.
	deadline := time.Now().Add(9 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}

	out := dbg.String()
	if strings.Contains(out, "burst limit") {
		t.Fatalf("staggered startup announces tripped the burst limiter:\n%s", out)
	}
	// Lower bound on the wire by t+9s: delivery 3 (start, t2, t6),
	// propagation 3, control 2 (t2, t6). Generation shifts everything
	// slightly later, so accept a margin below the full 8.
	if got := int(datagrams.Load()); got < 5 {
		t.Fatalf("only %d datagrams on the wire after 9s, want at least 5 (staggered announces missing)", got)
	} else {
		t.Logf("datagrams in first 9s: %d", got)
	}
}
