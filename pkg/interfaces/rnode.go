// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build !js

package interfaces

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
)

const (
	rnodeSerialSpeed      = 115200
	rnodeTCPDefaultPort   = 7633
	rnodeDefaultConfigure = 2 * time.Second
	rnodeDefaultValidate  = 250 * time.Millisecond
	rnodeDefaultDetect    = 200 * time.Millisecond
	rnodeTCPDetect        = 5 * time.Second
	rnodeDefaultReconnect = 5 * time.Second
	rnodeReadChunk        = 4096
)

// RNodeDialFunc dials an RNode TCP endpoint.
type RNodeDialFunc func(ctx context.Context, network, address string) (net.Conn, error)

// RNodeOptions configures an RNodeInterface.
type RNodeOptions struct {
	Port                  string
	Frequency             int64
	Bandwidth             int
	TXPower               int
	SF                    int
	CR                    int
	FlowControl           bool
	IDInterval            time.Duration
	Callsign              string
	STAirTimeLock         *float64
	LTAirTimeLock         *float64
	Open                  SerialOpenFunc
	Dial                  RNodeDialFunc
	ConfigureDelay        time.Duration
	ValidateDelay         time.Duration
	DetectTimeout         time.Duration
	ReconnectDelay        time.Duration
	SkipFirmwareCheck     bool
	MaxReconnectTries     int
	PanicOnInterfaceError bool
}

type rnodeRadioReport struct {
	frequency int64
	bandwidth int
	txPower   int
	sf        int
	cr        int
	state     byte
	haveFreq  bool
	haveBW    bool
	haveTX    bool
	haveSF    bool
	haveCR    bool
	haveState bool
}

// RNodeInterface carries Reticulum packets over an RNode KISS connection.
type RNodeInterface struct {
	BaseInterface

	opts RNodeOptions

	port    SerialPort
	txMu    sync.Mutex
	txFrame []byte

	lifecycleMu   sync.Mutex
	reconnecting  bool
	reconnectLeft int
	done          chan struct{}
	stopOnce      sync.Once
	beaconOnce    sync.Once

	stateMu       sync.Mutex
	detected      bool
	firmwareSeen  bool
	firmwareMajor int
	firmwareMinor int
	platform      byte
	mcu           byte
	report        rnodeRadioReport
	rssi          float64
	snr           float64
	quality       float64
	startupErr    error

	queueMu        sync.Mutex
	packetQueue    [][]byte
	interfaceReady bool
	firstTX        time.Time
	callsign       []byte
}

// NewRNodeInterface constructs an RNode interface. Call Start to open it.
func NewRNodeInterface(name string, enabled bool, opts RNodeOptions) (*RNodeInterface, error) {
	opts = normalizeRNodeOptions(opts)
	if err := validateRNodeOptions(opts); err != nil {
		return nil, err
	}
	r := &RNodeInterface{
		BaseInterface: NewBaseInterface(name, common.IFTypeRNode, enabled),
		opts:          opts,
		done:          make(chan struct{}),
		reconnectLeft: NormalizeMaxReconnectTries(opts.MaxReconnectTries),
		quality:       -1,
		txFrame:       make([]byte, 0, rnodeHWMTU*2+4),
		callsign:      []byte(opts.Callsign),
	}
	r.In = true
	r.Out = true
	r.MTU = rnodeHWMTU
	r.Bitrate = int64(rnodeComputeBitrate(opts.SF, opts.CR, opts.Bandwidth))
	return r, nil
}

func normalizeRNodeOptions(opts RNodeOptions) RNodeOptions {
	if opts.ConfigureDelay <= 0 {
		opts.ConfigureDelay = rnodeDefaultConfigure
	}
	if opts.ValidateDelay <= 0 {
		opts.ValidateDelay = rnodeDefaultValidate
	}
	if opts.DetectTimeout <= 0 {
		scheme, _, isURI := parseRNodePortURI(opts.Port)
		switch {
		case strings.HasPrefix(strings.ToLower(opts.Port), "tcp://"):
			opts.DetectTimeout = rnodeTCPDetect
		case isURI && (scheme == "ble" || scheme == "bt"):
			opts.DetectTimeout = rnodeTCPDetect
		default:
			opts.DetectTimeout = rnodeDefaultDetect
		}
	}
	if opts.ReconnectDelay <= 0 {
		opts.ReconnectDelay = rnodeDefaultReconnect
	}
	if opts.Dial == nil {
		opts.Dial = (&net.Dialer{Timeout: 5 * time.Second}).DialContext
	}
	if opts.Open == nil {
		opts.Open = openRNodeSerial
	}
	return opts
}

func validateRNodeOptions(opts RNodeOptions) error {
	if strings.TrimSpace(opts.Port) == "" {
		return errors.New("no port specified for RNode interface")
	}
	if scheme, _, ok := parseRNodePortURI(opts.Port); ok {
		switch scheme {
		case "ble", "bt", "usb":
			// Openers are resolved at Start time so hosts can register first.
		default:
			return fmt.Errorf("unsupported RNode URI scheme %q", scheme)
		}
	}
	if opts.Frequency < rnodeFreqMin || opts.Frequency > rnodeFreqMax {
		return fmt.Errorf("RNode frequency must be between %d and %d", rnodeFreqMin, rnodeFreqMax)
	}
	if opts.TXPower < 0 || opts.TXPower > 37 {
		return errors.New("RNode TX power must be between 0 and 37")
	}
	if opts.Bandwidth < 7800 || opts.Bandwidth > 1625000 {
		return errors.New("RNode bandwidth must be between 7800 and 1625000")
	}
	if opts.SF < 5 || opts.SF > 12 {
		return errors.New("RNode spreading factor must be between 5 and 12")
	}
	if opts.CR < 5 || opts.CR > 8 {
		return errors.New("RNode coding rate must be between 5 and 8")
	}
	if err := validateRNodeAirtime(opts.STAirTimeLock, "short-term"); err != nil {
		return err
	}
	if err := validateRNodeAirtime(opts.LTAirTimeLock, "long-term"); err != nil {
		return err
	}
	if len([]byte(opts.Callsign)) > rnodeCallsignMaxLen {
		return fmt.Errorf("RNode callsign exceeds %d bytes", rnodeCallsignMaxLen)
	}
	return nil
}

func validateRNodeAirtime(value *float64, name string) error {
	if value != nil && (*value < 0 || *value > 100) {
		return fmt.Errorf("RNode %s airtime limit must be between 0 and 100", name)
	}
	return nil
}

func (r *RNodeInterface) String() string {
	return fmt.Sprintf("RNodeInterface[%s]", r.Name)
}

// Start opens, detects, configures, and validates the RNode.
func (r *RNodeInterface) Start() error {
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()
	r.Mutex.RLock()
	if r.Online {
		r.Mutex.RUnlock()
		return nil
	}
	enabled := r.Enabled
	detached := r.Detached
	r.Mutex.RUnlock()
	if !enabled {
		return errors.New("RNode interface is not enabled")
	}
	if detached {
		return errors.New("RNode interface is detached")
	}
	return r.startLocked()
}

func (r *RNodeInterface) startLocked() error {
	port, err := r.openPort()
	if err != nil {
		return err
	}
	r.Mutex.Lock()
	r.port = port
	r.Online = false
	r.Mutex.Unlock()
	r.resetStartupState()
	if !sleepRNode(r.done, r.opts.ConfigureDelay) {
		r.closePort()
		return context.Canceled
	}
	go r.readLoop(port)
	if err := r.writeFrameBytes(appendRNodeDetect(nil)); err != nil {
		r.closePort()
		return err
	}
	if !r.waitDetected(r.opts.DetectTimeout) {
		r.closePort()
		return errors.New("RNode detection timed out")
	}
	if err := r.checkFirmware(rnodeRequiredFWMaj, rnodeRequiredFWMin); err != nil {
		r.closePort()
		return err
	}
	if err := r.initRadio(); err != nil {
		r.closePort()
		return err
	}
	if !sleepRNode(r.done, r.opts.ValidateDelay) {
		r.closePort()
		return context.Canceled
	}
	if err := r.validateRadioState(); err != nil {
		r.closePort()
		return err
	}
	r.queueMu.Lock()
	r.interfaceReady = true
	r.queueMu.Unlock()
	r.Mutex.Lock()
	r.Online = true
	r.Mutex.Unlock()
	if r.opts.IDInterval > 0 && r.opts.Callsign != "" {
		r.beaconOnce.Do(func() { go r.idLoop() })
	}
	return nil
}

func (r *RNodeInterface) openPort() (SerialPort, error) {
	lower := strings.ToLower(r.opts.Port)
	if strings.HasPrefix(lower, "tcp://") {
		address, err := parseRNodeTCPAddress(r.opts.Port)
		if err != nil {
			return nil, err
		}
		conn, err := r.opts.Dial(context.Background(), "tcp", address)
		if err != nil {
			return nil, err
		}
		return conn, nil
	}
	if _, _, isURI := parseRNodePortURI(r.opts.Port); isURI {
		return openRNodeRegisteredPort(r.opts.Port)
	}
	if opener := lookupRNodePortOpener("usb"); opener != nil {
		// Bare Android USB device names (usb4a-style) can be registered as usb.
		if port, err := opener(r.opts.Port); err == nil {
			return port, nil
		}
	}
	return r.opts.Open(SerialOptions{
		Device:   r.opts.Port,
		Speed:    rnodeSerialSpeed,
		DataBits: 8,
		Parity:   "N",
		StopBits: 1,
	})
}

func parseRNodeTCPAddress(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("invalid RNode TCP port %q", raw)
	}
	host := u.Hostname()
	if host == "" {
		return "", fmt.Errorf("invalid RNode TCP host %q", raw)
	}
	port := u.Port()
	if port == "" {
		port = strconv.Itoa(rnodeTCPDefaultPort)
	}
	return net.JoinHostPort(host, port), nil
}

func (r *RNodeInterface) resetStartupState() {
	r.stateMu.Lock()
	r.detected = false
	r.firmwareSeen = false
	r.firmwareMajor = 0
	r.firmwareMinor = 0
	r.startupErr = nil
	r.report = rnodeRadioReport{}
	r.stateMu.Unlock()
}

func (r *RNodeInterface) waitDetected(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		r.stateMu.Lock()
		ok := r.detected && (r.firmwareSeen || r.opts.SkipFirmwareCheck)
		err := r.startupErr
		r.stateMu.Unlock()
		if ok {
			return true
		}
		if err != nil {
			return false
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

func (r *RNodeInterface) checkFirmware(major, minor int) error {
	if r.opts.SkipFirmwareCheck {
		return nil
	}
	r.stateMu.Lock()
	seen := r.firmwareSeen
	gotMajor := r.firmwareMajor
	gotMinor := r.firmwareMinor
	r.stateMu.Unlock()
	if !seen {
		return errors.New("RNode did not report firmware version")
	}
	if !rnodeFirmwareOK(gotMajor, gotMinor, major, minor) {
		return fmt.Errorf("RNode firmware %d.%d is older than required %d.%d", gotMajor, gotMinor, major, minor)
	}
	return nil
}

func (r *RNodeInterface) initRadio() error {
	var frames []byte
	frames = appendRNodeU32Frame(frames, rnodeCmdFrequency, uint32(r.opts.Frequency))
	frames = appendRNodeU32Frame(frames, rnodeCmdBandwidth, uint32(r.opts.Bandwidth))
	frames = appendRNodeFrame(frames, rnodeCmdTXPower, []byte{byte(r.opts.TXPower)})
	frames = appendRNodeFrame(frames, rnodeCmdSF, []byte{byte(r.opts.SF)})
	frames = appendRNodeFrame(frames, rnodeCmdCR, []byte{byte(r.opts.CR)})
	frames = appendRNodeAirtimeFrames(frames, r.opts.STAirTimeLock, r.opts.LTAirTimeLock)
	frames = appendRNodeFrame(frames, rnodeCmdRadioState, []byte{rnodeRadioStateOn})
	return r.writeFrameBytes(frames)
}

func appendRNodeAirtimeFrames(dst []byte, short, long *float64) []byte {
	if short != nil {
		var payload [2]byte
		binary.BigEndian.PutUint16(payload[:], uint16(*short*100))
		dst = appendRNodeFrame(dst, rnodeCmdSTAlock, payload[:])
	}
	if long != nil {
		var payload [2]byte
		binary.BigEndian.PutUint16(payload[:], uint16(*long*100))
		dst = appendRNodeFrame(dst, rnodeCmdLTAlock, payload[:])
	}
	return dst
}

func (r *RNodeInterface) validateRadioState() error {
	r.stateMu.Lock()
	report := r.report
	startupErr := r.startupErr
	r.stateMu.Unlock()
	if startupErr != nil {
		return startupErr
	}
	if !report.haveFreq || absInt64(report.frequency-r.opts.Frequency) > 100 {
		return errors.New("RNode frequency validation failed")
	}
	if !report.haveBW || report.bandwidth != r.opts.Bandwidth {
		return errors.New("RNode bandwidth validation failed")
	}
	if !report.haveTX || report.txPower != r.opts.TXPower {
		return errors.New("RNode TX power validation failed")
	}
	if !report.haveSF || report.sf != r.opts.SF {
		return errors.New("RNode spreading factor validation failed")
	}
	if !report.haveState || report.state != rnodeRadioStateOn {
		return errors.New("RNode radio state validation failed")
	}
	return nil
}

func absInt64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

func (r *RNodeInterface) readLoop(port SerialPort) {
	decoder := newRNodeCmdDecoder(rnodeHWMTU, r.handleEvent)
	buf := make([]byte, rnodeReadChunk)
	for {
		select {
		case <-r.done:
			return
		default:
		}
		n, err := port.Read(buf)
		if n > 0 {
			decoder.feed(buf[:n])
		}
		if err != nil {
			if isSerialTimeout(err) {
				continue
			}
			r.handleIOError(port, err)
			return
		}
		if n == 0 {
			time.Sleep(5 * time.Millisecond)
		}
	}
}

func (r *RNodeInterface) handleEvent(cmd byte, payload []byte) {
	switch cmd {
	case rnodeCmdData:
		if len(payload) > 0 {
			r.ProcessIncoming(payload)
		}
	case rnodeCmdDetect:
		r.stateMu.Lock()
		r.detected = len(payload) > 0 && payload[0] == rnodeDetectResp
		r.stateMu.Unlock()
	case rnodeCmdFWVersion:
		if len(payload) >= 2 {
			r.stateMu.Lock()
			r.firmwareMajor = int(payload[0])
			r.firmwareMinor = int(payload[1])
			r.firmwareSeen = true
			r.stateMu.Unlock()
		}
	case rnodeCmdPlatform:
		if len(payload) > 0 {
			r.stateMu.Lock()
			r.platform = payload[0]
			r.stateMu.Unlock()
		}
	case rnodeCmdMCU:
		if len(payload) > 0 {
			r.stateMu.Lock()
			r.mcu = payload[0]
			r.stateMu.Unlock()
		}
	case rnodeCmdReady:
		r.processQueue()
	case rnodeCmdError:
		if len(payload) > 0 {
			r.stateMu.Lock()
			r.startupErr = fmt.Errorf("RNode hardware error 0x%02x", payload[0])
			r.stateMu.Unlock()
		}
	default:
		r.handleRadioReport(cmd, payload)
	}
}

func (r *RNodeInterface) handleRadioReport(cmd byte, payload []byte) {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()
	switch cmd {
	case rnodeCmdFrequency:
		if len(payload) >= 4 {
			r.report.frequency = int64(binary.BigEndian.Uint32(payload))
			r.report.haveFreq = true
		}
	case rnodeCmdBandwidth:
		if len(payload) >= 4 {
			r.report.bandwidth = int(binary.BigEndian.Uint32(payload))
			r.report.haveBW = true
		}
	case rnodeCmdTXPower:
		if len(payload) > 0 {
			r.report.txPower = int(payload[0])
			r.report.haveTX = true
		}
	case rnodeCmdSF:
		if len(payload) > 0 {
			r.report.sf = int(payload[0])
			r.report.haveSF = true
		}
	case rnodeCmdCR:
		if len(payload) > 0 {
			r.report.cr = int(payload[0])
			r.report.haveCR = true
		}
	case rnodeCmdRadioState:
		if len(payload) > 0 {
			r.report.state = payload[0]
			r.report.haveState = true
		}
	case rnodeCmdStatRSSI:
		if len(payload) > 0 {
			r.rssi = float64(int(payload[0]) - rnodeRSSIOffset)
		}
	case rnodeCmdStatSNR:
		if len(payload) > 0 {
			r.snr = float64(int8(payload[0])) * 0.25
			r.quality = rnodeLinkQuality(r.report.sf, r.snr)
		}
	}
}

// PhyStats returns the latest RSSI, SNR, and link quality reports.
func (r *RNodeInterface) PhyStats() (rssi, snr, quality float64) {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()
	return r.rssi, r.snr, r.quality
}

func (r *RNodeInterface) writeFrameBytes(frame []byte) error {
	r.txMu.Lock()
	defer r.txMu.Unlock()
	return r.writeFrameBytesLocked(frame)
}

func (r *RNodeInterface) writeFrameBytesLocked(frame []byte) error {
	r.Mutex.RLock()
	port := r.port
	r.Mutex.RUnlock()
	if port == nil {
		return errors.New("RNode interface has no open port")
	}
	for written := 0; written < len(frame); {
		n, err := port.Write(frame[written:])
		written += n
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}

// ProcessOutgoing frames one packet or queues it while flow control is busy.
func (r *RNodeInterface) ProcessOutgoing(data []byte) error {
	r.Mutex.RLock()
	online := r.Online
	r.Mutex.RUnlock()
	if !online {
		return errors.New("RNode interface is offline")
	}
	r.queueMu.Lock()
	if !r.interfaceReady {
		r.packetQueue = append(r.packetQueue, append([]byte(nil), data...))
		r.queueMu.Unlock()
		return nil
	}
	if r.opts.FlowControl {
		r.interfaceReady = false
	}
	if len(r.callsign) > 0 && bytes.Equal(data, r.callsign) {
		r.firstTX = time.Time{}
	} else if r.firstTX.IsZero() {
		r.firstTX = time.Now()
	}
	r.queueMu.Unlock()

	r.txMu.Lock()
	frame := appendRNodeDataFrame(r.txFrame[:0], data)
	r.txFrame = frame
	err := r.writeFrameBytesLocked(frame)
	r.txMu.Unlock()
	if err != nil {
		r.handleIOError(nil, err)
		return err
	}
	return nil
}

func (r *RNodeInterface) processQueue() {
	r.queueMu.Lock()
	if len(r.packetQueue) == 0 {
		r.interfaceReady = true
		r.queueMu.Unlock()
		return
	}
	data := r.packetQueue[0]
	r.packetQueue[0] = nil
	r.packetQueue = r.packetQueue[1:]
	r.interfaceReady = true
	r.queueMu.Unlock()
	if err := r.ProcessOutgoing(data); err != nil {
		debug.Log(debug.DebugError, "RNode queued transmit failed", "name", r.Name, "error", err)
	}
}

func (r *RNodeInterface) idLoop() {
	ticker := time.NewTicker(80 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-r.done:
			return
		case <-ticker.C:
			r.queueMu.Lock()
			due := !r.firstTX.IsZero() && time.Since(r.firstTX) >= r.opts.IDInterval
			r.queueMu.Unlock()
			if due && len(r.callsign) > 0 {
				_ = r.ProcessOutgoing(r.callsign)
			}
		}
	}
}

func (r *RNodeInterface) Send(data []byte, _ string) error {
	if err := common.RejectReceiveOnly(r); err != nil {
		return err
	}
	masked, err := common.ApplyIFACOutbound(r, data)
	if err != nil {
		return err
	}
	if err := r.ProcessOutgoing(masked); err != nil {
		return err
	}
	r.updateBandwidthStats(uint64(len(masked)))
	return nil
}

func (r *RNodeInterface) handleIOError(source SerialPort, err error) {
	r.Mutex.Lock()
	if source != nil && r.port != source {
		r.Mutex.Unlock()
		return
	}
	r.Online = false
	enabled := r.Enabled
	detached := r.Detached
	r.closePortLocked()
	r.Mutex.Unlock()
	debug.Log(debug.DebugError, "RNode I/O error", "name", r.Name, "error", err)
	if r.opts.PanicOnInterfaceError {
		panic(fmt.Sprintf("RNode interface error: %v", err))
	}
	if enabled && !detached {
		go r.reconnectLoop()
	}
}

func (r *RNodeInterface) reconnectLoop() {
	r.lifecycleMu.Lock()
	if r.reconnecting {
		r.lifecycleMu.Unlock()
		return
	}
	r.reconnecting = true
	r.lifecycleMu.Unlock()
	defer func() {
		r.lifecycleMu.Lock()
		r.reconnecting = false
		r.lifecycleMu.Unlock()
	}()
	for {
		r.Mutex.RLock()
		enabled := r.Enabled
		detached := r.Detached
		left := r.reconnectLeft
		r.Mutex.RUnlock()
		if !enabled || detached || left == 0 {
			return
		}
		if !sleepRNode(r.done, r.opts.ReconnectDelay) {
			return
		}
		if left > 0 {
			r.Mutex.Lock()
			r.reconnectLeft--
			r.Mutex.Unlock()
		}
		r.lifecycleMu.Lock()
		err := r.startLocked()
		r.lifecycleMu.Unlock()
		if err == nil {
			r.Mutex.Lock()
			r.reconnectLeft = NormalizeMaxReconnectTries(r.opts.MaxReconnectTries)
			r.Mutex.Unlock()
			return
		}
		debug.Log(debug.DebugError, "RNode reconnect failed", "name", r.Name, "error", err)
	}
}

// Detach powers down the radio and closes the transport.
func (r *RNodeInterface) Detach() {
	r.Mutex.Lock()
	if r.Detached {
		r.Mutex.Unlock()
		return
	}
	r.Detached = true
	r.Enabled = false
	r.Online = false
	r.Mutex.Unlock()
	_ = r.writeFrameBytes(appendRNodeFrame(nil, rnodeCmdRadioState, []byte{rnodeRadioStateOff}))
	_ = r.writeFrameBytes(appendRNodeFrame(nil, rnodeCmdLeave, []byte{0xFF}))
	r.stopOnce.Do(func() { close(r.done) })
	r.closePort()
}

func (r *RNodeInterface) Stop() error {
	r.Detach()
	return nil
}

func (r *RNodeInterface) closePort() {
	r.Mutex.Lock()
	r.closePortLocked()
	r.Online = false
	r.Mutex.Unlock()
}

func (r *RNodeInterface) closePortLocked() {
	if r.port != nil {
		_ = r.port.Close()
		r.port = nil
	}
}

func (r *RNodeInterface) GetConn() net.Conn {
	r.Mutex.RLock()
	defer r.Mutex.RUnlock()
	conn, _ := r.port.(net.Conn)
	return conn
}

func (r *RNodeInterface) SendPathRequest([]byte) error { return nil }

func (r *RNodeInterface) SendLinkPacket([]byte, []byte, time.Time) error { return nil }

func (r *RNodeInterface) GetBandwidthAvailable() bool {
	return r.IsOnline() && !r.ReceiveOnly
}

func sleepRNode(done <-chan struct{}, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-done:
		return false
	case <-timer.C:
		return true
	}
}
