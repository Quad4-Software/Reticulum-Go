// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"testing"
)

func FuzzUnmarshalRateTable(f *testing.F) {
	valid := marshalRateTable(map[[16]byte]rateTableEntry{
		{1}: {last: 1.5, blocked: 0, viol: 0, timestamps: []float64{1, 2, 3}},
	})
	f.Add(valid)
	f.Add(valid[:20])
	f.Add([]byte("RNSART"))
	f.Fuzz(func(t *testing.T, blob []byte) {
		entries, ok := unmarshalRateTable(blob)
		if !ok {
			return
		}
		for k, e := range entries {
			if len(e.timestamps) > rateTableMaxTS {
				t.Fatalf("entry %x carries %d timestamps over cap", k, len(e.timestamps))
			}
		}
	})
}
