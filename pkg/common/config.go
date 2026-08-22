// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package common

import "time"

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

	// Gravity is pathing affinity (RNS 1.4.1). Higher values win path table
	// contests when the same announce emission arrives on multiple interfaces.
	// Zero is the Python default unless default_gravity is set globally.
	Gravity    int
	GravitySet bool

	// RecursivePRs enables path discovery for unknown destinations on this
	// interface.
	RecursivePRs bool

	// AnnouncesFromInternal controls whether announces learned via an
	// internal-mode next hop are rebroadcast. Default true when unset.
	AnnouncesFromInternal    bool
	AnnouncesFromInternalSet bool

	// AnnouncesToInternal allows a boundary-mode next hop to forward announces
	// onto internal-mode interfaces (RNS 1.4.1). Default false when unset.
	AnnouncesToInternal    bool
	AnnouncesToInternalSet bool

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
	// DiscoveryStampValue overrides the proof-of-work cost (default 16).
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

	// DefaultGravity is the pathing affinity applied to interfaces that do not
	// set gravity explicitly (RNS 1.4.1). Zero matches Python DEFAULT_GRAVITY.
	DefaultGravity    int
	DefaultGravitySet bool

	// AutoconnectInterfaceGravity is applied to discovered autoconnect peers
	// when autoconnect is enabled (RNS 1.4.1).
	AutoconnectInterfaceGravity    int
	AutoconnectInterfaceGravitySet bool

	// AutoconnectInterfaceMode overrides the mode for autoconnected interfaces.
	AutoconnectInterfaceMode string

	// AutoconnectAnnouncesToInternal sets announces_to_internal on autoconnect peers.
	AutoconnectAnnouncesToInternal    bool
	AutoconnectAnnouncesToInternalSet bool

	// AutoconnectDiscoveredInterfaces is the max number of concurrent
	// autoconnected discovery peers from rnstransport (Backbone, TCP, I2P).
	// Zero disables autoconnect (Python autoconnect_discovered_interfaces).
	AutoconnectDiscoveredInterfaces int

	// PublishBlackhole registers rnstransport.info.blackhole with a /list
	// request handler so peers can fetch this instance's blackhole table.
	PublishBlackhole bool

	// BlackholeSources lists remote transport identity hashes to pull blackhole
	// lists from (Python blackhole_sources).
	BlackholeSources [][]byte

	// BlackholeUpdateInterval is how often to pull each blackhole source.
	// Zero means the Python default of 60 minutes. Values below 2 minutes are
	// raised to 2 minutes when parsed from config.
	BlackholeUpdateInterval time.Duration

	// AllowLinkPathRebalance enables LRPROOF-based hop rebalancing (RNS 1.4.1).
	// Default true. Go adds dampening and gravity-aware refusals on top.
	AllowLinkPathRebalance    bool
	AllowLinkPathRebalanceSet bool

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

	// DoSProtection selects IDS/IPS style flood and OOM gates off detect prevent or auto.
	// Go-only. Default off until the gates are proven not to drop legitimate
	// public-mesh path requests and resource transfers. Detect warns on stdout
	// and increments health counters. Prevent also sheds ingress refuses excess
	// accepts and drops overloaded handlers. Auto learns quietly persists
	// baselines via msgpack then arms prevent and relearns on change.
	DoSProtection string

	// DoSProtectionSet is true when dos_protection appeared in the config file.
	DoSProtectionSet bool

	DoSMaxPPS       float64
	DoSMaxBPS       float64
	DoSFloorPPS     float64
	DoSFloorBPS     float64
	DoSMaxConns     int
	DoSMaxResources int
	DoSMaxCrypto    int
	DoSMaxHandshake int

	// IdentityBackend selects identity at-rest storage: "file" (default),
	// "secretservice" (Freedesktop Secret Service), or "keyring" (Linux kernel
	// keyring, no D-Bus). When a non-file backend fails, persistence returns an error.
	IdentityBackend string

	// MaxInMemoryPaths caps the live path table in RAM. Zero uses
	// DefaultMaxInMemoryPaths. Negative disables the cap.
	MaxInMemoryPaths    int
	MaxInMemoryPathsSet bool

	// MaxInMemoryKnownDestinations caps known destinations in RAM. Zero uses
	// DefaultMaxInMemoryKnownDestinations. Negative disables the cap.
	MaxInMemoryKnownDestinations    int
	MaxInMemoryKnownDestinationsSet bool

	// MaxInMemoryResourceBytes caps staged split-resource bytes when
	// in-memory storage is active. Zero uses DefaultMaxInMemoryResourceBytes.
	// Negative disables the cap.
	MaxInMemoryResourceBytes int64

	// MaxPacketHashlist caps the packet hash loop filter. Zero selects a
	// default from EnableTransport. Negative forces the full transport-sized
	// default. Positive is an explicit entry budget.
	MaxPacketHashlist    int
	MaxPacketHashlistSet bool

	// MaxPacketHandlers is the HandlePacket worker count and queue depth.
	// Zero uses DefaultMaxPacketHandlers (512).
	MaxPacketHandlers    int
	MaxPacketHandlersSet bool

	// NodeProfile selects a Go-only overlay that fills unset knobs:
	// default, core_router, or embedded.
	NodeProfile string

	// SandboxStrict makes Landlock, seccomp, OpenBSD pledge/unveil lock, and
	// FreeBSD CapEnter failures fatal. Default false. Platforms with no
	// sandbox mechanism still start.
	SandboxStrict bool

	// SandboxProfile selects Landlock path rules: full (default, includes
	// /bin for pipe and pageserver exec) or router (omits /bin trees).
	// Other OS policies are unchanged. Never inferred from NodeProfile.
	SandboxProfile string

	// SandboxExtraPaths is an operator list of extra filesystem paths to
	// allow in Landlock and OpenBSD unveil.
	SandboxExtraPaths []string

	// SandboxExecRlimits applies conservative rlimits to pipe, discovery, and
	// dynamic page child processes on Linux. Default false.
	SandboxExecRlimits bool

	// SandboxSkipScoped skips Landlock V6 RestrictScoped. GUI processes that
	// spawn WebKit helpers need abstract UNIX sockets and signals.
	SandboxSkipScoped bool

	// ControlAPISocket is an optional Unix socket path for the control API.
	// TCP listen stays enabled when the control API is on.
	ControlAPISocket string

	// BackboneIO selects the kernel I/O multiplexer for backbone and local shared
	// instance sockets: auto, epoll, kqueue, io_uring, or go.
	BackboneIO    string
	BackboneIOSet bool

	// DiscoverInterfaces enables rnstransport discovery listening and
	// AutoInterface NIC rescan when supported.
	DiscoverInterfaces bool

	// WatchInterfaces enables periodic NIC monitoring via net.Interfaces where supported.
	WatchInterfaces    bool
	WatchInterfacesSet bool

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

	// EnableRemoteManagement registers rnstransport.remote.management so
	// remote rgopath / rgostatus (and Python rnpath / rnstatus) can query
	// path tables and interface stats over a link. Default false.
	EnableRemoteManagement bool

	// RemoteManagementAllowed is the identity-hash ACL for remote management
	// request handlers (Python remote_management_allowed).
	RemoteManagementAllowed [][]byte

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
