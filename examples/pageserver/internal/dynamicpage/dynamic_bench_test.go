// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package dynamicpage

import (
	"os"
	"path/filepath"
	"testing"
)

func BenchmarkReadOrExecuteStaticSmallMU(b *testing.B) {
	dir := b.TempDir()
	p := filepath.Join(dir, "page.mu")
	if err := os.WriteFile(p, []byte(">Title\n\nbody\n"), 0o644); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := ReadOrExecute(p, nil, nil, nil); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBuildScriptEnvEmptyData(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = buildScriptEnv(nil, nil, nil)
	}
}
