// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package discovery

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"math"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"quad4/msgpack/v5/pkg/msgpack"
)

const (
	thresholdUnknown = 24 * time.Hour
	thresholdStale   = 3 * 24 * time.Hour
	thresholdRemove  = 7 * 24 * time.Hour

	statusAvailable = "available"
	statusUnknown   = "unknown"
	statusStale     = "stale"
)

var discoverableTypes = map[string]struct{}{
	"BackboneInterface":  {},
	"TCPServerInterface": {},
	"I2PInterface":       {},
	"RNodeInterface":     {},
	"WeaveInterface":     {},
	"KISSInterface":      {},
}

// DiscoveredInterface is one persisted rnstransport discovery record.
type DiscoveredInterface struct {
	Type                string
	Name                string
	Transport           bool
	ReachableOn         string
	Port                int64
	HasPort             bool
	TransportID         []byte
	NetworkID           []byte
	OperatorLXMFAddress []byte
	IFACNetname         string
	IFACNetkey          string
	Hops                uint8
	Value               int
	Received            float64
	Discovered          float64
	LastHeard           float64
	HeardCount          int
	Status              string
	StatusCode          int
	ConfigEntry         string
	Latitude            *float64
	Longitude           *float64
	Height              *float64
}

// ListOptions filters list_discovered_interfaces output.
type ListOptions struct {
	OnlyAvailable bool
	OnlyTransport bool
	NameFilter    string
}

// DiscoveryHash returns the SHA-256 of hex(transport_id)+name (Python discovery_hash).
func DiscoveryHash(transportID []byte, name string) []byte {
	tidHex := hex.EncodeToString(transportID)
	sum := sha256.Sum256([]byte(tidHex + name))
	return sum[:]
}

// PersistDiscoveredInterface writes info under storageDir/discovery/interfaces/.
func PersistDiscoveredInterface(storageDir string, info *ReceivedAnnounceInfo) error {
	if storageDir == "" || info == nil {
		return nil
	}
	rec := buildDiscoveredRecord(info, info.Hops, float64(time.Now().Unix()))
	return persistDiscoveredRecord(storageDir, rec)
}

func buildDiscoveredRecord(info *ReceivedAnnounceInfo, hops uint8, now float64) *DiscoveredInterface {
	if info == nil {
		return nil
	}
	rec := &DiscoveredInterface{
		Type:        info.Info.Type,
		Name:        sanitizeDiscoveryName(info.Info.Name),
		Transport:   info.Info.Transport,
		ReachableOn: info.Info.ReachableOn,
		Port:        info.Info.Port,
		HasPort:     info.Info.HasPort,
		TransportID: append([]byte(nil), info.Info.TransportID...),
		NetworkID:   append([]byte(nil), info.RemoteIdentity...),
		IFACNetname: info.Info.IFACNetname,
		IFACNetkey:  info.Info.IFACNetkey,
		Hops:        hops,
		Value:       info.StampValue,
		Received:    now,
	}
	if info.Info.HasGeo {
		lat := info.Info.Latitude
		lon := info.Info.Longitude
		height := info.Info.Height
		rec.Latitude = &lat
		rec.Longitude = &lon
		rec.Height = &height
	}
	if len(info.Info.OperatorLXMFAddress) > 0 {
		rec.OperatorLXMFAddress = append([]byte(nil), info.Info.OperatorLXMFAddress...)
	}
	rec.ConfigEntry = configEntryForDiscovered(rec)
	return rec
}

func persistDiscoveredRecord(storageDir string, rec *DiscoveredInterface) error {
	if storageDir == "" || rec == nil || len(rec.TransportID) == 0 || rec.Name == "" {
		return nil
	}
	if _, ok := discoverableTypes[rec.Type]; !ok {
		return nil
	}
	root, err := discoveryInterfacesRoot(storageDir)
	if err != nil {
		return err
	}
	defer root.Close()

	dh := DiscoveryHash(rec.TransportID, rec.Name)
	if len(dh) == 0 {
		return nil
	}
	name := hex.EncodeToString(dh)
	if !isSafeDiscoveryPersistName(name) {
		return nil
	}

	var existing map[string]any
	if raw, err := root.ReadFile(name); err == nil {
		_ = msgpack.Unmarshal(raw, &existing)
	}
	now := rec.Received
	if now == 0 {
		now = float64(time.Now().Unix())
	}
	payload := discoveredToMap(rec, existing, now)
	packed, err := msgpack.Marshal(payload)
	if err != nil {
		return err
	}
	tmp := name + ".tmp"
	if err := root.WriteFile(tmp, packed, 0o600); err != nil {
		return err
	}
	return root.Rename(tmp, name)
}

func discoveredToMap(rec *DiscoveredInterface, existing map[string]any, now float64) map[string]any {
	discovered := now
	heardCount := 0
	if existing != nil {
		if v, ok := existing["discovered"].(float64); ok {
			discovered = v
		} else if v, ok := existing["discovered"].(int64); ok {
			discovered = float64(v)
		}
		switch c := existing["heard_count"].(type) {
		case int64:
			heardCount = int(c)
		case int:
			heardCount = c
		case float64:
			heardCount = int(c)
		}
		if existing["discovered"] != nil {
			heardCount++
		}
	}
	m := map[string]any{
		"type":           rec.Type,
		"name":           rec.Name,
		"transport":      rec.Transport,
		"transport_id":   hex.EncodeToString(rec.TransportID),
		"network_id":     hex.EncodeToString(rec.NetworkID),
		"hops":           rec.Hops,
		"value":          rec.Value,
		"received":       now,
		"discovered":     discovered,
		"last_heard":     now,
		"heard_count":    heardCount,
		"discovery_hash": DiscoveryHash(rec.TransportID, rec.Name),
	}
	if rec.ReachableOn != "" {
		m["reachable_on"] = rec.ReachableOn
	}
	if rec.HasPort {
		m["port"] = rec.Port
	}
	if rec.IFACNetname != "" {
		m["ifac_netname"] = rec.IFACNetname
	}
	if rec.IFACNetkey != "" {
		m["ifac_netkey"] = rec.IFACNetkey
	}
	if len(rec.OperatorLXMFAddress) > 0 {
		m["operator_lxmf_address"] = hex.EncodeToString(rec.OperatorLXMFAddress)
	}
	if rec.Latitude != nil {
		m["latitude"] = *rec.Latitude
	}
	if rec.Longitude != nil {
		m["longitude"] = *rec.Longitude
	}
	if rec.Height != nil {
		m["height"] = *rec.Height
	}
	if rec.ConfigEntry != "" {
		m["config_entry"] = rec.ConfigEntry
	}
	return m
}

// ListDiscoveredInterfaces reads persisted discovery records (Python list_discovered_interfaces).
func ListDiscoveredInterfaces(storageDir string, opts ListOptions) ([]*DiscoveredInterface, error) {
	if storageDir == "" {
		return nil, nil
	}
	root, err := discoveryInterfacesRoot(storageDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer root.Close()

	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	now := time.Now()
	out := make([]*DiscoveredInterface, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !isSafeDiscoveryPersistName(name) {
			continue
		}
		raw, err := root.ReadFile(name)
		if err != nil {
			continue
		}
		var m map[string]any
		if err := msgpack.Unmarshal(raw, &m); err != nil {
			_ = root.Remove(name)
			continue
		}
		rec, remove := normalizeDiscoveredRecord(m, now)
		if remove {
			_ = root.Remove(name)
			continue
		}
		if opts.NameFilter != "" && !strings.Contains(strings.ToLower(rec.Name), strings.ToLower(opts.NameFilter)) {
			continue
		}
		if opts.OnlyAvailable && rec.Status != statusAvailable {
			continue
		}
		if opts.OnlyTransport && !rec.Transport {
			continue
		}
		out = append(out, rec)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StatusCode != out[j].StatusCode {
			return out[i].StatusCode > out[j].StatusCode
		}
		if out[i].Value != out[j].Value {
			return out[i].Value > out[j].Value
		}
		return out[i].LastHeard > out[j].LastHeard
	})
	return out, nil
}

func normalizeDiscoveredRecord(m map[string]any, now time.Time) (*DiscoveredInterface, bool) {
	if m == nil {
		return nil, true
	}
	rec := &DiscoveredInterface{}
	rec.Type, _ = m["type"].(string)
	rec.Name = sanitizeDiscoveryName(stringField(m["name"]))
	rec.Transport, _ = m["transport"].(bool)
	rec.ReachableOn, _ = m["reachable_on"].(string)
	rec.IFACNetname, _ = m["ifac_netname"].(string)
	rec.IFACNetkey, _ = m["ifac_netkey"].(string)
	rec.ConfigEntry, _ = m["config_entry"].(string)
	rec.TransportID = decodeHexField(m["transport_id"])
	rec.NetworkID = decodeHexField(m["network_id"])
	if op := decodeHexField(m["operator_lxmf_address"]); len(op) > 0 {
		rec.OperatorLXMFAddress = op
	}
	switch p := m["port"].(type) {
	case int64:
		rec.Port = p
		rec.HasPort = true
	case int:
		rec.Port = int64(p)
		rec.HasPort = true
	case float64:
		rec.Port = int64(p)
		rec.HasPort = true
	}
	switch v := m["value"].(type) {
	case int:
		rec.Value = v
	case int8:
		rec.Value = int(v)
	case int16:
		rec.Value = int(v)
	case int32:
		rec.Value = int(v)
	case int64:
		rec.Value = int(v)
	case uint8:
		rec.Value = int(v)
	case uint16:
		rec.Value = int(v)
	case uint32:
		rec.Value = int(v)
	case uint64:
		if v > uint64(math.MaxInt) {
			rec.Value = math.MaxInt
		} else {
			rec.Value = int(v)
		}
	case float64:
		rec.Value = int(v)
	}
	switch h := m["hops"].(type) {
	case uint8:
		rec.Hops = h
	case int:
		rec.Hops = hopsToUint8(int64(h))
	case int64:
		rec.Hops = hopsToUint8(h)
	case float64:
		rec.Hops = hopsToUint8(int64(h))
	}
	rec.Received = floatField(m["received"])
	rec.Discovered = floatField(m["discovered"])
	rec.LastHeard = floatField(m["last_heard"])
	switch c := m["heard_count"].(type) {
	case int:
		rec.HeardCount = c
	case int64:
		rec.HeardCount = int(c)
	case float64:
		rec.HeardCount = int(c)
	}
	if lat, ok := m["latitude"].(float64); ok {
		rec.Latitude = &lat
	}
	if lon, ok := m["longitude"].(float64); ok {
		rec.Longitude = &lon
	}
	if height, ok := m["height"].(float64); ok {
		rec.Height = &height
	}

	lastHeard := time.Unix(int64(rec.LastHeard), 0)
	heardDelta := now.Sub(lastHeard)
	if heardDelta > thresholdRemove {
		return nil, true
	}
	if len(rec.TransportID) == 0 || len(rec.NetworkID) == 0 {
		return nil, true
	}
	if _, ok := discoverableTypes[rec.Type]; !ok {
		return nil, true
	}
	if rec.ReachableOn != "" && !isReachableHost(rec.ReachableOn) {
		return nil, true
	}
	switch {
	case heardDelta > thresholdStale:
		rec.Status = statusStale
		rec.StatusCode = 1
	case heardDelta > thresholdUnknown:
		rec.Status = statusUnknown
		rec.StatusCode = 2
	default:
		rec.Status = statusAvailable
		rec.StatusCode = 3
	}
	return rec, false
}

func decodeHexField(v any) []byte {
	if s, ok := v.(string); ok {
		if b, err := hex.DecodeString(s); err == nil {
			return b
		}
		return []byte(s)
	}
	return asBytes(v)
}

func isReachableHost(host string) bool {
	host = strings.TrimSpace(host)
	if host == "" {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return true
	}
	if len(host) > 253 {
		return false
	}
	for part := range strings.SplitSeq(host, ".") {
		if part == "" || len(part) > 63 {
			return false
		}
	}
	return true
}

func discoveryInterfacesRoot(storageDir string) (*os.Root, error) {
	dir := filepath.Join(storageDir, "discovery", "interfaces")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	return os.OpenRoot(dir)
}

func sanitizeDiscoveryName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return name
	}
	return name
}

func stringField(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func hopsToUint8(v int64) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

func floatField(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int64:
		return float64(x)
	case int:
		return float64(x)
	default:
		return 0
	}
}

func configEntryForDiscovered(rec *DiscoveredInterface) string {
	if rec == nil {
		return ""
	}
	tidHex := hex.EncodeToString(rec.TransportID)
	netname := ""
	netkey := ""
	if rec.IFACNetname != "" {
		netname = "\n  network_name = " + rec.IFACNetname
	}
	if rec.IFACNetkey != "" {
		netkey = "\n  passphrase = " + rec.IFACNetkey
	}
	identityStr := "\n  transport_identity = " + tidHex
	switch rec.Type {
	case "BackboneInterface", "TCPServerInterface":
		remoteKey := "remote"
		ifaceType := "BackboneInterface"
		if rec.Type == "TCPServerInterface" {
			ifaceType = "TCPClientInterface"
			remoteKey = "target_host"
		}
		return fmt.Sprintf("[[%s]]\n  type = %s\n  enabled = yes\n  %s = %s\n  target_port = %d%s%s%s",
			rec.Name, ifaceType, remoteKey, rec.ReachableOn, rec.Port, identityStr, netname, netkey)
	case "I2PInterface":
		return fmt.Sprintf("[[%s]]\n  type = I2PInterface\n  enabled = yes\n  peers = %s%s%s%s",
			rec.Name, rec.ReachableOn, identityStr, netname, netkey)
	default:
		return ""
	}
}

func LoadPersistedInterfaces(storageDir string) ([]*ReceivedAnnounceInfo, error) {
	list, err := ListDiscoveredInterfaces(storageDir, ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]*ReceivedAnnounceInfo, 0, len(list))
	for _, rec := range list {
		if rec == nil || rec.ReachableOn == "" {
			continue
		}
		info := &ReceivedAnnounceInfo{
			StampValue:     rec.Value,
			RemoteIdentity: append([]byte(nil), rec.NetworkID...),
			Hops:           rec.Hops,
		}
		info.Info.Type = rec.Type
		info.Info.Name = rec.Name
		info.Info.Transport = rec.Transport
		info.Info.ReachableOn = rec.ReachableOn
		info.Info.Port = rec.Port
		info.Info.HasPort = rec.HasPort
		info.Info.TransportID = append([]byte(nil), rec.TransportID...)
		info.Info.IFACNetname = rec.IFACNetname
		info.Info.IFACNetkey = rec.IFACNetkey
		out = append(out, info)
	}
	return out, nil
}
