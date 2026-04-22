package common

import (
	"testing"
	"time"
)

func TestNewBaseInterface(t *testing.T) {
	iface := NewBaseInterface("test0", IFTypeUDP, true)

	if iface.Name != "test0" {
		t.Errorf("Name = %q, want %q", iface.Name, "test0")
	}
	if iface.Type != IFTypeUDP {
		t.Errorf("Type = %v, want %v", iface.Type, IFTypeUDP)
	}
	if iface.Mode != IFModeFull {
		t.Errorf("Mode = %v, want %v", iface.Mode, IFModeFull)
	}
	if !iface.Enabled {
		t.Errorf("Enabled = %v, want true", iface.Enabled)
	}
	if iface.MTU != DefaultMTU {
		t.Errorf("MTU = %d, want %d", iface.MTU, DefaultMTU)
	}
	if iface.Bitrate != BitrateMinimum {
		t.Errorf("Bitrate = %d, want %d", iface.Bitrate, BitrateMinimum)
	}
}

func TestBaseInterface_GetType(t *testing.T) {
	iface := NewBaseInterface("test1", IFTypeTCP, true)
	if iface.GetType() != IFTypeTCP {
		t.Errorf("GetType() = %v, want %v", iface.GetType(), IFTypeTCP)
	}
}

func TestBaseInterface_GetMode(t *testing.T) {
	iface := NewBaseInterface("test2", IFTypeUDP, true)
	if iface.GetMode() != IFModeFull {
		t.Errorf("GetMode() = %v, want %v", iface.GetMode(), IFModeFull)
	}
}

func TestBaseInterface_GetMTU(t *testing.T) {
	iface := NewBaseInterface("test3", IFTypeUDP, true)
	if iface.GetMTU() != DefaultMTU {
		t.Errorf("GetMTU() = %d, want %d", iface.GetMTU(), DefaultMTU)
	}
}

func TestBaseInterface_GetName(t *testing.T) {
	iface := NewBaseInterface("test4", IFTypeUDP, true)
	if iface.GetName() != "test4" {
		t.Errorf("GetName() = %q, want %q", iface.GetName(), "test4")
	}
}

func TestBaseInterface_IsEnabled(t *testing.T) {
	iface := NewBaseInterface("test5", IFTypeUDP, true)
	iface.Online = true
	iface.Detached = false

	if !iface.IsEnabled() {
		t.Error("IsEnabled() = false, want true")
	}

	iface.Enabled = false
	if iface.IsEnabled() {
		t.Error("IsEnabled() = true, want false when disabled")
	}

	iface.Enabled = true
	iface.Online = false
	if iface.IsEnabled() {
		t.Error("IsEnabled() = true, want false when offline")
	}

	iface.Online = true
	iface.Detached = true
	if iface.IsEnabled() {
		t.Error("IsEnabled() = true, want false when detached")
	}
}

func TestBaseInterface_IsOnline(t *testing.T) {
	iface := NewBaseInterface("test6", IFTypeUDP, true)
	iface.Online = true

	if !iface.IsOnline() {
		t.Error("IsOnline() = false, want true")
	}

	iface.Online = false
	if iface.IsOnline() {
		t.Error("IsOnline() = true, want false")
	}
}

func TestBaseInterface_IsDetached(t *testing.T) {
	iface := NewBaseInterface("test7", IFTypeUDP, true)
	iface.Detached = true

	if !iface.IsDetached() {
		t.Error("IsDetached() = false, want true")
	}

	iface.Detached = false
	if iface.IsDetached() {
		t.Error("IsDetached() = true, want false")
	}
}

func TestBaseInterface_SetPacketCallback(t *testing.T) {
	iface := NewBaseInterface("test8", IFTypeUDP, true)

	callback := func(data []byte, ni NetworkInterface) {}
	iface.SetPacketCallback(callback)

	if iface.GetPacketCallback() == nil {
		t.Error("GetPacketCallback() = nil, want callback")
	}
}

func TestBaseInterface_GetPacketCallback(t *testing.T) {
	iface := NewBaseInterface("test9", IFTypeUDP, true)

	if iface.GetPacketCallback() != nil {
		t.Error("GetPacketCallback() != nil, want nil")
	}

	callback := func(data []byte, ni NetworkInterface) {}
	iface.SetPacketCallback(callback)

	if iface.GetPacketCallback() == nil {
		t.Error("GetPacketCallback() = nil, want callback")
	}
}

func TestBaseInterface_Detach(t *testing.T) {
	iface := NewBaseInterface("test10", IFTypeUDP, true)
	iface.Online = true
	iface.Detached = false

	iface.Detach()

	if !iface.IsDetached() {
		t.Error("IsDetached() = false, want true after Detach()")
	}
	if iface.IsOnline() {
		t.Error("IsOnline() = true, want false after Detach()")
	}
}

func TestBaseInterface_Enable(t *testing.T) {
	iface := NewBaseInterface("test11", IFTypeUDP, false)
	iface.Online = false

	iface.Enable()

	if !iface.Enabled {
		t.Error("Enabled = false, want true after Enable()")
	}
	if !iface.IsOnline() {
		t.Error("IsOnline() = false, want true after Enable()")
	}
}

func TestBaseInterface_Disable(t *testing.T) {
	iface := NewBaseInterface("test12", IFTypeUDP, true)
	iface.Online = true

	iface.Disable()

	if iface.Enabled {
		t.Error("Enabled = true, want false after Disable()")
	}
	if iface.IsOnline() {
		t.Error("IsOnline() = true, want false after Disable()")
	}
}

func TestBaseInterface_Start(t *testing.T) {
	iface := NewBaseInterface("test13", IFTypeUDP, true)
	if err := iface.Start(); err != nil {
		t.Errorf("Start() error = %v, want nil", err)
	}
}

func TestBaseInterface_Stop(t *testing.T) {
	iface := NewBaseInterface("test14", IFTypeUDP, true)
	if err := iface.Stop(); err != nil {
		t.Errorf("Stop() error = %v, want nil", err)
	}
}

func TestBaseInterface_GetConn(t *testing.T) {
	iface := NewBaseInterface("test15", IFTypeUDP, true)
	if iface.GetConn() != nil {
		t.Error("GetConn() != nil, want nil")
	}
}

func TestBaseInterface_Send(t *testing.T) {
	iface := NewBaseInterface("test16", IFTypeUDP, true)
	data := []byte("test data")

	if err := iface.Send(data, ""); err == nil {
		t.Error("Send() on abstract BaseInterface must propagate the ProcessOutgoing error so dispatch bugs surface immediately")
	}
}

func TestBaseInterface_ProcessIncoming(t *testing.T) {
	iface := NewBaseInterface("test17", IFTypeUDP, true)

	called := false
	callback := func(data []byte, ni NetworkInterface) {
		called = true
	}
	iface.SetPacketCallback(callback)

	data := []byte("test")
	iface.ProcessIncoming(data)

	if !called {
		t.Error("ProcessIncoming() did not call callback")
	}

	iface.SetPacketCallback(nil)
	iface.ProcessIncoming(data)
}

func TestBaseInterface_ProcessOutgoing(t *testing.T) {
	iface := NewBaseInterface("test18", IFTypeUDP, true)
	data := []byte("test data")

	if err := iface.ProcessOutgoing(data); err == nil {
		t.Error("ProcessOutgoing() on abstract BaseInterface must return an error so dispatch bugs surface immediately")
	}
}

func TestBaseInterface_SendPathRequest(t *testing.T) {
	iface := NewBaseInterface("test19", IFTypeUDP, true)
	data := []byte("path request")

	if err := iface.SendPathRequest(data); err == nil {
		t.Error("SendPathRequest() on abstract BaseInterface must propagate the ProcessOutgoing error")
	}
}

func TestBaseInterface_SendLinkPacket(t *testing.T) {
	iface := NewBaseInterface("test20", IFTypeUDP, true)
	dest := []byte("destination")
	data := []byte("link data")
	timestamp := time.Now()

	if err := iface.SendLinkPacket(dest, data, timestamp); err == nil {
		t.Error("SendLinkPacket() on abstract BaseInterface must propagate the ProcessOutgoing error")
	}
}

func TestBaseInterface_GetBandwidthAvailable(t *testing.T) {
	iface := NewBaseInterface("test21", IFTypeUDP, true)

	if !iface.GetBandwidthAvailable() {
		t.Error("GetBandwidthAvailable() = false, want true when no recent transmission")
	}

	iface.lastTx = time.Now()
	iface.TxBytes = 0
	if !iface.GetBandwidthAvailable() {
		t.Error("GetBandwidthAvailable() = false, want true when TxBytes is 0")
	}

	iface.lastTx = time.Now().Add(-500 * time.Millisecond)
	iface.TxBytes = 1000
	iface.Bitrate = 1000000

	if !iface.GetBandwidthAvailable() {
		t.Error("GetBandwidthAvailable() = false, want true when usage is below threshold")
	}

	iface.TxBytes = 10000000
	iface.Bitrate = 1000
	if iface.GetBandwidthAvailable() {
		t.Error("GetBandwidthAvailable() = true, want false when usage exceeds threshold")
	}
}
