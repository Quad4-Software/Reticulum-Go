// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package identity

import (
	"sync"
	"testing"
)

// TestGetCurrentRatchetKeyFromEmptyMapConcurrent covers the unlock-then-rotate
// path when ratchets are empty.
func TestGetCurrentRatchetKeyFromEmptyMapConcurrent(t *testing.T) {
	id, err := New()
	if err != nil {
		t.Fatal(err)
	}
	id.mutex.Lock()
	for k, buf := range id.ratchets {
		if buf != nil {
			_ = buf.Close()
		}
		delete(id.ratchets, k)
		delete(id.ratchetExpiry, k)
	}
	id.mutex.Unlock()

	var wg sync.WaitGroup
	errs := make(chan string, 32)
	for range 32 {
		wg.Go(func() {
			k := id.GetCurrentRatchetKey()
			if len(k) == 0 {
				errs <- "nil or empty ratchet"
			}
		})
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Fatal(e)
	}
}
