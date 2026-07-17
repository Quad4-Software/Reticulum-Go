// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package discovery

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/transport"
)

// JobInterval is how often the announcer scans for due interfaces.
const JobInterval = 60 * time.Second

// DefaultAnnounceInterval is used when announce_interval is unset (6 hours).
const DefaultAnnounceInterval = 6 * time.Hour

// MinAnnounceInterval matches Python Discovery (5 minutes).
const MinAnnounceInterval = 5 * time.Minute

// DiscoverableInterfaceTypes are interface type names that can publish
// discovery announces without radio-specific fields.
var DiscoverableInterfaceTypes = map[string]struct{}{
	"BackboneInterface":  {},
	"TCPServerInterface": {},
	"TCPClientInterface": {},
	"I2PInterface":       {},
	"RNodeInterface":     {},
	"WeaveInterface":     {},
	"KISSInterface":      {},
}

// InterfaceAnnouncer periodically announces discoverable TCP or Backbone
// (and similar) interfaces over rnstransport.discovery.interface.
type InterfaceAnnouncer struct {
	transport *transport.Transport
	config    *common.ReticulumConfig
	dest      *destination.Destination

	mu           sync.Mutex
	lastAnnounce map[string]time.Time
	stampCache   map[string][]byte
	stop         chan struct{}
	done         chan struct{}
	jobInterval  time.Duration
	now          func() time.Time
}

// NewInterfaceAnnouncer builds an announcer bound to tr and cfg.
// identity is used for the discovery destination (network identity preferred).
func NewInterfaceAnnouncer(tr *transport.Transport, cfg *common.ReticulumConfig, id *identity.Identity) (*InterfaceAnnouncer, error) {
	if tr == nil || cfg == nil || id == nil {
		return nil, errAnnouncerArgs
	}
	dest, err := destination.New(id, destination.In, destination.Single, AppName, tr, Aspects...)
	if err != nil {
		return nil, err
	}
	return &InterfaceAnnouncer{
		transport:    tr,
		config:       cfg,
		dest:         dest,
		lastAnnounce: make(map[string]time.Time),
		stampCache:   make(map[string][]byte),
		jobInterval:  JobInterval,
		now:          time.Now,
	}, nil
}

var errAnnouncerArgs = errString("discovery: announcer requires transport config and identity")

type errString string

func (e errString) Error() string { return string(e) }

// Start begins the announce job loop.
func (a *InterfaceAnnouncer) Start() {
	if a == nil {
		return
	}
	a.mu.Lock()
	if a.stop != nil {
		a.mu.Unlock()
		return
	}
	a.stop = make(chan struct{})
	a.done = make(chan struct{})
	stop := a.stop
	done := a.done
	a.mu.Unlock()
	go a.job(stop, done)
}

// Stop ends the announce job loop and waits for exit.
func (a *InterfaceAnnouncer) Stop() {
	if a == nil {
		return
	}
	a.mu.Lock()
	stop := a.stop
	done := a.done
	a.stop = nil
	a.done = nil
	a.mu.Unlock()
	if stop == nil {
		return
	}
	close(stop)
	<-done
}

func (a *InterfaceAnnouncer) job(stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(a.jobInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			a.announceDue()
		}
	}
}

func (a *InterfaceAnnouncer) announceDue() {
	if a == nil || a.config == nil {
		return
	}
	now := a.now()
	type due struct {
		name  string
		iface *common.InterfaceConfig
		age   time.Duration
	}
	var list []due
	for name, iface := range a.config.Interfaces {
		if iface == nil || !iface.Enabled || !iface.Discoverable {
			continue
		}
		if !isDiscoverableType(iface.Type) {
			continue
		}
		interval := announceInterval(iface)
		a.mu.Lock()
		last := a.lastAnnounce[name]
		a.mu.Unlock()
		if !last.IsZero() && now.Sub(last) < interval {
			continue
		}
		age := now.Sub(last)
		if last.IsZero() {
			age = interval + time.Second
		}
		list = append(list, due{name: name, iface: iface, age: age})
	}
	if len(list) == 0 {
		return
	}
	sort.Slice(list, func(i, j int) bool { return list[i].age > list[j].age })
	selected := list[0]
	appData, err := a.buildAnnounceData(selected.iface)
	if err != nil {
		debug.Log(debug.DebugError, "Could not generate interface discovery announce", "name", selected.name, "error", err)
		return
	}
	if len(appData) == 0 {
		debug.Log(debug.DebugError, "Empty interface discovery announce data", "name", selected.name)
		return
	}
	a.dest.SetDefaultAppData(appData)
	if err := a.dest.Announce(false, nil, nil); err != nil {
		debug.Log(debug.DebugError, "Failed to send interface discovery announce", "name", selected.name, "error", err)
		return
	}
	a.mu.Lock()
	a.lastAnnounce[selected.name] = now
	a.mu.Unlock()
	debug.Log(debug.DebugVerbose, "Sent interface discovery announce", "name", selected.name, "bytes", len(appData))
}

func (a *InterfaceAnnouncer) buildAnnounceData(iface *common.InterfaceConfig) ([]byte, error) {
	info, err := a.infoForInterface(iface)
	if err != nil {
		return nil, err
	}
	stampCost := DefaultStampValue
	if iface.DiscoveryStampValue > 0 {
		stampCost = iface.DiscoveryStampValue
	}
	packed, err := EncodeInfo(*info)
	if err != nil {
		return nil, err
	}
	infoHash := InfoHash(packed)
	key := string(infoHash)
	a.mu.Lock()
	stamp, ok := a.stampCache[key]
	a.mu.Unlock()
	if !ok {
		stamp, _, err = GenerateStamp(infoHash, stampCost, WorkblockExpandRounds)
		if err != nil {
			return nil, err
		}
		a.mu.Lock()
		a.stampCache[key] = stamp
		a.mu.Unlock()
	}
	if iface.DiscoveryEncrypt {
		netID := a.transport.NetworkIdentity()
		if netID == nil {
			return nil, errString("discovery: discovery_encrypt requires network identity")
		}
		plain := make([]byte, 0, len(packed)+len(stamp))
		plain = append(plain, packed...)
		plain = append(plain, stamp...)
		cipher, err := netID.Encrypt(plain, nil)
		if err != nil {
			return nil, err
		}
		out := make([]byte, 0, 1+len(cipher))
		out = append(out, FlagEncrypted)
		out = append(out, cipher...)
		return out, nil
	}
	return EncodeAppData(0x00, packed, stamp)
}

func (a *InterfaceAnnouncer) infoForInterface(iface *common.InterfaceConfig) (*Info, error) {
	transportID := a.transport.TransportIdentityHash()
	if len(transportID) == 0 {
		return nil, errString("discovery: transport identity unavailable")
	}
	ifaceType := normalizeIfaceType(iface.Type)
	info := &Info{
		Type:        ifaceType,
		Transport:   a.transport.GetConfig() != nil && a.transport.GetConfig().EnableTransport,
		TransportID: append([]byte(nil), transportID...),
		Name:        sanitize(iface.DiscoveryName),
	}
	if iface.HasDiscoveryGeo {
		info.Latitude = iface.DiscoveryLatitude
		info.Longitude = iface.DiscoveryLongitude
		info.Height = iface.DiscoveryHeight
		info.HasGeo = true
	}
	switch ifaceType {
	case "BackboneInterface", "TCPServerInterface":
		reachable, err := resolveReachableOn(iface.ReachableOn)
		if err != nil {
			return nil, err
		}
		if reachable == "" {
			return nil, errString("discovery: reachable_on required for " + ifaceType)
		}
		info.ReachableOn = reachable
		info.Port = int64(iface.Port)
		info.HasPort = true
	case "I2PInterface":
		if !iface.I2PConnectable {
			return nil, errString("discovery: I2PInterface must be connectable")
		}
		// b32 is filled by runtime peers when available. Config-only announce
		// uses reachable_on when set.
		reachable := sanitize(iface.ReachableOn)
		if reachable == "" {
			return nil, errString("discovery: I2PInterface needs reachable_on or b32")
		}
		info.ReachableOn = reachable
	case "TCPClientInterface":
		if !iface.KISSFraming {
			return nil, errString("discovery: TCPClientInterface requires kiss_framing for discovery")
		}
		ifaceType = "KISSInterface"
		info.Type = ifaceType
	}
	if iface.PublishIFAC {
		info.IFACNetname = sanitize(iface.IFACNetname)
		info.IFACNetkey = sanitize(iface.IFACNetkey)
	}
	return info, nil
}

func announceInterval(iface *common.InterfaceConfig) time.Duration {
	if iface == nil || iface.DiscoveryAnnounceIntervalSec <= 0 {
		return DefaultAnnounceInterval
	}
	d := time.Duration(iface.DiscoveryAnnounceIntervalSec) * time.Second
	if d < MinAnnounceInterval {
		return MinAnnounceInterval
	}
	return d
}

func isDiscoverableType(t string) bool {
	_, ok := DiscoverableInterfaceTypes[normalizeIfaceType(t)]
	return ok
}

func normalizeIfaceType(t string) string {
	t = strings.TrimSpace(t)
	switch strings.ToLower(t) {
	case "backbone", "backboneinterface":
		return "BackboneInterface"
	case "tcpserver", "tcpserverinterface":
		return "TCPServerInterface"
	case "tcpclient", "tcpclientinterface":
		return "TCPClientInterface"
	case "i2p", "i2pinterface":
		return "I2PInterface"
	case "rnode", "rnodeinterface":
		return "RNodeInterface"
	case "weave", "weaveinterface":
		return "WeaveInterface"
	case "kiss", "kissinterface":
		return "KISSInterface"
	default:
		return t
	}
}

func sanitize(in string) string {
	if in == "" {
		return ""
	}
	out := strings.ReplaceAll(in, "\n", "")
	out = strings.ReplaceAll(out, "\r", "")
	return strings.TrimSpace(out)
}

func resolveReachableOn(raw string) (string, error) {
	reachable := sanitize(raw)
	if reachable == "" {
		return "", nil
	}
	execPath := os.ExpandEnv(reachable)
	if st, err := os.Stat(execPath); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
		cmd := exec.Command(execPath) // #nosec G204 -- operator-configured reachable_on script
		out, err := cmd.Output()
		if err != nil {
			return "", err
		}
		reachable = sanitize(string(out))
		if reachable == "" {
			return "", errString("discovery: reachable_on script produced empty output")
		}
		_ = filepath.Base(execPath)
	}
	return reachable, nil
}

// HasDiscoverableInterfaces reports whether cfg lists any discoverable iface.
func HasDiscoverableInterfaces(cfg *common.ReticulumConfig) bool {
	if cfg == nil {
		return false
	}
	for _, iface := range cfg.Interfaces {
		if iface != nil && iface.Enabled && iface.Discoverable && isDiscoverableType(iface.Type) {
			return true
		}
	}
	return false
}
