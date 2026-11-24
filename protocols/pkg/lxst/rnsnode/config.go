// SPDX-License-Identifier: LicenseRef-Reticulum
package rnsnode

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/reticulumconfig"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxst/sandbox"
)

const (
	maxConfigBytes = 1 << 20
	scanBufInit    = 64 * 1024
	scanBufMax     = 1024 * 1024
)

// ConfigFilePath returns {dir}/config, using DefaultConfigDir when dir is empty.
func ConfigFilePath(dir string) (string, error) {
	if dir == "" {
		dir = DefaultConfigDir()
		if dir == "" {
			return "", fmt.Errorf("no home directory")
		}
	}
	return filepath.Join(dir, "config"), nil
}

// EnsureDefaultConfig writes a reticulum-go starter config at {dir}/config when
// the file is missing. Returns true when a new file was created.
func EnsureDefaultConfig(dir string) (bool, error) {
	path, err := ConfigFilePath(dir)
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(path); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}
	if err := reticulumconfig.CreateDefaultConfig(path); err != nil {
		return false, err
	}
	_ = os.Chmod(path, 0o600)
	return true, nil
}

// LoadOfficialDir loads {dir}/config with reticulum-go's official parser.
func LoadOfficialDir(dir string) (*common.ReticulumConfig, error) {
	path, err := ConfigFilePath(dir)
	if err != nil {
		return nil, err
	}
	return reticulumconfig.LoadConfig(path)
}

// LoadReticulumDir reads {dir}/config into a ReticulumConfig.
func LoadReticulumDir(dir string) (*common.ReticulumConfig, error) {
	path, err := ConfigFilePath(dir)
	if err != nil {
		return nil, err
	}
	return LoadReticulumFile(path)
}

// LoadReticulumDirLenient reads {dir}/config and ignores sandbox control keys.
func LoadReticulumDirLenient(dir string) (*common.ReticulumConfig, error) {
	path, err := ConfigFilePath(dir)
	if err != nil {
		return nil, err
	}
	return loadReticulumFile(path, true)
}

func LoadReticulumFile(path string) (*common.ReticulumConfig, error) {
	return loadReticulumFile(path, false)
}

func loadReticulumFile(path string, lenient bool) (*common.ReticulumConfig, error) {
	if strings.ContainsRune(path, 0) {
		return nil, fmt.Errorf("invalid path")
	}
	f, err := os.Open(path) // #nosec G304 -- operator-supplied local reticulum config after NUL check
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	if st, err := f.Stat(); err == nil && st.Size() > maxConfigBytes {
		return nil, fmt.Errorf("%s: config larger than 1MB", path)
	}
	cfg := common.DefaultConfig()
	cfg.ConfigPath = path
	cfg.Interfaces = map[string]*common.InterfaceConfig{}
	section := ""
	ifaceName := ""
	var iface *common.InterfaceConfig
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, scanBufInit), scanBufMax)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if err := parseReticulumLine(cfg, &section, &ifaceName, &iface, line, lenient); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, lineNo, err)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func parseReticulumLine(cfg *common.ReticulumConfig, section, ifaceName *string, iface **common.InterfaceConfig, line string, lenient bool) error {
	if strings.HasPrefix(line, "[[") && strings.HasSuffix(line, "]]") {
		*ifaceName = strings.TrimSpace(line[2 : len(line)-2])
		*iface = &common.InterfaceConfig{Name: *ifaceName, Enabled: true}
		cfg.Interfaces[*ifaceName] = *iface
		*section = "interface"
		return nil
	}
	if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
		*section = strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
		if sandbox.ForbiddenConfigKey(*section) {
			if lenient {
				return nil
			}
			return fmt.Errorf("%s cannot configure or disable the sandbox", *section)
		}
		*iface = nil
		*ifaceName = ""
		return nil
	}
	key, val, ok := strings.Cut(line, "=")
	if !ok {
		return fmt.Errorf("expected key = value")
	}
	key = strings.ToLower(strings.TrimSpace(key))
	val = strings.TrimSpace(val)
	if sandbox.ForbiddenConfigKey(key) {
		if lenient {
			return nil
		}
		return fmt.Errorf("%s cannot configure or disable the sandbox", key)
	}
	if *section == "interface" && *iface != nil {
		applyIfaceKey(*iface, key, val)
		return nil
	}
	if *section == "reticulum" {
		applyReticulumKey(cfg, key, val)
	}
	return nil
}

func applyReticulumKey(cfg *common.ReticulumConfig, key, val string) {
	switch key {
	case "enable_transport":
		cfg.EnableTransport = parseYes(val)
	case "share_instance":
		cfg.ShareInstance = parseYes(val)
	case "shared_instance_port":
		if n, err := strconv.Atoi(val); err == nil {
			cfg.SharedInstancePort = n
		}
	case "instance_control_port":
		if n, err := strconv.Atoi(val); err == nil {
			cfg.InstanceControlPort = n
		}
	case "instance_name":
		cfg.InstanceName = val
	case "in_memory_path_table":
		cfg.InMemoryPathTable = parseYes(val)
	case "in_memory_known_destinations":
		cfg.InMemoryKnownDestinations = parseYes(val)
	case "panic_on_interface_error":
		cfg.PanicOnInterfaceErr = parseYes(val)
	}
}

func applyIfaceKey(ic *common.InterfaceConfig, key, val string) {
	switch key {
	case "type":
		ic.Type = val
	case "enabled", "interface_enabled":
		ic.Enabled = parseYes(val)
	case "listen_ip", "listenip", "address":
		ic.Address = val
	case "listen_port", "port":
		if n, err := strconv.Atoi(val); err == nil {
			ic.Port = n
		} else {
			ic.Device = val
		}
	case "target_host", "targethost", "forward_ip":
		ic.TargetHost = val
	case "target_port", "targetport", "forward_port":
		if n, err := strconv.Atoi(val); err == nil {
			ic.TargetPort = n
		}
	case "target", "target_address":
		ic.TargetAddress = val
	case "device":
		ic.Device = val
	case "devices":
		ic.Devices = splitList(val)
	case "interface":
		ic.Interface = val
	case "kiss_framing":
		ic.KISSFraming = parseYes(val)
	case "group_id":
		ic.GroupID = val
	case "discovery_scope":
		ic.DiscoveryScope = val
	case "discovery_port":
		if n, err := strconv.Atoi(val); err == nil {
			ic.DiscoveryPort = n
		}
	case "data_port":
		if n, err := strconv.Atoi(val); err == nil {
			ic.DataPort = n
		}
	}
}

func parseYes(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "yes", "true", "on", "1":
		return true
	default:
		return false
	}
}

func splitList(v string) []string {
	parts := strings.FieldsFunc(v, func(r rune) bool {
		return r == ',' || r == ' '
	})
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
