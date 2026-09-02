// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build !js

package interfaces

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"maps"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"quad4/reticulum-go/pkg/common"
)

// RNodeMultiOptions configures an RNodeMultiInterface.
type RNodeMultiOptions struct {
	RNodeOptions
	SubInterfaces  map[string]*common.InterfaceConfig
	RegisterPeer   func(name string, peer common.NetworkInterface) error
	UnregisterPeer func(name string)
	SetupPeer      func(peer common.NetworkInterface)
}

// RNodeMultiInterface owns one transport and multiple virtual RNode radios.
type RNodeMultiInterface struct {
	BaseInterface

	opts          RNodeMultiOptions
	port          SerialPort
	txMu          sync.Mutex
	txFrame       []byte
	lifecycleMu   sync.Mutex
	reconnecting  bool
	reconnectLeft int
	done          chan struct{}
	stopOnce      sync.Once
	beaconOnce    sync.Once

	stateMu        sync.Mutex
	detected       bool
	firmwareSeen   bool
	firmwareMajor  int
	firmwareMinor  int
	platform       byte
	mcu            byte
	interfaceTypes map[int]string
	selectedIndex  int
	startupErr     error

	subsMu        sync.RWMutex
	subinterfaces map[int]*RNodeSubInterface
	idMu          sync.Mutex
	firstTX       time.Time
	callsign      []byte
}

// RNodeSubInterface is one virtual radio on an RNodeMultiInterface.
type RNodeSubInterface struct {
	BaseInterface

	parent        *RNodeMultiInterface
	index         int
	interfaceType string
	frequency     int64
	bandwidth     int
	txPower       int
	sf            int
	cr            int
	flowControl   bool
	stAirTimeLock *float64
	ltAirTimeLock *float64

	stateMu        sync.Mutex
	report         rnodeRadioReport
	rssi           float64
	snr            float64
	quality        float64
	interfaceReady bool
	packetQueue    [][]byte
}

// NewRNodeMultiInterface constructs an RNode multi-radio parent.
func NewRNodeMultiInterface(name string, enabled bool, opts RNodeMultiOptions) (*RNodeMultiInterface, error) {
	opts.RNodeOptions = normalizeRNodeOptions(opts.RNodeOptions)
	if opts.Port == "" {
		return nil, errors.New("no port specified for RNodeMulti interface")
	}
	if strings.HasPrefix(strings.ToLower(opts.Port), "ble://") {
		return nil, errors.New("RNode BLE transport is not supported")
	}
	if len(opts.SubInterfaces) == 0 {
		return nil, errors.New("no subinterfaces configured for RNodeMulti interface")
	}
	if len([]byte(opts.Callsign)) > rnodeCallsignMaxLen {
		return nil, fmt.Errorf("RNodeMulti callsign exceeds %d bytes", rnodeCallsignMaxLen)
	}
	m := &RNodeMultiInterface{
		BaseInterface:  NewBaseInterface(name, common.IFTypeRNodeMulti, enabled),
		opts:           opts,
		done:           make(chan struct{}),
		interfaceTypes: make(map[int]string),
		subinterfaces:  make(map[int]*RNodeSubInterface),
		reconnectLeft:  NormalizeMaxReconnectTries(opts.MaxReconnectTries),
		txFrame:        make([]byte, 0, rnodeHWMTU*2+8),
		callsign:       []byte(opts.Callsign),
	}
	m.In = false
	m.Out = false
	m.MTU = rnodeHWMTU
	return m, nil
}

func (m *RNodeMultiInterface) String() string {
	return fmt.Sprintf("RNodeMultiInterface[%s]", m.Name)
}

// Start opens the parent, discovers radios, and creates configured children.
func (m *RNodeMultiInterface) Start() error {
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()
	m.Mutex.RLock()
	if m.Online {
		m.Mutex.RUnlock()
		return nil
	}
	enabled := m.Enabled
	detached := m.Detached
	m.Mutex.RUnlock()
	if !enabled {
		return errors.New("RNodeMulti interface is not enabled")
	}
	if detached {
		return errors.New("RNodeMulti interface is detached")
	}
	return m.startLocked()
}

func (m *RNodeMultiInterface) startLocked() error {
	helper := &RNodeInterface{opts: m.opts.RNodeOptions}
	port, err := helper.openPort()
	if err != nil {
		return err
	}
	m.Mutex.Lock()
	m.port = port
	m.Online = false
	m.Mutex.Unlock()
	m.resetStartupState()
	if !sleepRNode(m.done, m.opts.ConfigureDelay) {
		m.closePort()
		return errors.New("RNodeMulti startup canceled")
	}
	go m.readLoop(port)
	if err := m.writeFrameBytes(appendRNodeMultiDetect(nil)); err != nil {
		m.closePort()
		return err
	}
	if !m.waitDetected(m.opts.DetectTimeout) {
		m.closePort()
		return errors.New("RNodeMulti detection timed out")
	}
	if err := m.checkFirmware(); err != nil {
		m.closePort()
		return err
	}
	if err := m.spawnSubinterfaces(); err != nil {
		m.teardownSubinterfaces()
		m.closePort()
		return err
	}
	m.Mutex.Lock()
	m.Online = true
	m.Mutex.Unlock()
	if m.opts.IDInterval > 0 && m.opts.Callsign != "" {
		m.beaconOnce.Do(func() { go m.idLoop() })
	}
	return nil
}

func (m *RNodeMultiInterface) resetStartupState() {
	m.stateMu.Lock()
	m.detected = false
	m.firmwareSeen = false
	m.firmwareMajor = 0
	m.firmwareMinor = 0
	m.interfaceTypes = make(map[int]string)
	m.selectedIndex = 0
	m.startupErr = nil
	m.stateMu.Unlock()
}

func (m *RNodeMultiInterface) waitDetected(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		m.stateMu.Lock()
		detected := m.detected
		haveTypes := len(m.interfaceTypes) > 0
		failed := m.startupErr != nil
		m.stateMu.Unlock()
		if detected && haveTypes {
			return true
		}
		if failed {
			return false
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

func (m *RNodeMultiInterface) checkFirmware() error {
	if m.opts.SkipFirmwareCheck {
		return nil
	}
	m.stateMu.Lock()
	seen := m.firmwareSeen
	major := m.firmwareMajor
	minor := m.firmwareMinor
	m.stateMu.Unlock()
	if !seen {
		return errors.New("RNodeMulti did not report firmware version")
	}
	if !rnodeFirmwareOK(major, minor, rnodeMultiRequiredMaj, rnodeMultiRequiredMin) {
		return fmt.Errorf("RNodeMulti firmware %d.%d is older than required %d.%d", major, minor, rnodeMultiRequiredMaj, rnodeMultiRequiredMin)
	}
	return nil
}

func (m *RNodeMultiInterface) spawnSubinterfaces() error {
	names := make([]string, 0, len(m.opts.SubInterfaces))
	for name := range m.opts.SubInterfaces {
		names = append(names, name)
	}
	sort.Strings(names)
	spawned := 0
	for _, name := range names {
		cfg := m.opts.SubInterfaces[name]
		if cfg == nil {
			continue
		}
		if !cfg.VPortSet {
			return fmt.Errorf("RNode subinterface %q has no vport", name)
		}
		m.stateMu.Lock()
		interfaceType, exists := m.interfaceTypes[cfg.VPort]
		m.stateMu.Unlock()
		if !exists {
			return fmt.Errorf("RNode virtual port %d for %q does not exist", cfg.VPort, name)
		}
		sub, err := newRNodeSubInterface(name, m, cfg, interfaceType)
		if err != nil {
			return err
		}
		m.subsMu.Lock()
		if _, duplicate := m.subinterfaces[sub.index]; duplicate {
			m.subsMu.Unlock()
			return fmt.Errorf("duplicate RNode virtual port %d", sub.index)
		}
		m.subinterfaces[sub.index] = sub
		m.subsMu.Unlock()
		if err := sub.configure(); err != nil {
			return err
		}
		if m.GetIFAC() != nil {
			sub.SetIFAC(m.GetIFAC())
		}
		sub.Mode = m.Mode
		sub.RecursivePRs = m.RecursivePRs
		sub.AnnouncesFromInternal = m.AnnouncesFromInternal
		sub.AnnouncesToInternal = m.AnnouncesToInternal
		sub.Gravity = m.Gravity
		if m.opts.SetupPeer != nil {
			m.opts.SetupPeer(sub)
		}
		if m.opts.RegisterPeer != nil {
			if err := m.opts.RegisterPeer(name, sub); err != nil {
				return err
			}
		}
		spawned++
	}
	if spawned == 0 {
		return errors.New("no RNode subinterfaces enabled")
	}
	return nil
}

func newRNodeSubInterface(name string, parent *RNodeMultiInterface, cfg *common.InterfaceConfig, interfaceType string) (*RNodeSubInterface, error) {
	short := airtimePointer(cfg.AirtimeLimitShort, cfg.AirtimeLimitShortSet)
	long := airtimePointer(cfg.AirtimeLimitLong, cfg.AirtimeLimitLongSet)
	opts := RNodeOptions{
		Port:          parent.opts.Port,
		Frequency:     cfg.FrequencyHz,
		Bandwidth:     cfg.Bandwidth,
		TXPower:       cfg.TXPower,
		SF:            cfg.SpreadingFactor,
		CR:            cfg.CodingRate,
		STAirTimeLock: short,
		LTAirTimeLock: long,
	}
	if err := validateRNodeSubOptions(opts, interfaceType); err != nil {
		return nil, fmt.Errorf("RNode subinterface %q: %w", name, err)
	}
	s := &RNodeSubInterface{
		BaseInterface: NewBaseInterface(name, common.IFTypeRNode, true),
		parent:        parent,
		index:         cfg.VPort,
		interfaceType: interfaceType,
		frequency:     cfg.FrequencyHz,
		bandwidth:     cfg.Bandwidth,
		txPower:       cfg.TXPower,
		sf:            cfg.SpreadingFactor,
		cr:            cfg.CodingRate,
		flowControl:   cfg.FlowControl,
		stAirTimeLock: short,
		ltAirTimeLock: long,
		quality:       -1,
	}
	s.In = true
	s.Out = !cfg.OutgoingSet || cfg.Outgoing
	s.ReceiveOnly = !s.Out
	s.MTU = rnodeHWMTU
	s.Bitrate = int64(rnodeComputeBitrate(s.sf, s.cr, s.bandwidth))
	return s, nil
}

func airtimePointer(value float64, set bool) *float64 {
	if !set {
		return nil
	}
	v := value
	return &v
}

func validateRNodeSubOptions(opts RNodeOptions, interfaceType string) error {
	switch interfaceType {
	case "SX126X", "SX127X":
		if opts.Frequency < 137000000 || opts.Frequency > 1000000000 {
			return errors.New("frequency must be between 137000000 and 1000000000")
		}
	case "SX128X":
		if opts.Frequency < 2200000000 || opts.Frequency > 2600000000 {
			return errors.New("frequency must be between 2200000000 and 2600000000")
		}
	default:
		return fmt.Errorf("unsupported radio type %q", interfaceType)
	}
	if opts.TXPower < -9 || opts.TXPower > 37 {
		return errors.New("TX power must be between -9 and 37")
	}
	if opts.Bandwidth < 7800 || opts.Bandwidth > 1625000 {
		return errors.New("bandwidth must be between 7800 and 1625000")
	}
	if opts.SF < 5 || opts.SF > 12 {
		return errors.New("spreading factor must be between 5 and 12")
	}
	if opts.CR < 5 || opts.CR > 8 {
		return errors.New("coding rate must be between 5 and 8")
	}
	if err := validateRNodeAirtime(opts.STAirTimeLock, "short-term"); err != nil {
		return err
	}
	return validateRNodeAirtime(opts.LTAirTimeLock, "long-term")
}

func (s *RNodeSubInterface) configure() error {
	s.stateMu.Lock()
	s.report = rnodeRadioReport{}
	s.stateMu.Unlock()
	var frames []byte
	frames = appendRNodeSelectedFrame(frames, s.index, rnodeCmdFrequency, uint32Payload(rnodeWireU32(s.frequency)))
	frames = appendRNodeSelectedFrame(frames, s.index, rnodeCmdBandwidth, uint32Payload(rnodeWireU32Int(s.bandwidth)))
	frames = appendRNodeSelectedFrame(frames, s.index, rnodeCmdTXPower, []byte{rnodeWireTXPower(s.txPower)})
	frames = appendRNodeSelectedFrame(frames, s.index, rnodeCmdSF, []byte{rnodeWireByte(s.sf)})
	frames = appendRNodeSelectedFrame(frames, s.index, rnodeCmdCR, []byte{rnodeWireByte(s.cr)})
	if s.stAirTimeLock != nil {
		frames = appendRNodeSelectedFrame(frames, s.index, rnodeCmdSTAlock, airtimePayload(*s.stAirTimeLock))
	}
	if s.ltAirTimeLock != nil {
		frames = appendRNodeSelectedFrame(frames, s.index, rnodeCmdLTAlock, airtimePayload(*s.ltAirTimeLock))
	}
	frames = appendRNodeSelectedFrame(frames, s.index, rnodeCmdRadioState, []byte{rnodeRadioStateOn})
	if err := s.parent.writeFrameBytes(frames); err != nil {
		return err
	}
	if !sleepRNode(s.parent.done, s.parent.opts.ValidateDelay) {
		return errors.New("RNode subinterface configuration canceled")
	}
	s.stateMu.Lock()
	report := s.report
	s.stateMu.Unlock()
	if !report.haveFreq || absInt64(report.frequency-s.frequency) > 100 ||
		!report.haveBW || report.bandwidth != s.bandwidth ||
		!report.haveTX || report.txPower != s.txPower ||
		!report.haveSF || report.sf != s.sf ||
		!report.haveState || report.state != rnodeRadioStateOn {
		return fmt.Errorf("radio state validation failed for RNode subinterface %q", s.Name)
	}
	s.stateMu.Lock()
	s.interfaceReady = true
	s.stateMu.Unlock()
	s.Mutex.Lock()
	s.Online = true
	s.Mutex.Unlock()
	return nil
}

func appendRNodeSelectedFrame(dst []byte, index int, cmd byte, payload []byte) []byte {
	dst = appendRNodeSelIntFrame(dst, rnodeWireByte(index))
	return appendRNodeFrame(dst, cmd, payload)
}

func uint32Payload(value uint32) []byte {
	var payload [4]byte
	binary.BigEndian.PutUint32(payload[:], value)
	return payload[:]
}

func airtimePayload(value float64) []byte {
	var payload [2]byte
	binary.BigEndian.PutUint16(payload[:], uint16(value*100))
	return payload[:]
}

func (m *RNodeMultiInterface) readLoop(port SerialPort) {
	decoder := newRNodeCmdDecoder(rnodeHWMTU, m.handleEvent)
	buf := make([]byte, rnodeReadChunk)
	for {
		select {
		case <-m.done:
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
			m.handleIOError(port, err)
			return
		}
		if n == 0 {
			time.Sleep(5 * time.Millisecond)
		}
	}
}

func (m *RNodeMultiInterface) handleEvent(cmd byte, payload []byte) {
	if index, ok := rnodeIntDataIndex(cmd); ok {
		if cmd == rnodeCmdData {
			m.stateMu.Lock()
			index = m.selectedIndex
			m.stateMu.Unlock()
		}
		m.subsMu.RLock()
		sub := m.subinterfaces[index]
		m.subsMu.RUnlock()
		if sub != nil && len(payload) > 0 {
			sub.ProcessIncoming(payload)
		}
		return
	}
	switch cmd {
	case rnodeCmdSelInt:
		if len(payload) > 0 {
			m.stateMu.Lock()
			m.selectedIndex = int(payload[0])
			m.stateMu.Unlock()
		}
	case rnodeCmdDetect:
		m.stateMu.Lock()
		m.detected = len(payload) > 0 && payload[0] == rnodeDetectResp
		m.stateMu.Unlock()
	case rnodeCmdFWVersion:
		if len(payload) >= 2 {
			m.stateMu.Lock()
			m.firmwareMajor = int(payload[0])
			m.firmwareMinor = int(payload[1])
			m.firmwareSeen = true
			m.stateMu.Unlock()
		}
	case rnodeCmdPlatform:
		if len(payload) > 0 {
			m.stateMu.Lock()
			m.platform = payload[0]
			m.stateMu.Unlock()
		}
	case rnodeCmdMCU:
		if len(payload) > 0 {
			m.stateMu.Lock()
			m.mcu = payload[0]
			m.stateMu.Unlock()
		}
	case rnodeCmdInterfaces:
		if len(payload) >= 2 {
			m.stateMu.Lock()
			m.interfaceTypes[int(payload[0])] = rnodeInterfaceTypeString(payload[1])
			m.stateMu.Unlock()
		}
	case rnodeCmdReady:
		m.subsMu.RLock()
		subs := make([]*RNodeSubInterface, 0, len(m.subinterfaces))
		for _, sub := range m.subinterfaces {
			subs = append(subs, sub)
		}
		m.subsMu.RUnlock()
		for _, sub := range subs {
			sub.processQueue()
		}
	case rnodeCmdError:
		if len(payload) > 0 {
			m.stateMu.Lock()
			m.startupErr = fmt.Errorf("RNodeMulti hardware error 0x%02x", payload[0])
			m.stateMu.Unlock()
		}
	default:
		m.handleSubReport(cmd, payload)
	}
}

func (m *RNodeMultiInterface) handleSubReport(cmd byte, payload []byte) {
	m.stateMu.Lock()
	index := m.selectedIndex
	m.stateMu.Unlock()
	m.subsMu.RLock()
	sub := m.subinterfaces[index]
	m.subsMu.RUnlock()
	if sub == nil {
		return
	}
	sub.stateMu.Lock()
	defer sub.stateMu.Unlock()
	switch cmd {
	case rnodeCmdFrequency:
		if len(payload) >= 4 {
			sub.report.frequency = int64(binary.BigEndian.Uint32(payload))
			sub.report.haveFreq = true
		}
	case rnodeCmdBandwidth:
		if len(payload) >= 4 {
			sub.report.bandwidth = int(binary.BigEndian.Uint32(payload))
			sub.report.haveBW = true
		}
	case rnodeCmdTXPower:
		if len(payload) > 0 {
			sub.report.txPower = rnodeTXPowerFromWire(payload[0])
			sub.report.haveTX = true
		}
	case rnodeCmdSF:
		if len(payload) > 0 {
			sub.report.sf = int(payload[0])
			sub.report.haveSF = true
		}
	case rnodeCmdCR:
		if len(payload) > 0 {
			sub.report.cr = int(payload[0])
			sub.report.haveCR = true
		}
	case rnodeCmdRadioState:
		if len(payload) > 0 {
			sub.report.state = payload[0]
			sub.report.haveState = true
		}
	case rnodeCmdStatRSSI:
		if len(payload) > 0 {
			sub.rssi = float64(int(payload[0]) - rnodeRSSIOffset)
		}
	case rnodeCmdStatSNR:
		if len(payload) > 0 {
			sub.snr = rnodeSNRFromWire(payload[0])
			sub.quality = rnodeLinkQuality(sub.sf, sub.snr)
		}
	}
}

func (m *RNodeMultiInterface) writeFrameBytes(frame []byte) error {
	m.txMu.Lock()
	defer m.txMu.Unlock()
	return m.writeFrameBytesLocked(frame)
}

func (m *RNodeMultiInterface) writeFrameBytesLocked(frame []byte) error {
	m.Mutex.RLock()
	port := m.port
	m.Mutex.RUnlock()
	if port == nil {
		return errors.New("RNodeMulti has no open port")
	}
	for written := 0; written < len(frame); {
		n, err := port.Write(frame[written:])
		written += n
		if err != nil {
			return err
		}
		if n == 0 {
			return errors.New("RNodeMulti short write")
		}
	}
	return nil
}

// ProcessOutgoing is intentionally a no-op because only children transmit.
func (m *RNodeMultiInterface) ProcessOutgoing([]byte) error { return nil }

func (s *RNodeSubInterface) String() string {
	return fmt.Sprintf("%s[%s]", s.parent.Name, s.Name)
}

// ProcessOutgoing transmits through the child's selected virtual radio.
func (s *RNodeSubInterface) ProcessOutgoing(data []byte) error {
	if !s.IsOnline() {
		return errors.New("RNode subinterface is offline")
	}
	s.stateMu.Lock()
	if !s.interfaceReady {
		s.packetQueue = append(s.packetQueue, append([]byte(nil), data...))
		s.stateMu.Unlock()
		return nil
	}
	if s.flowControl {
		s.interfaceReady = false
	}
	s.stateMu.Unlock()
	s.parent.idMu.Lock()
	if len(s.parent.callsign) > 0 && bytes.Equal(data, s.parent.callsign) {
		s.parent.firstTX = time.Time{}
	} else if s.parent.firstTX.IsZero() {
		s.parent.firstTX = time.Now()
	}
	s.parent.idMu.Unlock()

	s.parent.txMu.Lock()
	frame := appendRNodeSelIntFrame(s.parent.txFrame[:0], rnodeWireByte(s.index))
	frame = appendRNodeDataFrame(frame, data)
	s.parent.txFrame = frame
	err := s.parent.writeFrameBytesLocked(frame)
	s.parent.txMu.Unlock()
	return err
}

func (s *RNodeSubInterface) processQueue() {
	s.stateMu.Lock()
	if len(s.packetQueue) == 0 {
		s.interfaceReady = true
		s.stateMu.Unlock()
		return
	}
	data := s.packetQueue[0]
	s.packetQueue[0] = nil
	s.packetQueue = s.packetQueue[1:]
	s.interfaceReady = true
	s.stateMu.Unlock()
	_ = s.ProcessOutgoing(data)
}

func (s *RNodeSubInterface) Send(data []byte, _ string) error {
	if err := common.RejectReceiveOnly(s); err != nil {
		return err
	}
	masked, err := common.ApplyIFACOutbound(s, data)
	if err != nil {
		return err
	}
	if err := s.ProcessOutgoing(masked); err != nil {
		return err
	}
	s.updateBandwidthStats(uint64(len(masked)))
	return nil
}

// PhyStats returns this virtual radio's RSSI, SNR, and link quality.
func (s *RNodeSubInterface) PhyStats() (rssi, snr, quality float64) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	return s.rssi, s.snr, s.quality
}

func (s *RNodeSubInterface) Start() error { return nil }

func (s *RNodeSubInterface) Stop() error {
	s.Mutex.Lock()
	s.Online = false
	s.Enabled = false
	s.Mutex.Unlock()
	return nil
}

func (s *RNodeSubInterface) Detach() {
	_ = s.parent.writeFrameBytes(appendRNodeSelectedFrame(nil, s.index, rnodeCmdRadioState, []byte{rnodeRadioStateOff}))
	_ = s.Stop()
}

func (s *RNodeSubInterface) GetConn() net.Conn { return s.parent.GetConn() }

func (s *RNodeSubInterface) SendPathRequest([]byte) error { return nil }

func (s *RNodeSubInterface) SendLinkPacket([]byte, []byte, time.Time) error { return nil }

func (s *RNodeSubInterface) GetBandwidthAvailable() bool {
	return s.IsOnline() && !s.ReceiveOnly
}

// SubInterfaces returns the currently active virtual radios by vport.
func (m *RNodeMultiInterface) SubInterfaces() map[int]*RNodeSubInterface {
	m.subsMu.RLock()
	defer m.subsMu.RUnlock()
	out := make(map[int]*RNodeSubInterface, len(m.subinterfaces))
	maps.Copy(out, m.subinterfaces)
	return out
}

func (m *RNodeMultiInterface) handleIOError(source SerialPort, err error) {
	m.Mutex.Lock()
	if source != nil && m.port != source {
		m.Mutex.Unlock()
		return
	}
	m.Online = false
	enabled := m.Enabled
	detached := m.Detached
	m.closePortLocked()
	m.Mutex.Unlock()
	m.teardownSubinterfaces()
	if m.opts.PanicOnInterfaceError {
		panic(fmt.Sprintf("RNodeMulti interface error: %v", err))
	}
	if enabled && !detached {
		go m.reconnectLoop()
	}
}

func (m *RNodeMultiInterface) reconnectLoop() {
	m.lifecycleMu.Lock()
	if m.reconnecting {
		m.lifecycleMu.Unlock()
		return
	}
	m.reconnecting = true
	m.lifecycleMu.Unlock()
	defer func() {
		m.lifecycleMu.Lock()
		m.reconnecting = false
		m.lifecycleMu.Unlock()
	}()
	for {
		m.Mutex.RLock()
		enabled := m.Enabled
		detached := m.Detached
		left := m.reconnectLeft
		m.Mutex.RUnlock()
		if !enabled || detached || left == 0 {
			return
		}
		if !sleepRNode(m.done, m.opts.ReconnectDelay) {
			return
		}
		if left > 0 {
			m.Mutex.Lock()
			m.reconnectLeft--
			m.Mutex.Unlock()
		}
		m.lifecycleMu.Lock()
		err := m.startLocked()
		m.lifecycleMu.Unlock()
		if err == nil {
			m.Mutex.Lock()
			m.reconnectLeft = NormalizeMaxReconnectTries(m.opts.MaxReconnectTries)
			m.Mutex.Unlock()
			return
		}
	}
}

func (m *RNodeMultiInterface) idLoop() {
	ticker := time.NewTicker(80 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-m.done:
			return
		case <-ticker.C:
			m.idMu.Lock()
			due := !m.firstTX.IsZero() && time.Since(m.firstTX) >= m.opts.IDInterval
			m.idMu.Unlock()
			if !due || len(m.callsign) == 0 {
				continue
			}
			m.subsMu.RLock()
			for _, sub := range m.subinterfaces {
				_ = sub.ProcessOutgoing(m.callsign)
			}
			m.subsMu.RUnlock()
		}
	}
}

func (m *RNodeMultiInterface) teardownSubinterfaces() {
	m.subsMu.Lock()
	subs := m.subinterfaces
	m.subinterfaces = make(map[int]*RNodeSubInterface)
	m.subsMu.Unlock()
	for _, sub := range subs {
		sub.Mutex.Lock()
		sub.Online = false
		sub.Mutex.Unlock()
		if m.opts.UnregisterPeer != nil {
			m.opts.UnregisterPeer(sub.Name)
		}
	}
}

// Detach powers down every virtual radio and closes the parent transport.
func (m *RNodeMultiInterface) Detach() {
	m.Mutex.Lock()
	if m.Detached {
		m.Mutex.Unlock()
		return
	}
	m.Detached = true
	m.Enabled = false
	m.Online = false
	m.Mutex.Unlock()
	m.subsMu.RLock()
	for index := range m.subinterfaces {
		_ = m.writeFrameBytes(appendRNodeSelectedFrame(nil, index, rnodeCmdRadioState, []byte{rnodeRadioStateOff}))
	}
	m.subsMu.RUnlock()
	_ = m.writeFrameBytes(appendRNodeFrame(nil, rnodeCmdLeave, []byte{0xFF}))
	m.stopOnce.Do(func() { close(m.done) })
	m.teardownSubinterfaces()
	m.closePort()
}

func (m *RNodeMultiInterface) Stop() error {
	m.Detach()
	return nil
}

func (m *RNodeMultiInterface) closePort() {
	m.Mutex.Lock()
	m.closePortLocked()
	m.Online = false
	m.Mutex.Unlock()
}

func (m *RNodeMultiInterface) closePortLocked() {
	if m.port != nil {
		_ = m.port.Close()
		m.port = nil
	}
}

func (m *RNodeMultiInterface) GetConn() net.Conn {
	m.Mutex.RLock()
	defer m.Mutex.RUnlock()
	conn, _ := m.port.(net.Conn)
	return conn
}

func (m *RNodeMultiInterface) SendPathRequest([]byte) error { return nil }

func (m *RNodeMultiInterface) SendLinkPacket([]byte, []byte, time.Time) error { return nil }

func (m *RNodeMultiInterface) GetBandwidthAvailable() bool { return false }
