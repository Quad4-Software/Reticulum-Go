// SPDX-License-Identifier: LicenseRef-Reticulum

package session

import (
	"sync"
	"time"
)

const (
	DefaultRateInterval = time.Minute
	DefaultRateBurst    = 20
	rateLimitMaxBuckets = 4096
)

type rateLimiter struct {
	mutex    sync.Mutex
	interval time.Duration
	burst    int
	buckets  map[[16]byte]*rateBucket
}

type rateBucket struct {
	tokens float64
	last   time.Time
}

func newRateLimiter(interval time.Duration, burst int) *rateLimiter {
	if interval <= 0 {
		interval = DefaultRateInterval
	}
	if burst <= 0 {
		burst = DefaultRateBurst
	}
	return &rateLimiter{interval: interval, burst: burst, buckets: map[[16]byte]*rateBucket{}}
}

func (r *rateLimiter) Allow(hash []byte) bool {
	if r == nil {
		return true
	}
	key := rateHashKey(hash)
	now := time.Now()
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if len(r.buckets) >= rateLimitMaxBuckets {
		r.pruneLocked(now)
	}
	b := r.buckets[key]
	if b == nil {
		if len(r.buckets) >= rateLimitMaxBuckets {
			r.pruneLocked(now)
		}
		if len(r.buckets) >= rateLimitMaxBuckets {
			return false
		}
		b = &rateBucket{tokens: float64(r.burst), last: now}
		r.buckets[key] = b
	}
	elapsed := now.Sub(b.last).Seconds() / r.interval.Seconds()
	b.tokens += elapsed
	if b.tokens > float64(r.burst) {
		b.tokens = float64(r.burst)
	}
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func (r *rateLimiter) pruneLocked(now time.Time) {
	stale := max(r.interval*2, time.Minute)
	for k, b := range r.buckets {
		if now.Sub(b.last) > stale {
			delete(r.buckets, k)
		}
	}
	if len(r.buckets) < rateLimitMaxBuckets {
		return
	}
	n := 0
	limit := rateLimitMaxBuckets / 2
	for k := range r.buckets {
		delete(r.buckets, k)
		n++
		if n >= limit {
			return
		}
	}
}

func rateHashKey(hash []byte) (key [16]byte) {
	copy(key[:], hash)
	return key
}
