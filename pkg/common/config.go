// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package common

type ConfigProvider interface {
	GetConfigPath() string
	GetLogLevel() int
	GetInterfaces() map[string]InterfaceConfig
}

// InterfaceConfig is per-interface settings (announce_* / ic_* and related keys).
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

	// SerialInterface device settings. Device holds the TTY path. Python uses
	// port=/dev/ttyUSB0 which Go also accepts when port is non-numeric.
	Device            string
	Speed             int
	DataBits          int
	Parity            string
	StopBits          int
	RTSCTS            bool
	DSRDTR            bool
	XONXOFF           bool
	SerialFrameIdleMs int

	// LocalInterface unix socket settings (interface block).
	SharedInstanceType string
	InstanceName       string

	// QUIC / WebTransport TLS settings.
	CertFile string
	KeyFile  string
	PeerKey  string
	SNI      string

	// Path is the WebTransport URL path (default /rns).
	Path string

	// TransportMode selects WebTransport datagram, stream, or dual carriage.
	TransportMode string

	// Domain is the DNS name for DNSRendezvousInterface TXT lookups.
	Domain string

	// ResolveIntervalSec is how often DNSRendezvous re-queries (default 60).
	ResolveIntervalSec int

	// ContextID is the AF_VSOCK peer context ID (CID). 1 is local/host on Linux.
	ContextID int

	// LongPollSec is HTTPS long-poll timeout seconds (default 25).
	LongPollSec int

	// Mode is the interface operational mode (full, gateway, internal, ...).
	// Empty means full.
	Mode string

	// RecursivePRs enables path discovery for unknown destinations on this
	// interface.
	RecursivePRs bool

	// AnnouncesFromInternal controls whether announces learned via an
	// internal-mode next hop are rebroadcast. Default true when unset.
	AnnouncesFromInternal    bool
	AnnouncesFromInternalSet bool

	// Outgoing allows the interface to transmit. Default true when unset.
	// When false the interface is receive-only (Python OUT = False).
	Outgoing    bool
	OutgoingSet bool

	// Discoverable enables rnstransport interface discovery announces.
	Discoverable bool
	// DiscoveryName is the human-readable name published in discovery announces.
	DiscoveryName string
	// ReachableOn is the public hostname or IP peers should dial.
	ReachableOn string
	// DiscoveryAnnounceIntervalSec is seconds between discovery announces.
	// Zero means the Python default of 6 hours. Config key announce_interval
	// is minutes and is converted at parse time.
	DiscoveryAnnounceIntervalSec int
	// DiscoveryStampValue overrides the proof-of-work cost (default 14).
	DiscoveryStampValue int
	// DiscoveryEncrypt encrypts announces with the network identity.
	DiscoveryEncrypt bool
	// DiscoveryLocationCmd is an optional executable that prints
	// "lat,lon,height" used for discovery geo fields.
	DiscoveryLocationCmd string
	// DiscoveryLatitude Longitude Height are optional geo fields.
	DiscoveryLatitude  float64
	DiscoveryLongitude float64
	DiscoveryHeight    float64
	HasDiscoveryGeo    bool

	// BackboneInterface fast-flapping client block (RNS 1.3.9).
	// Zero/unset values use Python defaults when the interface is created.
	BlockFastFlapping        bool
	BlockFastFlappingSet     bool
	FastFlappingThreshold    float64 // seconds connected under this counts as a flap
	FastFlappingGrace        int     // flaps allowed before block
	FastFlappingBlockTimeMin float64 // block duration in minutes

	// Modem73Interface control socket and modem policy.
	ControlHost       string
	ControlPort       int
	MTUOverhead       int
	AutoFragmentation bool
	AutoFragSet       bool
	ShortFrames       string
	ShortMTU          int
	HandshakeX2       bool
	ProofX2           bool
	AutoBitrate       bool
	AutoBitrateSet    bool
	CSMAOverhead      bool
	CSMAOverheadSet   bool
	TimeoutMargin     float64

	// SDRInterface radio and modem settings.
	FrequencyHz int64
	SampleRate  int
	Bandwidth   int
	RXGain      float64
	TXGain      float64
	Modem       string
	SerialNum   string
}

// SharedInstanceType values for [reticulum] shared_instance_type.
// Empty config values resolve via ResolveSharedInstanceType (Unix on Linux,
// TCP elsewhere) to match Python RNS platform defaults.
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

	// EnableSeccomp installs a Linux seccomp denylist after Landlock when the
	// sandbox is enabled. Default true. Soft-fails if the kernel rejects the filter.
	EnableSeccomp bool

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

	// InMemoryStorage runs the stack fully ephemeral: no disk writes for path
	// tables, known destinations, transport identity, blackhole entries, or
	// split-resource staging. Implies both InMemoryPathTable and
	// InMemoryKnownDestinations. Library use with an empty ConfigPath and no
	// RETICULUM_STORAGE_PATH also behaves as in-memory storage.
	InMemoryStorage bool

	// SoftMemoryLimitBytes installs a Go soft heap limit via
	// runtime/debug.SetMemoryLimit when greater than zero. Near the limit the
	// runtime GCs more aggressively and large allocations may fail instead of
	// growing unbounded. Zero leaves the runtime default (unlimited).
	SoftMemoryLimitBytes int64

	// IdentityBackend selects identity at-rest storage: "file" (default),
	// "secretservice" (Freedesktop Secret Service), or "keyring" (Linux kernel
	// keyring, no D-Bus). When a non-file backend fails, persistence returns an error.
	IdentityBackend string

	// MaxInMemoryPaths caps the live path table when in-memory storage is
	// active. Zero uses DefaultMaxInMemoryPaths. Negative disables the cap.
	MaxInMemoryPaths int

	// MaxInMemoryKnownDestinations caps known destinations when in-memory
	// storage is active. Zero uses DefaultMaxInMemoryKnownDestinations.
	// Negative disables the cap.
	MaxInMemoryKnownDestinations int

	// MaxInMemoryResourceBytes caps staged split-resource bytes when
	// in-memory storage is active. Zero uses DefaultMaxInMemoryResourceBytes.
	// Negative disables the cap.
	MaxInMemoryResourceBytes int64

	// BackboneIO selects the kernel I/O multiplexer for backbone and local shared
	// instance sockets: auto, epoll, kqueue, io_uring, or go.
	BackboneIO string

	// DiscoverInterfaces enables rnstransport discovery listening and
	// AutoInterface NIC rescan when supported.
	DiscoverInterfaces bool

	// WatchInterfaces enables periodic NIC monitoring via net.Interfaces where supported.
	WatchInterfaces bool

	// StaticTransportIdentity keeps the persisted transport identity on the
	// wire even when enable_transport is no. When false and transport is
	// disabled, an ephemeral identity is used for transport while RPC auth
	// still uses the persisted identity.
	StaticTransportIdentity bool

	// LocalHopsDelta enables hop-field mangling for local-origin packets.
	// When true, outbound hop-0 packets use a random delta (2-7) instead of 0.
	LocalHopsDelta bool

	// RespondToProbes registers a transport probe destination that proves
	// all inbound data packets (rnprobe / reticulum-go probe).
	RespondToProbes bool

	// NetworkIdentityPath is the path to the network identity file used to
	// sign and encrypt interface discovery announces (Python network_identity).
	NetworkIdentityPath string

	// LogDestination is stderr, file, or both (see pkg/debug and reticulumconfig).
	LogDestination string

	// LogFile is an optional path when LogDestination includes file output.
	LogFile string

	// LogFormat is text or json for structured logs.
	LogFormat string
}

// NewReticulumConfig creates a new ReticulumConfig with default values
func NewReticulumConfig() *ReticulumConfig {
	return &ReticulumConfig{
		EnableTransport:     true,
		ShareInstance:       true,
		SharedInstancePort:  DefaultSharedInstancePort,
		InstanceControlPort: DefaultInstanceControlPort,
		SharedInstanceType:  DefaultSharedInstanceType(),
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
		return ErrConfigf("invalid shared instance port: %d", c.SharedInstancePort)
	}
	if c.InstanceControlPort < MinPort || c.InstanceControlPort > MaxPort {
		return ErrConfigf("invalid instance control port: %d", c.InstanceControlPort)
	}
	if c.EnableControlAPI {
		if c.ControlAPIPort < MinPort || c.ControlAPIPort > MaxPort {
			return ErrConfigf("invalid control api port: %d", c.ControlAPIPort)
		}
		if len(c.RPCKey) == 0 {
			return ErrConfigf("control api requires rpc_key to be set")
		}
	}
	return nil
}

// GetConfigPath implements ConfigProvider.
func (c *ReticulumConfig) GetConfigPath() string {
	if c == nil {
		return ""
	}
	return c.ConfigPath
}

// GetLogLevel implements ConfigProvider.
func (c *ReticulumConfig) GetLogLevel() int {
	if c == nil {
		return DefaultLogLevel
	}
	return c.LogLevel
}

// GetInterfaces implements ConfigProvider.
func (c *ReticulumConfig) GetInterfaces() map[string]InterfaceConfig {
	if c == nil {
		return map[string]InterfaceConfig{}
	}
	out := make(map[string]InterfaceConfig, len(c.Interfaces))
	for name, iface := range c.Interfaces {
		if iface == nil {
			continue
		}
		out[name] = *iface
	}
	return out
}

// DefaultConfig returns a ReticulumConfig with built-in defaults.
func DefaultConfig() *ReticulumConfig {
	return &ReticulumConfig{
		EnableTransport:     true,
		ShareInstance:       true,
		SharedInstancePort:  DefaultSharedInstancePort,
		InstanceControlPort: DefaultInstanceControlPort,
		SharedInstanceType:  DefaultSharedInstanceType(),
		PanicOnInterfaceErr: false,
		LogLevel:            DefaultLogLevel,
		Interfaces:          make(map[string]*InterfaceConfig),
		AppName:             "Go Client",
		AppAspect:           "node",
		EnableSandbox:       true,
		EnableSeccomp:       true,
		ControlAPIHost:      DefaultControlAPIHost,
		ControlAPIPort:      DefaultControlAPIPort,
	}
}
