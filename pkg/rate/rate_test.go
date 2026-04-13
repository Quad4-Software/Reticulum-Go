package rate

import (
	"testing"
	"time"
)

func TestNewLimiter(t *testing.T) {
	limiter := NewLimiter(10.0, 1.0)
	if limiter == nil {
		t.Fatal("NewLimiter() returned nil")
	}
}

func TestLimiter_Allow(t *testing.T) {
	limiter := NewLimiter(10.0, 1.0)

	if !limiter.Allow() {
		t.Error("Allow() should return true initially")
	}

	for range 10 {
		limiter.Allow()
	}

	if limiter.Allow() {
		t.Error("Allow() should return false after exceeding rate")
	}

	time.Sleep(1100 * time.Millisecond)

	if !limiter.Allow() {
		t.Error("Allow() should return true after waiting")
	}
}

func TestNewAnnounceRateControl(t *testing.T) {
	arc := NewAnnounceRateControl(3600.0, 3, 7200.0)
	if arc == nil {
		t.Fatal("NewAnnounceRateControl() returned nil")
	}
}

func TestAnnounceRateControl_AllowAnnounce(t *testing.T) {
	arc := NewAnnounceRateControl(1.0, 2, 2.0)

	hash := "test-dest-hash"

	if !arc.AllowAnnounce(hash) {
		t.Error("AllowAnnounce() should return true for first announce")
	}

	if !arc.AllowAnnounce(hash) {
		t.Error("AllowAnnounce() should return true for second announce (within grace)")
	}

	if arc.AllowAnnounce(hash) {
		t.Error("AllowAnnounce() should return false for third announce (exceeds grace)")
	}

	time.Sleep(1100 * time.Millisecond)

	if !arc.AllowAnnounce(hash) {
		t.Error("AllowAnnounce() should return true after waiting")
	}
}

func TestAnnounceRateControl_AllowAnnounce_DifferentHashes(t *testing.T) {
	arc := NewAnnounceRateControl(1.0, 1, 1.0)

	hash1 := "hash1"
	hash2 := "hash2"

	if !arc.AllowAnnounce(hash1) {
		t.Error("AllowAnnounce() should return true for hash1")
	}

	if !arc.AllowAnnounce(hash2) {
		t.Error("AllowAnnounce() should return true for hash2 (different hash)")
	}
}

func TestNewIngressControl(t *testing.T) {
	ic := NewIngressControl(true)
	if ic == nil {
		t.Fatal("NewIngressControl() returned nil")
	}
}

func TestIngressControl_ProcessAnnounce(t *testing.T) {
	ic := NewIngressControl(true)

	hash := "test-hash"
	data := []byte("announce data")

	ic.mutex.Lock()
	ic.lastBurst = time.Now().Add(-time.Second)
	ic.mutex.Unlock()

	if !ic.ProcessAnnounce(hash, data, false) {
		t.Error("ProcessAnnounce() should return true for first announce")
	}

	time.Sleep(10 * time.Millisecond)

	for range 200 {
		ic.ProcessAnnounce(hash, data, false)
	}

	result := ic.ProcessAnnounce(hash, data, false)
	if result {
		t.Error("ProcessAnnounce() should return false when burst frequency exceeded")
	}
}

func TestIngressControl_ProcessAnnounce_Disabled(t *testing.T) {
	ic := NewIngressControl(false)

	hash := "test-hash"
	data := []byte("announce data")

	if !ic.ProcessAnnounce(hash, data, false) {
		t.Error("ProcessAnnounce() should return true when disabled")
	}
}

func TestIngressControl_ReleaseHeldAnnounce(t *testing.T) {
	ic := NewIngressControl(true)

	_, _, found := ic.ReleaseHeldAnnounce()
	if found {
		t.Error("ReleaseHeldAnnounce() should return false when no announces held")
	}

	ic.ProcessAnnounce("hash1", []byte("data1"), false)
	for range 200 {
		ic.ProcessAnnounce("hash1", []byte("data1"), false)
	}

	hash, data, found := ic.ReleaseHeldAnnounce()
	if !found {
		t.Error("ReleaseHeldAnnounce() should return true when announces are held")
	}
	if hash == "" {
		t.Error("ReleaseHeldAnnounce() should return non-empty hash")
	}
	if len(data) == 0 {
		t.Error("ReleaseHeldAnnounce() should return non-empty data")
	}
}
