package pathfinder

import (
	"testing"
	"time"
)

func TestNewPathFinder(t *testing.T) {
	pf := NewPathFinder()
	if pf == nil {
		t.Fatal("NewPathFinder() returned nil")
	}
	if pf.paths == nil {
		t.Error("NewPathFinder() paths map is nil")
	}
}

func TestPathFinder_AddPath(t *testing.T) {
	pf := NewPathFinder()

	destHash := "test-dest-hash"
	nextHop := []byte{0x01, 0x02, 0x03, 0x04}
	iface := "eth0"
	hops := byte(5)

	pf.AddPath(destHash, nextHop, iface, hops)

	path, exists := pf.GetPath(destHash)
	if !exists {
		t.Fatal("GetPath() returned false after AddPath()")
	}

	if string(path.NextHop) != string(nextHop) {
		t.Errorf("NextHop = %v, want %v", path.NextHop, nextHop)
	}
	if path.Interface != iface {
		t.Errorf("Interface = %s, want %s", path.Interface, iface)
	}
	if path.HopCount != hops {
		t.Errorf("HopCount = %d, want %d", path.HopCount, hops)
	}
	if path.LastUpdated == 0 {
		t.Error("LastUpdated should be set")
	}
}

func TestPathFinder_GetPath(t *testing.T) {
	pf := NewPathFinder()

	destHash := "test-dest-hash"
	_, exists := pf.GetPath(destHash)
	if exists {
		t.Error("GetPath() should return false for non-existent path")
	}

	nextHop := []byte{0x01, 0x02}
	pf.AddPath(destHash, nextHop, "eth0", 3)

	path, exists := pf.GetPath(destHash)
	if !exists {
		t.Fatal("GetPath() returned false for existing path")
	}
	if string(path.NextHop) != string(nextHop) {
		t.Errorf("NextHop = %v, want %v", path.NextHop, nextHop)
	}
}

func TestPathFinder_UpdatePath(t *testing.T) {
	pf := NewPathFinder()

	destHash := "test-dest-hash"
	nextHop1 := []byte{0x01, 0x02}
	nextHop2 := []byte{0x03, 0x04}

	pf.AddPath(destHash, nextHop1, "eth0", 3)
	time.Sleep(10 * time.Millisecond)
	firstUpdate := time.Now().Unix()

	pf.AddPath(destHash, nextHop2, "eth1", 5)

	path, exists := pf.GetPath(destHash)
	if !exists {
		t.Fatal("GetPath() returned false")
	}

	if string(path.NextHop) != string(nextHop2) {
		t.Errorf("NextHop = %v, want %v", path.NextHop, nextHop2)
	}
	if path.Interface != "eth1" {
		t.Errorf("Interface = %s, want eth1", path.Interface)
	}
	if path.HopCount != 5 {
		t.Errorf("HopCount = %d, want 5", path.HopCount)
	}
	if path.LastUpdated < firstUpdate {
		t.Error("LastUpdated should be updated")
	}
}

func TestPathFinder_MultiplePaths(t *testing.T) {
	pf := NewPathFinder()

	paths := []struct {
		hash    string
		nextHop []byte
		iface   string
		hops    byte
	}{
		{"hash1", []byte{0x01}, "eth0", 1},
		{"hash2", []byte{0x02}, "eth1", 2},
		{"hash3", []byte{0x03}, "eth2", 3},
	}

	for _, p := range paths {
		pf.AddPath(p.hash, p.nextHop, p.iface, p.hops)
	}

	for _, p := range paths {
		path, exists := pf.GetPath(p.hash)
		if !exists {
			t.Errorf("GetPath() returned false for %s", p.hash)
			continue
		}
		if string(path.NextHop) != string(p.nextHop) {
			t.Errorf("NextHop for %s = %v, want %v", p.hash, path.NextHop, p.nextHop)
		}
		if path.Interface != p.iface {
			t.Errorf("Interface for %s = %s, want %s", p.hash, path.Interface, p.iface)
		}
		if path.HopCount != p.hops {
			t.Errorf("HopCount for %s = %d, want %d", p.hash, path.HopCount, p.hops)
		}
	}
}
