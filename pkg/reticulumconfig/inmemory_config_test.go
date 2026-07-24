// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package reticulumconfig

import (
	"path/filepath"
	"testing"
)

func TestLoadConfig_InMemoryStorage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	writeFile(t, path, `[reticulum]
  in_memory_storage = yes
  soft_memory_limit = 128M
  max_in_memory_paths = 500
  max_in_memory_known_destinations = 600
  max_in_memory_resource_bytes = 16M
  max_packet_hashlist = 4096
`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if !cfg.InMemoryStorage {
		t.Fatal("InMemoryStorage")
	}
	if !cfg.InMemoryPathTable || !cfg.InMemoryKnownDestinations {
		t.Fatal("InMemoryStorage should imply table flags")
	}
	if cfg.SoftMemoryLimitBytes != 128<<20 {
		t.Fatalf("SoftMemoryLimitBytes = %d", cfg.SoftMemoryLimitBytes)
	}
	if cfg.MaxInMemoryPaths != 500 {
		t.Fatalf("MaxInMemoryPaths = %d", cfg.MaxInMemoryPaths)
	}
	if cfg.MaxInMemoryKnownDestinations != 600 {
		t.Fatalf("MaxInMemoryKnownDestinations = %d", cfg.MaxInMemoryKnownDestinations)
	}
	if cfg.MaxInMemoryResourceBytes != 16<<20 {
		t.Fatalf("MaxInMemoryResourceBytes = %d", cfg.MaxInMemoryResourceBytes)
	}
	if cfg.MaxPacketHashlist != 4096 {
		t.Fatalf("MaxPacketHashlist = %d", cfg.MaxPacketHashlist)
	}
}
