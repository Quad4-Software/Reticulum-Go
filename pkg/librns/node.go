// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package librns

import (
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"time"

	"quad4/reticulum-go/pkg/announce"
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/node"
	"quad4/reticulum-go/pkg/reticulumconfig"
)

type announceHandler struct {
	rec *nodeRecord
}

func (h *announceHandler) AspectFilter() []string { return []string{"*"} }

func (h *announceHandler) ReceivePathResponses() bool { return true }

func (h *announceHandler) ReceivedAnnounce(destHash []byte, announcedIdentity any, appData []byte, hops uint8) error {
	ev := Event{
		Kind:            EventAnnounce,
		DestinationHash: append([]byte(nil), destHash...),
		AppData:         append([]byte(nil), appData...),
		Hops:            hops,
	}
	if id, ok := announcedIdentity.(*identity.Identity); ok {
		if h, err := hex.DecodeString(id.GetHexHash()); err == nil {
			ev.IdentityHash = h
		}
	}
	h.rec.enqueue(ev)
	return nil
}

// NodeCreate loads configuration and constructs a node handle.
// An empty configPath uses in-memory defaults.
func NodeCreate(configPath string) (uint64, int) {
	var cfg *common.ReticulumConfig
	var err error
	if configPath == "" {
		cfg = common.DefaultConfig()
		// In-memory defaults for embedders: no shared-instance bind.
		cfg.ShareInstance = false
	} else {
		if err = validatePath(configPath); err != nil {
			return 0, setLastError(err)
		}
		cfg, err = reticulumconfig.LoadConfig(configPath)
		if err != nil {
			return 0, setLastError(fmt.Errorf("%w: %v", errIO, err))
		}
	}

	n, err := node.New(cfg)
	if err != nil {
		return 0, setLastError(fmt.Errorf("%w: %v", errInternal, err))
	}

	rec := newNodeRecord(n)
	n.Transport().RegisterAnnounceHandler(&announceHandler{rec: rec})

	runtimeMu.Lock()
	id := handles.insert(kindNode, rec)
	rec.handle = id
	runtimeMu.Unlock()
	return id, OK
}

// NodeStart starts transport and configured interfaces.
func NodeStart(nodeHandle uint64) int {
	rec, err := nodeByHandle(nodeHandle)
	if err != nil {
		return setLastError(err)
	}
	if rec.started {
		return OK
	}
	if err := rec.node.Start(); err != nil {
		return setLastError(fmt.Errorf("%w: %v", errInternal, err))
	}
	rec.started = true
	return OK
}

// NodeStop stops transport and interfaces without destroying the handle.
func NodeStop(nodeHandle uint64) int {
	rec, err := nodeByHandle(nodeHandle)
	if err != nil {
		return setLastError(err)
	}
	if !rec.started {
		return OK
	}
	if err := rec.node.Stop(); err != nil {
		return setLastError(fmt.Errorf("%w: %v", errInternal, err))
	}
	rec.started = false
	return OK
}

// NodeDestroy tears down a node and invalidates its handle.
func NodeDestroy(nodeHandle uint64) int {
	rec, err := nodeByHandle(nodeHandle)
	if err != nil {
		return setLastError(err)
	}
	rec.stopCallback()
	if rec.started {
		_ = rec.node.Stop()
		rec.started = false
	}
	rec.queue.close()

	runtimeMu.Lock()
	defer runtimeMu.Unlock()
	if !handles.delete(nodeHandle) {
		return setLastError(errInvalidHandle)
	}
	return OK
}

// NodeResume resumes interfaces after a pause (network available).
func NodeResume(nodeHandle uint64) int {
	rec, err := nodeByHandle(nodeHandle)
	if err != nil {
		return setLastError(err)
	}
	if err := rec.node.OnNetworkAvailable(); err != nil {
		return setLastError(fmt.Errorf("%w: %v", errInternal, err))
	}
	return OK
}

// NodePause pauses interfaces (network lost).
func NodePause(nodeHandle uint64) int {
	rec, err := nodeByHandle(nodeHandle)
	if err != nil {
		return setLastError(err)
	}
	if err := rec.node.OnNetworkLost(); err != nil {
		return setLastError(fmt.Errorf("%w: %v", errInternal, err))
	}
	return OK
}

// NodeRefreshPaths expires stale paths and requests fresh ones.
// When destHashes is empty, watched destinations are refreshed.
func NodeRefreshPaths(nodeHandle uint64, destHashes ...[]byte) int {
	rec, err := nodeByHandle(nodeHandle)
	if err != nil {
		return setLastError(err)
	}
	for _, h := range destHashes {
		if len(h) != identity.TruncatedHashLength/8 {
			return setLastError(errInvalidArg)
		}
	}
	if err := rec.node.RefreshPaths(destHashes...); err != nil {
		return setLastError(fmt.Errorf("%w: %v", errInternal, err))
	}
	return OK
}

// NodeInterfaces returns a snapshot of registered transport interfaces.
func NodeInterfaces(nodeHandle uint64) ([]InterfaceEntry, int) {
	rec, err := nodeByHandle(nodeHandle)
	if err != nil {
		return nil, setLastError(err)
	}
	ifaces := rec.node.Transport().GetInterfaces()
	out := make([]InterfaceEntry, 0, len(ifaces))
	for name, iface := range ifaces {
		if iface == nil {
			continue
		}
		entry := InterfaceEntry{
			Name:      name,
			Type:      interfaceTypeName(iface.GetType()),
			Online:    iface.IsOnline(),
			Enabled:   iface.IsEnabled(),
			RxBytes:   iface.GetRxBytes(),
			TxBytes:   iface.GetTxBytes(),
			RxPackets: iface.GetRxPackets(),
			TxPackets: iface.GetTxPackets(),
		}
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out, OK
}

func interfaceTypeName(t common.InterfaceType) string {
	switch t {
	case common.IFTypeUDP:
		return "UDP"
	case common.IFTypeTCP:
		return "TCP"
	case common.IFTypeUnix:
		return "Backbone (unix)"
	case common.IFTypeI2P:
		return "I2P"
	case common.IFTypeBluetooth:
		return "Bluetooth"
	case common.IFTypeSerial:
		return "Serial"
	case common.IFTypeAuto:
		return "Auto"
	case common.IFTypeBackbone:
		return "Backbone"
	case common.IFTypePipe:
		return "Pipe"
	case common.IFTypeQUIC:
		return "QUIC"
	case common.IFTypeWebTransport:
		return "WebTransport"
	case common.IFTypeDNSRendezvous:
		return "DNS"
	case common.IFTypeVSOCK:
		return "VSOCK"
	case common.IFTypeHTTPS:
		return "HTTPS"
	case common.IFTypeModem73:
		return "Modem"
	case common.IFTypeSDR:
		return "SDR"
	default:
		return "Unknown"
	}
}

// PathTable returns a snapshot of known paths.
// maxHops < 0 means no hop filter.
func PathTable(nodeHandle uint64, maxHops int) ([]PathEntry, int) {
	rec, err := nodeByHandle(nodeHandle)
	if err != nil {
		return nil, setLastError(err)
	}
	var filter *int
	if maxHops >= 0 {
		filter = &maxHops
	}
	rows := rec.node.Transport().GetPathTable(filter)
	out := make([]PathEntry, 0, len(rows))
	for _, row := range rows {
		out = append(out, PathEntry{
			Hash:      append([]byte(nil), row.Hash...),
			Via:       append([]byte(nil), row.Via...),
			Hops:      row.Hops,
			Interface: row.Interface,
			Timestamp: row.Timestamp,
			Expires:   row.Expires,
		})
	}
	return out, OK
}

// SetEventCallback registers cb to drain the same event queue as EventPoll.
// Pass nil to clear. Only one callback per node.
func SetEventCallback(nodeHandle uint64, cb EventCallback) int {
	rec, err := nodeByHandle(nodeHandle)
	if err != nil {
		return setLastError(err)
	}
	rec.stopCallback()
	if cb == nil {
		return OK
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	rec.cbMu.Lock()
	rec.callback = cb
	rec.cbStop = stop
	rec.cbDone = done
	rec.cbMu.Unlock()
	go drainEvents(rec, cb, stop, done)
	return OK
}

func drainEvents(rec *nodeRecord, cb EventCallback, stop, done chan struct{}) {
	defer close(done)
	for {
		select {
		case <-stop:
			return
		default:
		}
		ev, err := rec.queue.poll(50 * time.Millisecond)
		if err != nil {
			if errors.Is(err, errState) {
				return
			}
			continue
		}
		cb(cloneEvent(ev))
	}
}

// NodeSetIdentity attaches an identity to the node transport.
func NodeSetIdentity(nodeHandle, identityHandle uint64) int {
	rec, err := nodeByHandle(nodeHandle)
	if err != nil {
		return setLastError(err)
	}
	identRec, err := identityByHandle(identityHandle)
	if err != nil {
		return setLastError(err)
	}
	rec.identity = identRec.identity
	rec.node.Transport().SetIdentity(identRec.identity)
	return OK
}

// PathRequest asks the transport to resolve a path to destHash.
func PathRequest(nodeHandle uint64, destHash []byte) int {
	rec, err := nodeByHandle(nodeHandle)
	if err != nil {
		return setLastError(err)
	}
	if len(destHash) != identity.TruncatedHashLength/8 {
		return setLastError(errInvalidArg)
	}
	if !rec.started {
		return setLastError(errState)
	}
	if err := rec.node.Transport().RequestPath(destHash, "", nil, false); err != nil {
		return setLastError(err)
	}
	return OK
}

// EventPoll waits up to timeout for the next event on nodeHandle.
func EventPoll(nodeHandle uint64, timeout time.Duration) (Event, int) {
	rec, err := nodeByHandle(nodeHandle)
	if err != nil {
		return Event{}, setLastError(err)
	}
	ev, err := rec.queue.poll(timeout)
	if err != nil {
		return Event{}, setLastError(err)
	}
	return ev, OK
}

// CopyEventField copies one event byte field into dst and reports truncation.
func CopyEventField(dst, src []byte) (written int, truncated bool) {
	if len(src) == 0 {
		return 0, false
	}
	n := copy(dst, src)
	return n, n < len(src)
}

// DecodeHexHash decodes a 32-character hex destination or identity hash.
func DecodeHexHash(hexHash string) ([]byte, error) {
	b, err := hex.DecodeString(hexHash)
	if err != nil || len(b) != identity.TruncatedHashLength/8 {
		return nil, errInvalidArg
	}
	return b, nil
}

var _ announce.Handler = (*announceHandler)(nil)
