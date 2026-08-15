// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package reticulumconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_SerialDevicePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	body := `[reticulum]
  enable_transport = no
  share_instance = no

[interfaces]
  [[USB Radio]]
    type = SerialInterface
    enabled = yes
    port = /dev/ttyUSB0
    speed = 115200
    databits = 8
    parity = N
    stopbits = 1
    frame_idle_ms = 150
  [[Numeric Port Still Works]]
    type = UDPInterface
    enabled = yes
    address = 0.0.0.0
    port = 4242
    target_address = 127.0.0.1:4243
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	ser := cfg.Interfaces["USB Radio"]
	if ser == nil {
		t.Fatal("missing USB Radio")
	}
	if ser.Device != "/dev/ttyUSB0" {
		t.Fatalf("Device: got %q", ser.Device)
	}
	if ser.Speed != 115200 || ser.DataBits != 8 || ser.StopBits != 1 {
		t.Fatalf("serial params: %+v", ser)
	}
	if ser.SerialFrameIdleMs != 150 {
		t.Fatalf("frame_idle_ms: %d", ser.SerialFrameIdleMs)
	}
	udp := cfg.Interfaces["Numeric Port Still Works"]
	if udp == nil || udp.Port != 4242 {
		t.Fatalf("UDP port: %+v", udp)
	}
}
