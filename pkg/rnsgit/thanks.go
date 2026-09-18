// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import (
	"os"
	"sync"

	"github.com/Quad4-Software/Reticulum-Go/pkg/cryptography"
	"github.com/Quad4-Software/msgpack/v5/pkg/msgpack"
)

// thanksDedupBound matches the Python deque maxlen of 256 recent thankers.
const thanksDedupBound = 256

// thanksTracker deduplicates thanks per link and target within a bounded
// ring, matching the Python thanks_deque.
type thanksTracker struct {
	mu    sync.Mutex
	seen  map[[32]byte]struct{}
	order [][32]byte
}

func (t *thanksTracker) dedup(linkID []byte, target string) bool {
	if len(linkID) == 0 {
		return true
	}
	sum := cryptography.Hash(append(append([]byte(nil), linkID...), target...))
	var h [32]byte
	copy(h[:], sum)
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.seen == nil {
		t.seen = map[[32]byte]struct{}{}
	}
	if _, ok := t.seen[h]; ok {
		return false
	}
	t.seen[h] = struct{}{}
	t.order = append(t.order, h)
	if len(t.order) > thanksDedupBound {
		delete(t.seen, t.order[0])
		t.order = t.order[1:]
	}
	return true
}

// thanksCount reads or increments the persisted thanks counter at path,
// which stores msgpack {"count": n} like the reference implementation.
func thanksCount(path string, add bool, linkID []byte, tracker *thanksTracker) int64 {
	if add && !tracker.dedup(linkID, path) {
		add = false
	}
	var count int64
	if b, err := os.ReadFile(path); err == nil { // #nosec G304 -- repo-adjacent thanks file
		var data map[string]any
		if err := msgpack.Unmarshal(b, &data); err == nil {
			count = toInt64(data["count"])
		}
	}
	if add {
		count++
	}
	if add || count == 0 {
		if b, err := msgpack.Marshal(map[string]any{"count": count}); err == nil {
			tmp := path + ".tmp"
			if err := os.WriteFile(tmp, b, 0o644); err == nil { // #nosec G306 -- thanks counter file
				_ = os.Rename(tmp, path)
			} else {
				_ = os.Remove(tmp)
			}
		}
	}
	return count
}

// repositoryThanks mirrors the Python repository_thanks helper.
func (n *Node) repositoryThanks(repoPath string, add bool, linkID []byte) int64 {
	return thanksCount(repoPath+".thanks", add, linkID, &n.thanks)
}

// releaseThanks mirrors the Python release_thanks helper.
func (n *Node) releaseThanks(releaseDir string, add bool, linkID []byte) int64 {
	return thanksCount(releaseDir+"/THANKS", add, linkID, &n.thanks)
}
