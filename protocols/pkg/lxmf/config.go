// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Config is lxmd-style [lxmf], [propagation], and [logging] settings (defaults match upstream lxmd).
type Config struct {
	LXMF        LXMFConfig
	Propagation PropagationConfig
	Logging     LoggingConfig
}

// LXMFConfig is the [lxmf] section.
type LXMFConfig struct {
	DisplayName                     string
	AnnounceAtStart                 bool
	AnnounceIntervalMinutes         int
	DeliveryTransferMaxAcceptedSize float64
	StampCost                       int
	OnInbound                       string
}

// PropagationConfig is the [propagation] section.
type PropagationConfig struct {
	EnableNode                       bool
	NodeName                         string
	AuthRequired                     bool
	AnnounceAtStart                  bool
	AnnounceIntervalMinutes          int
	Autopeer                         bool
	AutopeerMaxDepth                 int
	MessageStorageLimitMB            float64
	PropagationTransferMaxAcceptedKB float64
	PropagationMessageMaxAcceptedKB  float64
	PropagationSyncMaxAcceptedKB     float64
	PropagationStampCostTarget       int
	PropagationStampCostFlexibility  int
	PeeringCost                      int
	RemotePeeringCostMax             int
	PrioritiseDestinations           []string
	ControlAllowed                   []string
	StaticPeers                      []string
	MaxPeers                         int
	FromStaticOnly                   bool
	MaxInboundSyncs                  int
	SequentialPNStampValidation      bool
	StaticPeersBypassSequential      bool
}

// LoggingConfig is the [logging] section.
type LoggingConfig struct {
	Level int
}

// DefaultConfig returns upstream lxmd defaults.
func DefaultConfig() Config {
	return Config{
		LXMF: LXMFConfig{
			DisplayName:                     "Anonymous Peer",
			AnnounceAtStart:                 false,
			AnnounceIntervalMinutes:         0,
			DeliveryTransferMaxAcceptedSize: 1000,
			StampCost:                       12,
		},
		Propagation: PropagationConfig{
			EnableNode:                       false,
			AuthRequired:                     false,
			AnnounceAtStart:                  true,
			AnnounceIntervalMinutes:          360,
			Autopeer:                         true,
			AutopeerMaxDepth:                 4,
			MessageStorageLimitMB:            500,
			PropagationTransferMaxAcceptedKB: 256,
			PropagationMessageMaxAcceptedKB:  256,
			PropagationSyncMaxAcceptedKB:     10240,
			PropagationStampCostTarget:       16,
			PropagationStampCostFlexibility:  3,
			PeeringCost:                      18,
			RemotePeeringCostMax:             26,
			MaxPeers:                         20,
			FromStaticOnly:                   false,
			MaxInboundSyncs:                  3,
			SequentialPNStampValidation:      true,
			StaticPeersBypassSequential:      true,
		},
		Logging: LoggingConfig{
			Level: LogInfo,
		},
	}
}

// LoadConfig reads lxmd-style sectioned key=value text (# comments, comma lists).
func LoadConfig(path string) (Config, error) {
	f, err := os.Open(filepath.Clean(path)) // #nosec G304 -- path is supplied by operator
	if err != nil {
		return Config{}, err
	}
	defer f.Close()
	return ParseConfig(f)
}

// ParseConfig merges r into DefaultConfig.
func ParseConfig(r io.Reader) (Config, error) {
	cfg := DefaultConfig()
	sections, err := readSectionedKV(r)
	if err != nil {
		return cfg, err
	}

	if section, ok := sections["lxmf"]; ok {
		if v, ok := section["display_name"]; ok {
			cfg.LXMF.DisplayName = v
		}
		if v, ok := section["announce_at_start"]; ok {
			cfg.LXMF.AnnounceAtStart = parseBool(v)
		}
		if v, ok := section["announce_interval"]; ok {
			cfg.LXMF.AnnounceIntervalMinutes = parseInt(v)
		}
		if v, ok := section["delivery_transfer_max_accepted_size"]; ok {
			cfg.LXMF.DeliveryTransferMaxAcceptedSize = parseFloatOrZero(v)
			if cfg.LXMF.DeliveryTransferMaxAcceptedSize < 0.38 {
				cfg.LXMF.DeliveryTransferMaxAcceptedSize = 0.38
			}
		}
		if v, ok := section["stamp_cost"]; ok {
			cfg.LXMF.StampCost = max(parseInt(v), 1)
		}
		if v, ok := section["on_inbound"]; ok {
			cfg.LXMF.OnInbound = v
		}
	}

	if section, ok := sections["propagation"]; ok {
		if v, ok := section["enable_node"]; ok {
			cfg.Propagation.EnableNode = parseBool(v)
		}
		if v, ok := section["node_name"]; ok {
			cfg.Propagation.NodeName = v
		}
		if v, ok := section["auth_required"]; ok {
			cfg.Propagation.AuthRequired = parseBool(v)
		}
		if v, ok := section["announce_at_start"]; ok {
			cfg.Propagation.AnnounceAtStart = parseBool(v)
		}
		if v, ok := section["announce_interval"]; ok {
			cfg.Propagation.AnnounceIntervalMinutes = parseInt(v)
		}
		if v, ok := section["autopeer"]; ok {
			cfg.Propagation.Autopeer = parseBool(v)
		}
		if v, ok := section["autopeer_maxdepth"]; ok {
			cfg.Propagation.AutopeerMaxDepth = parseInt(v)
		}
		if v, ok := section["message_storage_limit"]; ok {
			cfg.Propagation.MessageStorageLimitMB = parseFloatOrZero(v)
			if cfg.Propagation.MessageStorageLimitMB < 0.005 {
				cfg.Propagation.MessageStorageLimitMB = 0.005
			}
		}
		if v, ok := section["propagation_transfer_max_accepted_size"]; ok {
			cfg.Propagation.PropagationTransferMaxAcceptedKB = parseFloatOrZero(v)
			if cfg.Propagation.PropagationTransferMaxAcceptedKB < 0.38 {
				cfg.Propagation.PropagationTransferMaxAcceptedKB = 0.38
			}
		}
		if v, ok := section["propagation_message_max_accepted_size"]; ok {
			cfg.Propagation.PropagationMessageMaxAcceptedKB = parseFloatOrZero(v)
			if cfg.Propagation.PropagationMessageMaxAcceptedKB < 0.38 {
				cfg.Propagation.PropagationMessageMaxAcceptedKB = 0.38
			}
		}
		if v, ok := section["propagation_sync_max_accepted_size"]; ok {
			cfg.Propagation.PropagationSyncMaxAcceptedKB = parseFloatOrZero(v)
			if cfg.Propagation.PropagationSyncMaxAcceptedKB < 0.38 {
				cfg.Propagation.PropagationSyncMaxAcceptedKB = 0.38
			}
		}
		if v, ok := section["propagation_stamp_cost_target"]; ok {
			cfg.Propagation.PropagationStampCostTarget = max(parseInt(v), PropagationStampCostMin)
		}
		if v, ok := section["propagation_stamp_cost_flexibility"]; ok {
			cfg.Propagation.PropagationStampCostFlexibility = max(parseInt(v), 0)
		}
		if v, ok := section["peering_cost"]; ok {
			cfg.Propagation.PeeringCost = max(parseInt(v), 0)
		}
		if v, ok := section["remote_peering_cost_max"]; ok {
			cfg.Propagation.RemotePeeringCostMax = max(parseInt(v), 0)
		}
		if v, ok := section["prioritise_destinations"]; ok {
			cfg.Propagation.PrioritiseDestinations = parseList(v)
		}
		if v, ok := section["control_allowed"]; ok {
			cfg.Propagation.ControlAllowed = parseList(v)
		}
		if v, ok := section["static_peers"]; ok {
			cfg.Propagation.StaticPeers = parseList(v)
		}
		if v, ok := section["max_peers"]; ok {
			cfg.Propagation.MaxPeers = parseInt(v)
		}
		if v, ok := section["from_static_only"]; ok {
			cfg.Propagation.FromStaticOnly = parseBool(v)
		}
		if v, ok := section["max_inbound_syncs"]; ok {
			cfg.Propagation.MaxInboundSyncs = max(parseInt(v), 1)
		}
		if v, ok := section["sequential_pn_stamp_validation"]; ok {
			cfg.Propagation.SequentialPNStampValidation = parseBool(v)
		}
		if v, ok := section["static_peers_bypass_sequential"]; ok {
			cfg.Propagation.StaticPeersBypassSequential = parseBool(v)
		}
	}

	if section, ok := sections["logging"]; ok {
		if v, ok := section["loglevel"]; ok {
			cfg.Logging.Level = min(max(parseInt(v), LogCritical), LogExtreme)
		}
	}

	return cfg, nil
}

// SaveConfig writes WriteConfig output to path atomically.
func SaveConfig(cfg Config, path string) error {
	var buf bytes.Buffer
	if err := WriteConfig(cfg, &buf); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// WriteConfig emits lxmd-style text parseable by ParseConfig.
func WriteConfig(cfg Config, w io.Writer) error {
	buf := bufio.NewWriter(w)
	emitSection := func(name string, kv []kvPair) {
		fmt.Fprintf(buf, "[%s]\n\n", name)
		for _, p := range kv {
			fmt.Fprintf(buf, "%s = %s\n", p.k, p.v)
		}
		fmt.Fprintln(buf)
	}
	emitSection("lxmf", []kvPair{
		{"display_name", cfg.LXMF.DisplayName},
		{"announce_at_start", boolStr(cfg.LXMF.AnnounceAtStart)},
		{"announce_interval", strconv.Itoa(cfg.LXMF.AnnounceIntervalMinutes)},
		{"delivery_transfer_max_accepted_size", floatStr(cfg.LXMF.DeliveryTransferMaxAcceptedSize)},
		{"stamp_cost", strconv.Itoa(cfg.LXMF.StampCost)},
		{"on_inbound", cfg.LXMF.OnInbound},
	})
	emitSection("propagation", []kvPair{
		{"enable_node", boolStr(cfg.Propagation.EnableNode)},
		{"node_name", cfg.Propagation.NodeName},
		{"auth_required", boolStr(cfg.Propagation.AuthRequired)},
		{"announce_at_start", boolStr(cfg.Propagation.AnnounceAtStart)},
		{"announce_interval", strconv.Itoa(cfg.Propagation.AnnounceIntervalMinutes)},
		{"autopeer", boolStr(cfg.Propagation.Autopeer)},
		{"autopeer_maxdepth", strconv.Itoa(cfg.Propagation.AutopeerMaxDepth)},
		{"message_storage_limit", floatStr(cfg.Propagation.MessageStorageLimitMB)},
		{"propagation_transfer_max_accepted_size", floatStr(cfg.Propagation.PropagationTransferMaxAcceptedKB)},
		{"propagation_message_max_accepted_size", floatStr(cfg.Propagation.PropagationMessageMaxAcceptedKB)},
		{"propagation_sync_max_accepted_size", floatStr(cfg.Propagation.PropagationSyncMaxAcceptedKB)},
		{"propagation_stamp_cost_target", strconv.Itoa(cfg.Propagation.PropagationStampCostTarget)},
		{"propagation_stamp_cost_flexibility", strconv.Itoa(cfg.Propagation.PropagationStampCostFlexibility)},
		{"peering_cost", strconv.Itoa(cfg.Propagation.PeeringCost)},
		{"remote_peering_cost_max", strconv.Itoa(cfg.Propagation.RemotePeeringCostMax)},
		{"prioritise_destinations", strings.Join(cfg.Propagation.PrioritiseDestinations, ", ")},
		{"control_allowed", strings.Join(cfg.Propagation.ControlAllowed, ", ")},
		{"static_peers", strings.Join(cfg.Propagation.StaticPeers, ", ")},
		{"max_peers", strconv.Itoa(cfg.Propagation.MaxPeers)},
		{"from_static_only", boolStr(cfg.Propagation.FromStaticOnly)},
		{"max_inbound_syncs", strconv.Itoa(cfg.Propagation.MaxInboundSyncs)},
		{"sequential_pn_stamp_validation", boolStr(cfg.Propagation.SequentialPNStampValidation)},
		{"static_peers_bypass_sequential", boolStr(cfg.Propagation.StaticPeersBypassSequential)},
	})
	emitSection("logging", []kvPair{
		{"loglevel", strconv.Itoa(cfg.Logging.Level)},
	})
	return buf.Flush()
}

// WriteDefaultConfigFile writes DefaultConfigText to path atomically.
func WriteDefaultConfigFile(path string) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(DefaultConfigText), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// StaticPeerHashes parses StaticPeers as destination-length hex hashes.
func (p PropagationConfig) StaticPeerHashes() ([][]byte, error) {
	out := make([][]byte, 0, len(p.StaticPeers))
	for _, s := range p.StaticPeers {
		h, err := decodeHashHex(s)
		if err != nil {
			return nil, fmt.Errorf("static peer %q: %w", s, err)
		}
		out = append(out, h)
	}
	return out, nil
}

// PrioritisedHashes parses PrioritiseDestinations as hex hashes.
func (p PropagationConfig) PrioritisedHashes() ([][]byte, error) {
	out := make([][]byte, 0, len(p.PrioritiseDestinations))
	for _, s := range p.PrioritiseDestinations {
		h, err := decodeHashHex(s)
		if err != nil {
			return nil, fmt.Errorf("prioritised %q: %w", s, err)
		}
		out = append(out, h)
	}
	return out, nil
}

// ControlAllowedHashes parses ControlAllowed as hex identity hashes.
func (p PropagationConfig) ControlAllowedHashes() ([][]byte, error) {
	out := make([][]byte, 0, len(p.ControlAllowed))
	for _, s := range p.ControlAllowed {
		h, err := decodeHashHex(s)
		if err != nil {
			return nil, fmt.Errorf("control identity %q: %w", s, err)
		}
		out = append(out, h)
	}
	return out, nil
}

func decodeHashHex(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if len(s) != 2*DestinationLength {
		return nil, fmt.Errorf("expected %d hex characters", 2*DestinationLength)
	}
	return hex.DecodeString(s)
}

type kvPair struct{ k, v string }

func parseBool(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "yes", "y", "true", "on":
		return true
	default:
		return false
	}
}

func parseInt(v string) int {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0
	}
	return n
}

func parseFloatOrZero(v string) float64 {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0
	}
	return f
}

func parseList(v string) []string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func boolStr(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func floatStr(f float64) string {
	if f == float64(int64(f)) {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func readSectionedKV(r io.Reader) (map[string]map[string]string, error) {
	out := map[string]map[string]string{}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 1<<16), 1<<20)
	current := ""
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		if i := strings.Index(line, "#"); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			current = strings.TrimSpace(line[1 : len(line)-1])
			if current == "" {
				return nil, fmt.Errorf("lxmf: empty section header at line %d", lineNo)
			}
			if _, ok := out[current]; !ok {
				out[current] = map[string]string{}
			}
			continue
		}
		before, after, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("lxmf: malformed line %d: %q", lineNo, line)
		}
		if current == "" {
			return nil, fmt.Errorf("lxmf: key %q outside any section at line %d", strings.TrimSpace(before), lineNo)
		}
		k := strings.TrimSpace(before)
		v := strings.TrimSpace(after)
		if k == "" {
			return nil, fmt.Errorf("lxmf: empty key at line %d", lineNo)
		}
		out[current][k] = v
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// SectionNames returns map keys sorted.
func SectionNames(sections map[string]map[string]string) []string {
	names := make([]string, 0, len(sections))
	for k := range sections {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// ErrConfigNotFound is returned when the config path is missing (first-run detection).
var ErrConfigNotFound = errors.New("lxmf: config file not found")

// DefaultConfigText is the stock commented lxmd template.
const DefaultConfigText = `# This is an LXM Daemon config file.
# It is a plaintext sectioned key=value file. Lines starting with '#'
# are comments. List values are comma separated. Boolean values accept
# yes/no, true/false, 1/0, on/off.

[propagation]

# Whether this daemon should run as a propagation node.
enable_node = no

# Optional human-readable node name advertised in propagation announces.
# node_name = Anonymous Propagation Node

# Identity hashes allowed to issue control commands to this node.
# control_allowed = 7d7e542829b40f32364499b27438dba8, 437229f8e29598b2282b88bad5e44698

# Automatic announce interval in minutes (6 hours by default).
announce_interval = 360

# Whether to announce the propagation destination at startup.
announce_at_start = yes

# Whether to automatically peer with other propagation nodes on the network.
autopeer = yes

# Maximum hop depth for automatically peered nodes.
autopeer_maxdepth = 4

# Maximum disk usage for the propagation message store, in megabytes.
# message_storage_limit = 500

# Maximum accepted size of a single incoming propagation message, in KB.
# propagation_message_max_accepted_size = 256

# Maximum accepted size of a single inbound propagation node sync, in KB.
# propagation_sync_max_accepted_size = 10240

# Maximum number of simultaneous inbound peer sync transfers.
# max_inbound_syncs = 3

# Target stamp cost required to deliver messages via this node.
# propagation_stamp_cost_target = 16

# Stamp cost flexibility (in bits below target accepted from peers).
# propagation_stamp_cost_flexibility = 3

# Peering key cost target required for remote nodes peering with this node.
# peering_cost = 18

# Maximum acceptable peering cost when peering with remote nodes.
# remote_peering_cost_max = 26

# Destinations whose messages should be retained preferentially.
# prioritise_destinations = 41d20c727598a3fbbdf9106133a3a0ed, d924b81822ca24e68e2effea99bcb8cf

# Maximum number of automatically peered nodes.
# max_peers = 20

# Static peer destination hashes to always peer with.
# static_peers = e17f833c4ddf8890dd3a79a6fea8161d, 5a2d0029b6e5ec87020abaea0d746da4

# When yes, only accept inbound propagation traffic from configured static peers.
# from_static_only = no

# Whether to require client identity authentication for sync requests.
auth_required = no


[lxmf]

# Display name advertised by the local LXMF delivery destination.
display_name = Anonymous Peer

# Whether to announce the local delivery destination at startup.
announce_at_start = no

# Optional automatic announce interval, in minutes.
# announce_interval = 360

# Maximum unpacked size of inbound delivery messages, in kilobytes.
delivery_transfer_max_accepted_size = 1000

# Stamp cost required for inbound messages, advertised in delivery
# announces so senders can generate a valid stamp.
# stamp_cost = 12

# Optional command executed for every received message. The full path of
# the saved message file is appended as a single quoted argument.
# on_inbound = rm


[logging]
# Valid log levels are 1 through 7:
#   1: Critical
#   2: Error
#   3: Warning
#   4: Info / Notice (default)
#   5: Verbose
#   6: Debug
#   7: Extreme
loglevel = 4
`

// PropagationStampCostMin mirrors LXMRouter.PROPAGATION_COST_MIN.
const PropagationStampCostMin = 13
