// SPDX-License-Identifier: LicenseRef-Reticulum
package call

import (
	"testing"
	"time"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/proto"
)

func TestIdentifyTimeoutEndsConnectingCall(t *testing.T) {
	c := NewCall(nil, Config{UseAudio: false, ConnectTime: 40 * time.Millisecond})
	if !c.state.CompareAndSwap(int32(StateIdle), int32(StateConnecting)) {
		t.Fatal("not idle")
	}
	c.incoming.Store(true)
	c.armTimeout(c.cfg.ConnectTime, func() bool {
		return c.incoming.Load() && c.state.Load() == int32(StateConnecting)
	}, "identify timeout")
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if c.State() == StateEnded {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("state %v", c.State())
}

func TestEnsureRecvDecoderIgnoresCodecFlip(t *testing.T) {
	c := NewCall(nil, Config{UseAudio: false})
	params := proto.ProfileParams(proto.DefaultProfile)
	dec, err := newDecoderForCodec(proto.CodecOpus, params)
	if err != nil {
		t.Fatal(err)
	}
	c.mutex.Lock()
	c.decoder = dec
	c.recvKind = proto.CodecOpus
	c.mutex.Unlock()
	if err := c.ensureRecvDecoder(proto.CodecRaw, []byte{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	c.mutex.Lock()
	kind := c.recvKind
	c.mutex.Unlock()
	if kind != proto.CodecOpus {
		t.Fatalf("codec flipped to %d", kind)
	}
	_ = dec.Close()
}
