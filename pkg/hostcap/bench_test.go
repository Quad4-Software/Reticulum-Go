// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package hostcap

import (
	"context"
	"testing"
	"time"
)

func BenchmarkMemCopyProbe(b *testing.B) {
	ctx := context.Background()
	b.SetBytes(copyChunkBytes)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = measureMemCopy(ctx, 10*time.Millisecond)
	}
}

func BenchmarkCPUProbe(b *testing.B) {
	ctx := context.Background()
	b.SetBytes(cpuChunkBytes)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = measureCPU(ctx, 10*time.Millisecond)
	}
}

func BenchmarkFullProbe(b *testing.B) {
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Probe(ctx, 50*time.Millisecond)
	}
}
