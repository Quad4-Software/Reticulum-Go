// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"testing"
	"time"

	"quad4/msgpack/v5/pkg/msgpack"
)

func TestDecodePathTableEntriesSkipsStaleLastUpdated(t *testing.T) {
	now := time.Now()
	dest := make([]byte, 16)
	nextHop := make([]byte, 16)
	oldTS := float64(now.Add(-time.Duration(PathRequestTTL+60) * time.Second).Unix())
	entry := []any{dest, oldTS, nextHop, uint8(1), float64(0), []any{}, interfacePersistKey("wan"), []byte{}}
	data, err := msgpack.Marshal([]any{entry})
	if err != nil {
		t.Fatal(err)
	}
	records, _, err := decodePathTableEntries(data, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 0 {
		t.Fatalf("expected 0 records, got %d", len(records))
	}
}
