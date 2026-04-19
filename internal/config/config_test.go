package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.quad4.io/Networks/Reticulum-Go/pkg/common"
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
}

// TestParseBool covers every truthy and falsy spelling accepted by the parser.
func TestParseBool(t *testing.T) {
	truthy := []string{"true", "True", "TRUE", "yes", "Yes", "YES", "y", "Y", "on", "ON", "1", "  yes  "}
	falsy := []string{"false", "no", "n", "off", "0", "", "maybe", "2", "garbage"}

	for _, v := range truthy {
		if !parseBool(v) {
			t.Errorf("parseBool(%q) = false, want true", v)
		}
	}
	for _, v := range falsy {
		if parseBool(v) {
			t.Errorf("parseBool(%q) = true, want false", v)
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
		"value":               "value",
		"value # comment":     "value",
		"value\t; comment":    "value",
		"value#nocomment":     "value#nocomment",
		"abc;def":             "abc;def",
		"only a value":        "only a value",
		"trailing  ":          "trailing  ",
		"77.37.146.243 # ip ": "77.37.146.243",
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

  [[Catz Node (TCP)]]
    type = TCPClientInterface
    enabled = yes
    target_host = 77.37.146.243
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

	catz, ok := cfg.Interfaces["Catz Node (TCP)"]
	if !ok {
		t.Fatal("Catz Node (TCP) missing")
	}
	if catz.Type != "TCPClientInterface" || !catz.Enabled {
		t.Errorf("Catz core fields: %+v", catz)
	}
	if catz.TargetHost != "77.37.146.243" || catz.TargetPort != 4242 {
		t.Errorf("Catz target: got %s:%d", catz.TargetHost, catz.TargetPort)
	}
	if catz.KISSFraming || catz.I2PTunneled {
		t.Errorf("Catz framing flags should be false: kiss=%v i2p=%v", catz.KISSFraming, catz.I2PTunneled)
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
		"  [[Catz Node (TCP)]]",
		"    type = TCPClientInterface",
		"    enabled = yes",
		"    target_host = 77.37.146.243",
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

	catz, ok := cfg.Interfaces["Catz Node (TCP)"]
	if !ok {
		t.Fatal("the well-formed Catz interface should still register")
	}
	if catz.Type != "TCPClientInterface" || !catz.Enabled {
		t.Errorf("Catz core fields after garbage neighbours: %+v", catz)
	}
	if catz.TargetPort != 4242 {
		t.Errorf("TargetPort should keep first valid value: got %d", catz.TargetPort)
	}
	if catz.Bitrate != 0 || catz.MTU != 0 {
		t.Errorf("invalid numeric fields should remain zero: bitrate=%d mtu=%d", catz.Bitrate, catz.MTU)
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
	cfg.Interfaces["Catz Node (TCP)"] = &common.InterfaceConfig{
		Name:       "Catz Node (TCP)",
		Type:       "TCPClientInterface",
		Enabled:    true,
		TargetHost: "77.37.146.243",
		TargetPort: 4242,
	}

	if err := SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	for _, want := range []string{"[reticulum]", "[logging]", "[interfaces]", "[[Catz Node (TCP)]]"} {
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
	catz, ok := loaded.Interfaces["Catz Node (TCP)"]
	if !ok {
		t.Fatal("Catz interface missing after round-trip")
	}
	if catz.TargetHost != "77.37.146.243" || catz.TargetPort != 4242 || !catz.Enabled {
		t.Errorf("Catz round-trip mismatch: %+v", catz)
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

// TestCreateDefaultConfig writes the starter file and checks the resulting
// interface set covers the defaults the binaries depend on.
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

	for _, name := range []string{"Auto Discovery", "Beleth RNS Hub", "Catz Node (TCP)"} {
		iface, ok := cfg.Interfaces[name]
		if !ok {
			t.Errorf("default interface %q missing", name)
			continue
		}
		if !iface.Enabled {
			t.Errorf("default interface %q should be enabled", name)
		}
	}

	catz := cfg.Interfaces["Catz Node (TCP)"]
	if catz == nil || catz.TargetHost != "77.37.146.243" || catz.TargetPort != 4242 {
		t.Errorf("Catz default mismatch: %+v", catz)
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

// TestEnsureConfigDir creates the configured directory; we accept the host
// home directory because the call is idempotent.
func TestEnsureConfigDir(t *testing.T) {
	if err := EnsureConfigDir(); err != nil {
		t.Errorf("EnsureConfigDir: %v", err)
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
