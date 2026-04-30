// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package config

import (
	"git.quad4.io/Networks/Reticulum-Go/pkg/common"
	"git.quad4.io/Networks/Reticulum-Go/pkg/reticulumconfig"
)

const (
	DefaultSharedInstancePort  = reticulumconfig.DefaultSharedInstancePort
	DefaultInstanceControlPort = reticulumconfig.DefaultInstanceControlPort
	DefaultLogLevel            = reticulumconfig.DefaultLogLevel
	DefaultConfigDirName       = reticulumconfig.DefaultConfigDirName
	DefaultConfigFileName      = reticulumconfig.DefaultConfigFileName
)

// DefaultConfig returns a ReticulumConfig populated with built-in defaults.
func DefaultConfig() *common.ReticulumConfig { return reticulumconfig.DefaultConfig() }

// GetConfigPath returns ~/.reticulum-go/config.
func GetConfigPath() (string, error) { return reticulumconfig.GetConfigPath() }

// EnsureConfigDir creates ~/.reticulum-go with restrictive permissions if it
// does not already exist.
func EnsureConfigDir() error { return reticulumconfig.EnsureConfigDir() }

// LoadConfig parses the configuration file at path.
func LoadConfig(path string) (*common.ReticulumConfig, error) {
	return reticulumconfig.LoadConfig(path)
}

// SaveConfig writes cfg to cfg.ConfigPath.
func SaveConfig(cfg *common.ReticulumConfig) error { return reticulumconfig.SaveConfig(cfg) }

// CreateDefaultConfig writes a starter configuration file at path containing
// the built-in interface defaults.
func CreateDefaultConfig(path string) error { return reticulumconfig.CreateDefaultConfig(path) }

// InitConfig loads ~/.reticulum-go/config, creating the file with default
// contents when it is missing.
func InitConfig() (*common.ReticulumConfig, error) { return reticulumconfig.InitConfig() }
