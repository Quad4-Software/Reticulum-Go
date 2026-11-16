// SPDX-License-Identifier: LicenseRef-Reticulum
package session_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/rnv/proto"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/rnv/session"
)

func TestLiveClipAndAudioVideo(t *testing.T) {
	if testing.Short() {
		t.Skip("live rnv mesh skipped in -short")
	}
	portA := freeUDP(t)
	portB := freeUDP(t)
	trA, idA, ifA := startNode(t, "A", portA, portB)
	trB, idB, ifB := startNode(t, "B", portB, portA)
	defer ifA.Stop()
	defer ifB.Stop()

	clipCh := make(chan []byte, 1)
	audioCh := make(chan []byte, 4)
	videoCh := make(chan []byte, 4)

	cfgB := session.SafeConfig()
	cfgB.Handlers = session.Handlers{
		OnClip: func(_ *session.Conn, _ proto.ClipMeta, data []byte) {
			clipCh <- append([]byte(nil), data...)
		},
		OnAudio: func(_ *session.Conn, fr proto.Frame) {
			audioCh <- append([]byte(nil), fr.Payload...)
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

	clip := []byte("rnv-clip-payload-bytes-for-live-test")
	if err := conn.SendClip(context.Background(), clip, proto.ClipMeta{Mime: "application/octet-stream"}, nil); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-clipCh:
		if !bytes.Equal(got, clip) {
			t.Fatalf("clip mismatch %d vs %d", len(got), len(clip))
		}
	case <-time.After(20 * time.Second):
		t.Fatal("clip timeout")
	}

	sc, err := conn.OpenStream(context.Background(), proto.StreamOffer{
		Profile: proto.ProfileMedium,
		Tracks:  proto.TrackVideo | proto.TrackAudio,
		Video:   proto.CodecJPEG,
		Audio:   proto.CodecOpus,
	})
	if err != nil {
		t.Fatal(err)
	}
	jpegBytes := tinyJPEG(t)
	frame := jpegBytes
	if len(frame) > proto.MaxStreamFrameBytes {
		frame = frame[:proto.MaxStreamFrameBytes]
	}
	if err := sc.SendVideo(frame); err != nil {
		t.Fatal(err)
	}
	opusish := []byte{0x01, 0x02, 0x03, 0x04, 0x05}
	if err := sc.SendAudio(opusish); err != nil {
		t.Fatal(err)
	}
	select {
	case <-videoCh:
	case <-time.After(5 * time.Second):
		t.Fatal("video timeout")
	}
	select {
	case got := <-audioCh:
		if !bytes.Equal(got, opusish) {
			t.Fatalf("audio mismatch")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("audio timeout")
	}
}
