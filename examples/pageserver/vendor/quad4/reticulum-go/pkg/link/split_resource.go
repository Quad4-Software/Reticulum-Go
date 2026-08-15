// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"quad4/msgpack/v5/pkg/msgpack"
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/resource"
)

var (
	splitResourceMu      sync.Mutex
	splitResourceMem     = make(map[string]*splitResourceBuf)
	splitResourceBudget  = common.NewMemoryBudget(0)
	splitResourceMetaMem = make(map[string][]byte)
)

type splitResourceBuf struct {
	data []byte
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
// application callback runs only after the last segment arrives.
func (l *Link) handleSplitSegmentComplete(payload []byte, adv *resource.ResourceAdvertisement) error {
	if adv == nil || len(adv.OriginalHash) == 0 {
		return fmt.Errorf("split resource missing original hash")
	}
	if l.useInMemoryResources() {
		return l.handleSplitSegmentInMemory(payload, adv)
	}
	return l.handleSplitSegmentOnDisk(payload, adv)
}

func (l *Link) handleSplitSegmentInMemory(payload []byte, adv *resource.ResourceAdvertisement) error {
	key := hex.EncodeToString(adv.OriginalHash)
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

	l.incomingMu.Lock()
	pending := l.incomingResourceRequest
	l.incomingResourceRequest = nil
	l.incomingMu.Unlock()
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

func (l *Link) handleSplitSegmentOnDisk(payload []byte, adv *resource.ResourceAdvertisement) error {
	dir := l.resourceStorageDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path := l.resourceStoragePath(adv.OriginalHash)
	metaPath := path + ".meta"

	fileBytes := payload
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

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600) // #nosec G304
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

	if adv.IsRequest {
		requestID := identity.TruncatedHash(data)
		return l.handleRequest(data, requestID)
	}

	l.incomingMu.Lock()
	pending := l.incomingResourceRequest
	l.incomingResourceRequest = nil
	l.incomingMu.Unlock()
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
}
