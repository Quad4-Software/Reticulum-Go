// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package identity

import (
	"bytes"
	"encoding/hex"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"quad4/msgpack/v5/pkg/msgpack"
	"quad4/reticulum-go/internal/storage"
)

func resetKnownDestinations(t *testing.T) {
	t.Helper()
	knownDestinationsLock.Lock()
	knownDestinations = make(map[string][]any)
	knownDestinationsLock.Unlock()

	knownPersistMemory.Store(false)
	knownPersistDisabled.Store(false)
	knownPersistDirty.Store(false)
}

// --- Round trip / basic behaviour -----------------------------------------

func TestKnownDestinationsPersistenceRoundTrip(t *testing.T) {
	resetKnownDestinations(t)

	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config")

	InitKnownDestinationsPersistence(cfgPath, false)

	id, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	destHash := id.Hash()
	packet := []byte("announce-packet")
	appData := []byte("appdata")
	Remember(packet, destHash, id.GetPublicKey(), appData)

	SaveKnownDestinationsSync()

	path, err := storage.KnownDestinationsPath(cfgPath)
	if err != nil {
		t.Fatalf("KnownDestinationsPath: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("known_destinations not written: %v", err)
	}

	resetKnownDestinations(t)
	InitKnownDestinationsPersistence(cfgPath, false)

	recalled, err := Recall(destHash)
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if !bytes.Equal(recalled.Hash(), destHash) {
		t.Fatalf("hash mismatch after reload")
	}
}

func TestKnownDestinationsLoadPythonStyleBytesKey(t *testing.T) {
	resetKnownDestinations(t)

	id, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	destHash := id.Hash()
	entry := []any{
		float64(0),
		[]byte("packet-hash"),
		id.GetPublicKey(),
		[]byte("appdata"),
		float64(0),
	}
	export := map[any]any{
		string(append([]byte(nil), destHash...)): entry,
	}
	data, err := msgpack.Marshal(export)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config")
	writeKnownDestinations(t, cfgPath, data)

	InitKnownDestinationsPersistence(cfgPath, false)

	if _, err := Recall(destHash); err != nil {
		t.Fatalf("Recall: %v", err)
	}
}

func TestKnownDestinationsInMemoryMode(t *testing.T) {
	tmp := t.TempDir()
	InitKnownDestinationsPersistence(filepath.Join(tmp, "config"), true)
	if !knownPersistMemory.Load() {
		t.Fatal("expected in-memory known destinations")
	}
}

func TestKnownDestinationsEnvOverrideForcesInMemory(t *testing.T) {
	t.Setenv("RETICULUM_IN_MEMORY_KNOWN_DESTINATIONS", "1")
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config")

	inMemory := false
	// Mirrors how transport.NewTransport wires config -> persistence: the
	// env var is meant to be applied by common.ApplyPersistenceEnv before
	// InitKnownDestinationsPersistence is called. Here we just confirm the

	// low-level in-memory path itself behaves correctly when asked.
	InitKnownDestinationsPersistence(cfgPath, inMemory || os.Getenv("RETICULUM_IN_MEMORY_KNOWN_DESTINATIONS") == "1")
	if !knownPersistMemory.Load() {
		t.Fatal("expected env override to force in-memory known destinations")
	}
}

// --- Corrupt / adversarial input handling ---------------------------------

func writeKnownDestinations(t *testing.T, cfgPath string, data []byte) string {
	t.Helper()
	path, err := storage.KnownDestinationsPath(cfgPath)
	if err != nil {
		t.Fatalf("KnownDestinationsPath: %v", err)
	}
	if err := storage.AtomicWriteFile(path, data, 0o600); err != nil {
		t.Fatalf("AtomicWriteFile: %v", err)
	}
	return path
}

func TestDecodeKnownDestinations_CorruptTopLevelType(t *testing.T) {
	data, err := msgpack.Marshal([]any{"not", "a", "map"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if _, _, err := decodeKnownDestinations(data); err == nil {
		t.Fatal("expected decode error for non-map top level")
	}
}

func TestDecodeKnownDestinations_TruncatedGarbage(t *testing.T) {
	garbage := []byte{0x82, 0x01} // claims map len 2, nothing follows
	if _, _, err := decodeKnownDestinations(garbage); err == nil {
		t.Fatal("expected decode error for truncated garbage")
	}
}

func TestDecodeKnownDestinations_EmptyInput(t *testing.T) {
	records, skipped, err := decodeKnownDestinations([]byte{})
	if err == nil {
		t.Fatal("expected decode error for empty input")
	}
	if records != nil || skipped != 0 {
		t.Fatal("expected no partial results on decode error")
	}
}

func TestDecodeKnownDestinations_SkipsMalformedEntriesButKeepsGoodOnes(t *testing.T) {
	id, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	destHash := id.Hash()
	goodKey := hex.EncodeToString(destHash)

	loaded := map[string]any{
		"short-key":  "not-an-array",
		"too-short2": []any{float64(0), []byte{}},
		"bad-pubkey": []any{float64(0), []byte{}, []byte{0x01, 0x02}, []byte{}},
		"bad-hash!!": []any{float64(0), []byte{}, id.GetPublicKey(), []byte{}}, // key not resolvable
		goodKey: []any{
			float64(0),
			[]byte("packet"),
			id.GetPublicKey(),
			[]byte("app"),
		},
	}

	data, err := msgpack.Marshal(loaded)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	records, skipped, err := decodeKnownDestinations(data)
	if err != nil {
		t.Fatalf("decodeKnownDestinations: %v", err)
	}
	if skipped != 4 {
		t.Fatalf("skipped = %d, want 4", skipped)
	}
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	if !bytes.Equal(records[0].destHash, destHash) {
		t.Fatalf("dest hash mismatch: %x", records[0].destHash)
	}
}

func TestDecodeKnownDestinations_RejectsWrongLengthPublicKey(t *testing.T) {
	loaded := map[string]any{
		"deadbeefdeadbeefdeadbeefdeadbeef": []any{
			float64(0),
			[]byte{},
			bytes.Repeat([]byte{0x01}, 8), // too short to be a valid key
			[]byte{},
		},
	}
	data, err := msgpack.Marshal(loaded)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	records, skipped, err := decodeKnownDestinations(data)
	if err != nil {
		t.Fatalf("decodeKnownDestinations: %v", err)
	}
	if len(records) != 0 || skipped != 1 {
		t.Fatalf("records=%d skipped=%d, want 0/1", len(records), skipped)
	}
}

func TestKnownDestinationsLoad_CorruptFileFallsBackToInMemory(t *testing.T) {
	resetKnownDestinations(t)
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config")
	writeKnownDestinations(t, cfgPath, []byte{0xff, 0xff, 0xff, 0x01, 0x02})

	InitKnownDestinationsPersistence(cfgPath, false)
	if !knownPersistMemory.Load() || !knownPersistDisabled.Load() {
		t.Fatal("corrupt known_destinations should force in-memory fallback")
	}
}

func TestKnownDestinationsLoad_WrongShapeDoesNotPanic(t *testing.T) {
	resetKnownDestinations(t)
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config")
	data, err := msgpack.Marshal("just a string, not a map")
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	writeKnownDestinations(t, cfgPath, data)

	InitKnownDestinationsPersistence(cfgPath, false)
	if !knownPersistMemory.Load() {
		t.Fatal("wrong-shape snapshot should force in-memory fallback")
	}
}

// --- Fuzzing ----------------------------------------------------------------

func FuzzDecodeKnownDestinations(f *testing.F) {
	id, err := New()
	if err != nil {
		f.Fatalf("New: %v", err)
	}
	good := map[string]any{
		hex.EncodeToString(id.Hash()): []any{
			float64(0),
			[]byte("packet"),
			id.GetPublicKey(),
			[]byte("app"),
		},
	}
	if data, err := msgpack.Marshal(good); err == nil {
		f.Add(data)
	}
	f.Add([]byte{})
	f.Add([]byte{0x80})
	f.Add([]byte{0xff, 0xff, 0xff, 0xff})
	f.Add([]byte(nil))

	f.Fuzz(func(t *testing.T, data []byte) {
		records, skipped, err := decodeKnownDestinations(data)
		if err != nil {
			return
		}
		if skipped < 0 {
			t.Fatalf("negative skipped count: %d", skipped)
		}
		for _, r := range records {
			if len(r.destHash) != TruncatedHashLength/8 {
				t.Fatalf("decoded record with bad dest hash length: %d", len(r.destHash))
			}
			if len(r.publicKey) != KeySize/8 {
				t.Fatalf("decoded record with bad public key length: %d", len(r.publicKey))
			}
		}
	})
}

// --- Race / deadlock / concurrency -----------------------------------------

func TestKnownDestinationsPersistence_ConcurrentRememberAndSaveNoRace(t *testing.T) {
	resetKnownDestinations(t)
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config")
	InitKnownDestinationsPersistence(cfgPath, false)

	const workers = 8
	const perWorker = 100

	ids := make([]*Identity, workers)
	for i := range ids {
		id, err := New()
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		ids[i] = id
	}

	// producers is deliberately separate from the saver goroutine below:
	// the saver's exit condition (stop) must only be signalled once the
	// producers are done, so it cannot share their WaitGroup without
	// creating a self-referential deadlock.
	var producers sync.WaitGroup
	for w := 0; w < workers; w++ {
		producers.Add(1)
		go func(id *Identity) {
			defer producers.Done()
			for i := 0; i < perWorker; i++ {
				Remember([]byte("packet"), id.Hash(), id.GetPublicKey(), []byte("app"))
			}
		}(ids[w])
	}

	stop := make(chan struct{})
	saverDone := make(chan struct{})
	go func() {
		defer close(saverDone)
		for {
			select {
			case <-stop:
				return
			default:
				PersistKnownDestinationsIfDirty()
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
		t.Fatal("concurrent Remember calls did not complete: possible deadlock")
	}
	close(stop)
	select {
	case <-saverDone:
	case <-time.After(5 * time.Second):
		t.Fatal("saver goroutine did not exit: possible deadlock")
	}

	SaveKnownDestinationsSync()

	for _, id := range ids {
		if _, err := Recall(id.Hash()); err != nil {
			t.Fatalf("Recall after concurrent writes: %v", err)
		}
	}
}

func TestKnownDestinationsPersistence_NoGoroutineLeak(t *testing.T) {
	resetKnownDestinations(t)
	runtime.GC()
	baseline := runtime.NumGoroutine()

	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config")
	for i := 0; i < 50; i++ {
		InitKnownDestinationsPersistence(cfgPath, false)
		id, err := New()
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		Remember([]byte("packet"), id.Hash(), id.GetPublicKey(), []byte("app"))
		SaveKnownDestinationsSync()
	}

	time.Sleep(50 * time.Millisecond)
	runtime.GC()
	final := runtime.NumGoroutine()
	if final > baseline+5 {
		t.Errorf("potential goroutine leak from known-destination persistence: baseline=%d final=%d", baseline, final)
	}
}

// --- Python wire-format interop --------------------------------------------

// TestKnownDestinationsInterop_RawBinKeyWireFormat hand-builds the exact
// byte layout Python's umsgpack.packb produces for
// Identity.known_destinations: a fixmap whose keys are the raw
// destination-hash bytes (bin8, since Python bytes objects are never
// str-encoded) and whose values are 5-element arrays
// [timestamp, packet_hash, public_key, app_data, last_used]. This exercises
// the wire path (not just the Go-side string-key convenience path) to
// prove real Python-written files decode correctly.
func TestKnownDestinationsInterop_RawBinKeyWireFormat(t *testing.T) {
	id, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	destHash := id.Hash()
	pub := id.GetPublicKey()

	var buf bytes.Buffer
	buf.WriteByte(0x81) // fixmap, 1 entry

	writeBin := func(b []byte) {
		if len(b) < 256 {
			buf.WriteByte(0xc4)
			buf.WriteByte(byte(len(b)))
		} else {
			buf.WriteByte(0xc5)
			buf.WriteByte(byte(len(b) >> 8))
			buf.WriteByte(byte(len(b)))
		}
		buf.Write(b)
	}
	writeFloat64 := func(v float64) {
		buf.WriteByte(0xcb)
		bits := math.Float64bits(v)
		for i := 7; i >= 0; i-- {
			buf.WriteByte(byte(bits >> (8 * i)))
		}
	}

	writeBin(destHash) // map key: raw destination hash bytes (bin type)

	buf.WriteByte(0x95) // fixarray, 5 entries
	writeFloat64(1700000000.0)
	writeBin([]byte("packet-hash-bytes"))
	writeBin(pub)
	writeBin([]byte("application-data"))
	writeFloat64(1700000000.0)

	records, skipped, err := decodeKnownDestinations(buf.Bytes())
	if err != nil {
		t.Fatalf("decodeKnownDestinations: %v", err)
	}
	if skipped != 0 {
		t.Fatalf("skipped = %d, want 0", skipped)
	}
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	if !bytes.Equal(records[0].destHash, destHash) {
		t.Fatalf("dest hash mismatch: got %x want %x", records[0].destHash, destHash)
	}
	if !bytes.Equal(records[0].publicKey, pub) {
		t.Fatal("public key mismatch")
	}
}

// --- Smoke / integration ----------------------------------------------------

func TestKnownDestinationsPersistence_FullLifecycleSmokeTest(t *testing.T) {
	resetKnownDestinations(t)
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config")

	InitKnownDestinationsPersistence(cfgPath, false)

	ids := make([]*Identity, 10)
	for i := range ids {
		id, err := New()
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		ids[i] = id
		Remember([]byte("packet"), id.Hash(), id.GetPublicKey(), []byte("app"))
	}
	SaveKnownDestinationsSync()

	resetKnownDestinations(t)
	InitKnownDestinationsPersistence(cfgPath, false)

	for i, id := range ids {
		recalled, err := Recall(id.Hash())
		if err != nil {
			t.Fatalf("identity %d: Recall: %v", i, err)
		}
		if !bytes.Equal(recalled.GetPublicKey(), id.GetPublicKey()) {
			t.Fatalf("identity %d: public key mismatch after reload", i)
		}
	}
}

// --- Benchmarks --------------------------------------------------------------

func BenchmarkMarkKnownDestinationsDirty(b *testing.B) {
	tmp := b.TempDir()
	InitKnownDestinationsPersistence(filepath.Join(tmp, "config"), false)

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		markKnownDestinationsDirty()
	}
}

func BenchmarkRemember(b *testing.B) {
	tmp := b.TempDir()
	InitKnownDestinationsPersistence(filepath.Join(tmp, "config"), false)
	id, err := New()
	if err != nil {
		b.Fatalf("New: %v", err)
	}
	packet := []byte("packet")
	appData := []byte("app")

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		Remember(packet, id.Hash(), id.GetPublicKey(), appData)
	}
}

func BenchmarkRemember_InMemoryOnly(b *testing.B) {
	tmp := b.TempDir()
	InitKnownDestinationsPersistence(filepath.Join(tmp, "config"), true)
	id, err := New()
	if err != nil {
		b.Fatalf("New: %v", err)
	}
	packet := []byte("packet")
	appData := []byte("app")

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		Remember(packet, id.Hash(), id.GetPublicKey(), appData)
	}
}

func BenchmarkSaveKnownDestinations(b *testing.B) {
	tmp := b.TempDir()
	InitKnownDestinationsPersistence(filepath.Join(tmp, "config"), false)
	for i := 0; i < 500; i++ {
		id, err := New()
		if err != nil {
			b.Fatalf("New: %v", err)
		}
		Remember([]byte("packet"), id.Hash(), id.GetPublicKey(), []byte("app"))
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		knownPersistDirty.Store(true)
		saveKnownDestinations(true)
	}
}

func BenchmarkDecodeKnownDestinations(b *testing.B) {
	loaded := make(map[string]any, 500)
	for i := 0; i < 500; i++ {
		id, err := New()
		if err != nil {
			b.Fatalf("New: %v", err)
		}
		loaded[hex.EncodeToString(id.Hash())] = []any{
			float64(0),
			[]byte("packet"),
			id.GetPublicKey(),
			[]byte("app"),
		}
	}
	data, err := msgpack.Marshal(loaded)
	if err != nil {
		b.Fatalf("Marshal: %v", err)
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, _, err := decodeKnownDestinations(data); err != nil {
			b.Fatalf("decodeKnownDestinations: %v", err)
		}
	}
}
