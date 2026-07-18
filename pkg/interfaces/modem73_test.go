// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

func TestModem73ControlRoundTrip(t *testing.T) {
	msg := map[string]any{"cmd": "get_config", "payload_size": float64(500)}
	frame, err := modem73EncodeControl(msg)
	if err != nil {
		t.Fatal(err)
	}
	got, err := modem73ReadControl(bytes.NewReader(frame))
	if err != nil {
		t.Fatal(err)
	}
	if got["cmd"] != "get_config" {
		t.Fatalf("cmd=%v", got["cmd"])
	}
}

func TestModem73ComputeMTU(t *testing.T) {
	if g := modem73ComputeMTU(515, 15, 500); g != 500 {
		t.Fatalf("mtu=%d", g)
	}
	if g := modem73ComputeMTU(600, 15, 500); g != 585 {
		t.Fatalf("mtu=%d", g)
	}
	if !modem73NeedsFragmentation(500, 15, 500) {
		t.Fatal("expected fragmentation")
	}
}

func TestModem73FakeDualPort(t *testing.T) {
	kissLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer kissLn.Close()
	ctrlLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ctrlLn.Close()

	var (
		gotPayload []byte
		gotMu      sync.Mutex
		shortTX    int
	)

	go func() {
		for {
			c, err := kissLn.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				dec := newKISSStreamDecoder(2048, func(p []byte) {
					gotMu.Lock()
					gotPayload = append([]byte(nil), p...)
					gotMu.Unlock()
				})
				buf := make([]byte, 4096)
				for {
					n, err := conn.Read(buf)
					if n > 0 {
						dec.feed(buf[:n])
					}
					if err != nil {
						return
					}
				}
			}(c)
		}
	}()

	go func() {
		for {
			c, err := ctrlLn.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				for {
					msg, err := modem73ReadControl(conn)
					if err != nil {
						return
					}
					if msg["cmd"] == "get_config" {
						_ = modem73WriteControl(conn, map[string]any{
							"ok":            true,
							"payload_size":  float64(600),
							"modem_type":    float64(0),
							"modulation":    "QPSK",
							"code_rate":     "1/2",
							"frame_size":    float64(1),
							"csma_enabled":  true,
							"csma_quiet_ms": float64(500),
							"csma_cw":       float64(8),
							"slot_time_ms":  float64(500),
							"csma_burst":    float64(1),
						})
						continue
					}
					if msg["cmd"] == "tx" {
						gotMu.Lock()
						shortTX++
						gotMu.Unlock()
						if s, ok := msg["data"].(string); ok {
							raw, _ := base64.StdEncoding.DecodeString(s)
							gotMu.Lock()
							gotPayload = raw
							gotMu.Unlock()
						}
					}
				}
			}(c)
		}
	}()

	kissAddr := kissLn.Addr().(*net.TCPAddr)
	ctrlAddr := ctrlLn.Addr().(*net.TCPAddr)

	m, err := NewModem73Interface("m73", true, Modem73Options{
		TargetHost:        "127.0.0.1",
		TargetPort:        kissAddr.Port,
		ControlHost:       "127.0.0.1",
		ControlPort:       ctrlAddr.Port,
		AutoFragmentation: true,
		AutoBitrate:       true,
		CSMAOverhead:      true,
		ShortFrames:       "off",
		Dial:               (&net.Dialer{}).DialContext,
		ControlDialTimeout: 500 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Start(); err != nil {
		t.Fatal(err)
	}
	defer m.Detach()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if m.IsOnline() && m.GetMTU() == 585 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if m.GetMTU() != 585 {
		t.Fatalf("MTU=%d want 585", m.GetMTU())
	}

	payload := []byte("hello-modem73")
	if err := m.ProcessOutgoing(payload); err != nil {
		// may race before data online
		deadline = time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if m.IsOnline() {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		if err := m.ProcessOutgoing(payload); err != nil {
			t.Fatal(err)
		}
	}
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		gotMu.Lock()
		ok := bytes.Equal(gotPayload, payload)
		gotMu.Unlock()
		if ok {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	gotMu.Lock()
	defer gotMu.Unlock()
	t.Fatalf("payload=%q shortTX=%d", gotPayload, shortTX)
}

func TestModem73InvalidShortFrames(t *testing.T) {
	_, err := NewModem73Interface("x", false, Modem73Options{ShortFrames: "nope"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestModem73DialContextCancel(t *testing.T) {
	m, err := NewModem73Interface("x", true, Modem73Options{
		TargetHost:  "127.0.0.1",
		TargetPort:  1,
		ControlHost: "127.0.0.1",
		ControlPort: 1,
		ShortFrames:        "off",
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
		ControlDialTimeout: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Start(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	m.Detach()
}

func TestModem73HandshakeDetect(t *testing.T) {
	pkt := make([]byte, 40)
	pkt[0] = modem73PktLinkRequest
	if !modem73IsHandshake(pkt) {
		t.Fatal("link request")
	}
	pkt[0] = modem73PktProof
	if !modem73IsProof(pkt) {
		t.Fatal("proof")
	}
}

var _ io.Reader
