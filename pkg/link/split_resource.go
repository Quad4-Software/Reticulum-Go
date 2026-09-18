// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/debug"
	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/Reticulum-Go/pkg/resource"
	"github.com/Quad4-Software/msgpack/v5/pkg/msgpack"
)

var (
	splitResourceMu      sync.Mutex
	splitResourceMem     = make(map[string]*splitResourceBuf)
	splitResourceBudget  = common.NewMemoryBudget(0)
	splitResourceMetaMem = make(map[string][]byte)
	splitAssemblies      = make(map[string]*splitAsm)
)

// maxSplitAssembliesPerLink bounds distinct in-flight split resources on one
// link. Upstream paces segments so effectively one assembly advances at a
// time; a small allowance covers pipelined senders without letting a peer
// accumulate unbounded staged state.
const maxSplitAssembliesPerLink = 4

// maxSplitAssembliesTotal bounds the tracker map across all links.
const maxSplitAssembliesTotal = 1024

// maxSplitAssemblyBytes is the hard ceiling on staged bytes per split
// resource regardless of advertised sizes. Upstream trusts the declared
// total implicitly; a finite bound keeps a hostile peer from appending
// unbounded data to memory or disk.
const maxSplitAssemblyBytes = 1 << 30

type splitResourceBuf struct {
	data []byte
}

// splitAsm tracks one in-flight split-resource assembly keyed by
// linkID:originalHash. Every segment advertisement declares the same
// total_size and total_segments, so the receiver can bound staged bytes by
// the declared total and require sequential indexes.
type splitAsm struct {
	nextIndex    uint16
	declaredSegs uint16
	declaredSize int64
	written      int64
	origHash     []byte
}

func (l *Link) useInMemoryResources() bool {
	if l == nil || l.transport == nil {
		return true
	}
	cfg := l.transport.GetConfig()
	if cfg == nil {
		return true
	}
	// Explicit InMemoryStorage or fully auto-ephemeral (no config/storage path).
	return cfg.InMemoryStorage || cfg.UseInMemoryStorage()
}

func (l *Link) resourceBudget() *common.MemoryBudget {
	limit := int64(0)
	if l != nil && l.transport != nil {
		if cfg := l.transport.GetConfig(); cfg != nil {
			limit = cfg.EffectiveMaxInMemoryResourceBytes()
		}
	}
	splitResourceBudget.SetLimit(limit)
	return splitResourceBudget
}

func (l *Link) resourceStorageDir() string {
	if l != nil && l.transport != nil {
		if cfg := l.transport.GetConfig(); cfg != nil && cfg.ConfigPath != "" && !cfg.UseInMemoryStorage() {
			return filepath.Join(filepath.Dir(cfg.ConfigPath), "storage", "resources")
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", "storage", "resources")
	}
	return filepath.Join(home, ".reticulum-go", "storage", "resources")
}

func (l *Link) resourceStoragePath(originalHash []byte) string {
	return filepath.Join(l.resourceStorageDir(), hex.EncodeToString(originalHash))
}

// handleSplitSegmentComplete appends a finished resource segment to durable
// storage or an in-memory buffer when InMemoryStorage is active. The
// application callback runs only after the last segment arrives. pending is
// the request receipt bound to this response transfer, if any.
func (l *Link) handleSplitSegmentComplete(payload []byte, adv *resource.ResourceAdvertisement, pending *RequestReceipt) error {
	if adv == nil || len(adv.OriginalHash) == 0 {
		return fmt.Errorf("split resource missing original hash")
	}
	key := l.splitResourceKey(adv.OriginalHash)
	if err := admitSplitSegment(key, adv, len(payload)); err != nil {
		return err
	}
	if l.useInMemoryResources() {
		return l.handleSplitSegmentInMemory(payload, adv, pending)
	}
	return l.handleSplitSegmentOnDisk(payload, adv, pending)
}

func (l *Link) splitResourceKey(originalHash []byte) string {
	linkPart := ""
	if l != nil && len(l.linkID) > 0 {
		linkPart = hex.EncodeToString(l.linkID)
	}
	return linkPart + ":" + hex.EncodeToString(originalHash)
}

// admitSplitSegment validates one completed segment against the assembly
// record for its linkID:originalHash key. Upstream concludes segments
// strictly in order (1..l) and repeats the same total_size/total_segments on
// every advertisement, so sequential indexes and a cumulative byte bound
// derived from the declared total reject replay, reordering, and append
// amplification without affecting honest transfers. A segment index of 1
// after a partial assembly means the sender restarted the transfer; the
// record resets and callers truncate previously staged bytes. The record is
// removed by finishSplitAssembly or dropSplitAssemblies.
func admitSplitSegment(key string, adv *resource.ResourceAdvertisement, payloadLen int) error {
	splitResourceMu.Lock()
	defer splitResourceMu.Unlock()

	asm, ok := splitAssemblies[key]
	if ok && adv.SegmentIndex == 1 {
		asm.nextIndex = 1
		asm.declaredSegs = adv.TotalSegments
		asm.declaredSize = adv.DataSize
		asm.written = 0
	}
	if !ok {
		if len(splitAssemblies) >= maxSplitAssembliesTotal {
			return fmt.Errorf("split resource assembly limit reached")
		}
		prefix := key[:len(key)-len(hex.EncodeToString(adv.OriginalHash))]
		count := 0
		for k := range splitAssemblies {
			if strings.HasPrefix(k, prefix) {
				count++
			}
		}
		if count >= maxSplitAssembliesPerLink {
			return fmt.Errorf("per-link split resource assembly limit reached")
		}
		if adv.SegmentIndex != 1 {
			return fmt.Errorf("split resource begins at segment %d, want 1", adv.SegmentIndex)
		}
		asm = &splitAsm{
			nextIndex:    1,
			declaredSegs: adv.TotalSegments,
			declaredSize: adv.DataSize,
			origHash:     append([]byte(nil), adv.OriginalHash...),
		}
		splitAssemblies[key] = asm
	}

	if adv.SegmentIndex != asm.nextIndex {
		return fmt.Errorf("split resource segment %d out of order, want %d", adv.SegmentIndex, asm.nextIndex)
	}
	if adv.TotalSegments != asm.declaredSegs {
		return fmt.Errorf("split resource segment count changed %d to %d", asm.declaredSegs, adv.TotalSegments)
	}
	// Peers compliant with upstream advertise data_size as the whole
	// resource total. Some implementations advertise only the current
	// segment body, so the segment count bound covers both conventions.
	limit := asm.declaredSize
	if segCap := int64(asm.declaredSegs) * int64(resource.MaxEfficientSize); segCap > limit {
		limit = segCap
	}
	if limit > maxSplitAssemblyBytes {
		limit = maxSplitAssemblyBytes
	}
	if asm.written+int64(payloadLen) > limit {
		return fmt.Errorf("split resource exceeds staged size limit %d", limit)
	}
	asm.written += int64(payloadLen)
	asm.nextIndex++
	return nil
}

// finishSplitAssembly drops the assembly record for key once the last
// segment completes.
func finishSplitAssembly(key string) {
	splitResourceMu.Lock()
	delete(splitAssemblies, key)
	splitResourceMu.Unlock()
}

// dropSplitAssemblies releases all staged split-resource state for this
// link, including partial on-disk files. Called once when the link closes.
func (l *Link) dropSplitAssemblies() {
	if l == nil {
		return
	}
	prefix := ""
	if len(l.linkID) > 0 {
		prefix = hex.EncodeToString(l.linkID) + ":"
	}
	var staged []string
	splitResourceMu.Lock()
	for k, asm := range splitAssemblies {
		if strings.HasPrefix(k, prefix) {
			delete(splitAssemblies, k)
			staged = append(staged, hex.EncodeToString(asm.origHash))
			if buf := splitResourceMem[k]; buf != nil {
				splitResourceBudget.Release(int64(len(buf.data)))
				delete(splitResourceMem, k)
			}
			if meta := splitResourceMetaMem[k]; meta != nil {
				splitResourceBudget.Release(int64(len(meta)))
				delete(splitResourceMetaMem, k)
			}
		}
	}
	splitResourceMu.Unlock()
	for _, hashHex := range staged {
		path := filepath.Join(l.resourceStorageDir(), hashHex)
		_ = os.Remove(path)
		_ = os.Remove(path + ".meta")
	}
}

func (l *Link) handleSplitSegmentInMemory(payload []byte, adv *resource.ResourceAdvertisement, pending *RequestReceipt) error {
	key := l.splitResourceKey(adv.OriginalHash)
	if adv.SegmentIndex == 1 {
		// Fresh transfer or restart: discard bytes and metadata staged by a
		// previous incomplete attempt at the same original hash.
		splitResourceMu.Lock()
		if buf, ok := splitResourceMem[key]; ok && buf != nil {
			l.resourceBudget().Release(int64(len(buf.data)))
			buf.data = nil
		}
		if prev, ok := splitResourceMetaMem[key]; ok {
			l.resourceBudget().Release(int64(len(prev)))
			delete(splitResourceMetaMem, key)
		}
		splitResourceMu.Unlock()
	}
	fileBytes := payload
	if adv.HasMetadata && adv.SegmentIndex == 1 {
		if len(payload) < 3 {
			return fmt.Errorf("split segment metadata too short")
		}
		metaSize := int(payload[0])<<16 | int(payload[1])<<8 | int(payload[2])
		if metaSize < 0 || 3+metaSize > len(payload) {
			return fmt.Errorf("split segment metadata size invalid")
		}
		meta := append([]byte(nil), payload[3:3+metaSize]...)
		if err := l.resourceBudget().TryReserve(int64(len(meta))); err != nil {
			return err
		}
		splitResourceMu.Lock()
		if prev, ok := splitResourceMetaMem[key]; ok {
			l.resourceBudget().Release(int64(len(prev)))
		}
		splitResourceMetaMem[key] = meta
		splitResourceMu.Unlock()
		fileBytes = payload[3+metaSize:]
	}

	if err := l.resourceBudget().TryReserve(int64(len(fileBytes))); err != nil {
		return err
	}

	splitResourceMu.Lock()
	buf, ok := splitResourceMem[key]
	if !ok {
		buf = &splitResourceBuf{}
		splitResourceMem[key] = buf
	}
	buf.data = append(buf.data, fileBytes...)
	splitResourceMu.Unlock()

	if adv.SegmentIndex < adv.TotalSegments {
		debug.Log(debug.DebugInfo, "Resource segment received waiting for next",
			"segment", adv.SegmentIndex,
			"total", adv.TotalSegments,
			"original", fmt.Sprintf("%x", adv.OriginalHash),
			"storage", "memory")
		return nil
	}

	splitResourceMu.Lock()
	data := append([]byte(nil), buf.data...)
	metaRaw := splitResourceMetaMem[key]
	delete(splitResourceMem, key)
	delete(splitResourceMetaMem, key)
	splitResourceMu.Unlock()
	finishSplitAssembly(key)

	l.resourceBudget().Release(int64(len(data)))
	if len(metaRaw) > 0 {
		l.resourceBudget().Release(int64(len(metaRaw)))
	}

	var metadata map[string]any
	if len(metaRaw) > 0 {
		_ = msgpack.Unmarshal(metaRaw, &metadata)
	}

	if adv.IsRequest {
		requestID := identity.TruncatedHash(data)
		return l.handleRequest(data, requestID)
	}

	if pending != nil {
		l.completeRequestWithResourcePayload(pending, data, metadata)
		return nil
	}

	if l.resourceConcludedCallback != nil {
		if metadata != nil {
			l.resourceConcludedCallback(IncomingResource{
				Data:     data,
				Metadata: metadata,
				Hash:     append([]byte(nil), adv.OriginalHash...),
			})
		} else {
			l.resourceConcludedCallback(data)
		}
	}
	return nil
}

func (l *Link) handleSplitSegmentOnDisk(payload []byte, adv *resource.ResourceAdvertisement, pending *RequestReceipt) error {
	dir := l.resourceStorageDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path := l.resourceStoragePath(adv.OriginalHash)
	metaPath := path + ".meta"

	fileBytes := payload
	if adv.SegmentIndex == 1 {
		// Fresh transfer or restart: staged bytes and metadata from a
		// previous incomplete attempt at this hash are discarded.
		_ = os.Remove(metaPath)
	}
	if adv.HasMetadata && adv.SegmentIndex == 1 {
		if len(payload) < 3 {
			return fmt.Errorf("split segment metadata too short")
		}
		metaSize := int(payload[0])<<16 | int(payload[1])<<8 | int(payload[2])
		if metaSize < 0 || 3+metaSize > len(payload) {
			return fmt.Errorf("split segment metadata size invalid")
		}
		if err := os.WriteFile(metaPath, payload[3:3+metaSize], 0o600); err != nil {
			return err
		}
		fileBytes = payload[3+metaSize:]
	}

	openFlags := os.O_CREATE | os.O_APPEND | os.O_WRONLY
	if adv.SegmentIndex == 1 {
		openFlags = os.O_CREATE | os.O_TRUNC | os.O_WRONLY
	}
	f, err := os.OpenFile(path, openFlags, 0o600) // #nosec G304
	if err != nil {
		return err
	}
	_, werr := f.Write(fileBytes)
	_ = f.Close()
	if werr != nil {
		return werr
	}

	if adv.SegmentIndex < adv.TotalSegments {
		debug.Log(debug.DebugInfo, "Resource segment received waiting for next",
			"segment", adv.SegmentIndex,
			"total", adv.TotalSegments,
			"original", fmt.Sprintf("%x", adv.OriginalHash))
		return nil
	}

	data, err := os.ReadFile(path) // #nosec G304
	if err != nil {
		return err
	}
	var metadata map[string]any
	if b, err := os.ReadFile(metaPath); err == nil { // #nosec G304
		_ = msgpack.Unmarshal(b, &metadata)
		_ = os.Remove(metaPath)
	}
	_ = os.Remove(path)
	finishSplitAssembly(l.splitResourceKey(adv.OriginalHash))

	if adv.IsRequest {
		requestID := identity.TruncatedHash(data)
		return l.handleRequest(data, requestID)
	}

	if pending != nil {
		l.completeRequestWithResourcePayload(pending, data, metadata)
		return nil
	}

	if l.resourceConcludedCallback != nil {
		if metadata != nil {
			l.resourceConcludedCallback(IncomingResource{
				Data:     data,
				Metadata: metadata,
				Hash:     append([]byte(nil), adv.OriginalHash...),
			})
		} else {
			l.resourceConcludedCallback(data)
		}
	}
	return nil
}

// resetSplitResourceMemoryForTest clears in-memory split staging. Tests only.
func resetSplitResourceMemoryForTest() {
	splitResourceMu.Lock()
	defer splitResourceMu.Unlock()
	for k, buf := range splitResourceMem {
		if buf != nil {
			splitResourceBudget.Release(int64(len(buf.data)))
		}
		delete(splitResourceMem, k)
	}
	for k, meta := range splitResourceMetaMem {
		splitResourceBudget.Release(int64(len(meta)))
		delete(splitResourceMetaMem, k)
	}
	clear(splitAssemblies)
}
