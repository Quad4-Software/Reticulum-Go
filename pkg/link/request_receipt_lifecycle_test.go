// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package link

// Request-receipt lifecycle vectors, checked against Python RNS
// RequestReceipt semantics:
//
//   - Python removes a timed-out receipt from pending_requests
//     (RequestReceipt.request_timed_out). Ret-go additionally must remove
//     it because pendingRequests is capped at MaxPendingRequests and gated
//     per pathHash; a leaked entry wedges the link permanently.
//   - Python suspends the response timeout once a response resource starts
//     arriving (status RECEIVING); the resource watchdog then owns the
//     transfer. Ret-go must not fail a receipt mid-transfer.
//   - Python Resource gives up after MAX_RETRIES (16) unproductive stall
//     rounds and cancels; the bound receipt must fail, not linger.
//   - Python response_received/response_resource_progress never fire on a
//     receipt already FAILED; a late completion must not resurrect it.

import (
	"bytes"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/resource"
)

func newTestRequestReceipt(l *Link, seed byte, timeout time.Duration) *RequestReceipt {
	return &RequestReceipt{
		link:      l,
		requestID: bytes.Repeat([]byte{seed}, 16),
		pathHash:  bytes.Repeat([]byte{seed ^ 0x5a}, 16),
		status:    StatusPending,
		sentAt:    time.Now(),
		timeout:   timeout,
	}
}

func pendingRequestCount(l *Link) int {
	l.requestMutex.Lock()
	defer l.requestMutex.Unlock()
	return len(l.pendingRequests)
}

func waitForCond(t *testing.T, d time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal(msg)
}

// A receipt whose timeout expires with no response must be failed AND
// removed from pendingRequests (Python request_timed_out removes it; the
// cap makes removal mandatory here).
func TestRequestReceiptTimeout_RemovesPendingEntry(t *testing.T) {
	l := &Link{}
	r := newTestRequestReceipt(l, 0x10, 40*time.Millisecond)
	if err := l.registerPendingRequest(r); err != nil {
		t.Fatalf("registerPendingRequest: %v", err)
	}
	failed := make(chan struct{}, 1)
	r.SetFailedCallback(func(*RequestReceipt) { failed <- struct{}{} })
	go r.startTimeout()

	waitForCond(t, 2*time.Second, func() bool { return pendingRequestCount(l) == 0 },
		"timed-out receipt still in pendingRequests")
	if got := r.GetStatus(); got != StatusFailed {
		t.Fatalf("status = %d, want Failed", got)
	}
	select {
	case <-failed:
	case <-time.After(time.Second):
		t.Fatal("failed callback did not fire")
	}
}

// After a timeout the same path must be requestable again; a leaked receipt
// would block it with ErrLinkRequestDuplicate forever.
func TestRequestReceiptTimeout_UnblocksDuplicatePath(t *testing.T) {
	l := &Link{}
	r1 := newTestRequestReceipt(l, 0x20, 40*time.Millisecond)
	if err := l.registerPendingRequest(r1); err != nil {
		t.Fatalf("register r1: %v", err)
	}
	go r1.startTimeout()
	waitForCond(t, 2*time.Second, func() bool { return pendingRequestCount(l) == 0 },
		"r1 not removed after timeout")

	r2 := newTestRequestReceipt(l, 0x21, time.Minute)
	r2.pathHash = r1.pathHash
	if err := l.registerPendingRequest(r2); err != nil {
		t.Fatalf("re-request of same path after timeout rejected: %v", err)
	}
}

// MaxPendingRequests must recover after timeouts; leaked entries otherwise
// wedge the link at ErrLinkRequestBusy after eight silent requests.
func TestRequestReceiptTimeout_FreesMaxPendingSlot(t *testing.T) {
	l := &Link{}
	for i := 0; i < MaxPendingRequests; i++ {
		r := newTestRequestReceipt(l, byte(0x30+i), 30*time.Millisecond)
		if err := l.registerPendingRequest(r); err != nil {
			t.Fatalf("register %d: %v", i, err)
		}
		go r.startTimeout()
	}
	if err := l.registerPendingRequest(newTestRequestReceipt(l, 0x7f, time.Minute)); err == nil {
		t.Fatal("expected ErrLinkRequestBusy while all slots are pending")
	}
	waitForCond(t, 2*time.Second, func() bool { return pendingRequestCount(l) == 0 },
		"timed-out receipts were not removed")
	if err := l.registerPendingRequest(newTestRequestReceipt(l, 0x7e, time.Minute)); err != nil {
		t.Fatalf("new request rejected after timeouts: %v", err)
	}
}

// Oracle: once the response resource is progressing the request deadline
// must not fire (Python RECEIVING suspends the response timeout job).
func TestRequestReceipt_ReceivingSurvivesDeadline(t *testing.T) {
	l := &Link{}
	r := newTestRequestReceipt(l, 0x40, 60*time.Millisecond)
	if err := l.registerPendingRequest(r); err != nil {
		t.Fatalf("register: %v", err)
	}
	go r.startTimeout()

	rx := &incomingResourceAsm{
		request:    r,
		adv:        &resource.ResourceAdvertisement{Hash: bytes.Repeat([]byte{0x41}, 32)},
		partSlots:  make([][]byte, 4),
		totalParts: 4,
	}
	l.incomingMu.Lock()
	l.incomingRx = rx
	l.incomingMu.Unlock()

	// First accepted part transitions the receipt into receiving.
	rx.partSlots[0] = []byte("part-0")
	l.incomingMu.Lock()
	l.reportIncomingResourceProgress(rx)
	l.incomingMu.Unlock()
	if got := r.GetStatus(); got != StatusReceiving {
		t.Fatalf("status = %d after first part, want Receiving", got)
	}

	time.Sleep(3 * 60 * time.Millisecond)
	if got := r.GetStatus(); got != StatusReceiving {
		t.Fatalf("status = %d past deadline, want still Receiving", got)
	}
	if r.Concluded() {
		t.Fatal("receipt concluded while response resource still transferring")
	}
	if pendingRequestCount(l) != 1 {
		t.Fatal("receiving receipt dropped from pendingRequests")
	}
}

// A transfer that completes after the request deadline still delivers its
// response exactly once and removes the receipt.
func TestRequestReceipt_ReceivingCompletesAfterDeadline(t *testing.T) {
	l := &Link{}
	r := newTestRequestReceipt(l, 0x42, 40*time.Millisecond)
	if err := l.registerPendingRequest(r); err != nil {
		t.Fatalf("register: %v", err)
	}
	go r.startTimeout()

	rx := &incomingResourceAsm{
		request:    r,
		adv:        &resource.ResourceAdvertisement{Hash: bytes.Repeat([]byte{0x43}, 32)},
		partSlots:  make([][]byte, 1),
		totalParts: 1,
	}
	l.incomingMu.Lock()
	l.incomingRx = rx
	l.incomingMu.Unlock()
	rx.partSlots[0] = []byte("x")
	l.incomingMu.Lock()
	l.reportIncomingResourceProgress(rx)
	l.incomingMu.Unlock()

	time.Sleep(3 * 40 * time.Millisecond)
	if got := r.GetStatus(); got != StatusReceiving {
		t.Fatalf("status = %d, want Receiving past deadline", got)
	}

	answered := make(chan *RequestReceipt, 1)
	r.SetResponseCallback(func(rr *RequestReceipt) { answered <- rr })
	l.completeRequestWithResourcePayload(r, []byte("payload"), nil)
	if got := r.GetStatus(); got != StatusActive {
		t.Fatalf("status = %d after completion, want Active", got)
	}
	if pendingRequestCount(l) != 0 {
		t.Fatal("completed receipt left in pendingRequests")
	}
	select {
	case <-answered:
	case <-time.After(time.Second):
		t.Fatal("response callback did not fire")
	}
}

// Bound to a resource advertisement but no part ever arrives: the request
// deadline applies, the receipt fails, and the stalled transfer is torn
// down (Python cancels the resource when the receipt fails).
func TestRequestReceipt_BoundWithoutProgress_TimesOutAndAborts(t *testing.T) {
	l := &Link{}
	r := newTestRequestReceipt(l, 0x44, 50*time.Millisecond)
	if err := l.registerPendingRequest(r); err != nil {
		t.Fatalf("register: %v", err)
	}
	go r.startTimeout()

	released := false
	rx := &incomingResourceAsm{
		request:        r,
		adv:            &resource.ResourceAdvertisement{Hash: bytes.Repeat([]byte{0x45}, 32)},
		partSlots:      make([][]byte, 1),
		mapHashes:      make([][]byte, 1),
		inflight:       make([]bool, 1),
		totalParts:     1,
		protectRelease: func() { released = true },
	}
	l.incomingMu.Lock()
	l.incomingRx = rx
	l.incomingMu.Unlock()

	waitForCond(t, 2*time.Second, func() bool { return r.GetStatus() == StatusFailed },
		"receipt not failed after deadline with stalled bound resource")
	if pendingRequestCount(l) != 0 {
		t.Fatal("failed receipt left in pendingRequests")
	}
	l.incomingMu.Lock()
	current := l.incomingRx
	l.incomingMu.Unlock()
	if current != nil {
		t.Fatal("stalled bound resource was not aborted")
	}
	if !released {
		t.Fatal("protect release not called on abort")
	}
}

// When the incoming transfer dies (cancel, teardown, watchdog abort, bad
// HMU, supersede) its bound receipt must fail and leave pendingRequests.
func TestResetIncomingResource_FailsBoundReceipt(t *testing.T) {
	l := &Link{}
	r := newTestRequestReceipt(l, 0x46, time.Minute)
	if err := l.registerPendingRequest(r); err != nil {
		t.Fatalf("register: %v", err)
	}
	failed := make(chan struct{}, 1)
	r.SetFailedCallback(func(*RequestReceipt) { failed <- struct{}{} })

	released := false
	rx := &incomingResourceAsm{
		request:        r,
		adv:            &resource.ResourceAdvertisement{Hash: bytes.Repeat([]byte{0x47}, 32)},
		partSlots:      make([][]byte, 1),
		totalParts:     1,
		protectRelease: func() { released = true },
	}
	l.incomingMu.Lock()
	l.incomingRx = rx
	l.incomingMu.Unlock()
	r.mutex.Lock()
	r.status = StatusReceiving
	r.mutex.Unlock()

	l.resetIncomingResource()
	if got := r.GetStatus(); got != StatusFailed {
		t.Fatalf("status = %d after transfer reset, want Failed", got)
	}
	if pendingRequestCount(l) != 0 {
		t.Fatal("receipt left pending after transfer reset")
	}
	if !released {
		t.Fatal("protect release not called")
	}
	select {
	case <-failed:
	case <-time.After(time.Second):
		t.Fatal("failed callback did not fire")
	}
}

// Python Resource.MAX_RETRIES: after 16 consecutive unproductive stalls the
// watchdog must abort the transfer and fail the bound receipt instead of
// retrying forever.
func TestTickIncomingResourceWatchdog_AbortsAfterMaxStallRetries(t *testing.T) {
	l := &Link{}
	r := newTestRequestReceipt(l, 0x48, time.Minute)
	if err := l.registerPendingRequest(r); err != nil {
		t.Fatalf("register: %v", err)
	}
	released := false
	rx := &incomingResourceAsm{
		request:           r,
		adv:               &resource.ResourceAdvertisement{Hash: bytes.Repeat([]byte{0x49}, 32)},
		lastProgressAt:    time.Now().Add(-time.Hour),
		outstandingParts:  1,
		totalParts:        1,
		mapHashes:         [][]byte{bytes.Repeat([]byte{0x4a}, resource.MapHashLen)},
		partSlots:         make([][]byte, 1),
		inflight:          []bool{true},
		consecutiveStalls: incomingResourceMaxStallRetries - 1,
		window:            4,
		windowMin:         1,
		windowMax:         4,
		protectRelease:    func() { released = true },
	}
	l.incomingMu.Lock()
	l.incomingRx = rx
	l.incomingMu.Unlock()

	if l.tickIncomingResourceWatchdog(rx) {
		t.Fatal("watchdog kept going after max stall retries")
	}
	l.incomingMu.Lock()
	current := l.incomingRx
	l.incomingMu.Unlock()
	if current != nil {
		t.Fatal("incomingRx not cleared after abort")
	}
	if got := r.GetStatus(); got != StatusFailed {
		t.Fatalf("bound receipt status = %d, want Failed", got)
	}
	if pendingRequestCount(l) != 0 {
		t.Fatal("bound receipt left pending after abort")
	}
	if !released {
		t.Fatal("protect release not called on abort")
	}
}

// A hard send failure on the retry path must abort the transfer rather than
// leave incomingRx installed forever holding buffers and protect budget.
func TestTickIncomingResourceWatchdog_SendFailureAbortsTransfer(t *testing.T) {
	l := &Link{}
	released := false
	rx := &incomingResourceAsm{
		adv:                  &resource.ResourceAdvertisement{Hash: bytes.Repeat([]byte{0x4b}, 32)},
		lastProgressAt:       time.Now().Add(-time.Hour),
		outstandingParts:     1,
		totalParts:           1,
		mapHashes:            [][]byte{bytes.Repeat([]byte{0x4c}, resource.MapHashLen)},
		partSlots:            make([][]byte, 1),
		inflight:             []bool{true},
		consecutiveCompleted: -1,
		window:               4,
		windowMin:            1,
		windowMax:            4,
		protectRelease:       func() { released = true },
	}
	l.incomingMu.Lock()
	l.incomingRx = rx
	l.incomingMu.Unlock()

	// Link is not active so SendPacketWithContext errors inside the retry.
	if l.tickIncomingResourceWatchdog(rx) {
		t.Fatal("watchdog kept going after send failure")
	}
	l.incomingMu.Lock()
	current := l.incomingRx
	l.incomingMu.Unlock()
	if current != nil {
		t.Fatal("incomingRx not cleared after send failure")
	}
	if !released {
		t.Fatal("protect release not called after send failure")
	}
}

// A new advertisement superseding a live transfer must release the old
// transfer's protect budget and fail its bound receipt; otherwise repeated
// advertisements exhaust the admission budget.
func TestBeginIncomingResource_SupersedeCleansUpOldTransfer(t *testing.T) {
	l := &Link{mdu: 384}
	l.status.Store(int32(StatusActive))

	r1 := newTestRequestReceipt(l, 0x50, time.Minute)
	if err := l.registerPendingRequest(r1); err != nil {
		t.Fatalf("register: %v", err)
	}
	advA := &resource.ResourceAdvertisement{
		Parts:        4,
		TransferSize: 4 * 384,
		DataSize:     1024,
		RandomHash:   bytes.Repeat([]byte{0x51}, resource.RandomHashSize),
		Hash:         bytes.Repeat([]byte{0x52}, 32),
		Hashmap:      makeFakeHashmap(1),
		RequestID:    r1.requestID,
		IsResponse:   true,
		Flags:        resource.AdvFlagIsResponse,
	}
	if err := l.beginIncomingResource(advA); err != nil {
		t.Fatalf("begin A: %v", err)
	}
	l.bindIncomingResponseRequest(advA, r1)

	advB := &resource.ResourceAdvertisement{
		Parts:        4,
		TransferSize: 4 * 384,
		DataSize:     1024,
		RandomHash:   bytes.Repeat([]byte{0x53}, resource.RandomHashSize),
		Hash:         bytes.Repeat([]byte{0x54}, 32),
		Hashmap:      makeFakeHashmap(1),
	}
	if err := l.beginIncomingResource(advB); err != nil {
		t.Fatalf("begin B: %v", err)
	}

	l.incomingMu.Lock()
	current := l.incomingRx
	l.incomingMu.Unlock()
	if current == nil || current.adv != advB {
		t.Fatal("superseding transfer not installed")
	}
	if got := r1.GetStatus(); got != StatusFailed {
		t.Fatalf("superseded receipt status = %d, want Failed", got)
	}
	if pendingRequestCount(l) != 0 {
		t.Fatal("superseded receipt left pending")
	}
	l.resetIncomingResource()
}

// A late resource completion for an already-failed receipt must not
// resurrect it or double-fire callbacks (Python response_received checks
// status != FAILED).
func TestCompleteRequestWithResourcePayload_SkipsFailedReceipt(t *testing.T) {
	l := &Link{}
	r := newTestRequestReceipt(l, 0x55, time.Minute)
	if err := l.registerPendingRequest(r); err != nil {
		t.Fatalf("register: %v", err)
	}
	l.failPendingRequest(r)

	answered := make(chan struct{}, 1)
	r.SetResponseCallback(func(*RequestReceipt) { answered <- struct{}{} })
	l.completeRequestWithResourcePayload(r, []byte("late"), nil)

	if got := r.GetStatus(); got != StatusFailed {
		t.Fatalf("status = %d, want still Failed", got)
	}
	select {
	case <-answered:
		t.Fatal("response callback fired on failed receipt")
	case <-time.After(100 * time.Millisecond):
	}
}

// A plain response that beat the in-flight resource also concludes the
// receipt; the resource's later completion must not fire again.
func TestCompleteRequestWithResourcePayload_SkipsActiveReceipt(t *testing.T) {
	l := &Link{}
	r := newTestRequestReceipt(l, 0x56, time.Minute)
	if err := l.registerPendingRequest(r); err != nil {
		t.Fatalf("register: %v", err)
	}
	var calls int32
	r.mutex.Lock()
	r.status = StatusActive
	r.responseCb = func(*RequestReceipt) { atomic.AddInt32(&calls, 1) }
	r.mutex.Unlock()
	l.removePendingRequest(r)

	l.completeRequestWithResourcePayload(r, []byte("late"), nil)
	time.Sleep(50 * time.Millisecond)
	if n := atomic.LoadInt32(&calls); n != 0 {
		t.Fatalf("response callback fired %d times on concluded receipt", n)
	}
}

// First part progress flips Pending to Receiving and fires the progress
// callback (Python response_resource_progress).
func TestReportIncomingResourceProgress_SetsReceivingAndFiresProgress(t *testing.T) {
	l := &Link{}
	r := newTestRequestReceipt(l, 0x57, time.Minute)
	if err := l.registerPendingRequest(r); err != nil {
		t.Fatalf("register: %v", err)
	}
	progress := make(chan int64, 4)
	r.SetProgressCallback(func(rr *RequestReceipt) {
		got, _ := rr.Progress()
		progress <- got
	})

	rx := &incomingResourceAsm{
		request:    r,
		partSlots:  make([][]byte, 2),
		totalParts: 2,
	}
	l.incomingMu.Lock()
	l.incomingRx = rx
	l.incomingMu.Unlock()
	rx.partSlots[0] = []byte("12345")

	l.incomingMu.Lock()
	l.reportIncomingResourceProgress(rx)
	l.incomingMu.Unlock()

	if got := r.GetStatus(); got != StatusReceiving {
		t.Fatalf("status = %d, want Receiving", got)
	}
	select {
	case n := <-progress:
		if n != 5 {
			t.Fatalf("progress = %d, want 5", n)
		}
	case <-time.After(time.Second):
		t.Fatal("progress callback did not fire")
	}
}
