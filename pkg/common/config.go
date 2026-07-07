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
	I2PPeers          []string
	I2PConnectable    bool
	I2PSAMAddress     string
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

	AnnounceCap           float64 // % of bitrate. 0 => default 2%
	AnnounceRateTarget    float64 // min seconds between same-dest rebroadcasts. 0 => off
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

	NetworkName string
	Passphrase  string
	IFACSize    int // bytes. Config ifac_size is stored in bits and converted at parse time
	IFACNetname string
	IFACNetkey  string
	PublishIFAC bool

	// PipeInterface subprocess command and respawn delay (seconds).
	Command      string
	RespawnDelay int

	// LocalInterface unix socket settings (interface block).
	SharedInstanceType string
	InstanceName       string
}

// SharedInstanceType values for [reticulum] shared_instance_type.
const (
	SharedInstanceTCP  = "tcp"
	SharedInstanceUnix = "unix"
)

// ReticulumConfig represents the main configuration structure
type ReticulumConfig struct {
	ConfigPath          string
	EnableTransport     bool
	ShareInstance       bool
	SharedInstancePort  int
	InstanceControlPort int
	SharedInstanceType  string
	InstanceName        string
	RPCKey              []byte
	PanicOnInterfaceErr bool
	LogLevel            int
	Interfaces          map[string]*InterfaceConfig
	AppName             string
	AppAspect           string
	EnableSandbox       bool

	// EnableControlAPI turns on the localhost JSON control API (pkg/controlapi)
	// that lets non-Go applications use destinations, links, and announces
	// without embedding the Reticulum stack.
	EnableControlAPI bool
	ControlAPIHost   string
	ControlAPIPort   int

	// ConnectedToSharedInstance is set at runtime when this process attaches
	// to an existing local shared instance instead of owning one.
	ConnectedToSharedInstance bool

	// InMemoryPathTable disables on-disk path table persistence when true.
	InMemoryPathTable bool

	// InMemoryKnownDestinations disables on-disk known destination persistence when true.
	InMemoryKnownDestinations bool

	// BackboneIO selects the kernel I/O multiplexer for backbone and local shared
	// instance sockets: auto, epoll, kqueue, io_uring, or go.
	BackboneIO string

	// DiscoverInterfaces enables periodic NIC rescan for AutoInterface peers.
	DiscoverInterfaces bool

	// WatchInterfaces enables periodic NIC monitoring via net.Interfaces where supported.
	WatchInterfaces bool
}

// NewReticulumConfig creates a new ReticulumConfig with default values
func NewReticulumConfig() *ReticulumConfig {
	return &ReticulumConfig{
		EnableTransport:     true,
		ShareInstance:       true,
		SharedInstancePort:  DefaultSharedInstancePort,
		InstanceControlPort: DefaultInstanceControlPort,
		SharedInstanceType:  SharedInstanceTCP,
		PanicOnInterfaceErr: false,
		LogLevel:            DefaultLogLevel,
		Interfaces:          make(map[string]*InterfaceConfig),
		ControlAPIHost:      DefaultControlAPIHost,
		ControlAPIPort:      DefaultControlAPIPort,
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
	if c.EnableControlAPI {
		if c.ControlAPIPort < MinPort || c.ControlAPIPort > MaxPort {
			return fmt.Errorf("invalid control api port: %d", c.ControlAPIPort)
		}
		if len(c.RPCKey) == 0 {
			return fmt.Errorf("control api requires rpc_key to be set")
		}
	}
	return nil
}

func DefaultConfig() *ReticulumConfig {
	return &ReticulumConfig{
		EnableTransport:     true,
		ShareInstance:       true,
		SharedInstancePort:  DefaultSharedInstancePort,
		InstanceControlPort: DefaultInstanceControlPort,
		SharedInstanceType:  SharedInstanceTCP,
		PanicOnInterfaceErr: false,
		LogLevel:            DefaultLogLevel,
		Interfaces:          make(map[string]*InterfaceConfig),
		AppName:             "Go Client",
		AppAspect:           "node",
		EnableSandbox:       true,
		ControlAPIHost:      DefaultControlAPIHost,
		ControlAPIPort:      DefaultControlAPIPort,
	}
}
