// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
)

// ExternalFactory builds a custom interface for an unknown config type.
// Embedders register factories for in-process drivers. Operators can also
// drop manifests or executables under {config_dir}/interfaces/.
type ExternalFactory func(name string, cfg *common.InterfaceConfig, ctx *FromConfigContext) (Interface, error)

var (
	externalFactoriesMu sync.RWMutex
	externalFactories   = map[string]ExternalFactory{}
)

// RegisterExternalFactory registers an in-process builder for typeName.
// Replaces any prior registration for the same name.
func RegisterExternalFactory(typeName string, factory ExternalFactory) {
	if typeName == "" || factory == nil {
		return
	}
	externalFactoriesMu.Lock()
	externalFactories[typeName] = factory
	externalFactoriesMu.Unlock()
}

// UnregisterExternalFactory removes a previously registered factory.
func UnregisterExternalFactory(typeName string) {
	externalFactoriesMu.Lock()
	delete(externalFactories, typeName)
	externalFactoriesMu.Unlock()
}

type pluginManifest struct {
	Driver       string `json:"driver"`
	Command      string `json:"command"`
	RespawnDelay int    `json:"respawn_delay"`
}

// loadExternalInterface resolves unknown types via registered factories,
// JSON manifests, or executable pipe drivers under interfaces/.
func loadExternalInterface(name string, cfg *common.InterfaceConfig, ctx *FromConfigContext) (Interface, error) {
	if cfg == nil || cfg.Type == "" {
		return nil, errors.New("nil interface config")
	}
	typeName := cfg.Type

	externalFactoriesMu.RLock()
	factory := externalFactories[typeName]
	externalFactoriesMu.RUnlock()
	if factory != nil {
		debug.Log(debug.DebugInfo, "Loading external interface from registered factory", "type", typeName, "name", name)
		return factory(name, cfg, ctx)
	}

	dir := interfacesPluginDir(ctx)
	if dir == "" {
		return nil, fmt.Errorf("unsupported interface type %q (no interfaces plugin directory)", typeName)
	}
	if err := validatePluginTypeName(typeName); err != nil {
		return nil, err
	}

	for _, base := range []string{
		filepath.Join(dir, typeName+".json"),
		filepath.Join(dir, typeName+".manifest"),
	} {
		if !pluginPathContained(dir, base) {
			return nil, fmt.Errorf("invalid plugin type %q", typeName)
		}
		iface, err := loadManifestInterface(name, cfg, ctx, base)
		if err == nil {
			debug.Log(debug.DebugInfo, "Loading external interface from manifest", "path", base, "name", name)
			return iface, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("external interface %q: %w", typeName, err)
		}
	}

	execPath := filepath.Join(dir, typeName)
	if !pluginPathContained(dir, execPath) {
		return nil, fmt.Errorf("invalid plugin type %q", typeName)
	}
	if st, err := os.Stat(execPath); err == nil && !st.IsDir() && isExecutable(st) {
		debug.Log(debug.DebugInfo, "Loading external interface from executable", "path", execPath, "name", name)
		return newPipeFromPlugin(name, cfg, ctx, execPath, 0)
	}

	return nil, fmt.Errorf("unsupported interface type %q (no factory, manifest, or executable in %s)", typeName, dir)
}

func validatePluginTypeName(typeName string) error {
	if typeName == "" || filepath.IsAbs(typeName) || typeName != filepath.Base(typeName) {
		return fmt.Errorf("invalid plugin type %q", typeName)
	}
	return nil
}

func pluginPathContained(dir, path string) bool {
	cleanDir := filepath.Clean(dir)
	cleanPath := filepath.Clean(path)
	rel, err := filepath.Rel(cleanDir, cleanPath)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func interfacesPluginDir(ctx *FromConfigContext) string {
	if ctx != nil && ctx.ConfigDir != "" {
		return filepath.Join(ctx.ConfigDir, "interfaces")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".reticulum-go", "interfaces")
}

func loadManifestInterface(name string, cfg *common.InterfaceConfig, ctx *FromConfigContext, path string) (Interface, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path under plugin dir after validatePluginTypeName and pluginPathContained
	if err != nil {
		return nil, err
	}
	var man pluginManifest
	if err := json.Unmarshal(data, &man); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	driver := man.Driver
	if driver == "" {
		driver = "pipe"
	}
	switch driver {
	case "pipe":
		cmd := man.Command
		if cmd == "" {
			return nil, fmt.Errorf("%s: pipe driver requires command", path)
		}
		return newPipeFromPlugin(name, cfg, ctx, cmd, man.RespawnDelay)
	default:
		return nil, fmt.Errorf("%s: unknown driver %q", path, driver)
	}
}

func newPipeFromPlugin(name string, cfg *common.InterfaceConfig, ctx *FromConfigContext, command string, respawnDelaySec int) (Interface, error) {
	delaySec := respawnDelaySec
	if cfg != nil && cfg.RespawnDelay > 0 {
		delaySec = cfg.RespawnDelay
	}
	if delaySec <= 0 {
		delaySec = 5
	}
	if cfg != nil && cfg.Command != "" {
		command = cfg.Command
	}
	panicOnErr := ctx != nil && ctx.PanicOnInterfaceError
	enabled := true
	if cfg != nil {
		enabled = cfg.Enabled
	}
	return NewPipeInterface(name, command, enabled, time.Duration(delaySec)*time.Second, panicOnErr)
}

func isExecutable(st os.FileInfo) bool {
	mode := st.Mode()
	return mode&0o111 != 0
}
