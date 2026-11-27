// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"sync"
	"time"
)

// logThrottle dedups repeated log emissions: a key may log at most once per
// interval. Used for diagnostics that would otherwise fire once per packet
// under flood (e.g. "no path to relay link request"). The map is bounded. On
// overflow expired entries are swept, and if still full it is reset.
type logThrottle struct {
	mu    sync.Mutex
	seen  map[string]time.Time
	every time.Duration
	max   int
}

func newLogThrottle(every time.Duration, max int) *logThrottle {
	return &logThrottle{
		seen:  make(map[string]time.Time, 256),
		every: every,
		max:   max,
	}
}

// allow reports whether key may be logged now, recording the emission.
func (l *logThrottle) allow(key string) bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	if last, ok := l.seen[key]; ok && now.Sub(last) < l.every {
		return false
	}
	if len(l.seen) >= l.max {
		for k, t := range l.seen {
			if now.Sub(t) >= l.every {
				delete(l.seen, k)
			}
		}
		if len(l.seen) >= l.max {
			l.seen = make(map[string]time.Time, l.max)
		}
	}
	l.seen[key] = now
	return true
}

// noPathLogRate limits "no path to relay link request" diagnostics. Each
// suppressed packet previously cost a full slog record plus a serialized
// write. Under link-request floods that dominated CPU.
var noPathLogRate = newLogThrottle(30*time.Second, 4096)

// allowNoPathLog reports whether a no-path diagnostic for destHash on
// ifaceName should be emitted.
func allowNoPathLog(destHash []byte, ifaceName string) bool {
	return noPathLogRate.allow(ifaceName + "|" + string(destHash))
}
