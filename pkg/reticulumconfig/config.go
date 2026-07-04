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

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/ifac"
)

// Default values used when a fresh configuration is created or fields are
// omitted from the on-disk file.
const (
	DefaultSharedInstancePort  = 37428
	DefaultInstanceControlPort = 37429
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
		EnableTransport:     true,
		ShareInstance:       true,
		SharedInstancePort:  DefaultSharedInstancePort,
		InstanceControlPort: DefaultInstanceControlPort,
		PanicOnInterfaceErr: false,
		LogLevel:            DefaultLogLevel,
		Interfaces:          make(map[string]*common.InterfaceConfig),
		EnableSandbox:       true,
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

// parseBool accepts yes/no/true/false/on/off/1/0, case-insensitive. Anything
// else evaluates to false.
func parseBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "yes", "y", "on", "1":
		return true
	default:
		return false
	}
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

// stripInlineComment removes a trailing "# comment" or "; comment" tail from a
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
// interface entry; depth 1 must match a reserved name.
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

	return cfg, nil
}

// applyGlobalOption sets a top-level [reticulum] key. Unknown keys and
// invalid integer values are ignored so a typo never aborts startup.
func applyGlobalOption(cfg *common.ReticulumConfig, key, value string) {
	switch strings.ToLower(key) {
	case "enable_transport":
		cfg.EnableTransport = parseBool(value)
	case "share_instance":
		cfg.ShareInstance = parseBool(value)
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
		cfg.PanicOnInterfaceErr = parseBool(value)
	case "loglevel":
		setInt(value, &cfg.LogLevel)
	case "enable_sandbox":
		cfg.EnableSandbox = parseBool(value)
	}
}

// applyLoggingOption handles keys under [logging].
func applyLoggingOption(cfg *common.ReticulumConfig, key, value string) {
	if strings.EqualFold(key, "loglevel") {
		setInt(value, &cfg.LogLevel)
	}
}

// applyInterfaceOption sets a single key on an interface configuration.
// Unknown keys are ignored.
func applyInterfaceOption(iface *common.InterfaceConfig, key, value string) {
	switch strings.ToLower(key) {
	case "type":
		iface.Type = value
	case "interface_enabled", "enabled":
		iface.Enabled = parseBool(value)
	case "address", "listen_ip":
		iface.Address = value
	case "port", "listen_port":
		setInt(value, &iface.Port)
	case "target_host":
		iface.TargetHost = value
	case "target_port":
		setInt(value, &iface.TargetPort)
	case "target_address":
		iface.TargetAddress = value
	case "interface":
		iface.Interface = value
	case "kiss_framing":
		iface.KISSFraming = parseBool(value)
	case "i2p_tunneled":
		iface.I2PTunneled = parseBool(value)
	case "peers":
		iface.I2PPeers = parseStringList(value)
	case "connectable":
		iface.I2PConnectable = parseBool(value)
	case "sam_address":
		iface.I2PSAMAddress = value
	case "prefer_ipv6":
		iface.PreferIPv6 = parseBool(value)
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
		iface.IngressControl = parseBool(value)
		iface.IngressControlSet = true
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
		iface.PublishIFAC = parseBool(value)
	}
}

// setInt assigns *dst from value when value parses cleanly as a base-10 int.
func setInt(value string, dst *int) {
	if v, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
		*dst = v
	}
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
	fmt.Fprintf(&b, "  shared_instance_port = %d\n", cfg.SharedInstancePort)
	fmt.Fprintf(&b, "  instance_control_port = %d\n", cfg.InstanceControlPort)
	fmt.Fprintf(&b, "  panic_on_interface_error = %s\n", boolStr(cfg.PanicOnInterfaceErr))
	fmt.Fprintf(&b, "  enable_sandbox = %s\n\n", boolStr(cfg.EnableSandbox))

	b.WriteString("[logging]\n")
	fmt.Fprintf(&b, "  loglevel = %d\n\n", cfg.LogLevel)

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
	b.WriteString("\n")
}

// boolStr renders a Go bool using the yes/no spelling expected on disk.
func boolStr(v bool) string {
	if v {
		return "yes"
	}
	return "no"
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
