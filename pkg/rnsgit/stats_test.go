// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
)

func TestStatsRecordAndPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stats")
	s := newStatsStore(path)
	s.record("public", "demo", statView)
	s.record("public", "demo", statFetch)
	s.record("public", "demo", statPush)
	s.persist()

	s2 := newStatsStore(path)
	day := todayKey()
	repo := s2.repos["public"]["demo"]
	if repo == nil {
		t.Fatal("stats not persisted")
	}
	if repo[statView][day] != 1 || repo[statFetch][day] != 1 || repo[statPush][day] != 1 {
		t.Fatalf("unexpected counts: %v", repo)
	}
}

func TestStatsGroupAndPageScopes(t *testing.T) {
	s := newStatsStore("")
	s.record("public", "", statView)
	s.record("", "", "front")
	day := todayKey()
	if s.groups["public"][statView][day] != 1 {
		t.Fatalf("group view not recorded: %v", s.groups)
	}
	if s.pages["front"][day] != 1 {
		t.Fatalf("page view not recorded: %v", s.pages)
	}
}

func TestStatsIgnoreLists(t *testing.T) {
	n := testPageNode(t)
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	n.cfg.StatsIgnoreIdentities = map[string]bool{
		fmt.Sprintf("%x", id.Hash()): true,
	}
	if !n.statsIgnored(id) {
		t.Fatal("identity not ignored")
	}
	other, _ := identity.New()
	if n.statsIgnored(other) {
		t.Fatal("unrelated identity ignored")
	}
	n.cfg.StatsPushIgnoreIdentities = map[string]bool{
		fmt.Sprintf("%x", other.Hash()): true,
	}
	if !n.statsPushIgnored(other) {
		t.Fatal("push ignore list not honored")
	}
}

func TestActivityLevels(t *testing.T) {
	for daily, want := range map[float64]string{
		0:  "inactive",
		2:  "low",
		9:  "moderate",
		50: "high",
	} {
		if got := activityLevel(daily); got != want {
			t.Fatalf("daily %v: got %q want %q", daily, got, want)
		}
	}
}

func TestThanksDedup(t *testing.T) {
	var tr thanksTracker
	link := []byte("link-id-1")
	if !tr.dedup(link, "target") {
		t.Fatal("first thanks not counted")
	}
	if tr.dedup(link, "target") {
		t.Fatal("duplicate thanks counted")
	}
	if !tr.dedup(link, "other") {
		t.Fatal("distinct target deduplicated wrongly")
	}
}

func TestThanksCountPersists(t *testing.T) {
	dir := t.TempDir()
	var tr thanksTracker
	c1 := thanksCount(filepath.Join(dir, "thanks"), true, []byte("a"), &tr)
	c2 := thanksCount(filepath.Join(dir, "thanks"), true, []byte("b"), &tr)
	if c1 != 1 || c2 != 2 {
		t.Fatalf("counts %d %d", c1, c2)
	}
	var tr2 thanksTracker
	got := thanksCount(filepath.Join(dir, "thanks"), false, nil, &tr2)
	if got != 2 {
		t.Fatalf("reloaded count %d", got)
	}
}
