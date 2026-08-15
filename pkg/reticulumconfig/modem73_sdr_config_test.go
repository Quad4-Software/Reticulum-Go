// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package reticulumconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfig_Modem73AndSDR(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	body := `[reticulum]
  enable_transport = no
  share_instance = no

[interfaces]
  [[MODEM73]]
    type = Modem73Interface
    enabled = yes
    target_host = 127.0.0.1
    target_port = 8001
    control_host = 127.0.0.1
    control_port = 8073
    mtu_overhead = 15
    bitrate = 400
    auto_fragmentation = yes
    short_frames = auto
    short_mtu = 170
    handshake_x2 = no
    proof_x2 = yes
    auto_bitrate = yes
    csma_overhead = yes
    timeout_margin = 0.35
    mode = boundary
  [[SDR0]]
    type = SDRInterface
    enabled = yes
    device = mock
    serial = ABC123
    frequency = 433000000
    sample_rate = 2000000
    bandwidth = 0
    rx_gain = 20
    tx_gain = 10
    modem = burst
    bitrate = 1200
    mode = boundary
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	m73 := cfg.Interfaces["MODEM73"]
	if m73 == nil {
		t.Fatal("missing MODEM73")
	}
	if m73.Type != "Modem73Interface" || m73.TargetPort != 8001 || m73.ControlPort != 8073 {
		t.Fatalf("modem73: %+v", m73)
	}
	if m73.MTUOverhead != 15 || m73.ShortFrames != "auto" || m73.ShortMTU != 170 {
		t.Fatalf("modem73 policy: %+v", m73)
	}
	if !m73.AutoFragSet || !m73.AutoFragmentation {
		t.Fatal("auto_fragmentation not set")
	}
	if !m73.ProofX2 || m73.HandshakeX2 {
		t.Fatalf("x2 flags: handshake=%v proof=%v", m73.HandshakeX2, m73.ProofX2)
	}
	if m73.TimeoutMargin != 0.35 || m73.Mode != "boundary" {
		t.Fatalf("modem73 misc: %+v", m73)
	}

	sdr := cfg.Interfaces["SDR0"]
	if sdr == nil {
		t.Fatal("missing SDR0")
	}
	if sdr.Device != "mock" || sdr.SerialNum != "ABC123" {
		t.Fatalf("sdr identity: %+v", sdr)
	}
	if sdr.FrequencyHz != 433000000 || sdr.SampleRate != 2000000 {
		t.Fatalf("sdr rf: %+v", sdr)
	}
	if sdr.RXGain != 20 || sdr.TXGain != 10 || sdr.Modem != "burst" {
		t.Fatalf("sdr modem: %+v", sdr)
	}
}

func TestSaveConfig_Modem73AndSDRRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	body := `[reticulum]
  enable_transport = no
  share_instance = no

[interfaces]
  [[M73]]
    type = Modem73Interface
    enabled = yes
    target_host = 10.0.0.2
    target_port = 8001
    control_host = 10.0.0.2
    control_port = 8073
    mtu_overhead = 20
    short_frames = always
    auto_fragmentation = no
    auto_bitrate = no
    csma_overhead = no
  [[Radio]]
    type = SDRInterface
    enabled = yes
    device = rtltcp
    frequency = 915000000
    sample_rate = 2400000
    modem = burst
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	out := filepath.Join(dir, "config.out")
	cfg.ConfigPath = out
	if err := SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, want := range []string{
		"type = Modem73Interface",
		"control_port = 8073",
		"mtu_overhead = 20",
		"short_frames = always",
		"auto_fragmentation = no",
		"type = SDRInterface",
		"device = rtltcp",
		"frequency = 915000000",
		"modem = burst",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("saved config missing %q\n%s", want, s)
		}
	}
	cfg2, err := LoadConfig(out)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if cfg2.Interfaces["M73"].MTUOverhead != 20 {
		t.Fatalf("reload mtu_overhead: %+v", cfg2.Interfaces["M73"])
	}
	if cfg2.Interfaces["Radio"].FrequencyHz != 915000000 {
		t.Fatalf("reload frequency: %+v", cfg2.Interfaces["Radio"])
	}
}
