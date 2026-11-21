// SPDX-License-Identifier: LicenseRef-Reticulum
package gorrcd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/rrc"
)

type Config struct {
	ConfigPath                     string
	RoomRegistryPath               string
	ConfigDir                      string
	IdentityPath                   string
	AnnounceOnStart                bool
	AnnouncePeriodS                float64
	HubName                        string
	Greeting                       string
	TrustedIdentities              []string
	BannedIdentities               []string
	RoomRegistryPruneAfterS        float64
	RoomRegistryPruneIntervalS     float64
	RoomInviteTimeoutS             float64
	IncludeJoinedMemberList        bool
	MaxNickBytes                   uint64
	MaxRoomsPerSession             uint64
	MaxRoomNameBytes               uint64
	MaxMsgBodyBytes                uint64
	RateLimitMsgsPerMinute         uint64
	PingIntervalS                  float64
	PingTimeoutS                   float64
	MaxResourceBytes               uint64
	MaxPendingResourceExpectations int
	ResourceExpectationTTLS        float64
	EnableResourceTransfer         bool
	LogLevel                       string
	LogRNSLevel                    string
	LogConsole                     bool
	ServiceMode                    bool
	LogFile                        string
	LogFormat                      string
	UDPListen                      string
	UDPForward                     string
	ReadyPath                      string
}

func DefaultConfig() Config {
	return Config{
		ConfigPath:                     DefaultConfigPath(),
		RoomRegistryPath:               DefaultRoomsPath(),
		IdentityPath:                   DefaultIdentityPath(),
		AnnounceOnStart:                true,
		HubName:                        "rrc",
		RoomRegistryPruneAfterS:        30 * 24 * 3600,
		RoomRegistryPruneIntervalS:     3600,
		RoomInviteTimeoutS:             900,
		MaxNickBytes:                   rrc.DefaultMaxNickBytes,
		MaxRoomsPerSession:             rrc.DefaultMaxRoomsPerSession,
		MaxRoomNameBytes:               rrc.DefaultMaxRoomNameBytes,
		MaxMsgBodyBytes:                rrc.DefaultMaxMsgBodyBytes,
		RateLimitMsgsPerMinute:         240,
		MaxResourceBytes:               rrc.DefaultMaxResourceBytes,
		MaxPendingResourceExpectations: 8,
		ResourceExpectationTTLS:        30,
		EnableResourceTransfer:         true,
		LogLevel:                       "INFO",
		LogRNSLevel:                    "WARNING",
		LogConsole:                     true,
	}
}

func (c Config) HubLimits() rrc.HubLimits {
	return rrc.HubLimits{
		MaxNickBytes:           c.MaxNickBytes,
		MaxRoomsPerSession:     c.MaxRoomsPerSession,
		MaxRoomNameBytes:       c.MaxRoomNameBytes,
		MaxMsgBodyBytes:        c.MaxMsgBodyBytes,
		RateLimitMsgsPerMinute: c.RateLimitMsgsPerMinute,
	}
}

func expandPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return p
	}
	p = os.ExpandEnv(p)
	if strings.HasPrefix(p, "~"+string(os.PathSeparator)) || p == "~" {
		home, err := os.UserHomeDir()
		if err == nil {
			p = filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}

func LoadConfigFile(path string) (Config, error) {
	cfg := DefaultConfig()
	cfg.ConfigPath = path
	raw, err := os.ReadFile(filepath.Clean(path)) // #nosec G304 -- operator config path
	if err != nil {
		return cfg, err
	}
	return applyTOML(cfg, string(raw))
}

func applyTOML(cfg Config, text string) (Config, error) {
	section := ""
	sc := bufio.NewScanner(strings.NewReader(text))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			continue
		}
		k, v, ok := splitKV(line)
		if !ok {
			continue
		}
		switch section {
		case "hub", "":
			cfg = applyHubKey(cfg, k, v)
		case "logging":
			cfg = applyLogKey(cfg, k, v)
		}
	}
	return cfg, sc.Err()
}

func splitKV(line string) (string, string, bool) {
	i := strings.IndexByte(line, '=')
	if i <= 0 {
		return "", "", false
	}
	k := strings.ToLower(strings.TrimSpace(line[:i]))
	v := strings.TrimSpace(line[i+1:])
	return k, v, true
}

func applyHubKey(cfg Config, k, v string) Config {
	switch k {
	case "configdir":
		cfg.ConfigDir = unquote(v)
	case "identity_path":
		cfg.IdentityPath = expandPath(unquote(v))
	case "room_registry_path":
		cfg.RoomRegistryPath = expandPath(unquote(v))
	case "announce_on_start", "announce":
		cfg.AnnounceOnStart = parseBool(v)
	case "announce_period_s":
		cfg.AnnouncePeriodS = parseFloat(v)
	case "hub_name":
		cfg.HubName = unquote(v)
	case "greeting":
		cfg.Greeting = unquote(v)
	case "trusted_identities":
		cfg.TrustedIdentities = parseStringList(v)
	case "banned_identities":
		cfg.BannedIdentities = parseStringList(v)
	case "room_registry_prune_after_s":
		cfg.RoomRegistryPruneAfterS = parseFloat(v)
	case "room_registry_prune_interval_s":
		cfg.RoomRegistryPruneIntervalS = parseFloat(v)
	case "room_invite_timeout_s":
		cfg.RoomInviteTimeoutS = parseFloat(v)
	case "include_joined_member_list":
		cfg.IncludeJoinedMemberList = parseBool(v)
	case "max_nick_bytes":
		cfg.MaxNickBytes = parseU64(v)
	case "max_room_name_bytes":
		cfg.MaxRoomNameBytes = parseU64(v)
	case "max_msg_body_bytes":
		cfg.MaxMsgBodyBytes = parseU64(v)
	case "max_rooms_per_session":
		cfg.MaxRoomsPerSession = parseU64(v)
	case "rate_limit_msgs_per_minute":
		cfg.RateLimitMsgsPerMinute = parseU64(v)
	case "ping_interval_s":
		cfg.PingIntervalS = parseFloat(v)
	case "ping_timeout_s":
		cfg.PingTimeoutS = parseFloat(v)
	case "enable_resource_transfer":
		cfg.EnableResourceTransfer = parseBool(v)
	case "max_resource_bytes":
		cfg.MaxResourceBytes = parseU64(v)
	case "max_pending_resource_expectations":
		n := min(parseU64(v), uint64(^uint(0)>>1))
		cfg.MaxPendingResourceExpectations = int(n) // #nosec G115 -- clamped to int range
	case "resource_expectation_ttl_s":
		cfg.ResourceExpectationTTLS = parseFloat(v)
	}
	return cfg
}

func applyLogKey(cfg Config, k, v string) Config {
	switch k {
	case "level":
		cfg.LogLevel = strings.ToUpper(unquote(v))
	case "rns_level":
		cfg.LogRNSLevel = strings.ToUpper(unquote(v))
	case "console":
		cfg.LogConsole = parseBool(v)
	case "file":
		cfg.LogFile = expandPath(unquote(v))
	case "format":
		cfg.LogFormat = unquote(v)
	}
	return cfg
}

func unquote(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
		if s, err := strconv.Unquote(v); err == nil {
			return s
		}
		inner := v[1 : len(v)-1]
		return strings.ReplaceAll(inner, `\"`, `"`)
	}
	if len(v) >= 2 && v[0] == '\'' && v[len(v)-1] == '\'' {
		return v[1 : len(v)-1]
	}
	return v
}

func parseBool(v string) bool {
	s := strings.ToLower(unquote(v))
	return s == "true" || s == "1" || s == "yes"
}

func parseFloat(v string) float64 {
	f, _ := strconv.ParseFloat(unquote(v), 64)
	return f
}

func parseU64(v string) uint64 {
	n, _ := strconv.ParseUint(unquote(v), 10, 64)
	return n
}

func parseStringList(v string) []string {
	v = strings.TrimSpace(v)
	if v == "" || v == "[]" {
		return nil
	}
	if strings.HasPrefix(v, "[") && strings.HasSuffix(v, "]") {
		v = strings.TrimSpace(v[1 : len(v)-1])
	}
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		s := unquote(strings.TrimSpace(p))
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func formatStringList(vals []string) string {
	if len(vals) == 0 {
		return "[]"
	}
	quoted := make([]string, len(vals))
	for i, s := range vals {
		quoted[i] = strconv.Quote(s)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

func WriteConfigFile(cfg Config, path string) error {
	b := marshalConfig(cfg)
	return atomicWrite(path, b, 0o600)
}

func marshalConfig(cfg Config) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "# gorrcd configuration\n\n[hub]\n")
	fmt.Fprintf(&b, "configdir = %s\n", strconv.Quote(cfg.ConfigDir))
	fmt.Fprintf(&b, "identity_path = %s\n", strconv.Quote(cfg.IdentityPath))
	fmt.Fprintf(&b, "room_registry_path = %s\n", strconv.Quote(cfg.RoomRegistryPath))
	fmt.Fprintf(&b, "announce_on_start = %v\n", cfg.AnnounceOnStart)
	fmt.Fprintf(&b, "announce_period_s = %v\n", cfg.AnnouncePeriodS)
	fmt.Fprintf(&b, "hub_name = %s\n", strconv.Quote(cfg.HubName))
	fmt.Fprintf(&b, "greeting = %s\n", strconv.Quote(cfg.Greeting))
	fmt.Fprintf(&b, "trusted_identities = %s\n", formatStringList(cfg.TrustedIdentities))
	fmt.Fprintf(&b, "banned_identities = %s\n", formatStringList(cfg.BannedIdentities))
	fmt.Fprintf(&b, "room_registry_prune_after_s = %v\n", cfg.RoomRegistryPruneAfterS)
	fmt.Fprintf(&b, "room_registry_prune_interval_s = %v\n", cfg.RoomRegistryPruneIntervalS)
	fmt.Fprintf(&b, "room_invite_timeout_s = %v\n", cfg.RoomInviteTimeoutS)
	fmt.Fprintf(&b, "include_joined_member_list = %v\n", cfg.IncludeJoinedMemberList)
	fmt.Fprintf(&b, "max_nick_bytes = %d\n", cfg.MaxNickBytes)
	fmt.Fprintf(&b, "max_room_name_bytes = %d\n", cfg.MaxRoomNameBytes)
	fmt.Fprintf(&b, "max_msg_body_bytes = %d\n", cfg.MaxMsgBodyBytes)
	fmt.Fprintf(&b, "max_rooms_per_session = %d\n", cfg.MaxRoomsPerSession)
	fmt.Fprintf(&b, "rate_limit_msgs_per_minute = %d\n", cfg.RateLimitMsgsPerMinute)
	fmt.Fprintf(&b, "ping_interval_s = %v\n", cfg.PingIntervalS)
	fmt.Fprintf(&b, "ping_timeout_s = %v\n", cfg.PingTimeoutS)
	fmt.Fprintf(&b, "enable_resource_transfer = %v\n", cfg.EnableResourceTransfer)
	fmt.Fprintf(&b, "max_resource_bytes = %d\n", cfg.MaxResourceBytes)
	fmt.Fprintf(&b, "max_pending_resource_expectations = %d\n", cfg.MaxPendingResourceExpectations)
	fmt.Fprintf(&b, "resource_expectation_ttl_s = %v\n\n", cfg.ResourceExpectationTTLS)
	fmt.Fprintf(&b, "[logging]\n")
	fmt.Fprintf(&b, "level = %s\n", strconv.Quote(cfg.LogLevel))
	fmt.Fprintf(&b, "rns_level = %s\n", strconv.Quote(cfg.LogRNSLevel))
	fmt.Fprintf(&b, "console = %v\n", cfg.LogConsole)
	fmt.Fprintf(&b, "file = %s\n", strconv.Quote(cfg.LogFile))
	return []byte(b.String())
}

func persistBannedIdentities(path string, banned []string) error {
	raw, err := os.ReadFile(filepath.Clean(path)) // #nosec G304 -- operator config path
	if err != nil {
		return err
	}
	line := "banned_identities = " + formatStringList(banned)
	text := string(raw)
	replaced := false
	var out strings.Builder
	sc := bufio.NewScanner(strings.NewReader(text))
	for sc.Scan() {
		l := sc.Text()
		trim := strings.TrimSpace(l)
		if strings.HasPrefix(trim, "banned_identities") && strings.Contains(trim, "=") {
			out.WriteString(line)
			out.WriteByte('\n')
			replaced = true
			continue
		}
		out.WriteString(l)
		out.WriteByte('\n')
	}
	if err := sc.Err(); err != nil {
		return err
	}
	if !replaced {
		out.WriteString(line)
		out.WriteByte('\n')
	}
	return atomicWrite(path, []byte(out.String()), 0o600)
}
