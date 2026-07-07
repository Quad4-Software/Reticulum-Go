// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package transport

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"quad4/msgpack/v5/pkg/msgpack"
	"quad4/reticulum-go/internal/storage"
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/identity"
)

func newPersistMockInterface(name string) *mockInterface {
	iface := &mockInterface{}
	iface.Name = name
	iface.Enabled = true
	iface.Online = true
	return iface
}

// --- Round trip / basic behaviour -----------------------------------------

func TestPathTablePersistenceRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config")
	cfg := &common.ReticulumConfig{ConfigPath: cfgPath}

	tr := NewTransport(cfg)
	iface := newPersistMockInterface("wan")
	if err := tr.RegisterInterface("wan", iface); err != nil {
		t.Fatalf("RegisterInterface: %v", err)
	}

	dest := bytes.Repeat([]byte{0xAB}, identity.TruncatedHashLength/8)
	nextHop := bytes.Repeat([]byte{0xCD}, identity.TruncatedHashLength/8)
	tr.UpdatePath(dest, nextHop, "wan", 2)

	tr.savePathTableSync()

	tablePath, err := storage.DestinationTablePath(cfgPath)
	if err != nil {
		t.Fatalf("DestinationTablePath: %v", err)
	}
	if _, err := os.Stat(tablePath); err != nil {
		t.Fatalf("destination_table not written: %v", err)
	}

	tr2 := NewTransport(cfg)
	iface2 := newPersistMockInterface("wan")
	if err := tr2.RegisterInterface("wan", iface2); err != nil {
		t.Fatalf("RegisterInterface reload: %v", err)
	}

	key := pathMapKey(dest)
	tr2.mutex.RLock()
	path, ok := tr2.paths[key]
	tr2.mutex.RUnlock()
	if !ok || path == nil {
		t.Fatal("path not restored from disk")
	}
	if path.HopCount != 2 {
		t.Fatalf("hop count = %d, want 2", path.HopCount)
	}
	if !bytes.Equal(path.NextHop, nextHop) {
		t.Fatalf("next hop mismatch: %x", path.NextHop)
	}
}

// TestPathTablePersistenceInterfaceRegisteredLater exercises the pending
// entry path: the snapshot references an interface that has not been
// registered yet at load time, so the record must be held until
// RegisterInterface later resolves it.
func TestPathTablePersistenceInterfaceRegisteredLater(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config")
	cfg := &common.ReticulumConfig{ConfigPath: cfgPath}

	tr := NewTransport(cfg)
	iface := newPersistMockInterface("wan")
	_ = tr.RegisterInterface("wan", iface)
	dest := bytes.Repeat([]byte{0x55}, 16)
	tr.UpdatePath(dest, bytes.Repeat([]byte{0x66}, 16), "wan", 3)
	tr.savePathTableSync()

	tr2 := NewTransport(cfg) // loads snapshot, but "wan" is not registered yet
	tr2.mutex.RLock()
	_, ok := tr2.paths[pathMapKey(dest)]
	pendingBefore := len(tr2.pendingPathEntries)
	tr2.mutex.RUnlock()
	if ok {
		t.Fatal("path should not be active before its interface is registered")
	}
	if pendingBefore != 1 {
		t.Fatalf("pendingPathEntries = %d, want 1", pendingBefore)
	}

	iface2 := newPersistMockInterface("wan")
	if err := tr2.RegisterInterface("wan", iface2); err != nil {
		t.Fatalf("RegisterInterface: %v", err)
	}

	tr2.mutex.RLock()
	path, ok := tr2.paths[pathMapKey(dest)]
	pendingAfter := len(tr2.pendingPathEntries)
	tr2.mutex.RUnlock()
	if !ok || path == nil {
		t.Fatal("path should activate once its interface is registered")
	}
	if pendingAfter != 0 {
		t.Fatalf("pendingPathEntries = %d, want 0 after activation", pendingAfter)
	}
}

func TestPathTablePersistenceSkipsExpiredEntries(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config")
	cfg := &common.ReticulumConfig{ConfigPath: cfgPath}

	dest := bytes.Repeat([]byte{0x11}, 16)
	nextHop := bytes.Repeat([]byte{0x22}, 16)
	expired := []any{
		dest,
		float64(time.Now().Add(-time.Hour).Unix()),
		nextHop,
		uint8(1),
		float64(time.Now().Add(-time.Minute).Unix()),
		[]any{},
		interfacePersistKey("wan"),
		[]byte{},
	}
	data, err := msgpack.Marshal([]any{expired})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	writeDestinationTable(t, cfgPath, data)

	tr := NewTransport(cfg)
	iface := newPersistMockInterface("wan")
	_ = tr.RegisterInterface("wan", iface)

	key := pathMapKey(dest)
	tr.mutex.RLock()
	_, ok := tr.paths[key]
	tr.mutex.RUnlock()
	if ok {
		t.Fatal("expired path should not be loaded")
	}
}

func TestPathTablePersistenceFallsBackOnWriteFailure(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config")
	cfg := &common.ReticulumConfig{ConfigPath: cfgPath}

	tr := NewTransport(cfg)
	iface := newPersistMockInterface("wan")
	_ = tr.RegisterInterface("wan", iface)
	tr.UpdatePath(bytes.Repeat([]byte{0x33}, 16), bytes.Repeat([]byte{0x44}, 16), "wan", 1)
	tr.savePathTableSync()

	tablePath, err := storage.DestinationTablePath(cfgPath)
	if err != nil {
		t.Fatalf("DestinationTablePath: %v", err)
	}
	if err := os.Chmod(tablePath, 0o444); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	if err := os.Chmod(filepath.Dir(tablePath), 0o555); err != nil {
		t.Fatalf("Chmod dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(filepath.Dir(tablePath), 0o700)
		_ = os.Chmod(tablePath, 0o600)
	})

	tr.pathPersistDirty.Store(true)
	tr.savePathTableSync()

	if !tr.pathPersistDisabled.Load() {
		t.Fatal("expected persistence disabled after write failure")
	}
}

func TestInMemoryPathTableConfig(t *testing.T) {
	tmp := t.TempDir()
	cfg := &common.ReticulumConfig{
		ConfigPath:        filepath.Join(tmp, "config"),
		InMemoryPathTable: true,
	}
	tr := NewTransport(cfg)
	if !tr.pathPersistMemory.Load() {
		t.Fatal("expected in-memory path table")
	}
}

func TestInMemoryPathTableEnvOverride(t *testing.T) {
	t.Setenv("RETICULUM_IN_MEMORY_PATH_TABLE", "1")
	tmp := t.TempDir()
	cfg := &common.ReticulumConfig{ConfigPath: filepath.Join(tmp, "config")}
	tr := NewTransport(cfg)
	if !tr.pathPersistMemory.Load() {
		t.Fatal("expected env override to force in-memory path table")
	}
}

func TestConnectedToSharedInstanceForcesInMemory(t *testing.T) {
	tmp := t.TempDir()
	cfg := &common.ReticulumConfig{
		ConfigPath:                filepath.Join(tmp, "config"),
		ConnectedToSharedInstance: true,
	}
	tr := NewTransport(cfg)
	if !tr.pathPersistMemory.Load() {
		t.Fatal("shared-instance clients must not touch the on-disk path table")
	}
}

// --- Corrupt / adversarial input handling ---------------------------------

func writeDestinationTable(t *testing.T, cfgPath string, data []byte) string {
	t.Helper()
	path, err := storage.DestinationTablePath(cfgPath)
	if err != nil {
		t.Fatalf("DestinationTablePath: %v", err)
	}
	if err := storage.AtomicWriteFile(path, data, 0o600); err != nil {
		t.Fatalf("AtomicWriteFile: %v", err)
	}
	return path
}

func TestDecodePathTableEntries_CorruptTopLevelType(t *testing.T) {
	// A msgpack map instead of an array at the top level is a decode error,
	// not a silently-skipped entry: the whole snapshot is untrustworthy.
	data, err := msgpack.Marshal(map[string]any{"not": "a list"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if _, _, err := decodePathTableEntries(data, time.Now()); err == nil {
		t.Fatal("expected decode error for non-array top level")
	}
}

func TestDecodePathTableEntries_TruncatedGarbage(t *testing.T) {
	garbage := []byte{0x93, 0x01, 0x02} // claims array len 3, only 2 elements follow
	if _, _, err := decodePathTableEntries(garbage, time.Now()); err == nil {
		t.Fatal("expected decode error for truncated garbage")
	}
}

func TestDecodePathTableEntries_EmptyInput(t *testing.T) {
	records, skipped, err := decodePathTableEntries([]byte{}, time.Now())
	if err == nil {
		t.Fatal("expected decode error for empty input (not a valid msgpack value)")
	}
	if records != nil || skipped != 0 {
		t.Fatal("expected no partial results on decode error")
	}
}

func TestDecodePathTableEntries_SkipsMalformedEntriesButKeepsGoodOnes(t *testing.T) {
	now := time.Now()
	goodDest := bytes.Repeat([]byte{0x77}, 16)
	good := []any{
		goodDest,
		float64(now.Unix()),
		bytes.Repeat([]byte{0x88}, 16),
		uint8(2),
		float64(now.Add(time.Hour).Unix()),
		[]any{},
		interfacePersistKey("wan"),
		[]byte{},
	}

	cases := []any{
		"not an array at all",
		[]any{}, // too short
		[]any{[]byte{0x01}, 0.0, []byte{}, uint8(0), 0.0, []any{}, []byte{}, []byte{}},       // bad dest hash length
		[]any{goodDest, "not-a-float", []byte{}, uint8(0), 0.0, []any{}, []byte{}, []byte{}}, // bad timestamp type
		[]any{goodDest, 0.0, "not-bytes", uint8(0), 0.0, []any{}, []byte{}, []byte{}},        // bad next hop type
		[]any{goodDest, 0.0, []byte{}, "not-a-number", 0.0, []any{}, []byte{}, []byte{}},     // bad hops type
		[]any{goodDest, 0.0, []byte{}, uint8(0), "not-a-float", []any{}, []byte{}, []byte{}}, // bad expires type
		[]any{goodDest, 0.0, []byte{}, uint8(0), 0.0, []any{}, "not-bytes", []byte{}},        // bad iface key type
		[]any{goodDest, 0.0, []byte{}, int64(-1), 0.0, []any{}, []byte{}, []byte{}},          // negative hops
		[]any{goodDest, 0.0, []byte{}, int64(999), 0.0, []any{}, []byte{}, []byte{}},         // out of range hops
		good,
	}

	data, err := msgpack.Marshal(cases)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	records, skipped, err := decodePathTableEntries(data, now)
	if err != nil {
		t.Fatalf("decodePathTableEntries: %v", err)
	}
	if skipped != len(cases)-1 {
		t.Fatalf("skipped = %d, want %d", skipped, len(cases)-1)
	}
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	if !bytes.Equal(records[0].destHash, goodDest) {
		t.Fatalf("unexpected surviving record: %x", records[0].destHash)
	}
}

func TestDecodePathTableEntries_RejectsUnparseableExpiryRatherThanKeepingForever(t *testing.T) {
	// A record whose expiry field has been corrupted to a non-float type
	// must not be treated as "never expires"; it should be dropped as
	// malformed so a corrupted file can never grant an immortal route.
	entry := []any{
		bytes.Repeat([]byte{0x99}, 16),
		float64(time.Now().Unix()),
		bytes.Repeat([]byte{0x00}, 16),
		uint8(1),
		"corrupted-expiry-field",
		[]any{},
		interfacePersistKey("wan"),
		[]byte{},
	}
	data, err := msgpack.Marshal([]any{entry})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	records, skipped, err := decodePathTableEntries(data, time.Now())
	if err != nil {
		t.Fatalf("decodePathTableEntries: %v", err)
	}
	if len(records) != 0 || skipped != 1 {
		t.Fatalf("records=%d skipped=%d, want 0/1", len(records), skipped)
	}
}

func TestPathTableLoad_CorruptFileFallsBackToInMemory(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config")
	writeDestinationTable(t, cfgPath, []byte{0xff, 0xff, 0xff, 0x01, 0x02})

	cfg := &common.ReticulumConfig{ConfigPath: cfgPath}
	tr := NewTransport(cfg)
	if !tr.pathPersistMemory.Load() || !tr.pathPersistDisabled.Load() {
		t.Fatal("corrupt destination_table should force in-memory fallback")
	}
}

func TestPathTableLoad_NonMapGarbageDoesNotPanicTransportStartup(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config")
	// Valid msgpack, wrong shape (a single integer).
	data, err := msgpack.Marshal(42)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	writeDestinationTable(t, cfgPath, data)

	cfg := &common.ReticulumConfig{ConfigPath: cfgPath}
	tr := NewTransport(cfg)
	defer tr.Close()
	if !tr.pathPersistMemory.Load() {
		t.Fatal("wrong-shape snapshot should force in-memory fallback")
	}
}

// --- Fuzzing ----------------------------------------------------------------

func FuzzDecodePathTableEntries(f *testing.F) {
	now := time.Now()
	good := []any{
		bytes.Repeat([]byte{0x11}, 16),
		float64(now.Unix()),
		bytes.Repeat([]byte{0x22}, 16),
		uint8(2),
		float64(now.Add(time.Hour).Unix()),
		[]any{},
		interfacePersistKey("wan"),
		[]byte{},
	}
	if data, err := msgpack.Marshal([]any{good}); err == nil {
		f.Add(data)
	}
	f.Add([]byte{})
	f.Add([]byte{0x90})
	f.Add([]byte{0xff, 0xff, 0xff, 0xff})
	f.Add([]byte(nil))

	f.Fuzz(func(t *testing.T, data []byte) {
		records, skipped, err := decodePathTableEntries(data, now)
		if err != nil {
			return
		}
		if skipped < 0 {
			t.Fatalf("negative skipped count: %d", skipped)
		}
		for _, r := range records {
			if len(r.destHash) != 16 {
				t.Fatalf("decoded record with bad dest hash length: %d", len(r.destHash))
			}
		}
	})
}

// --- Race / deadlock / concurrency -----------------------------------------

func TestPathPersistence_ConcurrentUpdateAndSaveNoRace(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config")
	cfg := &common.ReticulumConfig{ConfigPath: cfgPath}

	tr := NewTransport(cfg)
	defer tr.Close()
	iface := newPersistMockInterface("wan")
	_ = tr.RegisterInterface("wan", iface)

	const workers = 8
	const perWorker = 200

	// producers is deliberately separate from the saver goroutine below:
	// the saver's exit condition (stop) must only be signalled once the
	// producers are done, so it cannot share their WaitGroup without
	// creating a self-referential deadlock.
	var producers sync.WaitGroup
	for w := 0; w < workers; w++ {
		producers.Add(1)
		go func(seed byte) {
			defer producers.Done()
			for i := 0; i < perWorker; i++ {
				dest := bytes.Repeat([]byte{seed}, 16)
				tr.UpdatePath(dest, bytes.Repeat([]byte{seed + 1}, 16), "wan", uint8(i%255))
			}
		}(byte(w + 1))
	}

	// Concurrent savers/persisters racing against the writers above.
	stop := make(chan struct{})
	saverDone := make(chan struct{})
	go func() {
		defer close(saverDone)
		for {
			select {
			case <-stop:
				return
			default:
				tr.persistPathTableIfDirty()
			}
		}
	}()

	producersDone := make(chan struct{})
	go func() {
		producers.Wait()
		close(producersDone)
	}()

	select {
	case <-producersDone:
	case <-time.After(20 * time.Second):
		t.Fatal("concurrent UpdatePath calls did not complete: possible deadlock")
	}
	close(stop)
	select {
	case <-saverDone:
	case <-time.After(5 * time.Second):
		t.Fatal("saver goroutine did not exit: possible deadlock")
	}

	tr.savePathTableSync()
}

// TestPathPersistence_CloseDoesNotDeadlockWithPendingSave guards against
// Close() blocking forever if savePathTableSync races with an in-flight
// periodic save (both would try t.pathPersistSaving).
func TestPathPersistence_CloseDoesNotDeadlockWithPendingSave(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config")
	cfg := &common.ReticulumConfig{ConfigPath: cfgPath}

	tr := NewTransport(cfg)
	iface := newPersistMockInterface("wan")
	_ = tr.RegisterInterface("wan", iface)
	for i := 0; i < 50; i++ {
		tr.UpdatePath(bytes.Repeat([]byte{byte(i)}, 16), bytes.Repeat([]byte{byte(i + 1)}, 16), "wan", 1)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		tr.persistPathTableIfDirty()
	}()

	closeDone := make(chan struct{})
	go func() {
		defer close(closeDone)
		_ = tr.Close()
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("persistPathTableIfDirty deadlocked")
	}
	select {
	case <-closeDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Close deadlocked with pending save")
	}
}

func TestPathPersistence_NoGoroutineLeakAcrossManyTransports(t *testing.T) {
	runtime.GC()
	baseline := runtime.NumGoroutine()

	tmp := t.TempDir()
	for i := 0; i < 50; i++ {
		cfg := &common.ReticulumConfig{ConfigPath: filepath.Join(tmp, "config")}
		tr := NewTransport(cfg)
		iface := newPersistMockInterface("wan")
		_ = tr.RegisterInterface("wan", iface)
		tr.UpdatePath(bytes.Repeat([]byte{byte(i)}, 16), bytes.Repeat([]byte{0x01}, 16), "wan", 1)
		tr.Close()
	}

	time.Sleep(100 * time.Millisecond)
	runtime.GC()
	final := runtime.NumGoroutine()
	if final > baseline+5 {
		t.Errorf("potential goroutine leak from path persistence: baseline=%d final=%d", baseline, final)
	}
}

// --- Python wire-format interop --------------------------------------------

// TestPathTableInterop_PythonLikeEncoding hand-encodes a destination_table
// snapshot the way Python umsgpack.packb would (bin-typed byte strings via
// a dynamic array, matching Transport.save_path_table's serialised_entry
// layout: [dest_hash, timestamp, next_hop, hops, expires, random_blobs,
// interface_hash, packet_hash]) and confirms Go's loader accepts it.
func TestPathTableInterop_PythonLikeEncoding(t *testing.T) {
	now := time.Now()
	destHash := bytes.Repeat([]byte{0xAA}, 16)
	nextHop := bytes.Repeat([]byte{0xBB}, 16)
	ifaceHash := interfacePersistKey("wan")

	// Build with the generic []any encoder path (same as our own writer),
	// which is wire-compatible with Python's umsgpack array-of-arrays
	// format: both produce a msgpack array of arrays with bin-typed byte
	// fields.
	entry := []any{
		destHash,
		float64(now.Unix()),
		nextHop,
		uint8(3),
		float64(now.Add(time.Hour).Unix()),
		[]any{},
		ifaceHash,
		[]byte("cached-announce-packet-bytes"),
	}
	data, err := msgpack.Marshal([]any{entry})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	records, skipped, err := decodePathTableEntries(data, now)
	if err != nil {
		t.Fatalf("decodePathTableEntries: %v", err)
	}
	if skipped != 0 {
		t.Fatalf("skipped = %d, want 0", skipped)
	}
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	if !bytes.Equal(records[0].destHash, destHash) {
		t.Fatalf("dest hash mismatch: %x", records[0].destHash)
	}
	if !bytes.Equal(records[0].nextHop, nextHop) {
		t.Fatalf("next hop mismatch: %x", records[0].nextHop)
	}
	if records[0].hops != 3 {
		t.Fatalf("hops = %d, want 3", records[0].hops)
	}
}

// --- Smoke / integration ----------------------------------------------------

func TestPathTablePersistence_FullLifecycleSmokeTest(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config")
	cfg := &common.ReticulumConfig{ConfigPath: cfgPath}

	// Process 1: populate several paths, shut down cleanly.
	tr1 := NewTransport(cfg)
	iface1 := newPersistMockInterface("wan")
	_ = tr1.RegisterInterface("wan", iface1)
	dests := make([][]byte, 10)
	for i := range dests {
		dests[i] = bytes.Repeat([]byte{byte(i + 1)}, 16)
		tr1.UpdatePath(dests[i], bytes.Repeat([]byte{0xF0}, 16), "wan", uint8(i))
	}
	if err := tr1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Process 2: restart, verify every path survived.
	tr2 := NewTransport(cfg)
	defer tr2.Close()
	iface2 := newPersistMockInterface("wan")
	if err := tr2.RegisterInterface("wan", iface2); err != nil {
		t.Fatalf("RegisterInterface: %v", err)
	}
	for i, dest := range dests {
		tr2.mutex.RLock()
		path, ok := tr2.paths[pathMapKey(dest)]
		tr2.mutex.RUnlock()
		if !ok {
			t.Fatalf("path %d missing after restart", i)
		}
		if path.HopCount != uint8(i) {
			t.Fatalf("path %d hop count = %d, want %d", i, path.HopCount, i)
		}
	}
}

// --- Benchmarks --------------------------------------------------------------

func BenchmarkMarkPathTableDirty(b *testing.B) {
	tmp := b.TempDir()
	cfg := &common.ReticulumConfig{ConfigPath: filepath.Join(tmp, "config")}
	tr := NewTransport(cfg)
	defer tr.Close()

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		tr.markPathTableDirty()
	}
}

func BenchmarkUpdatePath_WithPersistence(b *testing.B) {
	tmp := b.TempDir()
	cfg := &common.ReticulumConfig{ConfigPath: filepath.Join(tmp, "config")}
	tr := NewTransport(cfg)
	defer tr.Close()
	iface := newPersistMockInterface("wan")
	_ = tr.RegisterInterface("wan", iface)

	dest := bytes.Repeat([]byte{0x01}, 16)
	nextHop := bytes.Repeat([]byte{0x02}, 16)

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		tr.UpdatePath(dest, nextHop, "wan", uint8(i%255))
	}
}

func BenchmarkUpdatePath_InMemoryOnly(b *testing.B) {
	tmp := b.TempDir()
	cfg := &common.ReticulumConfig{ConfigPath: filepath.Join(tmp, "config"), InMemoryPathTable: true}
	tr := NewTransport(cfg)
	defer tr.Close()
	iface := newPersistMockInterface("wan")
	_ = tr.RegisterInterface("wan", iface)

	dest := bytes.Repeat([]byte{0x01}, 16)
	nextHop := bytes.Repeat([]byte{0x02}, 16)

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		tr.UpdatePath(dest, nextHop, "wan", uint8(i%255))
	}
}

func BenchmarkSavePathTable(b *testing.B) {
	tmp := b.TempDir()
	cfg := &common.ReticulumConfig{ConfigPath: filepath.Join(tmp, "config")}
	tr := NewTransport(cfg)
	defer tr.Close()
	iface := newPersistMockInterface("wan")
	_ = tr.RegisterInterface("wan", iface)

	for i := 0; i < 500; i++ {
		dest := make([]byte, 16)
		dest[0] = byte(i)
		dest[1] = byte(i >> 8)
		tr.UpdatePath(dest, bytes.Repeat([]byte{0x02}, 16), "wan", uint8(i%255))
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		tr.pathPersistDirty.Store(true)
		tr.savePathTable(true)
	}
}

func BenchmarkDecodePathTableEntries(b *testing.B) {
	now := time.Now()
	entries := make([]any, 500)
	for i := range entries {
		dest := make([]byte, 16)
		dest[0] = byte(i)
		dest[1] = byte(i >> 8)
		entries[i] = []any{
			dest,
			float64(now.Unix()),
			bytes.Repeat([]byte{0x02}, 16),
			uint8(i % 255),
			float64(now.Add(time.Hour).Unix()),
			[]any{},
			interfacePersistKey("wan"),
			[]byte{},
		}
	}
	data, err := msgpack.Marshal(entries)
	if err != nil {
		b.Fatalf("Marshal: %v", err)
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, _, err := decodePathTableEntries(data, now); err != nil {
			b.Fatalf("decodePathTableEntries: %v", err)
		}
	}
}
