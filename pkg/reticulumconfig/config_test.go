// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package reticulumconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"quad4/reticulum-go/pkg/common"
)

// TestDefaultConfig sanity-checks the built-in defaults exposed by
// DefaultConfig.
func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg == nil {
		t.Fatal("DefaultConfig returned nil")
	}
	if !cfg.EnableTransport {
		t.Error("EnableTransport should be true by default")
	}
	if cfg.LogLevel != DefaultLogLevel {
		t.Errorf("LogLevel: got %d, want %d", cfg.LogLevel, DefaultLogLevel)
	}
	if cfg.SharedInstancePort != DefaultSharedInstancePort {
		t.Errorf("SharedInstancePort: got %d, want %d", cfg.SharedInstancePort, DefaultSharedInstancePort)
	}
	if cfg.Interfaces == nil {
		t.Error("Interfaces map must be initialised")
	}
	if cfg.DoSProtection != "auto" {
		t.Errorf("DoSProtection: got %q, want auto", cfg.DoSProtection)
	}
	if !cfg.EnableSandbox {
		t.Error("EnableSandbox should be true by default")
	}
}

// TestParseBool covers every truthy and falsy spelling accepted by the parser.
// Unrecognized values must not apply (ok=false) so defaults stay intact.
func TestParseBool(t *testing.T) {
	truthy := []string{"true", "True", "TRUE", "yes", "Yes", "YES", "y", "Y", "on", "ON", "1", "  yes  "}
	falsy := []string{"false", "no", "n", "off", "0"}
	unknown := []string{"", "maybe", "2", "garbage", "yeah", "enable"}

	for _, v := range truthy {
		got, ok := parseBool(v)
		if !ok || !got {
			t.Errorf("parseBool(%q) = %v,%v want true,true", v, got, ok)
		}
	}
	for _, v := range falsy {
		got, ok := parseBool(v)
		if !ok || got {
			t.Errorf("parseBool(%q) = %v,%v want false,true", v, got, ok)
		}
	}
	for _, v := range unknown {
		got, ok := parseBool(v)
		if ok {
			t.Errorf("parseBool(%q) = %v,%v want ok=false", v, got, ok)
		}
	}
}

// TestSectionHeader exercises both well-formed and malformed bracket headers
// to ensure the parser never misclassifies broken input as a section.
func TestSectionHeader(t *testing.T) {
	cases := []struct {
		line  string
		depth int
		name  string
		ok    bool
	}{
		{"[reticulum]", 1, "reticulum", true},
		{"[[Hub]]", 2, "Hub", true},
		{"[[[Deep Name]]]", 3, "Deep Name", true},
		{"[ spaced ]", 1, "spaced", true},

		{"", 0, "", false},
		{"[", 0, "", false},
		{"[]", 0, "", false},
		{"[[]]", 0, "", false},
		{"[unclosed", 0, "", false},
		{"unopened]", 0, "", false},
		{"[[mismatch]", 0, "", false},
		{"[[ ]]", 0, "", false},
		{"[\n]", 0, "", false},
	}
	for _, c := range cases {
		depth, name, ok := sectionHeader(c.line)
		if depth != c.depth || name != c.name || ok != c.ok {
			t.Errorf("sectionHeader(%q) = (%d, %q, %v), want (%d, %q, %v)",
				c.line, depth, name, ok, c.depth, c.name, c.ok)
		}
	}
}

// TestStripInlineComment ensures hashes inside a value (e.g. URL fragments)
// are preserved while trailing comments are removed.
func TestStripInlineComment(t *testing.T) {
	cases := map[string]string{
		"value":            "value",
		"value # comment":  "value",
		"value\t; comment": "value",
		"value#nocomment":  "value#nocomment",
		"abc;def":          "abc;def",
		"only a value":     "only a value",
		"trailing  ":       "trailing  ",
		"192.0.2.10 # ip ": "192.0.2.10",
	}
	for in, want := range cases {
		if got := stripInlineComment(in); got != want {
			t.Errorf("stripInlineComment(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestLoadConfig_NestedFormat parses a representative on-disk file and
// validates every field flows through to the in-memory config.
func TestLoadConfig_NestedFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")

	body := `# top-level comment
[reticulum]
  enable_transport = yes
  share_instance = No
  shared_instance_port = 55905
  instance_control_port = 55906
  panic_on_interface_error = no

[logging]
  loglevel = 6

[interfaces]

  [[Default Interface]]
    type = AutoInterface
    enabled = Yes
    group_id = reticulum

  [[Hub Node]]
    type = TCPClientInterface
    enabled = yes
    target_host = hub.example.com
    target_port = 4242
    kiss_framing = no
    i2p_tunneled = no
`
	writeFile(t, path, body)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if !cfg.EnableTransport {
		t.Error("EnableTransport should be true")
	}
	if cfg.ShareInstance {
		t.Error("ShareInstance should be false")
	}
	if cfg.SharedInstancePort != 55905 {
		t.Errorf("SharedInstancePort: got %d, want 55905", cfg.SharedInstancePort)
	}
	if cfg.InstanceControlPort != 55906 {
		t.Errorf("InstanceControlPort: got %d, want 55906", cfg.InstanceControlPort)
	}
	if cfg.LogLevel != 6 {
		t.Errorf("LogLevel: got %d, want 6", cfg.LogLevel)
	}

	for _, reserved := range []string{"reticulum", "logging", "interfaces"} {
		if _, ok := cfg.Interfaces[reserved]; ok {
			t.Errorf("reserved section %q must not register as an interface", reserved)
		}
	}

	auto, ok := cfg.Interfaces["Default Interface"]
	if !ok {
		t.Fatal("Default Interface missing")
	}
	if auto.Type != "AutoInterface" || !auto.Enabled || auto.GroupID != "reticulum" {
		t.Errorf("Default Interface: %+v", auto)
	}

	hub, ok := cfg.Interfaces["Hub Node"]
	if !ok {
		t.Fatal("Hub Node missing")
	}
	if hub.Type != "TCPClientInterface" || !hub.Enabled {
		t.Errorf("Hub core fields: %+v", hub)
	}
	if hub.TargetHost != "hub.example.com" || hub.TargetPort != 4242 {
		t.Errorf("Hub target: got %s:%d", hub.TargetHost, hub.TargetPort)
	}
	if hub.KISSFraming || hub.I2PTunneled {
		t.Errorf("Hub framing flags should be false: kiss=%v i2p=%v", hub.KISSFraming, hub.I2PTunneled)
	}
}

// TestLoadConfig_SpamProtectionKnobs covers all per-interface ingress and
// announce-rate options.
func TestLoadConfig_SpamProtectionKnobs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	body := `[interfaces]
  [[noisy]]
    type = TCPClientInterface
    enabled = true
    announce_cap = 5.5
    announce_rate_target = 1800
    announce_rate_grace = 4
    announce_rate_penalty = 600
    ingress_control = no
    ic_new_time = 60
    ic_burst_freq_new = 1.25
    ic_burst_freq = 7
    ic_max_held_announces = 64
    ic_burst_hold = 30
    ic_burst_penalty = 90
    ic_held_release_interval = 15
`
	writeFile(t, path, body)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	iface, ok := cfg.Interfaces["noisy"]
	if !ok {
		t.Fatal("noisy interface missing")
	}

	checks := []struct {
		name string
		got  any
		want any
	}{
		{"AnnounceCap", iface.AnnounceCap, 5.5},
		{"AnnounceRateTarget", iface.AnnounceRateTarget, 1800.0},
		{"AnnounceRateGrace", iface.AnnounceRateGrace, 4},
		{"AnnounceRatePenalty", iface.AnnounceRatePenalty, 600.0},
		{"IngressControl", iface.IngressControl, false},
		{"IngressControlSet", iface.IngressControlSet, true},
		{"ICNewTime", iface.ICNewTime, 60},
		{"ICBurstFreqNew", iface.ICBurstFreqNew, 1.25},
		{"ICBurstFreq", iface.ICBurstFreq, 7.0},
		{"ICMaxHeldAnnounces", iface.ICMaxHeldAnnounces, 64},
		{"ICBurstHold", iface.ICBurstHold, 30},
		{"ICBurstPenalty", iface.ICBurstPenalty, 90},
		{"ICHeldReleaseInterval", iface.ICHeldReleaseInterval, 15},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, c.got, c.want)
		}
	}
}

// TestLoadConfig_AutoInterfaceDevices covers AutoInterface devices and
// ignored_devices list parsing.
func TestLoadConfig_AutoInterfaceDevices(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	body := `[interfaces]
  [[auto]]
    type = AutoInterface
    enabled = yes
    devices = eth0, eth1
    ignored_devices = wlan0, dummy0
`
	writeFile(t, path, body)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	iface, ok := cfg.Interfaces["auto"]
	if !ok {
		t.Fatal("auto interface missing")
	}
	wantDevices := []string{"eth0", "eth1"}
	wantIgnored := []string{"wlan0", "dummy0"}
	if len(iface.Devices) != len(wantDevices) || iface.Devices[0] != wantDevices[0] || iface.Devices[1] != wantDevices[1] {
		t.Errorf("Devices: got %v, want %v", iface.Devices, wantDevices)
	}
	if len(iface.IgnoredDevices) != len(wantIgnored) || iface.IgnoredDevices[0] != wantIgnored[0] || iface.IgnoredDevices[1] != wantIgnored[1] {
		t.Errorf("IgnoredDevices: got %v, want %v", iface.IgnoredDevices, wantIgnored)
	}
}

// TestLoadConfig_InlineComments verifies trailing comment markers do not leak
// into string values.
func TestLoadConfig_InlineComments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	body := "[reticulum]\n  loglevel = 3 # be quiet\n[interfaces]\n  [[Hub]]\n" +
		"    type = TCPClientInterface\n    enabled = yes\n" +
		"    target_host = example.com ; trailing\n    target_port = 4242\n"
	writeFile(t, path, body)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.LogLevel != 3 {
		t.Errorf("LogLevel: got %d, want 3", cfg.LogLevel)
	}
	hub, ok := cfg.Interfaces["Hub"]
	if !ok {
		t.Fatal("Hub interface missing")
	}
	if hub.TargetHost != "example.com" {
		t.Errorf("TargetHost: got %q, want example.com", hub.TargetHost)
	}
}

// TestLoadConfig_UTF8BOM ensures a BOM-prefixed file still parses.
func TestLoadConfig_UTF8BOM(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	writeFile(t, path, "\ufeff[reticulum]\n  loglevel = 2\n")

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.LogLevel != 2 {
		t.Errorf("LogLevel: got %d, want 2", cfg.LogLevel)
	}
}

// TestLoadConfig_MalformedSafe asserts that a file packed with garbage,
// invalid numbers, broken section headers, and unknown keys still loads
// without error and yields a usable, default-filled config plus the one
// well-formed interface entry.
func TestLoadConfig_MalformedSafe(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	body := strings.Join([]string{
		"=value-without-key",
		"key-without-equals",
		"[unclosed",
		"unopened]",
		"[[mismatch]",
		"[]",
		"[[]]",
		"[unknown_top_level]",
		"  some_random = thing",
		"[reticulum]",
		"  shared_instance_port = not-a-number",
		"  loglevel = also-bad",
		"  unknown_key = whatever",
		"  enable_transport = yes",
		"[interfaces]",
		"  [[Hub Node]]",
		"    type = TCPClientInterface",
		"    enabled = yes",
		"    target_host = hub.example.com",
		"    target_port = 4242",
		"    target_port = oops",
		"    bitrate = NaN",
		"    mtu = ",
		"    unknown_field = ignored",
	}, "\n") + "\n"
	writeFile(t, path, body)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig should not error on malformed input: %v", err)
	}

	if cfg.SharedInstancePort != DefaultSharedInstancePort {
		t.Errorf("SharedInstancePort should keep default after bad value: got %d", cfg.SharedInstancePort)
	}
	if cfg.LogLevel != DefaultLogLevel {
		t.Errorf("LogLevel should keep default after bad value: got %d", cfg.LogLevel)
	}
	if !cfg.EnableTransport {
		t.Error("valid keys must still apply alongside invalid ones")
	}

	for _, name := range []string{"unknown_top_level", "unclosed", "unopened", "mismatch", "reticulum", "logging", "interfaces"} {
		if _, ok := cfg.Interfaces[name]; ok {
			t.Errorf("malformed/reserved section %q must not produce an interface", name)
		}
	}

	hub, ok := cfg.Interfaces["Hub Node"]
	if !ok {
		t.Fatal("the well-formed Hub interface should still register")
	}
	if hub.Type != "TCPClientInterface" || !hub.Enabled {
		t.Errorf("Hub core fields after garbage neighbours: %+v", hub)
	}
	if hub.TargetPort != 4242 {
		t.Errorf("TargetPort should keep first valid value: got %d", hub.TargetPort)
	}
	if hub.Bitrate != 0 || hub.MTU != 0 {
		t.Errorf("invalid numeric fields should remain zero: bitrate=%d mtu=%d", hub.Bitrate, hub.MTU)
	}
}

// TestLoadConfig_EmptyFile parses an empty file and checks that all defaults
// survive intact.
func TestLoadConfig_EmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	writeFile(t, path, "")

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.LogLevel != DefaultLogLevel {
		t.Errorf("LogLevel: got %d, want %d", cfg.LogLevel, DefaultLogLevel)
	}
	if !cfg.EnableTransport {
		t.Error("EnableTransport should fall back to default")
	}
	if len(cfg.Interfaces) != 0 {
		t.Errorf("Interfaces should be empty, got %d", len(cfg.Interfaces))
	}
}

// TestLoadConfig_MissingFile returns the underlying os error so callers can
// distinguish "no config" from "broken config".
func TestLoadConfig_MissingFile(t *testing.T) {
	_, err := LoadConfig(filepath.Join(t.TempDir(), "missing"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !os.IsNotExist(err) {
		t.Errorf("expected os.IsNotExist error, got %v", err)
	}
}

// TestLoadConfig_OversizeLine surfaces oversize input as a parse error rather
// than crashing the scanner.
func TestLoadConfig_OversizeLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	huge := strings.Repeat("a", maxLineBytes+10)
	writeFile(t, path, "[reticulum]\n  enable_transport = "+huge+"\n")

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for oversize line")
	}
}

// TestSaveConfig_RoundTrip writes a representative config and reloads it,
// ensuring every persisted field returns intact.
func TestSaveConfig_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")

	cfg := DefaultConfig()
	cfg.ConfigPath = path
	cfg.LogLevel = 2
	cfg.EnableTransport = false
	cfg.Interfaces["Hub Node"] = &common.InterfaceConfig{
		Name:       "Hub Node",
		Type:       "TCPClientInterface",
		Enabled:    true,
		TargetHost: "hub.example.com",
		TargetPort: 4242,
	}

	if err := SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	for _, want := range []string{"[reticulum]", "[logging]", "[interfaces]", "[[Hub Node]]"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("expected output to contain %q, got:\n%s", want, raw)
		}
	}

	loaded, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if loaded.LogLevel != 2 {
		t.Errorf("LogLevel: got %d, want 2", loaded.LogLevel)
	}
	if loaded.EnableTransport {
		t.Error("EnableTransport should round-trip as false")
	}
	hub, ok := loaded.Interfaces["Hub Node"]
	if !ok {
		t.Fatal("Hub interface missing after round-trip")
	}
	if hub.TargetHost != "hub.example.com" || hub.TargetPort != 4242 || !hub.Enabled {
		t.Errorf("Hub round-trip mismatch: %+v", hub)
	}
	if !loaded.EnableSandbox {
		t.Error("EnableSandbox should round-trip as true (default)")
	}
}

func TestLoadConfig_IdentityBackend(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	writeFile(t, path, `[reticulum]
  identity_backend = secretservice
`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.IdentityBackend != "secretservice" {
		t.Fatalf("IdentityBackend=%q", cfg.IdentityBackend)
	}
}

func TestLoadConfig_DoSProtection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	writeFile(t, path, `[reticulum]
  dos_protection = auto
`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.DoSProtection != "auto" {
		t.Fatalf("DoSProtection = %q", cfg.DoSProtection)
	}

	cfg.DoSProtection = "prevent"
	cfg.ConfigPath = path
	if err := SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	loaded, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if loaded.DoSProtection != "prevent" {
		t.Fatalf("round trip DoSProtection = %q", loaded.DoSProtection)
	}
}

// TestLoadConfig_EnableSandbox verifies the parser recognises the
// enable_sandbox key and that both truthy and falsy values are handled.
func TestLoadConfig_EnableSandbox(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")

	writeFile(t, path, `[reticulum]
  enable_sandbox = no
`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.EnableSandbox {
		t.Error("enable_sandbox = no should set EnableSandbox to false")
	}

	writeFile(t, path, `[reticulum]
  enable_sandbox = yes
`)
	cfg, err = LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if !cfg.EnableSandbox {
		t.Error("enable_sandbox = yes should set EnableSandbox to true")
	}
}

// TestLoadConfig_EnableSeccomp verifies enable_seccomp parsing and default.
func TestLoadConfig_EnableSeccomp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")

	writeFile(t, path, `[reticulum]
`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if !cfg.EnableSeccomp {
		t.Error("omitted enable_seccomp should default to true")
	}

	writeFile(t, path, `[reticulum]
  enable_seccomp = no
`)
	cfg, err = LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.EnableSeccomp {
		t.Error("enable_seccomp = no should set EnableSeccomp to false")
	}
}

// TestSaveConfig_EnableSandboxRoundTrip writes enable_sandbox = no and
// reloads it to ensure the field persists.
func TestSaveConfig_EnableSandboxRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")

	cfg := DefaultConfig()
	cfg.ConfigPath = path
	cfg.EnableSandbox = false

	if err := SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	loaded, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if loaded.EnableSandbox {
		t.Error("EnableSandbox should round-trip as false")
	}
}

// TestSaveConfig_RequiresPath verifies SaveConfig refuses to write without a
// destination path.
func TestSaveConfig_RequiresPath(t *testing.T) {
	if err := SaveConfig(&common.ReticulumConfig{}); err == nil {
		t.Fatal("expected error when ConfigPath is empty")
	}
	if err := SaveConfig(nil); err == nil {
		t.Fatal("expected error when cfg is nil")
	}
}

// TestCreateDefaultConfig writes the starter file and checks the only
// shipped default interface (Auto Discovery) is present and enabled. No
// external TCP hubs are baked into defaults. Users add their own.

func TestCreateDefaultConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")

	if err := CreateDefaultConfig(path); err != nil {
		t.Fatalf("CreateDefaultConfig: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("config file not created: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	auto, ok := cfg.Interfaces["Auto Discovery"]
	if !ok {
		t.Fatal("default Auto Discovery interface missing")
	}
	if !auto.Enabled {
		t.Error("Auto Discovery should be enabled")
	}
	if auto.Type != "AutoInterface" {
		t.Errorf("Auto Discovery type: got %q, want AutoInterface", auto.Type)
	}

	if len(cfg.Interfaces) != 1 {
		t.Errorf("default config should ship exactly one interface (Auto Discovery), got %d: %v",
			len(cfg.Interfaces), cfg.Interfaces)
	}
}

// TestGetConfigPath only validates the helper produces a non-empty path.
func TestGetConfigPath(t *testing.T) {
	path, err := GetConfigPath()
	if err != nil {
		t.Fatalf("GetConfigPath: %v", err)
	}
	if path == "" {
		t.Error("GetConfigPath returned empty string")
	}
	if !strings.HasSuffix(path, filepath.Join(DefaultConfigDirName, DefaultConfigFileName)) {
		t.Errorf("unexpected suffix: %s", path)
	}
}

// TestEnsureConfigDir creates the configured directory. We accept the host

// home directory because the call is idempotent.
func TestEnsureConfigDir(t *testing.T) {
	if err := EnsureConfigDir(); err != nil {
		t.Errorf("EnsureConfigDir: %v", err)
	}
}

// TestLoadConfigBackboneRemoteAlias maps community-style remote to target_host.
func TestLoadConfigBackboneRemoteAlias(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	body := `[interfaces]
  [[MichMesh]]
    type = BackboneInterface
    enabled = yes
    remote = michmesh.example
    target_port = 4242
`
	writeFile(t, path, body)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	iface := cfg.Interfaces["MichMesh"]
	if iface == nil {
		t.Fatal("MichMesh interface missing")
	}
	if iface.TargetHost != "michmesh.example" || iface.TargetPort != 4242 {
		t.Fatalf("target = %s:%d", iface.TargetHost, iface.TargetPort)
	}
}

// TestLoadConfigUDPForwardAliases maps Python UDP forward_* to target_*.
func TestLoadConfigUDPForwardAliases(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	body := `[interfaces]
  [[udp]]
    type = UDPInterface
    enabled = yes
    listen_ip = 127.0.0.1
    listen_port = 4242
    forward_ip = 127.0.0.1
    forward_port = 4243
`
	writeFile(t, path, body)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	iface := cfg.Interfaces["udp"]
	if iface == nil {
		t.Fatal("udp interface missing")
	}
	if iface.TargetHost != "127.0.0.1" || iface.TargetPort != 4243 {
		t.Fatalf("forward aliases: got %s:%d", iface.TargetHost, iface.TargetPort)
	}
	if iface.Address != "127.0.0.1" || iface.Port != 4242 {
		t.Fatalf("listen: got %s:%d", iface.Address, iface.Port)
	}
}

func TestLoadConfig_BackboneIO(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	writeFile(t, path, `[reticulum]
  backbone_io = epoll
`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.BackboneIO != "epoll" {
		t.Fatalf("BackboneIO = %q, want epoll", cfg.BackboneIO)
	}

	writeFile(t, path, `[reticulum]
  io_backend = kqueue
`)
	cfg, err = LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig io_backend: %v", err)
	}
	if cfg.BackboneIO != "kqueue" {
		t.Fatalf("BackboneIO = %q, want kqueue", cfg.BackboneIO)
	}
}

func TestLoadConfig_RNS136Options(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	writeFile(t, path, `[reticulum]
  enable_transport = no
  static_transport_identity = yes
  local_hops_delta = yes

[interfaces]
  [[mesh]]
    type = UDPInterface
    enabled = yes
    mode = internal
    recursive_prs = yes
    announces_from_internal = no
`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.EnableTransport {
		t.Error("EnableTransport should be false")
	}
	if !cfg.StaticTransportIdentity {
		t.Error("StaticTransportIdentity should be true")
	}
	if !cfg.LocalHopsDelta {
		t.Error("LocalHopsDelta should be true")
	}
	iface := cfg.Interfaces["mesh"]
	if iface == nil {
		t.Fatal("mesh interface missing")
	}
	if iface.Mode != "internal" {
		t.Errorf("Mode = %q, want internal", iface.Mode)
	}
	if !iface.RecursivePRs {
		t.Error("RecursivePRs should be true")
	}
	if !iface.AnnouncesFromInternalSet || iface.AnnouncesFromInternal {
		t.Errorf("AnnouncesFromInternal want false with set=true, got set=%v val=%v",
			iface.AnnouncesFromInternalSet, iface.AnnouncesFromInternal)
	}
}

func TestLoadConfigQUICKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	body := `[interfaces]
  [[QUIC Hub]]
    type = QUICServerInterface
    enabled = yes
    listen_ip = 0.0.0.0
    listen_port = 4242
    cert_file = /tmp/cert.pem
    key_file = /tmp/key.pem
    peer_key = aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899

  [[QUIC Uplink]]
    type = QUICClientInterface
    enabled = yes
    target_host = hub.example.com
    target_port = 4242
    sni = hub.example.com
    peer_key = aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899
`
	writeFile(t, path, body)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	srv := cfg.Interfaces["QUIC Hub"]
	if srv == nil || srv.Type != "QUICServerInterface" {
		t.Fatalf("server: %+v", srv)
	}
	if srv.Address != "0.0.0.0" || srv.Port != 4242 {
		t.Fatalf("listen %s:%d", srv.Address, srv.Port)
	}
	if srv.CertFile != "/tmp/cert.pem" || srv.KeyFile != "/tmp/key.pem" {
		t.Fatalf("cert paths %+v", srv)
	}
	cli := cfg.Interfaces["QUIC Uplink"]
	if cli == nil || cli.Type != "QUICClientInterface" {
		t.Fatalf("client: %+v", cli)
	}
	if cli.SNI != "hub.example.com" || cli.PeerKey == "" {
		t.Fatalf("client tls %+v", cli)
	}
	out := filepath.Join(t.TempDir(), "out")
	cfg.ConfigPath = out
	if err := SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, want := range []string{"cert_file", "key_file", "peer_key", "sni", "QUICClientInterface"} {
		if !strings.Contains(s, want) {
			t.Fatalf("SaveConfig missing %q in:\n%s", want, s)
		}
	}
}

// writeFile is a tiny test helper that writes content with a strict mode and
// fails the test on IO errors.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestApplyInterfaceOptionI2P(t *testing.T) {
	cfg := &common.InterfaceConfig{}
	applyInterfaceOption(cfg, "peers", "a.b32.i2p, b.b32.i2p")
	applyInterfaceOption(cfg, "connectable", "true")
	applyInterfaceOption(cfg, "sam_address", "127.0.0.1:7656")
	if len(cfg.I2PPeers) != 2 || cfg.I2PPeers[0] != "a.b32.i2p" {
		t.Fatalf("peers: %#v", cfg.I2PPeers)
	}
	if !cfg.I2PConnectable {
		t.Fatal("expected connectable")
	}
	if cfg.I2PSAMAddress != "127.0.0.1:7656" {
		t.Fatalf("sam_address: %q", cfg.I2PSAMAddress)
	}
}

func TestLoadConfigIFACAndSharedInstance(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	body := `
[reticulum]
  share_instance = yes
  shared_instance_port = 37428
  instance_control_port = 37429
  shared_instance_type = tcp
  instance_name = node1

[interfaces]

  [[Test UDP]]
    type = UDPInterface
    enabled = yes
    network_name = mynet
    passphrase = secret
    ifac_size = 128
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if !cfg.ShareInstance {
		t.Fatal("expected share_instance")
	}
	if cfg.SharedInstanceType != "tcp" {
		t.Fatalf("shared_instance_type = %q", cfg.SharedInstanceType)
	}
	if cfg.InstanceName != "node1" {
		t.Fatalf("instance_name = %q", cfg.InstanceName)
	}
	ic := cfg.Interfaces["Test UDP"]
	if ic == nil {
		t.Fatal("missing interface")
	}
	if ic.NetworkName != "mynet" || ic.Passphrase != "secret" {
		t.Fatalf("network creds: name=%q pass=%q", ic.NetworkName, ic.Passphrase)
	}
	if ic.IFACSize != 16 {
		t.Fatalf("IFACSize = %d, want 16", ic.IFACSize)
	}
}

func TestSetIFACSizeParser(t *testing.T) {
	var size int
	setIFACSize("128", &size)
	if size != 16 {
		t.Fatalf("128 bits => %d bytes, want 16", size)
	}
	size = 0
	setIFACSize("8", &size)
	if size != 1 {
		t.Fatalf("8 bits => %d bytes, want 1", size)
	}
}
