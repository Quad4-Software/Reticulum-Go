// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsutil

import (
	"fmt"
	"os"
	"path/filepath"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/cryptography"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/reticulumconfig"
)

// LoadConfigDir loads {dir}/config, or the default ~/.reticulum-go/config when
// dir is empty.
func LoadConfigDir(dir string) (*common.ReticulumConfig, error) {
	if dir == "" {
		return reticulumconfig.InitConfig()
	}
	path := filepath.Join(dir, reticulumconfig.DefaultConfigFileName)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("config not found: %s", path)
	}
	return reticulumconfig.LoadConfig(path)
}

// StorageDir returns the storage directory beside the config file.
func StorageDir(cfg *common.ReticulumConfig) string {
	if cfg == nil || cfg.ConfigPath == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(cfg.ConfigPath), "storage")
}

// ResolveAuthKey returns cfg.RPCKey when set, otherwise the SHA-256 of the
// transport identity private key (shared-instance authkey).
func ResolveAuthKey(cfg *common.ReticulumConfig) ([]byte, error) {
	if cfg != nil && len(cfg.RPCKey) > 0 {
		return append([]byte(nil), cfg.RPCKey...), nil
	}
	storage := StorageDir(cfg)
	if storage == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		storage = filepath.Join(home, reticulumconfig.DefaultConfigDirName, "storage")
	}
	ident, err := identity.FromFile(filepath.Join(storage, "transport_identity"))
	if err != nil {
		return nil, fmt.Errorf("load transport identity for rpc auth: %w", err)
	}
	priv, err := ident.GetPrivateKey()
	if err != nil {
		return nil, err
	}
	return cryptography.Hash(priv), nil
}
