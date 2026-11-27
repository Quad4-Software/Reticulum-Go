// SPDX-License-Identifier: LicenseRef-Reticulum
package stamps

import (
	"context"
	"testing"
)

// Workers must be joined before GenerateStampedKey returns, so callers can
// wipe material without racing in-flight worker copies. Run under -race.
func TestGenerateStampedKeyJoinsWorkers(t *testing.T) {
	for i := 0; i < 20; i++ {
		material := make([]byte, 64)
		for j := range material {
			material[j] = byte(i + j)
		}
		_, _, _, err := GenerateStampedKey(context.Background(), material, 2)
		if err != nil {
			t.Fatalf("iter %d: %v", i, err)
		}
		for j := range material {
			material[j] = 0
		}
	}
}

func TestGenerateStampJoinsWorkers(t *testing.T) {
	for i := 0; i < 20; i++ {
		material := make([]byte, 64)
		for j := range material {
			material[j] = byte(i + j)
		}
		if _, _, err := GenerateStamp(context.Background(), material, 2, 32); err != nil {
			t.Fatalf("iter %d: %v", i, err)
		}
		for j := range material {
			material[j] = 0
		}
	}
}
