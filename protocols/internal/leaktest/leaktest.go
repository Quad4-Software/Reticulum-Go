// Package leaktest waits for goroutine count to return near a baseline after tests.
package leaktest

import (
	"runtime"
	"testing"
	"time"
)

// Baseline returns runtime.NumGoroutine after GC and a short sleep.
func Baseline() int {
	runtime.GC()
	time.Sleep(40 * time.Millisecond)
	return runtime.NumGoroutine()
}

// AssertStable fails if NumGoroutine stays above baseline+maxExtra until timeout.
func AssertStable(t *testing.T, baseline, maxExtra int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		runtime.GC()
		n := runtime.NumGoroutine()
		if n <= baseline+maxExtra {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	n := runtime.NumGoroutine()
	t.Fatalf("goroutines did not settle: baseline=%d maxExtra=%d now=%d (allowed %d)",
		baseline, maxExtra, n, baseline+maxExtra)
}
