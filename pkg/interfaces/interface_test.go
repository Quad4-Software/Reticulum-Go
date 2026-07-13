// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"bytes"
	"net"
	"sync"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
)

func TestBaseInterfaceStateChanges(t *testing.T) {
	bi := NewBaseInterface("test", common.IFTypeTCP, false) // Start disabled

	if bi.IsEnabled() {
		t.Error("Newly created disabled interface reports IsEnabled() == true")
	}
	if bi.IsOnline() {
		t.Error("Newly created disabled interface reports IsOnline() == true")
	}
	if bi.IsDetached() {
		t.Error("Newly created interface reports IsDetached() == true")
	}

	bi.Enable()
	if !bi.IsEnabled() {
		t.Error("After Enable(), IsEnabled() == false")
	}
	if !bi.IsOnline() {
		t.Error("After Enable(), IsOnline() == false")
	}
	if bi.IsDetached() {
		t.Error("After Enable(), IsDetached() == true")
	}

	bi.Detach()
	if bi.IsEnabled() {
		t.Error("After Detach(), IsEnabled() == true")
	}
	if bi.IsOnline() {
		t.Error("After Detach(), IsOnline() == true")
	}
	if !bi.IsDetached() {
		t.Error("After Detach(), IsDetached() == false")
	}

	// Reset for Disable test
	bi = NewBaseInterface("test2", common.IFTypeUDP, true) // Start enabled
	if !bi.Enabled {                                       // Check the Enabled field directly first
		t.Error("Newly created enabled interface reports Enabled == false")
	}
	if bi.IsEnabled() { // IsEnabled should still be false because Online is false
		t.Error("Newly created enabled interface reports IsEnabled() == true before Enable() is called")
	}

	bi.Enable()          // Explicitly enable to set Online = true
	if !bi.IsEnabled() { // Now IsEnabled should be true
		t.Error("After Enable() on initially enabled interface, IsEnabled() == false")
	}

	bi.Disable()
	if bi.Enabled { // Check Enabled field after Disable()
		t.Error("After Disable(), Enabled == true")
	}
	if bi.IsOnline() {
		t.Error("After Disable(), IsOnline() == true")
	}
	if bi.IsDetached() { // Disable doesn't detach
		t.Error("After Disable(), IsDetached() == true")
	}
}

func TestBaseInterfaceGetters(t *testing.T) {
	bi := NewBaseInterface("getterTest", common.IFTypeAuto, true)

	if bi.GetName() != "getterTest" {
		t.Errorf("GetName() = %s; want getterTest", bi.GetName())
	}
	if bi.GetType() != common.IFTypeAuto {
		t.Errorf("GetType() = %v; want %v", bi.GetType(), common.IFTypeAuto)
	}
	if bi.GetMode() != common.IFModeFull {
		t.Errorf("GetMode() = %v; want %v", bi.GetMode(), common.IFModeFull)
	}
	if bi.GetMTU() != common.DefaultMTU { // Assuming default MTU
		t.Errorf("GetMTU() = %d; want %d", bi.GetMTU(), common.DefaultMTU)
	}
}

func TestBaseInterfaceCallbacks(t *testing.T) {
	bi := NewBaseInterface("callbackTest", common.IFTypeTCP, true)
	var wg sync.WaitGroup
	var callbackCalled bool

	callback := func(data []byte, iface common.NetworkInterface) {
		if len(data) != 5 {
			t.Errorf("Callback received data length %d; want 5", len(data))
		}
		if iface.GetName() != "callbackTest" {
			t.Errorf("Callback received interface name %s; want callbackTest", iface.GetName())
		}
		callbackCalled = true
		wg.Done()
	}

	bi.SetPacketCallback(callback)
	if bi.GetPacketCallback() == nil { // Cannot directly compare functions
		t.Error("GetPacketCallback() returned nil after SetPacketCallback()")
	}

	wg.Add(1)
	go bi.ProcessIncoming([]byte{1, 2, 3, 4, 5}) // Run in goroutine as callback might block

	// Wait for callback or timeout
	waitTimeout(&wg, 1*time.Second, t)

	if !callbackCalled {
		t.Error("Packet callback was not called after ProcessIncoming")
	}
}

func TestBaseInterfaceStats(t *testing.T) {
	bi := NewBaseInterface("statsTest", common.IFTypeUDP, true)
	bi.Enable() // Need to be Online for ProcessOutgoing

	data1 := []byte{1, 2, 3}
	data2 := []byte{4, 5, 6, 7, 8}

	bi.ProcessIncoming(data1)
	if bi.RxBytes != uint64(len(data1)) {
		t.Errorf("RxBytes = %d; want %d after first ProcessIncoming", bi.RxBytes, len(data1))
	}

	bi.ProcessIncoming(data2)
	if bi.RxBytes != uint64(len(data1)+len(data2)) {
		t.Errorf("RxBytes = %d; want %d after second ProcessIncoming", bi.RxBytes, len(data1)+len(data2))
	}

	// BaseInterface.ProcessOutgoing is now a fail-loud stub that the
	// concrete interface type is required to override. Calling it directly
	// must return an error and must NOT mutate TxBytes. Otherwise we lose

	// our compile/runtime guarantee that the abstract base never silently
	// swallows packets.
	if err := bi.ProcessOutgoing(data1); err == nil {
		t.Fatal("expected BaseInterface.ProcessOutgoing to return an error, got nil")
	}
	if bi.TxBytes != 0 {
		t.Errorf("TxBytes = %d; want 0 (BaseInterface.ProcessOutgoing must not update stats)", bi.TxBytes)
	}
}

func TestUpdateBandwidthStatsAccumulatesTxBytes(t *testing.T) {
	bi := NewBaseInterface("txStatsTest", common.IFTypeUDP, true)

	bi.updateBandwidthStats(128)
	bi.updateBandwidthStats(64)

	if bi.TxBytes != 192 {
		t.Errorf("TxBytes = %d; want 192 after updateBandwidthStats calls", bi.TxBytes)
	}
	if bi.GetTxBytes() != 192 {
		t.Errorf("GetTxBytes() = %d; want 192", bi.GetTxBytes())
	}
}

// Helper function to wait for a WaitGroup with a timeout
func waitTimeout(wg *sync.WaitGroup, timeout time.Duration, t *testing.T) {
	c := make(chan struct{})
	go func() {
		defer close(c)
		wg.Wait()
	}()
	select {
	case <-c:
		// Completed normally
	case <-time.After(timeout):
		t.Fatal("Timed out waiting for WaitGroup")
	}
}

// Minimal mock interface for InterceptedInterface test
type mockInterface struct {
	BaseInterface
	sendCalled bool
	sendData   []byte
}

func (m *mockInterface) Send(data []byte, addr string) error {
	m.sendCalled = true
	m.sendData = data
	return nil
}

func (m *mockInterface) GetType() common.InterfaceType                  { return common.IFTypeNone }
func (m *mockInterface) GetMode() common.InterfaceMode                  { return common.IFModeFull }
func (m *mockInterface) ProcessIncoming(data []byte)                    {}
func (m *mockInterface) ProcessOutgoing(data []byte) error              { return nil }
func (m *mockInterface) SendPathRequest([]byte) error                   { return nil }
func (m *mockInterface) SendLinkPacket([]byte, []byte, time.Time) error { return nil }
func (m *mockInterface) Start() error                                   { return nil }
func (m *mockInterface) Stop() error                                    { return nil }
func (m *mockInterface) GetConn() net.Conn                              { return nil }
func (m *mockInterface) GetBandwidthAvailable() bool                    { return true }

func TestInterceptedInterface(t *testing.T) {
	mockBase := &mockInterface{}
	var interceptorCalled bool
	var interceptedData []byte

	interceptor := func(data []byte, iface common.NetworkInterface) error {
		interceptorCalled = true
		interceptedData = data
		return nil
	}

	intercepted := NewInterceptedInterface(mockBase, interceptor)

	testData := []byte("intercept me")
	err := intercepted.Send(testData, "dummy_addr")
	if err != nil {
		t.Fatalf("Intercepted Send failed: %v", err)
	}

	if !interceptorCalled {
		t.Error("Interceptor function was not called")
	}
	if !bytes.Equal(interceptedData, testData) {
		t.Errorf("Interceptor received data %x; want %x", interceptedData, testData)
	}

	if !mockBase.sendCalled {
		t.Error("Original Send function was not called")
	}
	if !bytes.Equal(mockBase.sendData, testData) {
		t.Errorf("Original Send received data %x; want %x", mockBase.sendData, testData)
	}
}

func TestReceivedAnnounceFrequency(t *testing.T) {
	bi := NewBaseInterface("freq", common.IFTypeUDP, true)
	if bi.IncomingAnnounceFrequency() != 0 {
		t.Fatalf("expected zero frequency before samples, got %v", bi.IncomingAnnounceFrequency())
	}
	for range 4 {
		bi.ReceivedAnnounce()
		time.Sleep(5 * time.Millisecond)
	}
	freq := bi.IncomingAnnounceFrequency()
	if freq <= 0 {
		t.Fatalf("expected positive announce frequency after samples, got %v", freq)
	}
}

func TestSampleTrafficSpeeds(t *testing.T) {
	bi := NewBaseInterface("speed", common.IFTypeUDP, true)
	bi.SampleTraffic()
	if bi.GetRxSpeed() != 0 || bi.GetTxSpeed() != 0 {
		t.Fatalf("expected zero speeds on first sample, got rx=%v tx=%v", bi.GetRxSpeed(), bi.GetTxSpeed())
	}
	bi.Mutex.Lock()
	bi.RxBytes += 1000
	bi.TxBytes += 2000
	bi.Mutex.Unlock()
	time.Sleep(20 * time.Millisecond)
	bi.SampleTraffic()
	if bi.GetRxSpeed() <= 0 {
		t.Fatalf("expected non-zero RX speed after byte increase, got %v", bi.GetRxSpeed())
	}
	if bi.GetTxSpeed() <= 0 {
		t.Fatalf("expected non-zero TX speed after byte increase, got %v", bi.GetTxSpeed())
	}
}

func TestGetBandwidthAvailable_UsesSampledTX(t *testing.T) {
	bi := NewBaseInterface("bw", common.IFTypeTCP, true)
	bi.Bitrate = BitrateGuess
	bi.lastTx = time.Now()
	bi.TxBytes = 1 << 20 // lifetime bytes must not alone close the gate
	if !bi.GetBandwidthAvailable() {
		t.Fatal("expected available without a TX sample")
	}
	bi.currentTXS = float64(bi.Bitrate) * PropagationRate * 2
	if bi.GetBandwidthAvailable() {
		t.Fatal("expected unavailable when sampled TX exceeds announce cap")
	}
	bi.currentTXS = float64(bi.Bitrate) * PropagationRate * 0.1
	if !bi.GetBandwidthAvailable() {
		t.Fatal("expected available when sampled TX is under announce cap")
	}
}
