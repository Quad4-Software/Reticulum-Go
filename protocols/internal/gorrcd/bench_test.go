// SPDX-License-Identifier: LicenseRef-Reticulum
package gorrcd

import (
	"os"
	"path/filepath"
	"testing"
)

func BenchmarkKeyEqual(b *testing.B) {
	a := "s3cret-room-key"
	c := "s3cret-room-key"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !keyEqual(a, c) {
			b.Fatal("equal")
		}
	}
}

func BenchmarkParseIdentityHash(b *testing.B) {
	raw := "0123456789abcdef0123456789abcdef"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := parseIdentityHash(raw); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSplitCmd(b *testing.B) {
	line := "/mode lobby +k secret key"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = splitCmd(line)
	}
}

func BenchmarkAllowJoin(b *testing.B) {
	reg := NewRoomRegistry("", 900)
	founder := mustID(1)
	peer := mustID(2)
	reg.Ensure("bench", founder, true)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := reg.AllowJoin("bench", peer, nil, false); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkModeString(b *testing.B) {
	reg := NewRoomRegistry("", 900)
	f := mustID(1)
	reg.Ensure("x", f, true)
	_ = reg.SetFlag("x", "m", true, "")
	_ = reg.SetFlag("x", "n", true, "")
	_ = reg.SetFlag("x", "i", true, "")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = reg.ModeString("x")
	}
}

func BenchmarkStatsInc(b *testing.B) {
	s := NewStats()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Inc("joins", 1)
	}
}

func BenchmarkIDFrom(b *testing.B) {
	raw := mustPeer(9)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ok := idFrom(raw); !ok {
			b.Fatal("id")
		}
	}
}

func BenchmarkAtomicWrite(b *testing.B) {
	dir := b.TempDir()
	path := filepath.Join(dir, "bench")
	data := []byte("hello gorrcd")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := atomicWrite(path, data, 0o600); err != nil {
			b.Fatal(err)
		}
	}
	_ = os.Remove(path)
}
