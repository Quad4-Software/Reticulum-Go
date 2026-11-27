// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"git.quad4.io/libraries/msgpack/v5/pkg/msgpack"
)

func (r *Router) importRoot() string {
	return filepath.Join(r.storagePath, "golxmd")
}

func (r *Router) usesImportedState() bool {
	info, err := os.Stat(r.importRoot())
	return err == nil && info.IsDir()
}

func (r *Router) preparePythonStateImport() string {
	sourceStore := filepath.Join(r.storagePath, "messagestore")
	if r.usesImportedState() {
		return filepath.Join(r.importRoot(), "messagestore")
	}

	peersFile := filepath.Join(r.storagePath, "peers")
	peersIsPythonFile := false
	if info, err := os.Stat(peersFile); err == nil && !info.IsDir() {
		peersIsPythonFile = true
	}
	messages, err := scanPythonMessageStore(sourceStore)
	if err != nil {
		Warning("python messagestore scan failed", "path", sourceStore, "error", err)
	}
	if !peersIsPythonFile && len(messages) == 0 {
		return sourceStore
	}

	importStore := filepath.Join(r.importRoot(), "messagestore")
	importPeers := filepath.Join(r.importRoot(), "peers")
	if err := os.MkdirAll(importStore, 0o700); err != nil {
		Warning("python state import unavailable", "error", err)
		return sourceStore
	}
	if err := os.MkdirAll(importPeers, 0o700); err != nil {
		Warning("python peer import unavailable", "error", err)
		return sourceStore
	}

	copiedMessages, skippedMessages := importPythonMessages(messages, sourceStore, importStore, r.cfg.Propagation.MessageStorageLimitMB)
	if copiedMessages > 0 || skippedMessages > 0 {
		Notice("imported python messagestore", "copied", copiedMessages, "skipped", skippedMessages, "path", importStore)
	}
	if peersIsPythonFile {
		copied, skipped, err := importPythonPeers(peersFile, importPeers)
		if err != nil {
			Warning("python peer import failed", "path", peersFile, "error", err)
		} else if copied > 0 || skipped > 0 {
			Notice("imported python peers", "copied", copied, "skipped", skipped, "path", importPeers)
		}
	}
	return importStore
}

type pythonStoreFile struct {
	name       string
	receivedAt float64
	size       int64
}

func scanPythonMessageStore(dir string) ([]pythonStoreFile, error) {
	ents, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]pythonStoreFile, 0, len(ents))
	for _, ent := range ents {
		if ent.IsDir() || strings.HasSuffix(ent.Name(), ".tmp") {
			continue
		}
		_, receivedAt, _, err := ParseMessageStoreFilename(ent.Name())
		if err != nil {
			Warning("skipping invalid python messagestore filename", "name", ent.Name(), "error", err)
			continue
		}
		info, err := ent.Info()
		if err != nil {
			Warning("skipping unreadable python messagestore entry", "name", ent.Name(), "error", err)
			continue
		}
		out = append(out, pythonStoreFile{name: ent.Name(), receivedAt: receivedAt, size: info.Size()})
	}
	return out, nil
}

func importPythonMessages(files []pythonStoreFile, sourceStore, importStore string, limitMB float64) (copied, skipped int) {
	sort.Slice(files, func(i, j int) bool { return files[i].receivedAt > files[j].receivedAt })
	limitBytes := int64(0)
	if limitMB > 0 {
		limitBytes = int64(limitMB * 1000 * 1000)
	}
	var total int64
	seen := make(map[string]struct{})
	if existing, err := scanPythonMessageStore(importStore); err == nil {
		for _, ent := range existing {
			if tid, _, _, err := ParseMessageStoreFilename(ent.name); err == nil {
				seen[hex.EncodeToString(tid)] = struct{}{}
				total += ent.size
			}
		}
	}
	for _, file := range files {
		tid, _, _, err := ParseMessageStoreFilename(file.name)
		if err != nil {
			skipped++
			continue
		}
		tidHex := hex.EncodeToString(tid)
		if _, ok := seen[tidHex]; ok {
			skipped++
			continue
		}
		if limitBytes > 0 && total+file.size > limitBytes {
			skipped++
			Warning("python messagestore import skipped message over storage limit", "name", file.name)
			continue
		}
		src := filepath.Join(sourceStore, file.name)
		dst := filepath.Join(importStore, file.name)
		if _, err := os.Stat(dst); err == nil {
			seen[tidHex] = struct{}{}
			skipped++
			continue
		}
		data, err := os.ReadFile(filepath.Clean(src)) // #nosec G304 -- path under configured storage
		if err != nil || len(data) < DestinationLength {
			skipped++
			Warning("skipping unreadable python messagestore file", "path", src, "error", err)
			continue
		}
		if err := os.WriteFile(dst, data, 0o600); err != nil {
			skipped++
			Warning("python messagestore copy failed", "path", dst, "error", err)
			continue
		}
		seen[tidHex] = struct{}{}
		total += int64(len(data))
		copied++
	}
	return copied, skipped
}

func importPythonPeers(peersFile, importPeers string) (copied, skipped int, err error) {
	data, err := os.ReadFile(filepath.Clean(peersFile)) // #nosec G304 -- path under configured storage
	if err != nil {
		return 0, 0, err
	}
	if len(data) == 0 {
		return 0, 0, nil
	}
	var serialized []any
	if err := msgpack.Unmarshal(data, &serialized); err != nil {
		return 0, 0, fmt.Errorf("decode python peers file: %w", err)
	}
	for _, item := range serialized {
		raw, ok := item.([]byte)
		if !ok {
			skipped++
			Warning("skipping non-binary python peer entry", "type", fmt.Sprintf("%T", item))
			continue
		}
		var dict map[string]any
		if err := msgpack.Unmarshal(raw, &dict); err != nil {
			skipped++
			Warning("skipping invalid python peer entry", "error", err)
			continue
		}
		dest, ok := dict["destination_hash"].([]byte)
		if !ok || len(dest) != DestinationLength {
			skipped++
			Warning("skipping python peer entry with invalid destination hash")
			continue
		}
		name := hex.EncodeToString(dest)
		path := filepath.Join(importPeers, name)
		if _, err := os.Stat(path); err == nil {
			skipped++
			continue
		}
		if err := os.WriteFile(path, raw, 0o600); err != nil {
			skipped++
			Warning("python peer copy failed", "peer", name, "error", err)
			continue
		}
		copied++
	}
	return copied, skipped, nil
}
