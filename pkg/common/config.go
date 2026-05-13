// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package common

import (
	"fmt"
)

type ConfigProvider interface {
	GetConfigPath() string
	GetLogLevel() int
	GetInterfaces() map[string]InterfaceConfig
}

// InterfaceConfig is per-interface settings (announce_* / ic_* match reference Reticulum).
type InterfaceConfig struct {
	Name              string
	Type              string
	Enabled           bool
	Address           string
	Port              int
	TargetHost        string
	TargetPort        int
	TargetAddress     string
	Interface         string
	KISSFraming       bool
	I2PTunneled       bool
	PreferIPv6        bool
	MaxReconnTries    int
	Bitrate           int64
	MTU               int
	GroupID           string
	DiscoveryScope    string
	DiscoveryPort     int
	DataPort          int
	MulticastAddrType string
	Devices           []string
	IgnoredDevices    []string

	AnnounceCap           float64 // % of bitrate; 0 => default 2%
	AnnounceRateTarget    float64 // min seconds between same-dest rebroadcasts; 0 => off
	AnnounceRateGrace     int
	AnnounceRatePenalty   float64
	IngressControl        bool
	IngressControlSet     bool // false => use default (ingress on)
	ICNewTime             int
	ICBurstFreqNew        float64
	ICBurstFreq           float64
	ICMaxHeldAnnounces    int
	ICBurstHold           int
	ICBurstPenalty        int
	ICHeldReleaseInterval int

	// Path-request burst control
	ICPRBurstFreqNew float64
	ICPRBurstFreq    float64
	ECPRFreq         float64
	EgressControl    bool
	EgressControlSet bool // false => use default (egress off)
}

// ReticulumConfig represents the main configuration structure
type ReticulumConfig struct {
	ConfigPath          string
	EnableTransport     bool
	ShareInstance       bool
	SharedInstancePort  int
	InstanceControlPort int
	PanicOnInterfaceErr bool
	LogLevel            int
	Interfaces          map[string]*InterfaceConfig
	AppName             string
	AppAspect           string
	EnableSandbox       bool
}

// NewReticulumConfig creates a new ReticulumConfig with default values
func NewReticulumConfig() *ReticulumConfig {
	return &ReticulumConfig{
		EnableTransport:     true,
		ShareInstance:       false,
		SharedInstancePort:  DefaultSharedInstancePort,
		InstanceControlPort: DefaultInstanceControlPort,
		PanicOnInterfaceErr: false,
		LogLevel:            DefaultLogLevel,
		Interfaces:          make(map[string]*InterfaceConfig),
	}
}

// Validate checks if the configuration is valid
func (c *ReticulumConfig) Validate() error {
	if c.SharedInstancePort < MinPort || c.SharedInstancePort > MaxPort {
		return fmt.Errorf("invalid shared instance port: %d", c.SharedInstancePort)
	}
	if c.InstanceControlPort < MinPort || c.InstanceControlPort > MaxPort {
		return fmt.Errorf("invalid instance control port: %d", c.InstanceControlPort)
	}
	return nil
}

func DefaultConfig() *ReticulumConfig {
	return &ReticulumConfig{
		EnableTransport:     true,
		ShareInstance:       false,
		SharedInstancePort:  DefaultSharedInstancePort,
		InstanceControlPort: DefaultInstanceControlPort,
		PanicOnInterfaceErr: false,
		LogLevel:            DefaultLogLevel,
		Interfaces:          make(map[string]*InterfaceConfig),
		AppName:             "Go Client",
		AppAspect:           "node",
		EnableSandbox:       true,
	}
}
