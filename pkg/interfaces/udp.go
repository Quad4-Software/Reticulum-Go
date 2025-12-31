package interfaces

import (
	"fmt"
	"net"
	"sync"

	"git.quad4.io/Networks/Reticulum-Go/pkg/common"
	"git.quad4.io/Networks/Reticulum-Go/pkg/debug"
)

type UDPInterface struct {
	BaseInterface
	conn       *net.UDPConn
	addr       *net.UDPAddr
	targetAddr *net.UDPAddr
	readBuffer []byte
	done       chan struct{}
	stopOnce   sync.Once
}

func NewUDPInterface(name string, addr string, target string, enabled bool) (*UDPInterface, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}

	var targetAddr *net.UDPAddr
	if target != "" {
		targetAddr, err = net.ResolveUDPAddr("udp", target)
		if err != nil {
			return nil, err
		}
	}

	ui := &UDPInterface{
		BaseInterface: NewBaseInterface(name, common.IF_TYPE_UDP, enabled),
		addr:          udpAddr,
		targetAddr:    targetAddr,
		readBuffer:    make([]byte, common.NUM_1064),
		done:          make(chan struct{}),
	}

	ui.MTU = common.NUM_1064

	return ui, nil
}

func (ui *UDPInterface) GetName() string {
	return ui.Name
}

func (ui *UDPInterface) GetType() common.InterfaceType {
	return ui.Type
}

func (ui *UDPInterface) GetMode() common.InterfaceMode {
	return ui.Mode
}

func (ui *UDPInterface) IsOnline() bool {
	ui.Mutex.RLock()
	defer ui.Mutex.RUnlock()
	return ui.Online
}

func (ui *UDPInterface) IsDetached() bool {
	ui.Mutex.RLock()
	defer ui.Mutex.RUnlock()
	return ui.Detached
}

func (ui *UDPInterface) Detach() {
	ui.Mutex.Lock()
	defer ui.Mutex.Unlock()
	ui.Detached = true
	ui.Online = false
	if ui.conn != nil {
		ui.conn.Close() // #nosec G104
	}
	ui.stopOnce.Do(func() {
		if ui.done != nil {
			close(ui.done)
		}
	})
}

func (ui *UDPInterface) Send(data []byte, addr string) error {
	debug.Log(debug.DEBUG_ALL, "UDP interface sending bytes", "name", ui.Name, "bytes", len(data))

	if !ui.IsEnabled() {
		return fmt.Errorf("interface not enabled")
	}

	if ui.targetAddr == nil {
		return fmt.Errorf("no target address configured")
	}

	ui.Mutex.Lock()
	ui.TxBytes += uint64(len(data))
	ui.Mutex.Unlock()

	_, err := ui.conn.WriteTo(data, ui.targetAddr)
	if err != nil {
		debug.Log(debug.DEBUG_CRITICAL, "UDP interface write failed", "name", ui.Name, "error", err)
	} else {
		debug.Log(debug.DEBUG_ALL, "UDP interface sent bytes successfully", "name", ui.Name, "bytes", len(data))
	}
	return err
}

func (ui *UDPInterface) SetPacketCallback(callback common.PacketCallback) {
	ui.Mutex.Lock()
	defer ui.Mutex.Unlock()
	ui.packetCallback = callback
}

func (ui *UDPInterface) GetPacketCallback() common.PacketCallback {
	ui.Mutex.RLock()
	defer ui.Mutex.RUnlock()
	return ui.packetCallback
}

func (ui *UDPInterface) ProcessIncoming(data []byte) {
	if callback := ui.GetPacketCallback(); callback != nil {
		callback(data, ui)
	}
}

func (ui *UDPInterface) ProcessOutgoing(data []byte) error {
	if !ui.IsOnline() {
		return fmt.Errorf("interface offline")
	}

	if ui.targetAddr == nil {
		return fmt.Errorf("no target address configured")
	}

	_, err := ui.conn.WriteToUDP(data, ui.targetAddr)
	if err != nil {
		return fmt.Errorf("UDP write failed: %v", err)
	}

	ui.Mutex.Lock()
	ui.TxBytes += uint64(len(data))
	ui.Mutex.Unlock()

	return nil
}

func (ui *UDPInterface) GetConn() net.Conn {
	return ui.conn
}

func (ui *UDPInterface) GetTxBytes() uint64 {
	ui.Mutex.RLock()
	defer ui.Mutex.RUnlock()
	return ui.TxBytes
}

func (ui *UDPInterface) GetRxBytes() uint64 {
	ui.Mutex.RLock()
	defer ui.Mutex.RUnlock()
	return ui.RxBytes
}

func (ui *UDPInterface) GetMTU() int {
	return ui.MTU
}

func (ui *UDPInterface) GetBitrate() int {
	return int(ui.Bitrate)
}

func (ui *UDPInterface) Enable() {
	ui.Mutex.Lock()
	defer ui.Mutex.Unlock()
	ui.Online = true
}

func (ui *UDPInterface) Disable() {
	ui.Mutex.Lock()
	defer ui.Mutex.Unlock()
	ui.Online = false
}

func (ui *UDPInterface) Start() error {
	ui.Mutex.Lock()
	if ui.conn != nil {
		ui.Mutex.Unlock()
		return fmt.Errorf("UDP interface already started")
	}
	// Only recreate done if it's nil or was closed
	select {
	case <-ui.done:
		ui.done = make(chan struct{})
		ui.stopOnce = sync.Once{}
	default:
		if ui.done == nil {
			ui.done = make(chan struct{})
			ui.stopOnce = sync.Once{}
		}
	}
	ui.Mutex.Unlock()

	conn, err := net.ListenUDP("udp", ui.addr)
	if err != nil {
		return err
	}
	ui.conn = conn

	// Enable broadcast mode if we have a target address
	if ui.targetAddr != nil {
		// Get the raw connection file descriptor to set SO_BROADCAST
		if err := conn.SetReadBuffer(common.NUM_1064); err != nil {
			debug.Log(debug.DEBUG_ERROR, "Failed to set read buffer size", "error", err)
		}
		if err := conn.SetWriteBuffer(common.NUM_1064); err != nil {
			debug.Log(debug.DEBUG_ERROR, "Failed to set write buffer size", "error", err)
		}
	}

	ui.Mutex.Lock()
	ui.Online = true
	ui.Mutex.Unlock()

	// Start the read loop in a goroutine
	go ui.readLoop()

	return nil
}

func (ui *UDPInterface) Stop() error {
	ui.Detach()
	return nil
}

func (ui *UDPInterface) readLoop() {
	buffer := make([]byte, common.NUM_1064)
	for {
		ui.Mutex.RLock()
		online := ui.Online
		detached := ui.Detached
		conn := ui.conn
		done := ui.done
		ui.Mutex.RUnlock()

		if !online || detached || conn == nil {
			return
		}

		select {
		case <-done:
			return
		default:
		}

		n, remoteAddr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			ui.Mutex.RLock()
			stillOnline := ui.Online
			ui.Mutex.RUnlock()
			if stillOnline {
				debug.Log(debug.DEBUG_ERROR, "Error reading from UDP interface", "name", ui.Name, "error", err)
			}
			return
		}

		ui.Mutex.Lock()
		// #nosec G115 - Network read sizes are always positive and within safe range
		ui.RxBytes += uint64(n)

		// Auto-discover target address from first packet if not set
		if ui.targetAddr == nil {
			debug.Log(debug.DEBUG_ALL, "UDP interface discovered peer", "name", ui.Name, "peer", remoteAddr.String())
			ui.targetAddr = remoteAddr
		}
		callback := ui.packetCallback
		ui.Mutex.Unlock()

		if callback != nil {
			callback(buffer[:n], ui)
		}
	}
}

func (ui *UDPInterface) IsEnabled() bool {
	ui.Mutex.RLock()
	defer ui.Mutex.RUnlock()
	return ui.Enabled && ui.Online && !ui.Detached
}
