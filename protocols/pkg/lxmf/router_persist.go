// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"time"

	"github.com/Quad4-Software/msgpack/v5/pkg/msgpack"
)

func routerAtomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp := fmt.Sprintf("%s.tmp.%d", path, time.Now().UnixNano())
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func saveMsgpack(path string, v any) error {
	data, err := msgpack.Marshal(v)
	if err != nil {
		return err
	}
	return routerAtomicWrite(path, data)
}

func loadMsgpack(path string, v any) error {
	data, err := os.ReadFile(filepath.Clean(path)) // #nosec G304 -- path under router storage
	if err != nil {
		return err
	}
	return msgpack.Unmarshal(data, v)
}

func (r *Router) persistPath(name string) string {
	return filepath.Join(r.storagePath, name)
}

func (r *Router) peersDir() string {
	return filepath.Join(r.storagePath, "peers")
}

func (r *Router) loadPersistedState() {
	r.loadTransientCaches()
	r.loadNodeStats()
	r.loadPeers()
}

func (r *Router) loadTransientCaches() {
	var delivered map[string]float64
	if err := loadMsgpack(r.persistPath("local_deliveries"), &delivered); err == nil && delivered != nil {
		r.locallyDelivered = delivered
	} else {
		r.locallyDelivered = map[string]float64{}
	}
	var processed map[string]float64
	if err := loadMsgpack(r.persistPath("locally_processed"), &processed); err == nil && processed != nil {
		r.locallyProcessed = processed
	} else {
		r.locallyProcessed = map[string]float64{}
	}
	r.cleanTransientCaches()
}

func (r *Router) loadNodeStats() {
	var stats map[string]any
	if err := loadMsgpack(r.persistPath("node_stats"), &stats); err != nil || stats == nil {
		return
	}
	r.clientPropagationReceived = int64(asFloat(stats["client_propagation_messages_received"]))
	r.clientPropagationServed = int64(asFloat(stats["client_propagation_messages_served"]))
	r.unpeeredPropagationIncoming = int64(asFloat(stats["unpeered_propagation_incoming"]))
	r.unpeeredPropagationRxBytes = int64(asFloat(stats["unpeered_propagation_rx_bytes"]))
}

func (r *Router) loadPeers() {
	if r.store == nil {
		return
	}
	if err := os.MkdirAll(r.peersDir(), 0o700); err != nil {
		Warning("peer dir create failed", "error", err)
		return
	}
	ents, err := os.ReadDir(r.peersDir())
	if err != nil {
		return
	}
	for _, ent := range ents {
		if ent.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(r.peersDir(), ent.Name())) // #nosec G304 -- peer store dir
		if err != nil {
			continue
		}
		peer, err := peerFromBytes(r, data)
		if err != nil {
			Warning("peer load failed", "file", ent.Name(), "error", err)
			continue
		}
		r.peers[peerKey(peer.DestinationHash)] = peer
	}
}

func (r *Router) saveLocallyDelivered() {
	r.deliveredMu.Lock()
	data := make(map[string]float64, len(r.locallyDelivered))
	maps.Copy(data, r.locallyDelivered)
	r.deliveredMu.Unlock()
	if len(data) == 0 {
		return
	}
	if err := saveMsgpack(r.persistPath("local_deliveries"), data); err != nil {
		Warning("save local deliveries failed", "error", err)
	}
}

func (r *Router) saveLocallyProcessed() {
	r.processedMu.Lock()
	data := make(map[string]float64, len(r.locallyProcessed))
	maps.Copy(data, r.locallyProcessed)
	r.processedMu.Unlock()
	if len(data) == 0 {
		return
	}
	if err := saveMsgpack(r.persistPath("locally_processed"), data); err != nil {
		Warning("save locally processed failed", "error", err)
	}
}

func (r *Router) saveNodeStats() {
	stats := map[string]any{
		"client_propagation_messages_received": r.clientPropagationReceived,
		"client_propagation_messages_served":   r.clientPropagationServed,
		"unpeered_propagation_incoming":        r.unpeeredPropagationIncoming,
		"unpeered_propagation_rx_bytes":        r.unpeeredPropagationRxBytes,
	}
	if err := saveMsgpack(r.persistPath("node_stats"), stats); err != nil {
		Warning("save node stats failed", "error", err)
	}
}

func (r *Router) savePeers() {
	if err := os.MkdirAll(r.peersDir(), 0o700); err != nil {
		return
	}
	r.peersMu.RLock()
	defer r.peersMu.RUnlock()
	for key, peer := range r.peers {
		data, err := peer.toBytes()
		if err != nil {
			continue
		}
		path := filepath.Join(r.peersDir(), key)
		if err := routerAtomicWrite(path, data); err != nil {
			Warning("peer save failed", "peer", key, "error", err)
		}
	}
}

func (r *Router) cleanTransientCaches() {
	now := float64(time.Now().Unix())
	// Upstream keeps transient id caches for MESSAGE_EXPIRY*6.
	const maxAge = float64(messageExpirySeconds) * 6

	r.deliveredMu.Lock()
	for k, ts := range r.locallyDelivered {
		if now-ts > maxAge {
			delete(r.locallyDelivered, k)
		}
	}
	r.deliveredMu.Unlock()

	r.processedMu.Lock()
	for k, ts := range r.locallyProcessed {
		if now-ts > maxAge {
			delete(r.locallyProcessed, k)
		}
	}
	r.processedMu.Unlock()
}

func asFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case uint64:
		return float64(x)
	default:
		return 0
	}
}
