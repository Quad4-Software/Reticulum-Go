// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"sync"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/packet"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
)

// Path request timing and attempt limits matching upstream LXMRouter.
const (
	// PathRequestWait is upstream LXMRouter.PATH_REQUEST_WAIT.
	PathRequestWait = 7 * time.Second
	// DeliveryRetryWait is upstream LXMRouter.DELIVERY_RETRY_WAIT.
	DeliveryRetryWait = 10 * time.Second
	// PathRequestDebounce is upstream LXMRouter.PATH_REQUEST_DEBOUNCE.
	PathRequestDebounce = 60 * time.Second
	// SlowInterfaceBitrate is upstream LXMRouter.SLOW_INTERFACE_BITRATE.
	SlowInterfaceBitrate = 2000
	// MaxDeliveryAttempts is upstream LXMRouter.MAX_DELIVERY_ATTEMPTS.
	MaxDeliveryAttempts = 5
	// MaxPathlessTries is upstream LXMRouter.MAX_PATHLESS_TRIES.
	MaxPathlessTries = 1
)

// pathRequestTracker mirrors upstream LXMRouter.request_path and the
// medium-path timeout helpers added in LXMF 1.2.0. A destination is
// re-requested at most once per window; the window is
// PATH_REQUEST_DEBOUNCE while any interface slower than
// SLOW_INTERFACE_BITRATE is online, otherwise path_request_wait.
type pathRequestTracker struct {
	tr    *transport.Transport
	mu    sync.Mutex
	times map[string]time.Time
}

func newPathRequestTracker(tr *transport.Transport) *pathRequestTracker {
	return &pathRequestTracker{tr: tr, times: make(map[string]time.Time)}
}

// mediumPathTimeout mirrors upstream Reticulum.get_medium_path_timeout:
// a full MTU round trip on the slowest online interface plus
// DEFAULT_PER_HOP_TIMEOUT. Zero when no online outgoing interface
// advertises a bitrate.
func (t *pathRequestTracker) mediumPathTimeout() time.Duration {
	if t == nil || t.tr == nil {
		return 0
	}
	br := t.tr.SlowestOnlineBitrate()
	if br <= 0 {
		return 0
	}
	if minBr := int64(common.BitrateMinimum); br < minBr {
		br = minBr
	}
	sec := 2*(float64(packet.MTU)*8/float64(br)) + float64(common.EstablishmentTimeoutPerHop)
	return time.Duration(sec * float64(time.Second))
}

// pathRequestWait mirrors upstream LXMRouter.path_request_wait.
func (t *pathRequestTracker) pathRequestWait() time.Duration {
	return max(PathRequestWait, t.mediumPathTimeout())
}

// slowInterfaceOnline mirrors upstream LXMRouter.slow_interface_online.
func (t *pathRequestTracker) slowInterfaceOnline() bool {
	if t == nil || t.tr == nil {
		return false
	}
	br := t.tr.SlowestOnlineBitrate()
	return br != 0 && br < SlowInterfaceBitrate
}

// requestPath mirrors upstream LXMRouter.request_path: a debounced
// Transport.request_path per destination. It reports whether a request
// was emitted; recursive is forwarded to Transport.RequestPath.
func (t *pathRequestTracker) requestPath(destHash []byte, recursive bool) bool {
	if t == nil || t.tr == nil {
		return false
	}
	window := t.pathRequestWait()
	if t.slowInterfaceOnline() {
		window = PathRequestDebounce
	}
	key := peerKey(destHash)
	now := time.Now()
	t.mu.Lock()
	if last, ok := t.times[key]; ok && now.Sub(last) < window {
		t.mu.Unlock()
		Debug("path request still pending, not repeating", "destination", key, "window", window)
		return false
	}
	for k, ts := range t.times {
		if now.Sub(ts) >= window {
			delete(t.times, k)
		}
	}
	t.times[key] = now
	t.mu.Unlock()

	if err := t.tr.RequestPath(destHash, "", nil, recursive); err != nil {
		Debug("path request failed", "destination", key, "error", err)
	}
	return true
}
