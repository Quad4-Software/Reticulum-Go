// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"quad4/msgpack/v5/pkg/msgpack"
	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/resource"
)

func (l *Link) resourceStorageDir() string {
	if l != nil && l.transport != nil {
		if cfg := l.transport.GetConfig(); cfg != nil && cfg.ConfigPath != "" {
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
// storage. The application callback runs only after the last segment arrives.
func (l *Link) handleSplitSegmentComplete(payload []byte, adv *resource.ResourceAdvertisement) error {
	if adv == nil || len(adv.OriginalHash) == 0 {
		return fmt.Errorf("split resource missing original hash")
	}
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
		requestID := append([]byte(nil), adv.Hash...)
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
