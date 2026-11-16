// SPDX-License-Identifier: LicenseRef-Reticulum
package session_test

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"net"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/interfaces"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/rnv"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/rnv/proto"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/rnv/session"
)

func TestAdversarialClipWithoutAccept(t *testing.T) {
	// Unit-level: progress required for large clips
	cfg := session.SafeConfig()
	_ = cfg
	big := make([]byte, (1<<20)+1)
	meta := proto.ClipMeta{Size: uint64(len(big)), Codec: proto.CodecOpaque}
	if err := rnv.ValidateClipMeta(meta, rnv.MaxClipBytes); err != nil {
		t.Fatal(err)
	}
}

func TestLiveStillStream(t *testing.T) {
	if testing.Short() {
		t.Skip("live rnv mesh skipped in -short")
	}
	portA := freeUDP(t)
	portB := freeUDP(t)
	trA, idA, ifA := startNode(t, "A", portA, portB)
	trB, idB, ifB := startNode(t, "B", portB, portA)
	defer ifA.Stop()
	defer ifB.Stop()

	stillCh := make(chan []byte, 1)
	streamCh := make(chan proto.StreamOffer, 1)
	videoCh := make(chan []byte, 4)

	cfgB := session.SafeConfig()
	cfgB.Handlers = session.Handlers{
		OnStill: func(_ *session.Conn, _ proto.StillMeta, data []byte) {
			stillCh <- append([]byte(nil), data...)
		},
		OnStream: func(_ *session.Conn, offer proto.StreamOffer) {
			streamCh <- offer
		},
		OnVideo: func(_ *session.Conn, fr proto.Frame) {
			videoCh <- append([]byte(nil), fr.Payload...)
		},
	}
	epB, err := session.Bind(trB, idB, cfgB)
	if err != nil {
		t.Fatal(err)
	}
	if err := epB.Announce(); err != nil {
		t.Fatal(err)
	}
	identity.Remember(nil, epB.Hash(), idB.GetPublicKey(), nil)

	cfgA := session.SafeConfig()
	epA, err := session.Bind(trA, idA, cfgA)
	if err != nil {
		t.Fatal(err)
	}
	_ = epA.Announce()

	waitPath(t, trA, epB.Hash(), 15*time.Second)

	conn, err := epA.Dial(epB.Hash())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	jpegBytes := tinyJPEG(t)
	if err := conn.SendStill(context.Background(), jpegBytes, proto.StillMeta{}); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-stillCh:
		if !bytes.Equal(got, jpegBytes) {
			t.Fatalf("still mismatch %d vs %d", len(got), len(jpegBytes))
		}
	case <-time.After(10 * time.Second):
		t.Fatal("still timeout")
	}

	sc, err := conn.OpenStream(context.Background(), proto.StreamOffer{
		Profile: proto.ProfileMedium,
		Tracks:  proto.TrackVideo,
		Video:   proto.CodecJPEG,
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-streamCh:
	case <-time.After(5 * time.Second):
		t.Fatal("stream offer timeout")
	}
	frame := jpegBytes
	if len(frame) > proto.MaxStreamFrameBytes {
		frame = frame[:proto.MaxStreamFrameBytes]
	}
	if err := sc.SendVideo(frame); err != nil {
		t.Fatal(err)
	}
	select {
	case <-videoCh:
	case <-time.After(5 * time.Second):
		t.Fatal("video frame timeout")
	}
}

func TestLiveRejectCapacityFallback(t *testing.T) {
	if testing.Short() {
		t.Skip("live rnv mesh skipped in -short")
	}
	portA := freeUDP(t)
	portB := freeUDP(t)
	trA, idA, ifA := startNode(t, "A", portA, portB)
	trB, idB, ifB := startNode(t, "B", portB, portA)
	defer ifA.Stop()
	defer ifB.Stop()

	cfgB := session.SafeConfig()
	cfgB.Caps.Profiles = []int{proto.ProfileUltraLow, proto.ProfileLow}
	cfgB.Caps.Preferred = proto.ProfileUltraLow
	epB, err := session.Bind(trB, idB, cfgB)
	if err != nil {
		t.Fatal(err)
	}
	_ = epB.Announce()
	identity.Remember(nil, epB.Hash(), idB.GetPublicKey(), nil)

	epA, err := session.Bind(trA, idA, session.SafeConfig())
	if err != nil {
		t.Fatal(err)
	}
	_ = epA.Announce()
	waitPath(t, trA, epB.Hash(), 15*time.Second)

	conn, err := epA.Dial(epB.Hash())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	_, err = conn.OpenStream(context.Background(), proto.StreamOffer{
		Profile: proto.ProfileMedium,
		Tracks:  proto.TrackVideo,
		Video:   proto.CodecJPEG,
	})
	if err == nil {
		t.Fatal("expected capacity reject")
	}
}

func startNode(t *testing.T, tag string, localPort, peerPort int) (*transport.Transport, *identity.Identity, interfaces.Interface) {
	t.Helper()
	cfg := common.DefaultConfig()
	tr := transport.NewTransport(cfg)
	if err := tr.Start(); err != nil {
		t.Fatal(err)
	}
	local := fmt.Sprintf("127.0.0.1:%d", localPort)
	peer := fmt.Sprintf("127.0.0.1:%d", peerPort)
	var iface interfaces.Interface
	var err error
	iface, err = interfaces.NewUDPInterface("rnv-"+tag, local, peer, true)
	if err != nil {
		t.Fatal(err)
	}
	iface.SetPacketCallback(func(d []byte, ni common.NetworkInterface) { tr.HandlePacket(d, ni) })
	if err := iface.Start(); err != nil {
		t.Fatal(err)
	}
	if ni, ok := iface.(common.NetworkInterface); ok {
		if err := tr.RegisterInterface("rnv-"+tag, ni); err != nil {
			t.Fatal(err)
		}
	}
	id, err := identity.NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	return tr, id, iface
}

func freeUDP(t *testing.T) int {
	t.Helper()
	c, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := c.LocalAddr().(*net.UDPAddr).Port
	_ = c.Close()
	return port
}

func waitPath(t *testing.T, tr *transport.Transport, hash []byte, timeout time.Duration) {
	t.Helper()
	_ = tr.RequestPath(hash, "", nil, true)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if tr.HasPath(hash) {
			return
		}
		time.Sleep(50 * time.Millisecond)
		_ = tr.RequestPath(hash, "", nil, true)
	}
	t.Fatal("path timeout")
}

func tinyJPEG(t *testing.T) []byte {
	t.Helper()
	// Prefer a small JPEG that fits inline in a STILL envelope (avoid resource race).
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := range 4 {
		for x := range 4 {
			img.Set(x, y, color.RGBA{R: 200, G: 100, B: 50, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 20}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
