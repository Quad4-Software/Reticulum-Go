// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package reticulumconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStampsAndFilterSections(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config")
	cfg := `
[reticulum]
  enable_transport = yes

[stamps]
  enforce_stamps = yes
  stamp_destinations = yes
  stamp_prs = yes
  always_stamp_lrs = yes
  destination_cost = 33.25
  destination_value = 200.0
  pr_cost = 9.5
  pr_value = -4
  lr_cost = 60.0
  lr_stamp_timeout = 0.001
  pr_stamp_timeout = 20

[filter]
  required_only = yes
  ignored_types = single, group
  allowed_types = link
  hop_limiter = 999

[[interfaces]]
  [[interfaces.udp]]
    type = UDPInterface
    destination_cost = 7.5
    pr_cost = 300.0
`
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	s := c.Stamps
	if !s.EnforceStamps || !s.StampDestinations || !s.StampPRs || !s.AlwaysStampLRs {
		t.Fatal("stamps booleans not parsed")
	}
	settings := c.StampsSettings()
	if !settings.Enforced || !settings.LREnforced {
		t.Fatal("enforce_stamps did not set both flags")
	}
	if settings.DestinationCost != 33.25 {
		t.Fatalf("destination_cost=%v", settings.DestinationCost)
	}
	if settings.DestinationValue != 100 {
		t.Fatalf("destination_value=%v want 100 (capped)", settings.DestinationValue)
	}
	if settings.PRCost != 9.5 || settings.PRValue != 0 {
		t.Fatalf("pr=%v/%v", settings.PRCost, settings.PRValue)
	}
	if settings.LRCost != 40 {
		t.Fatalf("lr_cost=%v want 40 (link cap)", settings.LRCost)
	}
	if settings.LinkStampTimeout != 0.020 {
		t.Fatalf("lr_stamp_timeout=%v want 0.020 floor", settings.LinkStampTimeout)
	}
	if settings.PRStampTimeout != 20 {
		t.Fatalf("pr_stamp_timeout=%v", settings.PRStampTimeout)
	}

	f := c.Filter
	if !f.RequiredOnly || !f.RequiredOnlySet {
		t.Fatal("required_only not parsed")
	}
	if len(f.IgnoredTypes) != 2 || f.IgnoredTypes[0] != "single" {
		t.Fatalf("ignored_types=%v", f.IgnoredTypes)
	}
	if len(f.AllowedTypes) != 1 || f.AllowedTypes[0] != "link" {
		t.Fatalf("allowed_types=%v", f.AllowedTypes)
	}
	if f.HopLimiter != 128 || !f.HopLimiterSet {
		t.Fatalf("hop_limiter=%v want 128 (PATHFINDER_M cap)", f.HopLimiter)
	}

	iface := c.Interfaces["interfaces.udp"]
	if iface == nil {
		t.Fatalf("interfaces=%v", c.Interfaces)
	}
	if !iface.DestinationCostSet || iface.DestinationCost != 7.5 {
		t.Fatalf("iface destination_cost=%v", iface.DestinationCost)
	}
	if !iface.PRCostSet || iface.PRCost != 100 {
		t.Fatalf("iface pr_cost=%v want 100 (capped)", iface.PRCost)
	}
}
