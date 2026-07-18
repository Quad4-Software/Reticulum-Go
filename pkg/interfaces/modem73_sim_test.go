// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"bytes"
	"encoding/base64"
	"math"
	"net"
	"sync"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
)

func TestModem73PhyExploratoryRobustAirtime(t *testing.T) {
	if math.Abs(Modem73RobustAirtime(0)-3.56) > 0.01 {
		t.Fatalf("mode0 airtime=%v want ~3.56", Modem73RobustAirtime(0))
	}
	for mode := range modem73RobustModeCount {
		air := Modem73RobustAirtime(mode)
		if air <= 0 {
			t.Fatalf("mode %d airtime=%v", mode, air)
		}
		ps := Modem73RobustPayload(mode)
		if ps != 512 && ps != 172 {
			t.Fatalf("mode %d payload=%d", mode, ps)
		}
		phy := Modem73ComputePhy(modem73TypeRobust, mode, 0, 0, "", "")
		if phy.PayloadSize != ps || phy.MTUBytes != ps-2 {
			t.Fatalf("phy %+v", phy)
		}
		approx := float64(ps*8) / air
		doc := float64(modem73RobustBPSExt[mode])
		if math.Abs(approx-doc)/doc > 0.15 {
			t.Fatalf("mode %d bitrate approx=%v doc=%v", mode, approx, doc)
		}
	}
}

func TestModem73PhyExploratoryOFDMTables(t *testing.T) {
	phy := Modem73ComputePhy(modem73TypeOFDM, 0, 0, 1, "QPSK", "1/2")
	if phy.PayloadSize != 512 || phy.MTUBytes != 510 || phy.BitrateBPS != 1600 {
		t.Fatalf("QPSK 1/2 normal: %+v", phy)
	}
	if math.Abs(phy.AirtimeS-2.6) > 0.01 {
		t.Fatalf("airtime=%v", phy.AirtimeS)
	}
	short := Modem73ComputePhy(modem73TypeOFDM, 0, 0, 0, "BPSK", "1/2")
	if short.PayloadSize != 128 {
		t.Fatalf("short BPSK: %+v", short)
	}
}

func TestModem73BERExploratoryMonotonic(t *testing.T) {
	prev := 1.0
	for snr := -5.0; snr <= 12; snr++ {
		ber := Modem73BPSKBER(snr)
		if ber > prev+1e-9 {
			t.Fatalf("BER not decreasing at snr=%v", snr)
		}
		prev = ber
		fer := Modem73FrameErrorRate(ber, 512)
		if fer < 0 || fer > 1 {
			t.Fatalf("FER %v", fer)
		}
	}
	if Modem73BPSKBER(30) > 1e-6 {
		t.Fatal("high SNR BER should be tiny")
	}
}

func TestModem73SimulatorControlAndKISS(t *testing.T) {
	ch := NewModem73Channel(25, 42)
	a := NewModem73Simulator(Modem73SimConfig{
		ModemType: 0, Modulation: "QPSK", CodeRate: "1/2", FrameSize: 1,
		CSMAEnabled: false, InstantAirtime: true, SNRdB: 25, Seed: 42,
	}, ch)
	b := NewModem73Simulator(Modem73SimConfig{
		ModemType: 0, Modulation: "QPSK", CodeRate: "1/2", FrameSize: 1,
		CSMAEnabled: false, InstantAirtime: true, SNRdB: 25, Seed: 43,
	}, ch)

	_, ctrlA, err := a.Listen()
	if err != nil {
		t.Fatal(err)
	}
	kissB, _, err := b.Listen()
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	a.Serve(ctx)
	b.Serve(ctx)
	defer a.Close()
	defer b.Close()

	ctrlConn, err := net.Dial("tcp", ctrlA)
	if err != nil {
		t.Fatal(err)
	}
	defer ctrlConn.Close()
	if err := modem73WriteControl(ctrlConn, map[string]any{"cmd": "get_config"}); err != nil {
		t.Fatal(err)
	}
	cfg, err := modem73ReadControl(ctrlConn)
	if err != nil {
		t.Fatal(err)
	}
	ps, ok := modem73CfgInt(cfg, "payload_size")
	if !ok || ps != 512 {
		t.Fatalf("payload_size=%v cfg=%v", ps, cfg)
	}
	if err := modem73WriteControl(ctrlConn, map[string]any{"cmd": "get_status"}); err != nil {
		t.Fatal(err)
	}
	st, err := modem73ReadControl(ctrlConn)
	if err != nil {
		t.Fatal(err)
	}
	if st["audio_connected"] != true {
		t.Fatalf("status=%v", st)
	}

	var got []byte
	var mu sync.Mutex
	bKiss, err := net.Dial("tcp", kissB)
	if err != nil {
		t.Fatal(err)
	}
	defer bKiss.Close()

	payload := []byte("sim-phy-packet")
	aCtrl, err := net.DialTimeout("tcp", ctrlA, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer aCtrl.Close()
	if err := modem73WriteControl(aCtrl, map[string]any{
		"cmd":  "tx",
		"data": base64.StdEncoding.EncodeToString(payload),
	}); err != nil {
		t.Fatal(err)
	}
	resp, err := modem73ReadControl(aCtrl)
	if err != nil {
		t.Fatal(err)
	}
	if resp["ok"] != true {
		t.Fatalf("tx resp=%v", resp)
	}

	dec := newKISSStreamDecoder(2048, func(p []byte) {
		mu.Lock()
		got = append([]byte(nil), p...)
		mu.Unlock()
	})
	buf := make([]byte, 4096)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		_ = bKiss.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
		n, err := bKiss.Read(buf)
		if n > 0 {
			dec.feed(buf[:n])
		}
		mu.Lock()
		match := bytes.Equal(got, payload)
		mu.Unlock()
		if match {
			return
		}
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			break
		}
	}
	t.Fatalf("got=%q", got)
}

func TestModem73SimulatorWithInterface(t *testing.T) {
	ch := NewModem73Channel(30, 7)
	sim := NewModem73Simulator(Modem73SimConfig{
		Modulation: "QPSK", CodeRate: "1/2", FrameSize: 1,
		InstantAirtime: true, SNRdB: 30, Seed: 7,
	}, ch)
	if _, _, err := sim.Listen(); err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	sim.Serve(ctx)
	defer sim.Close()

	var got []byte
	var mu sync.Mutex
	iface, err := NewModem73Interface("vs-sim", true, Modem73Options{
		TargetHost: "127.0.0.1", TargetPort: sim.KISSPort(),
		ControlHost: "127.0.0.1", ControlPort: sim.ControlPort(),
		ShortFrames: "off", ControlDialTimeout: 500 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	iface.SetPacketCallback(func(data []byte, _ common.NetworkInterface) {
		mu.Lock()
		got = append([]byte(nil), data...)
		mu.Unlock()
	})
	if err := iface.Start(); err != nil {
		t.Fatal(err)
	}
	defer iface.Detach()

	peer := NewModem73Simulator(Modem73SimConfig{
		Modulation: "QPSK", CodeRate: "1/2", FrameSize: 1,
		InstantAirtime: true, SNRdB: 30, Seed: 8,
	}, ch)
	if _, _, err := peer.Listen(); err != nil {
		t.Fatal(err)
	}
	peer.Serve(ctx)
	defer peer.Close()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if iface.IsOnline() {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !iface.IsOnline() {
		t.Fatal("interface not online")
	}
	if mtu := iface.GetMTU(); mtu < modem73MTUFloor {
		t.Fatalf("MTU=%d", mtu)
	}

	payload := []byte("iface-sim-roundtrip")
	ctrl, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", itoaPort(peer.ControlPort())))
	if err != nil {
		t.Fatal(err)
	}
	defer ctrl.Close()
	if err := modem73WriteControl(ctrl, map[string]any{
		"cmd":  "tx",
		"data": base64.StdEncoding.EncodeToString(payload),
	}); err != nil {
		t.Fatal(err)
	}
	_, _ = modem73ReadControl(ctrl)

	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		ok := bytes.Equal(got, payload)
		mu.Unlock()
		if ok {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("got=%q", got)
}

func itoaPort(n int) string {
	if n == 0 {
		return "0"
	}
	var b [16]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
