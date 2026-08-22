// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package reticulumconfig

import (
	"bufio"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/ifac"
)

// Default values used when a fresh configuration is created or fields are
// omitted from the on-disk file.
const (
	DefaultSharedInstancePort  = 37428
	DefaultInstanceControlPort = 37429
	DefaultControlAPIPort      = 37430
	DefaultControlAPIHost      = "127.0.0.1"
	DefaultLogLevel            = 4
	DefaultConfigDirName       = ".reticulum-go"
	DefaultConfigFileName      = "config"
)

// section kinds tracked while walking the parser stack.
const (
	sectionReticulum  = "reticulum"
	sectionLogging    = "logging"
	sectionInterfaces = "interfaces"
	sectionInterface  = "interface"
	sectionUnknown    = "unknown"
)

// maxLineBytes caps the size of a single configuration line accepted by the
// parser. Longer lines surface as a parse error rather than an out-of-memory
// panic from bufio.Scanner.
const maxLineBytes = 1 << 20

// errEmptyConfigPath is returned by SaveConfig when ConfigPath is unset.
var errEmptyConfigPath = errors.New("config path not set")

// DefaultConfig returns a ReticulumConfig populated with built-in defaults.
func DefaultConfig() *common.ReticulumConfig {
	return &common.ReticulumConfig{
		EnableTransport:        true,
		ShareInstance:          true,
		SharedInstancePort:     DefaultSharedInstancePort,
		InstanceControlPort:    DefaultInstanceControlPort,
		PanicOnInterfaceErr:    false,
		LogLevel:               DefaultLogLevel,
		Interfaces:             make(map[string]*common.InterfaceConfig),
		EnableSandbox:          true,
		EnableSeccomp:          true,
		AllowLinkPathRebalance: true,
		ControlAPIHost:         DefaultControlAPIHost,
		ControlAPIPort:         DefaultControlAPIPort,
		DoSProtection:          "off",
	}
}

// GetConfigPath returns ~/.reticulum-go/config.
func GetConfigPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, DefaultConfigDirName, DefaultConfigFileName), nil
}

// EnsureConfigDir creates ~/.reticulum-go with restrictive permissions if it
// does not already exist.
func EnsureConfigDir() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	return os.MkdirAll(filepath.Join(homeDir, DefaultConfigDirName), 0o700) // #nosec G301
}

// parseBool accepts yes/no/true/false/on/off/1/0, case-insensitive.
// Unrecognized spellings return ok=false so callers keep their defaults
// instead of silently treating typos as false (which disabled sandbox/seccomp).
func parseBool(value string) (parsed, ok bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "yes", "y", "on", "1":
		return true, true
	case "false", "no", "n", "off", "0":
		return false, true
	default:
		return false, false
	}
}

// setBool assigns *dst when value is a recognized boolean spelling.
// It reports whether the value was applied.
func setBool(dst *bool, value string) bool {
	v, ok := parseBool(value)
	if ok {
		*dst = v
	}
	return ok
}

func splitCommaPaths(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

// sectionFrame is one entry in the parser's section stack.
type sectionFrame struct {
	depth int
	kind  string
	name  string
}

// sectionHeader recognises bracketed section headers with matching opening and
// closing bracket counts. depth equals the bracket count and is zero when the
// line is not a header.
func sectionHeader(line string) (depth int, name string, ok bool) {
	if len(line) < 3 || line[0] != '[' {
		return 0, "", false
	}
	for depth < len(line) && line[depth] == '[' {
		depth++
	}
	if depth*2 >= len(line) {
		return 0, "", false
	}
	closing := 0
	for closing < depth && line[len(line)-1-closing] == ']' {
		closing++
	}
	if closing != depth {
		return 0, "", false
	}
	if len(line)-depth*2 <= 0 {
		return 0, "", false
	}
	name = strings.TrimSpace(line[depth : len(line)-depth])
	if name == "" {
		return 0, "", false
	}
	return depth, name, true
}

// stripInlineComment removes a trailing "# comment" or ". Comment" tail from a

// value, requiring whitespace before the marker so URLs and hashes stay intact.
func stripInlineComment(value string) string {
	for i := 1; i < len(value); i++ {
		if (value[i] == '#' || value[i] == ';') && (value[i-1] == ' ' || value[i-1] == '\t') {
			return strings.TrimSpace(value[:i])
		}
	}
	return value
}

// stripBOM removes a UTF-8 byte-order mark from the start of s, if present.
func stripBOM(s string) string {
	const bom = "\ufeff"
	if strings.HasPrefix(s, bom) {
		return s[len(bom):]
	}
	return s
}

// classifySection assigns a kind to a header. Depth >= 2 is always an
// interface entry. Depth 1 must match a reserved name.

func classifySection(name string, depth int) string {
	if depth >= 2 {
		return sectionInterface
	}
	switch strings.ToLower(name) {
	case sectionReticulum:
		return sectionReticulum
	case sectionLogging:
		return sectionLogging
	case sectionInterfaces:
		return sectionInterfaces
	default:
		return sectionUnknown
	}
}

// LoadConfig parses the configuration file at path. Unknown keys, malformed
// section headers, and values that fail type conversion are skipped silently
// so that a damaged file still yields a usable, default-filled config rather
// than aborting the program. Only IO and oversize-line errors are returned.
func LoadConfig(path string) (*common.ReticulumConfig, error) {
	file, err := os.Open(path) // #nosec G304
	if err != nil {
		return nil, err
	}
	defer file.Close()

	cfg := DefaultConfig()
	cfg.ConfigPath = path

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineBytes)

	var stack []sectionFrame
	first := true

	for scanner.Scan() {
		line := scanner.Text()
		if first {
			line = stripBOM(line)
			first = false
		}
		line = strings.TrimSpace(line)

		if line == "" || line[0] == '#' || line[0] == ';' {
			continue
		}

		if depth, name, ok := sectionHeader(line); ok {
			for len(stack) > 0 && stack[len(stack)-1].depth >= depth {
				stack = stack[:len(stack)-1]
			}
			kind := classifySection(name, depth)
			stack = append(stack, sectionFrame{depth: depth, kind: kind, name: name})

			if kind == sectionInterface {
				if _, exists := cfg.Interfaces[name]; !exists {
					cfg.Interfaces[name] = &common.InterfaceConfig{Name: name}
				}
			}
			continue
		}

		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		if key == "" {
			continue
		}
		value := stripInlineComment(strings.TrimSpace(line[eq+1:]))

		if len(stack) == 0 {
			applyGlobalOption(cfg, key, value)
			continue
		}

		switch top := stack[len(stack)-1]; top.kind {
		case sectionReticulum:
			applyGlobalOption(cfg, key, value)
		case sectionLogging:
			applyLoggingOption(cfg, key, value)
		case sectionInterface:
			if iface, ok := cfg.Interfaces[top.name]; ok {
				applyInterfaceOption(iface, key, value)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read config %q: %w", path, err)
	}

	cfg.NormalizeInMemoryFlags()
	cfg.ApplyNodeProfile()
	return cfg, nil
}

// applyGlobalOption sets a top-level [reticulum] key. Unknown keys and
// invalid integer values are ignored so a typo never aborts startup.
func applyGlobalOption(cfg *common.ReticulumConfig, key, value string) {
	switch strings.ToLower(key) {
	case "enable_transport":
		setBool(&cfg.EnableTransport, value)
	case "share_instance":
		setBool(&cfg.ShareInstance, value)
	case "shared_instance_port":
		setInt(value, &cfg.SharedInstancePort)
	case "instance_control_port":
		setInt(value, &cfg.InstanceControlPort)
	case "shared_instance_type":
		v := strings.ToLower(strings.TrimSpace(value))
		if v == common.SharedInstanceTCP || v == common.SharedInstanceUnix {
			cfg.SharedInstanceType = v
		}
	case "instance_name":
		cfg.InstanceName = value
	case "rpc_key":
		if b, err := decodeRPCKey(value); err == nil {
			cfg.RPCKey = b
		}
	case "panic_on_interface_error":
		setBool(&cfg.PanicOnInterfaceErr, value)
	case "loglevel":
		setInt(value, &cfg.LogLevel)
	case "enable_sandbox":
		setBool(&cfg.EnableSandbox, value)
	case "enable_seccomp":
		setBool(&cfg.EnableSeccomp, value)
	case "sandbox_strict":
		setBool(&cfg.SandboxStrict, value)
	case "sandbox_profile":
		v := strings.ToLower(strings.TrimSpace(value))
		switch v {
		case common.SandboxProfileFull, common.SandboxProfileRouter, "":
			cfg.SandboxProfile = v
		}
	case "sandbox_extra_paths":
		cfg.SandboxExtraPaths = splitCommaPaths(value)
	case "sandbox_exec_rlimits":
		setBool(&cfg.SandboxExecRlimits, value)
	case "control_api_socket":
		cfg.ControlAPISocket = strings.TrimSpace(value)
	case "default_gravity":
		if v, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
			cfg.DefaultGravity = v
			cfg.DefaultGravitySet = true
		}
	case "autoconnect_interface_gravity":
		if v, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
			cfg.AutoconnectInterfaceGravity = v
			cfg.AutoconnectInterfaceGravitySet = true
		}
	case "autoconnect_interface_mode":
		cfg.AutoconnectInterfaceMode = strings.ToLower(strings.TrimSpace(value))
	case "autoconnect_announces_to_internal":
		if setBool(&cfg.AutoconnectAnnouncesToInternal, value) {
			cfg.AutoconnectAnnouncesToInternalSet = true
		}
	case "autoconnect_discovered_interfaces":
		var n int
		setInt(value, &n)
		if n > 0 {
			cfg.AutoconnectDiscoveredInterfaces = n
		}
	case "publish_blackhole":
		setBool(&cfg.PublishBlackhole, value)
	case "blackhole_sources":
		cfg.BlackholeSources = parseIdentityHashes(value)
	case "blackhole_update_interval":
		var minutes float64
		setFloat(value, &minutes)
		if minutes > 0 {
			if minutes < 2 {
				minutes = 2
			}
			cfg.BlackholeUpdateInterval = time.Duration(minutes * float64(time.Minute))
		}
	case "allow_link_path_rebalance":
		if setBool(&cfg.AllowLinkPathRebalance, value) {
			cfg.AllowLinkPathRebalanceSet = true
		}
	case "enable_control_api":
		setBool(&cfg.EnableControlAPI, value)
	case "control_api_host":
		cfg.ControlAPIHost = value
	case "control_api_port":
		setInt(value, &cfg.ControlAPIPort)
	case "in_memory_path_table":
		setBool(&cfg.InMemoryPathTable, value)
	case "in_memory_known_destinations":
		setBool(&cfg.InMemoryKnownDestinations, value)
	case "in_memory_storage":
		setBool(&cfg.InMemoryStorage, value)
	case "identity_backend":
		cfg.IdentityBackend = strings.TrimSpace(value)
	case "soft_memory_limit":
		if n, err := common.ParseByteSize(value); err == nil {
			cfg.SoftMemoryLimitBytes = n
		}
	case "dos_protection":
		v := strings.ToLower(strings.TrimSpace(value))
		switch v {
		case "off", "detect", "prevent", "auto", "ids", "ips", "block", "smart":
			if v == "ids" {
				v = "detect"
			}
			if v == "ips" || v == "block" {
				v = "prevent"
			}
			if v == "smart" {
				v = "auto"
			}
			cfg.DoSProtection = v
			cfg.DoSProtectionSet = true
		}
	case "dos_max_pps":
		setFloat(value, &cfg.DoSMaxPPS)
	case "dos_max_bps":
		if n, err := common.ParseByteSize(value); err == nil {
			cfg.DoSMaxBPS = float64(n)
		}
	case "dos_floor_pps":
		setFloat(value, &cfg.DoSFloorPPS)
	case "dos_floor_bps":
		if n, err := common.ParseByteSize(value); err == nil {
			cfg.DoSFloorBPS = float64(n)
		}
	case "dos_max_conns":
		setInt(value, &cfg.DoSMaxConns)
	case "dos_max_resources":
		setInt(value, &cfg.DoSMaxResources)
	case "dos_max_crypto":
		setInt(value, &cfg.DoSMaxCrypto)
	case "dos_max_handshake":
		setInt(value, &cfg.DoSMaxHandshake)
	case "max_in_memory_paths":
		setInt(value, &cfg.MaxInMemoryPaths)
		cfg.MaxInMemoryPathsSet = true
	case "max_in_memory_known_destinations":
		setInt(value, &cfg.MaxInMemoryKnownDestinations)
		cfg.MaxInMemoryKnownDestinationsSet = true
	case "max_in_memory_resource_bytes":
		if n, err := common.ParseByteSize(value); err == nil {
			cfg.MaxInMemoryResourceBytes = n
		}
	case "max_packet_hashlist":
		setInt(value, &cfg.MaxPacketHashlist)
		cfg.MaxPacketHashlistSet = true
	case "max_packet_handlers":
		setInt(value, &cfg.MaxPacketHandlers)
		cfg.MaxPacketHandlersSet = true
	case "node_profile":
		v := strings.ToLower(strings.TrimSpace(value))
		switch v {
		case common.NodeProfileDefault, common.NodeProfileCoreRouter, common.NodeProfileEmbedded, "":
			cfg.NodeProfile = v
		}
	case "discover_interfaces":
		setBool(&cfg.DiscoverInterfaces, value)
	case "watch_interfaces":
		if setBool(&cfg.WatchInterfaces, value) {
			cfg.WatchInterfacesSet = true
		}
	case "backbone_io", "io_backend":
		cfg.BackboneIO = strings.TrimSpace(value)
		cfg.BackboneIOSet = true
	case "static_transport_identity":
		setBool(&cfg.StaticTransportIdentity, value)
	case "local_hops_delta":
		setBool(&cfg.LocalHopsDelta, value)
	case "respond_to_probes", "allow_probes":
		setBool(&cfg.RespondToProbes, value)
	case "enable_remote_management":
		setBool(&cfg.EnableRemoteManagement, value)
	case "remote_management_allowed":
		cfg.RemoteManagementAllowed = parseIdentityHashes(value)
	case "network_identity":
		cfg.NetworkIdentityPath = strings.TrimSpace(value)
	}
}

// applyLoggingOption handles keys under [logging].
func applyLoggingOption(cfg *common.ReticulumConfig, key, value string) {
	switch strings.ToLower(key) {
	case "loglevel":
		setInt(value, &cfg.LogLevel)
	case "destination":
		cfg.LogDestination = strings.ToLower(strings.TrimSpace(value))
	case "logfile", "log_file":
		cfg.LogFile = strings.TrimSpace(value)
	case "format":
		cfg.LogFormat = strings.ToLower(strings.TrimSpace(value))
	}
}

// applyInterfaceOption sets a single key on an interface configuration.
// Unknown keys are ignored.
func applyInterfaceOption(iface *common.InterfaceConfig, key, value string) {
	switch strings.ToLower(key) {
	case "type":
		iface.Type = value
	case "interface_enabled", "enabled":
		setBool(&iface.Enabled, value)
	case "address", "listen_ip":
		iface.Address = value
	case "port", "listen_port":
		// Serial uses a device path in port=. Numeric values stay listen ports.
		if isNonNumericPort(value) {
			iface.Device = value
		} else {
			setInt(value, &iface.Port)
		}
	case "device":
		iface.Device = value
	case "speed", "baud":
		setInt(value, &iface.Speed)
	case "databits":
		setInt(value, &iface.DataBits)
	case "parity":
		iface.Parity = value
	case "stopbits":
		setInt(value, &iface.StopBits)
	case "rtscts":
		setBool(&iface.RTSCTS, value)
	case "dsrdtr":
		setBool(&iface.DSRDTR, value)
	case "xonxoff":
		setBool(&iface.XONXOFF, value)
	case "frame_idle_ms":
		setInt(value, &iface.SerialFrameIdleMs)
	case "path":
		iface.Path = value
	case "transport_mode":
		iface.TransportMode = value
	case "domain":
		iface.Domain = value
	case "resolve_interval":
		setInt(value, &iface.ResolveIntervalSec)
	case "context_id", "cid":
		setInt(value, &iface.ContextID)
	case "long_poll_sec":
		setInt(value, &iface.LongPollSec)
	case "target_host", "forward_ip":
		iface.TargetHost = value
	case "remote":
		if strings.TrimSpace(iface.TargetHost) == "" {
			iface.TargetHost = value
		}
	case "target_port", "forward_port":
		setInt(value, &iface.TargetPort)
	case "target_address":
		iface.TargetAddress = value
	case "interface":
		iface.Interface = value
	case "kiss_framing":
		setBool(&iface.KISSFraming, value)
	case "i2p_tunneled":
		setBool(&iface.I2PTunneled, value)
	case "peers":
		iface.I2PPeers = parseStringList(value)
	case "connectable":
		setBool(&iface.I2PConnectable, value)
	case "sam_address":
		iface.I2PSAMAddress = value
	case "prefer_ipv6":
		setBool(&iface.PreferIPv6, value)
	case "max_reconnect_tries":
		setInt(value, &iface.MaxReconnTries)
	case "bitrate":
		setInt64(value, &iface.Bitrate)
	case "mtu":
		setInt(value, &iface.MTU)
	case "discovery_port":
		setInt(value, &iface.DiscoveryPort)
	case "data_port":
		setInt(value, &iface.DataPort)
	case "discovery_scope":
		iface.DiscoveryScope = value
	case "group_id":
		iface.GroupID = value
	case "multicast_address_type":
		iface.MulticastAddrType = value
	case "devices":
		iface.Devices = parseStringList(value)
	case "ignored_devices":
		iface.IgnoredDevices = parseStringList(value)
	case "announce_cap":
		setFloat(value, &iface.AnnounceCap)
	case "announce_rate_target":
		setFloat(value, &iface.AnnounceRateTarget)
	case "announce_rate_grace":
		setInt(value, &iface.AnnounceRateGrace)
	case "announce_rate_penalty":
		setFloat(value, &iface.AnnounceRatePenalty)
	case "ingress_control":
		if setBool(&iface.IngressControl, value) {
			iface.IngressControlSet = true
		}
	case "ic_new_time":
		setInt(value, &iface.ICNewTime)
	case "ic_burst_freq_new":
		setFloat(value, &iface.ICBurstFreqNew)
	case "ic_burst_freq":
		setFloat(value, &iface.ICBurstFreq)
	case "ic_max_held_announces":
		setInt(value, &iface.ICMaxHeldAnnounces)
	case "ic_burst_hold":
		setInt(value, &iface.ICBurstHold)
	case "ic_burst_penalty":
		setInt(value, &iface.ICBurstPenalty)
	case "ic_held_release_interval":
		setInt(value, &iface.ICHeldReleaseInterval)
	case "network_name", "networkname":
		iface.NetworkName = value
	case "passphrase", "pass_phrase":
		iface.Passphrase = value
	case "ifac_netname":
		iface.IFACNetname = value
	case "ifac_netkey":
		iface.IFACNetkey = value
	case "ifac_size":
		setIFACSize(value, &iface.IFACSize)
	case "publish_ifac":
		setBool(&iface.PublishIFAC, value)
	case "command":
		iface.Command = value
	case "respawn_delay", "respawn_interval":
		setInt(value, &iface.RespawnDelay)
	case "shared_instance_type":
		iface.SharedInstanceType = strings.ToLower(strings.TrimSpace(value))
	case "instance_name":
		iface.InstanceName = value
	case "cert_file":
		iface.CertFile = value
	case "key_file":
		iface.KeyFile = value
	case "peer_key":
		iface.PeerKey = value
	case "sni":
		iface.SNI = value
	case "mode", "interface_mode":
		iface.Mode = strings.ToLower(strings.TrimSpace(value))
	case "recursive_prs":
		setBool(&iface.RecursivePRs, value)
	case "announces_from_internal":
		if setBool(&iface.AnnouncesFromInternal, value) {
			iface.AnnouncesFromInternalSet = true
		}
	case "announces_to_internal":
		if setBool(&iface.AnnouncesToInternal, value) {
			iface.AnnouncesToInternalSet = true
		}
	case "gravity":
		if v, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
			iface.Gravity = v
			iface.GravitySet = true
		}
	case "outgoing", "selected_outgoing":
		if setBool(&iface.Outgoing, value) {
			iface.OutgoingSet = true
		}
	case "discoverable":
		setBool(&iface.Discoverable, value)
	case "discovery_name":
		iface.DiscoveryName = value
	case "reachable_on":
		iface.ReachableOn = value
	case "announce_interval":
		// Discovery announce interval is configured in minutes.
		minutes := 0
		setInt(value, &minutes)
		if minutes > 0 {
			if minutes < 5 {
				minutes = 5
			}
			iface.DiscoveryAnnounceIntervalSec = minutes * 60
		}
	case "discovery_stamp_value":
		setInt(value, &iface.DiscoveryStampValue)
	case "discovery_encrypt":
		setBool(&iface.DiscoveryEncrypt, value)
	case "location_cmd":
		iface.DiscoveryLocationCmd = value
	case "block_fast_flapping":
		if setBool(&iface.BlockFastFlapping, value) {
			iface.BlockFastFlappingSet = true
		}
	case "fast_flapping_threshold":
		setFloat(value, &iface.FastFlappingThreshold)
	case "fast_flapping_grace":
		setInt(value, &iface.FastFlappingGrace)
	case "fast_flapping_block_time":
		setFloat(value, &iface.FastFlappingBlockTimeMin)
	case "latitude":
		setFloat(value, &iface.DiscoveryLatitude)
		iface.HasDiscoveryGeo = true
	case "longitude":
		setFloat(value, &iface.DiscoveryLongitude)
		iface.HasDiscoveryGeo = true
	case "height":
		setFloat(value, &iface.DiscoveryHeight)
		iface.HasDiscoveryGeo = true
	case "control_host":
		iface.ControlHost = value
	case "control_port":
		setInt(value, &iface.ControlPort)
	case "mtu_overhead":
		setInt(value, &iface.MTUOverhead)
	case "auto_fragmentation":
		if setBool(&iface.AutoFragmentation, value) {
			iface.AutoFragSet = true
		}
	case "short_frames":
		iface.ShortFrames = strings.ToLower(strings.TrimSpace(value))
	case "short_mtu":
		setInt(value, &iface.ShortMTU)
	case "handshake_x2":
		setBool(&iface.HandshakeX2, value)
	case "proof_x2":
		setBool(&iface.ProofX2, value)
	case "auto_bitrate":
		if setBool(&iface.AutoBitrate, value) {
			iface.AutoBitrateSet = true
		}
	case "csma_overhead":
		if setBool(&iface.CSMAOverhead, value) {
			iface.CSMAOverheadSet = true
		}
	case "timeout_margin":
		setFloat(value, &iface.TimeoutMargin)
	case "frequency", "frequency_hz":
		setInt64(value, &iface.FrequencyHz)
	case "sample_rate":
		setInt(value, &iface.SampleRate)
	case "bandwidth":
		setInt(value, &iface.Bandwidth)
	case "rx_gain":
		setFloat(value, &iface.RXGain)
	case "tx_gain":
		setFloat(value, &iface.TXGain)
	case "modem":
		iface.Modem = strings.ToLower(strings.TrimSpace(value))
	case "serial":
		iface.SerialNum = value
	}
}

// setInt assigns *dst from value when value parses cleanly as a base-10 int.
func setInt(value string, dst *int) {
	if v, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
		*dst = v
	}
}

func isNonNumericPort(value string) bool {
	s := strings.TrimSpace(value)
	if s == "" {
		return false
	}
	_, err := strconv.Atoi(s)
	return err != nil
}

// setInt64 mirrors setInt for int64 fields.
func setInt64(value string, dst *int64) {
	if v, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil {
		*dst = v
	}
}

// setFloat assigns *dst when value parses as a float64.
func setFloat(value string, dst *float64) {
	if v, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err == nil {
		*dst = v
	}
}

func parseStringList(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseIdentityHashes(value string) [][]byte {
	out := make([][]byte, 0)
	for _, p := range parseStringList(value) {
		p = strings.ReplaceAll(p, " ", "")
		if len(p) != 32 {
			continue
		}
		b, err := hex.DecodeString(p)
		if err != nil || len(b) != 16 {
			continue
		}
		out = append(out, b)
	}
	return out
}

func setIFACSize(value string, dst *int) {
	v, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || v < ifac.MinSize*8 {
		return
	}
	*dst = v / 8
}

func decodeRPCKey(value string) ([]byte, error) {
	s := strings.TrimSpace(value)
	if s == "" {
		return nil, errors.New("empty rpc_key")
	}
	return hex.DecodeString(s)
}

// SaveConfig writes cfg to cfg.ConfigPath using the nested [reticulum] /
// [logging] / [interfaces] layout that LoadConfig understands.
func SaveConfig(cfg *common.ReticulumConfig) error {
	if cfg == nil {
		return errEmptyConfigPath
	}
	if cfg.ConfigPath == "" {
		return errEmptyConfigPath
	}

	var b strings.Builder
	b.WriteString("# Reticulum-Go configuration\n\n")

	b.WriteString("[reticulum]\n")
	fmt.Fprintf(&b, "  enable_transport = %s\n", boolStr(cfg.EnableTransport))
	fmt.Fprintf(&b, "  share_instance = %s\n", boolStr(cfg.ShareInstance))
	if cfg.InstanceName != "" {
		fmt.Fprintf(&b, "  instance_name = %s\n", cfg.InstanceName)
	}
	if cfg.SharedInstanceType != "" {
		fmt.Fprintf(&b, "  shared_instance_type = %s\n", cfg.SharedInstanceType)
	}
	fmt.Fprintf(&b, "  shared_instance_port = %d\n", cfg.SharedInstancePort)
	fmt.Fprintf(&b, "  instance_control_port = %d\n", cfg.InstanceControlPort)
	if len(cfg.RPCKey) > 0 {
		fmt.Fprintf(&b, "  rpc_key = %x\n", cfg.RPCKey)
	}
	fmt.Fprintf(&b, "  panic_on_interface_error = %s\n", boolStr(cfg.PanicOnInterfaceErr))
	fmt.Fprintf(&b, "  enable_sandbox = %s\n", boolStr(cfg.EnableSandbox))
	fmt.Fprintf(&b, "  enable_seccomp = %s\n", boolStr(cfg.EnableSeccomp))
	fmt.Fprintf(&b, "  sandbox_strict = %s\n", boolStr(cfg.SandboxStrict))
	if cfg.SandboxProfile != "" && cfg.SandboxProfile != common.SandboxProfileFull {
		fmt.Fprintf(&b, "  sandbox_profile = %s\n", cfg.SandboxProfile)
	}
	if len(cfg.SandboxExtraPaths) > 0 {
		fmt.Fprintf(&b, "  sandbox_extra_paths = %s\n", strings.Join(cfg.SandboxExtraPaths, ", "))
	}
	fmt.Fprintf(&b, "  sandbox_exec_rlimits = %s\n", boolStr(cfg.SandboxExecRlimits))
	if cfg.ControlAPISocket != "" {
		fmt.Fprintf(&b, "  control_api_socket = %s\n", cfg.ControlAPISocket)
	}
	if cfg.DefaultGravitySet {
		fmt.Fprintf(&b, "  default_gravity = %d\n", cfg.DefaultGravity)
	}
	if cfg.AutoconnectInterfaceGravitySet {
		fmt.Fprintf(&b, "  autoconnect_interface_gravity = %d\n", cfg.AutoconnectInterfaceGravity)
	}
	if cfg.AutoconnectInterfaceMode != "" {
		fmt.Fprintf(&b, "  autoconnect_interface_mode = %s\n", cfg.AutoconnectInterfaceMode)
	}
	if cfg.AutoconnectAnnouncesToInternalSet {
		fmt.Fprintf(&b, "  autoconnect_announces_to_internal = %s\n", boolStr(cfg.AutoconnectAnnouncesToInternal))
	}
	if cfg.AutoconnectDiscoveredInterfaces > 0 {
		fmt.Fprintf(&b, "  autoconnect_discovered_interfaces = %d\n", cfg.AutoconnectDiscoveredInterfaces)
	}
	if cfg.PublishBlackhole {
		fmt.Fprintf(&b, "  publish_blackhole = yes\n")
	}
	if len(cfg.BlackholeSources) > 0 {
		parts := make([]string, 0, len(cfg.BlackholeSources))
		for _, h := range cfg.BlackholeSources {
			parts = append(parts, hex.EncodeToString(h))
		}
		fmt.Fprintf(&b, "  blackhole_sources = %s\n", strings.Join(parts, ", "))
	}
	if cfg.BlackholeUpdateInterval > 0 {
		fmt.Fprintf(&b, "  blackhole_update_interval = %g\n", cfg.BlackholeUpdateInterval.Minutes())
	}
	if cfg.AllowLinkPathRebalanceSet {
		fmt.Fprintf(&b, "  allow_link_path_rebalance = %s\n", boolStr(cfg.AllowLinkPathRebalance))
	}
	fmt.Fprintf(&b, "  enable_control_api = %s\n", boolStr(cfg.EnableControlAPI))
	fmt.Fprintf(&b, "  control_api_host = %s\n", controlAPIHostOrDefault(cfg.ControlAPIHost))
	fmt.Fprintf(&b, "  control_api_port = %d\n", controlAPIPortOrDefault(cfg.ControlAPIPort))
	fmt.Fprintf(&b, "  in_memory_path_table = %s\n", boolStr(cfg.InMemoryPathTable))
	fmt.Fprintf(&b, "  in_memory_known_destinations = %s\n", boolStr(cfg.InMemoryKnownDestinations))
	fmt.Fprintf(&b, "  in_memory_storage = %s\n", boolStr(cfg.InMemoryStorage))
	if cfg.NodeProfile != "" && cfg.NodeProfile != common.NodeProfileDefault {
		fmt.Fprintf(&b, "  node_profile = %s\n", cfg.NodeProfile)
	}
	if cfg.IdentityBackend != "" {
		fmt.Fprintf(&b, "  identity_backend = %s\n", cfg.IdentityBackend)
	}
	if cfg.SoftMemoryLimitBytes > 0 {
		fmt.Fprintf(&b, "  soft_memory_limit = %d\n", cfg.SoftMemoryLimitBytes)
	}
	dos := strings.ToLower(strings.TrimSpace(cfg.DoSProtection))
	if dos == "" {
		dos = "off"
	}
	fmt.Fprintf(&b, "  dos_protection = %s\n", dos)
	if cfg.DoSMaxPPS > 0 {
		fmt.Fprintf(&b, "  dos_max_pps = %g\n", cfg.DoSMaxPPS)
	}
	if cfg.DoSMaxBPS > 0 {
		fmt.Fprintf(&b, "  dos_max_bps = %g\n", cfg.DoSMaxBPS)
	}
	if cfg.DoSFloorPPS > 0 {
		fmt.Fprintf(&b, "  dos_floor_pps = %g\n", cfg.DoSFloorPPS)
	}
	if cfg.DoSFloorBPS > 0 {
		fmt.Fprintf(&b, "  dos_floor_bps = %g\n", cfg.DoSFloorBPS)
	}
	if cfg.DoSMaxConns > 0 {
		fmt.Fprintf(&b, "  dos_max_conns = %d\n", cfg.DoSMaxConns)
	}
	if cfg.DoSMaxResources > 0 {
		fmt.Fprintf(&b, "  dos_max_resources = %d\n", cfg.DoSMaxResources)
	}
	if cfg.DoSMaxCrypto > 0 {
		fmt.Fprintf(&b, "  dos_max_crypto = %d\n", cfg.DoSMaxCrypto)
	}
	if cfg.DoSMaxHandshake > 0 {
		fmt.Fprintf(&b, "  dos_max_handshake = %d\n", cfg.DoSMaxHandshake)
	}
	if cfg.MaxInMemoryPaths != 0 {
		fmt.Fprintf(&b, "  max_in_memory_paths = %d\n", cfg.MaxInMemoryPaths)
	}
	if cfg.MaxInMemoryKnownDestinations != 0 {
		fmt.Fprintf(&b, "  max_in_memory_known_destinations = %d\n", cfg.MaxInMemoryKnownDestinations)
	}
	if cfg.MaxInMemoryResourceBytes != 0 {
		fmt.Fprintf(&b, "  max_in_memory_resource_bytes = %d\n", cfg.MaxInMemoryResourceBytes)
	}
	if cfg.MaxPacketHashlist != 0 {
		fmt.Fprintf(&b, "  max_packet_hashlist = %d\n", cfg.MaxPacketHashlist)
	}
	if cfg.MaxPacketHandlers != 0 {
		fmt.Fprintf(&b, "  max_packet_handlers = %d\n", cfg.MaxPacketHandlers)
	}
	fmt.Fprintf(&b, "  discover_interfaces = %s\n", boolStr(cfg.DiscoverInterfaces))
	fmt.Fprintf(&b, "  watch_interfaces = %s\n", boolStr(cfg.WatchInterfaces))
	fmt.Fprintf(&b, "  static_transport_identity = %s\n", boolStr(cfg.StaticTransportIdentity))
	fmt.Fprintf(&b, "  local_hops_delta = %s\n", boolStr(cfg.LocalHopsDelta))
	fmt.Fprintf(&b, "  respond_to_probes = %s\n", boolStr(cfg.RespondToProbes))
	if cfg.EnableRemoteManagement {
		fmt.Fprintf(&b, "  enable_remote_management = %s\n", boolStr(cfg.EnableRemoteManagement))
	}
	if len(cfg.RemoteManagementAllowed) > 0 {
		parts := make([]string, len(cfg.RemoteManagementAllowed))
		for i, h := range cfg.RemoteManagementAllowed {
			parts[i] = hex.EncodeToString(h)
		}
		fmt.Fprintf(&b, "  remote_management_allowed = %s\n", strings.Join(parts, ", "))
	}
	if cfg.BackboneIO != "" {
		fmt.Fprintf(&b, "  backbone_io = %s\n", cfg.BackboneIO)
	}
	fmt.Fprintln(&b)

	b.WriteString("[logging]\n")
	fmt.Fprintf(&b, "  loglevel = %d\n", cfg.LogLevel)
	dest := strings.ToLower(strings.TrimSpace(cfg.LogDestination))
	if dest == "" {
		dest = "stderr"
	}
	fmt.Fprintf(&b, "  destination = %s\n", dest)
	if cfg.LogFile != "" {
		fmt.Fprintf(&b, "  logfile = %s\n", cfg.LogFile)
	}
	if cfg.LogFormat != "" {
		fmt.Fprintf(&b, "  format = %s\n", cfg.LogFormat)
	}
	fmt.Fprintln(&b)

	b.WriteString("[interfaces]\n\n")

	for _, name := range sortedInterfaceNames(cfg.Interfaces) {
		writeInterface(&b, name, cfg.Interfaces[name])
	}

	if err := os.MkdirAll(filepath.Dir(cfg.ConfigPath), 0o700); err != nil { // #nosec G301
		return err
	}
	return os.WriteFile(cfg.ConfigPath, []byte(b.String()), 0o600) // #nosec G306
}

// writeInterface serialises a single interface block.
func writeInterface(b *strings.Builder, name string, iface *common.InterfaceConfig) {
	if iface == nil {
		return
	}
	fmt.Fprintf(b, "  [[%s]]\n", name)
	if iface.Type != "" {
		fmt.Fprintf(b, "    type = %s\n", iface.Type)
	}
	fmt.Fprintf(b, "    enabled = %s\n", boolStr(iface.Enabled))

	if iface.Address != "" {
		fmt.Fprintf(b, "    address = %s\n", iface.Address)
	}
	if iface.Port != 0 {
		fmt.Fprintf(b, "    port = %d\n", iface.Port)
	}
	if iface.TargetHost != "" {
		fmt.Fprintf(b, "    target_host = %s\n", iface.TargetHost)
	}
	if iface.TargetPort != 0 {
		fmt.Fprintf(b, "    target_port = %d\n", iface.TargetPort)
	}
	if iface.TargetAddress != "" {
		fmt.Fprintf(b, "    target_address = %s\n", iface.TargetAddress)
	}
	if iface.Interface != "" {
		fmt.Fprintf(b, "    interface = %s\n", iface.Interface)
	}
	if iface.KISSFraming {
		fmt.Fprintf(b, "    kiss_framing = %s\n", boolStr(iface.KISSFraming))
	}
	if iface.I2PTunneled {
		fmt.Fprintf(b, "    i2p_tunneled = %s\n", boolStr(iface.I2PTunneled))
	}
	if iface.I2PConnectable {
		fmt.Fprintf(b, "    connectable = %s\n", boolStr(iface.I2PConnectable))
	}
	if iface.I2PSAMAddress != "" {
		fmt.Fprintf(b, "    sam_address = %s\n", iface.I2PSAMAddress)
	}
	if len(iface.I2PPeers) > 0 {
		fmt.Fprintf(b, "    peers = %s\n", strings.Join(iface.I2PPeers, ", "))
	}
	if iface.PreferIPv6 {
		fmt.Fprintf(b, "    prefer_ipv6 = %s\n", boolStr(iface.PreferIPv6))
	}
	if iface.MaxReconnTries != 0 {
		fmt.Fprintf(b, "    max_reconnect_tries = %d\n", iface.MaxReconnTries)
	}
	if iface.Bitrate != 0 {
		fmt.Fprintf(b, "    bitrate = %d\n", iface.Bitrate)
	}
	if iface.MTU != 0 {
		fmt.Fprintf(b, "    mtu = %d\n", iface.MTU)
	}
	if iface.DiscoveryPort != 0 {
		fmt.Fprintf(b, "    discovery_port = %d\n", iface.DiscoveryPort)
	}
	if iface.DataPort != 0 {
		fmt.Fprintf(b, "    data_port = %d\n", iface.DataPort)
	}
	if iface.DiscoveryScope != "" {
		fmt.Fprintf(b, "    discovery_scope = %s\n", iface.DiscoveryScope)
	}
	if iface.GroupID != "" {
		fmt.Fprintf(b, "    group_id = %s\n", iface.GroupID)
	}
	if iface.MulticastAddrType != "" {
		fmt.Fprintf(b, "    multicast_address_type = %s\n", iface.MulticastAddrType)
	}
	if len(iface.Devices) > 0 {
		fmt.Fprintf(b, "    devices = %s\n", strings.Join(iface.Devices, ", "))
	}
	if len(iface.IgnoredDevices) > 0 {
		fmt.Fprintf(b, "    ignored_devices = %s\n", strings.Join(iface.IgnoredDevices, ", "))
	}
	if iface.AnnounceCap != 0 {
		fmt.Fprintf(b, "    announce_cap = %g\n", iface.AnnounceCap)
	}
	if iface.AnnounceRateTarget != 0 {
		fmt.Fprintf(b, "    announce_rate_target = %g\n", iface.AnnounceRateTarget)
	}
	if iface.AnnounceRateGrace != 0 {
		fmt.Fprintf(b, "    announce_rate_grace = %d\n", iface.AnnounceRateGrace)
	}
	if iface.AnnounceRatePenalty != 0 {
		fmt.Fprintf(b, "    announce_rate_penalty = %g\n", iface.AnnounceRatePenalty)
	}
	if iface.IngressControlSet {
		fmt.Fprintf(b, "    ingress_control = %s\n", boolStr(iface.IngressControl))
	}
	if iface.Mode != "" {
		fmt.Fprintf(b, "    mode = %s\n", iface.Mode)
	}
	if iface.GravitySet {
		fmt.Fprintf(b, "    gravity = %d\n", iface.Gravity)
	}
	if iface.RecursivePRs {
		fmt.Fprintf(b, "    recursive_prs = %s\n", boolStr(iface.RecursivePRs))
	}
	if iface.AnnouncesFromInternalSet {
		fmt.Fprintf(b, "    announces_from_internal = %s\n", boolStr(iface.AnnouncesFromInternal))
	}
	if iface.AnnouncesToInternalSet {
		fmt.Fprintf(b, "    announces_to_internal = %s\n", boolStr(iface.AnnouncesToInternal))
	}
	if iface.CertFile != "" {
		fmt.Fprintf(b, "    cert_file = %s\n", iface.CertFile)
	}
	if iface.KeyFile != "" {
		fmt.Fprintf(b, "    key_file = %s\n", iface.KeyFile)
	}
	if iface.PeerKey != "" {
		fmt.Fprintf(b, "    peer_key = %s\n", iface.PeerKey)
	}
	if iface.SNI != "" {
		fmt.Fprintf(b, "    sni = %s\n", iface.SNI)
	}
	if iface.Path != "" {
		fmt.Fprintf(b, "    path = %s\n", iface.Path)
	}
	if iface.TransportMode != "" {
		fmt.Fprintf(b, "    transport_mode = %s\n", iface.TransportMode)
	}
	if iface.Domain != "" {
		fmt.Fprintf(b, "    domain = %s\n", iface.Domain)
	}
	if iface.ResolveIntervalSec != 0 {
		fmt.Fprintf(b, "    resolve_interval = %d\n", iface.ResolveIntervalSec)
	}
	if iface.ContextID != 0 {
		fmt.Fprintf(b, "    context_id = %d\n", iface.ContextID)
	}
	if iface.LongPollSec != 0 {
		fmt.Fprintf(b, "    long_poll_sec = %d\n", iface.LongPollSec)
	}
	if iface.Discoverable {
		fmt.Fprintf(b, "    discoverable = %s\n", boolStr(iface.Discoverable))
	}
	if iface.DiscoveryName != "" {
		fmt.Fprintf(b, "    discovery_name = %s\n", iface.DiscoveryName)
	}
	if iface.ReachableOn != "" {
		fmt.Fprintf(b, "    reachable_on = %s\n", iface.ReachableOn)
	}
	if iface.DiscoveryAnnounceIntervalSec > 0 {
		fmt.Fprintf(b, "    announce_interval = %d\n", iface.DiscoveryAnnounceIntervalSec/60)
	}
	if iface.DiscoveryStampValue != 0 {
		fmt.Fprintf(b, "    discovery_stamp_value = %d\n", iface.DiscoveryStampValue)
	}
	if iface.DiscoveryEncrypt {
		fmt.Fprintf(b, "    discovery_encrypt = %s\n", boolStr(iface.DiscoveryEncrypt))
	}
	if iface.DiscoveryLocationCmd != "" {
		fmt.Fprintf(b, "    location_cmd = %s\n", iface.DiscoveryLocationCmd)
	}
	if iface.BlockFastFlappingSet {
		fmt.Fprintf(b, "    block_fast_flapping = %s\n", boolStr(iface.BlockFastFlapping))
	}
	if iface.FastFlappingThreshold != 0 {
		fmt.Fprintf(b, "    fast_flapping_threshold = %g\n", iface.FastFlappingThreshold)
	}
	if iface.FastFlappingGrace != 0 {
		fmt.Fprintf(b, "    fast_flapping_grace = %d\n", iface.FastFlappingGrace)
	}
	if iface.FastFlappingBlockTimeMin != 0 {
		fmt.Fprintf(b, "    fast_flapping_block_time = %g\n", iface.FastFlappingBlockTimeMin)
	}
	if iface.HasDiscoveryGeo {
		fmt.Fprintf(b, "    latitude = %g\n", iface.DiscoveryLatitude)
		fmt.Fprintf(b, "    longitude = %g\n", iface.DiscoveryLongitude)
		fmt.Fprintf(b, "    height = %g\n", iface.DiscoveryHeight)
	}
	if iface.ControlHost != "" {
		fmt.Fprintf(b, "    control_host = %s\n", iface.ControlHost)
	}
	if iface.ControlPort != 0 {
		fmt.Fprintf(b, "    control_port = %d\n", iface.ControlPort)
	}
	if iface.MTUOverhead != 0 {
		fmt.Fprintf(b, "    mtu_overhead = %d\n", iface.MTUOverhead)
	}
	if iface.AutoFragSet {
		fmt.Fprintf(b, "    auto_fragmentation = %s\n", boolStr(iface.AutoFragmentation))
	}
	if iface.ShortFrames != "" {
		fmt.Fprintf(b, "    short_frames = %s\n", iface.ShortFrames)
	}
	if iface.ShortMTU != 0 {
		fmt.Fprintf(b, "    short_mtu = %d\n", iface.ShortMTU)
	}
	if iface.HandshakeX2 {
		fmt.Fprintf(b, "    handshake_x2 = %s\n", boolStr(iface.HandshakeX2))
	}
	if iface.ProofX2 {
		fmt.Fprintf(b, "    proof_x2 = %s\n", boolStr(iface.ProofX2))
	}
	if iface.AutoBitrateSet {
		fmt.Fprintf(b, "    auto_bitrate = %s\n", boolStr(iface.AutoBitrate))
	}
	if iface.CSMAOverheadSet {
		fmt.Fprintf(b, "    csma_overhead = %s\n", boolStr(iface.CSMAOverhead))
	}
	if iface.TimeoutMargin != 0 {
		fmt.Fprintf(b, "    timeout_margin = %g\n", iface.TimeoutMargin)
	}
	if iface.Device != "" {
		fmt.Fprintf(b, "    device = %s\n", iface.Device)
	}
	if iface.SerialNum != "" {
		fmt.Fprintf(b, "    serial = %s\n", iface.SerialNum)
	}
	if iface.FrequencyHz != 0 {
		fmt.Fprintf(b, "    frequency = %d\n", iface.FrequencyHz)
	}
	if iface.SampleRate != 0 {
		fmt.Fprintf(b, "    sample_rate = %d\n", iface.SampleRate)
	}
	if iface.Bandwidth != 0 {
		fmt.Fprintf(b, "    bandwidth = %d\n", iface.Bandwidth)
	}
	if iface.RXGain != 0 {
		fmt.Fprintf(b, "    rx_gain = %g\n", iface.RXGain)
	}
	if iface.TXGain != 0 {
		fmt.Fprintf(b, "    tx_gain = %g\n", iface.TXGain)
	}
	if iface.Modem != "" {
		fmt.Fprintf(b, "    modem = %s\n", iface.Modem)
	}
	b.WriteString("\n")
}

// boolStr renders a Go bool using the yes/no spelling expected on disk.
func boolStr(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

// controlAPIHostOrDefault fills in the default bind host for configs created
// before control_api_host existed or left blank on disk.
func controlAPIHostOrDefault(host string) string {
	if host == "" {
		return DefaultControlAPIHost
	}
	return host
}

// controlAPIPortOrDefault mirrors controlAPIHostOrDefault for the port.
func controlAPIPortOrDefault(port int) int {
	if port == 0 {
		return DefaultControlAPIPort
	}
	return port
}

// sortedInterfaceNames returns the interface map keys in lexicographic order
// so SaveConfig produces stable output.
func sortedInterfaceNames(m map[string]*common.InterfaceConfig) []string {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// CreateDefaultConfig writes a starter configuration file at path containing
// the built-in interface defaults.
func CreateDefaultConfig(path string) error {
	cfg := DefaultConfig()
	cfg.ConfigPath = path

	cfg.Interfaces["Auto Discovery"] = &common.InterfaceConfig{
		Name:           "Auto Discovery",
		Type:           "AutoInterface",
		Enabled:        true,
		GroupID:        "reticulum",
		DiscoveryScope: "link",
		DiscoveryPort:  29716,
		DataPort:       42671,
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil { // #nosec G301
		return err
	}

	return SaveConfig(cfg)
}

// InitConfig loads ~/.reticulum-go/config, creating the file with default
// contents when it is missing.
func InitConfig() (*common.ReticulumConfig, error) {
	configPath, err := GetConfigPath()
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if err := CreateDefaultConfig(configPath); err != nil {
			return nil, err
		}
	}

	return LoadConfig(configPath)
}
