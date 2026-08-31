// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package reticulumconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfig_RNodeAndMulti(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	body := `[reticulum]
  enable_transport = no
  share_instance = no

[interfaces]
  [[RNode0]]
    type = RNodeInterface
    enabled = yes
    port = /dev/ttyUSB0
    frequency = 915000000
    bandwidth = 125000
    txpower = 17
    spreadingfactor = 8
    codingrate = 5
    flow_control = yes
    id_interval = 600
    id_callsign = TEST1
    airtime_limit_short = 12.5
    airtime_limit_long = 5
  [[RNodeMulti0]]
    type = RNodeMultiInterface
    enabled = yes
    port = /dev/ttyACM0
    id_interval = 600
    id_callsign = MULTI1
    [[[Radio A]]]
      enabled = yes
      vport = 0
      frequency = 915000000
      bandwidth = 125000
      txpower = 10
      spreadingfactor = 7
      codingrate = 5
      outgoing = yes
    [[[Radio B]]]
      enabled = yes
      vport = 1
      frequency = 433000000
      bandwidth = 125000
      txpower = 14
      spreadingfactor = 9
      codingrate = 6
      airtime_limit_short = 20
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	rn := cfg.Interfaces["RNode0"]
	if rn == nil {
		t.Fatal("missing RNode0")
	}
	if rn.Type != "RNodeInterface" || rn.Device != "/dev/ttyUSB0" {
		t.Fatalf("rnode identity: %+v", rn)
	}
	if rn.FrequencyHz != 915000000 || rn.Bandwidth != 125000 || rn.TXPower != 17 {
		t.Fatalf("rnode rf: %+v", rn)
	}
	if rn.SpreadingFactor != 8 || rn.CodingRate != 5 || !rn.FlowControl {
		t.Fatalf("rnode modem: %+v", rn)
	}
	if rn.IDInterval != 600 || rn.IDCallsign != "TEST1" {
		t.Fatalf("rnode id: %+v", rn)
	}
	if !rn.AirtimeLimitShortSet || rn.AirtimeLimitShort != 12.5 {
		t.Fatalf("rnode airtime short: %+v", rn)
	}
	if !rn.AirtimeLimitLongSet || rn.AirtimeLimitLong != 5 {
		t.Fatalf("rnode airtime long: %+v", rn)
	}
	if _, exists := cfg.Interfaces["Radio A"]; exists {
		t.Fatal("nested subinterface leaked as top-level interface")
	}

	multi := cfg.Interfaces["RNodeMulti0"]
	if multi == nil {
		t.Fatal("missing RNodeMulti0")
	}
	if multi.Type != "RNodeMultiInterface" || multi.Device != "/dev/ttyACM0" {
		t.Fatalf("multi identity: %+v", multi)
	}
	if multi.IDCallsign != "MULTI1" || len(multi.SubInterfaces) != 2 {
		t.Fatalf("multi subs: %+v", multi)
	}
	a := multi.SubInterfaces["Radio A"]
	if a == nil || !a.VPortSet || a.VPort != 0 || a.FrequencyHz != 915000000 {
		t.Fatalf("radio a: %+v", a)
	}
	b := multi.SubInterfaces["Radio B"]
	if b == nil || !b.VPortSet || b.VPort != 1 || b.SpreadingFactor != 9 {
		t.Fatalf("radio b: %+v", b)
	}
	if !b.AirtimeLimitShortSet || b.AirtimeLimitShort != 20 {
		t.Fatalf("radio b airtime: %+v", b)
	}
}

func TestSaveConfig_RNodeRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	body := `[reticulum]
  enable_transport = no

[interfaces]
  [[RNode0]]
    type = RNodeInterface
    enabled = yes
    port = /dev/ttyUSB0
    frequency = 915000000
    bandwidth = 125000
    txpower = 10
    spreadingfactor = 7
    codingrate = 5
  [[Multi]]
    type = RNodeMultiInterface
    enabled = yes
    port = /dev/ttyACM0
    [[[Sub0]]]
      enabled = yes
      vport = 0
      frequency = 915000000
      bandwidth = 125000
      txpower = 10
      spreadingfactor = 7
      codingrate = 5
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "config.out")
	cfg.ConfigPath = out
	if err := SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, want := range []string{
		"type = RNodeInterface",
		"port = /dev/ttyUSB0",
		"frequency = 915000000",
		"spreadingfactor = 7",
		"type = RNodeMultiInterface",
		"[[[Sub0]]]",
		"vport = 0",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("saved config missing %q\n%s", want, s)
		}
	}
	cfg2, err := LoadConfig(out)
	if err != nil {
		t.Fatal(err)
	}
	if cfg2.Interfaces["RNode0"].TXPower != 10 {
		t.Fatalf("reload txpower: %+v", cfg2.Interfaces["RNode0"])
	}
	sub := cfg2.Interfaces["Multi"].SubInterfaces["Sub0"]
	if sub == nil || sub.VPort != 0 || sub.FrequencyHz != 915000000 {
		t.Fatalf("reload sub: %+v", sub)
	}
}
