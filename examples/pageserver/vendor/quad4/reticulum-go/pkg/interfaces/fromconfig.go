// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"quad4/reticulum-go/pkg/backbone"
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
)

// NewFromConfig constructs a logical interface from a loaded [common.InterfaceConfig].
func NewFromConfig(name string, cfg *common.InterfaceConfig) (Interface, error) {
	return NewFromConfigWithContext(name, cfg, nil)
}

// NewFromConfigWithContext constructs an interface using optional runtime context.
func NewFromConfigWithContext(name string, cfg *common.InterfaceConfig, ctx *FromConfigContext) (Interface, error) {
	if cfg == nil {
		return nil, errors.New("nil interface config")
	}
	var (
		iface Interface
		err   error
	)
	switch cfg.Type {
	case "UDPInterface":
		listen := cfg.Address
		if cfg.Port != 0 {
			host := cfg.Address
			if host == "" {
				host = "0.0.0.0"
			}
			if !strings.Contains(host, ":") {
				listen = net.JoinHostPort(host, strconv.Itoa(cfg.Port))
			}
		}
		target := cfg.TargetAddress
		if target == "" {
			target = cfg.TargetHost
			if target != "" && cfg.TargetPort != 0 && !strings.Contains(target, ":") {
				target = net.JoinHostPort(target, strconv.Itoa(cfg.TargetPort))
			}
		}
		iface, err = NewUDPInterfaceWithRetries(
			name,
			listen,
			target,
			cfg.Enabled,
			cfg.MaxReconnTries,
		)
	case "AutoInterface":
		iface, err = NewAutoInterface(name, cfg)
		if err == nil {
			if auto, ok := iface.(*AutoInterface); ok && ctx != nil && ctx.WatchInterfaces {
				auto.SetWatchInterfaces(true)
			}
		}
	case "TCPClientInterface":
		iface, err = NewTCPClientInterfaceWithRetries(
			name,
			cfg.TargetHost,
			cfg.TargetPort,
			cfg.KISSFraming,
			cfg.I2PTunneled,
			cfg.Enabled,
			cfg.MaxReconnTries,
		)
		if err == nil {
			if tc, ok := iface.(*TCPClientInterface); ok && ctx != nil && ctx.SynthesizeTunnel != nil {
				tc.SetTunnelSynth(ctx.SynthesizeTunnel)
			}
		}
	case "BackboneInterface", "BackboneClientInterface":
		var hub *backbone.Hub
		var spawn func(*BackboneClientInterface)
		if ctx != nil {
			hub = ctx.BackboneHub
			spawn = ctx.SpawnBackbone
		}
		iface, err = NewBackboneFromConfig(name, cfg, hub, spawn)
	case "WebSocketInterface":
		wsURL := cfg.Address
		if wsURL == "" {
			wsURL = cfg.TargetHost
		}
		iface, err = NewWebSocketInterface(name, wsURL, cfg.Enabled)
	case "TCPServerInterface":
		iface, err = NewTCPServerInterface(
			name,
			cfg.Address,
			cfg.Port,
			cfg.KISSFraming,
			cfg.I2PTunneled,
			cfg.PreferIPv6,
		)
	case "QUICClientInterface":
		iface, err = NewQUICClientInterfaceWithRetries(
			name,
			cfg.TargetHost,
			cfg.TargetPort,
			cfg.Enabled,
			cfg.MaxReconnTries,
			QUICClientOptions{
				CertFile: cfg.CertFile,
				KeyFile:  cfg.KeyFile,
				PeerKey:  cfg.PeerKey,
				SNI:      cfg.SNI,
			},
		)
	case "QUICServerInterface":
		iface, err = NewQUICServerInterface(
			name,
			cfg.Address,
			cfg.Port,
			QUICServerOptions{
				CertFile: cfg.CertFile,
				KeyFile:  cfg.KeyFile,
				PeerKey:  cfg.PeerKey,
			},
		)
	case "I2PInterface":
		parent, perr := NewI2PInterface(name, cfg, ctx)
		if perr != nil {
			return nil, perr
		}
		for _, peerAddr := range cfg.I2PPeers {
			peerName := name + " to " + peerAddr
			maxTries := cfg.MaxReconnTries
			peer := NewI2PInterfacePeer(parent, peerName, peerAddr, maxTries, cfg)
			parent.registerSpawnedPeer(peer)
		}
		iface = parent
	case "PipeInterface":
		delay := time.Duration(cfg.RespawnDelay) * time.Second
		panicOnErr := ctx != nil && ctx.PanicOnInterfaceError
		iface, err = NewPipeInterface(name, cfg.Command, cfg.Enabled, delay, panicOnErr)
	case "SerialInterface":
		frameIdle := time.Duration(cfg.SerialFrameIdleMs) * time.Millisecond
		device := cfg.Device
		if device == "" {
			device = cfg.Address
		}
		iface, err = NewSerialInterface(name, cfg.Enabled, SerialOptions{
			Device:            device,
			Speed:             cfg.Speed,
			DataBits:          cfg.DataBits,
			Parity:            cfg.Parity,
			StopBits:          cfg.StopBits,
			RTSCTS:            cfg.RTSCTS,
			DSRDTR:            cfg.DSRDTR,
			XONXOFF:           cfg.XONXOFF,
			FrameIdle:         frameIdle,
			MaxReconnectTries: cfg.MaxReconnTries,
			MTU:               cfg.MTU,
			Bitrate:           cfg.Bitrate,
		})
	case "Modem73Interface":
		autoFrag := true
		if cfg.AutoFragSet {
			autoFrag = cfg.AutoFragmentation
		}
		autoBitrate := true
		if cfg.AutoBitrateSet {
			autoBitrate = cfg.AutoBitrate
		}
		csma := true
		if cfg.CSMAOverheadSet {
			csma = cfg.CSMAOverhead
		}
		iface, err = NewModem73Interface(name, cfg.Enabled, Modem73Options{
			TargetHost:        cfg.TargetHost,
			TargetPort:        cfg.TargetPort,
			ControlHost:       cfg.ControlHost,
			ControlPort:       cfg.ControlPort,
			MTUOverhead:       cfg.MTUOverhead,
			Bitrate:           cfg.Bitrate,
			AutoFragmentation: autoFrag,
			ShortFrames:       cfg.ShortFrames,
			ShortMTU:          cfg.ShortMTU,
			HandshakeX2:       cfg.HandshakeX2,
			ProofX2:           cfg.ProofX2,
			AutoBitrate:       autoBitrate,
			CSMAOverhead:      csma,
			TimeoutMargin:     cfg.TimeoutMargin,
			MaxReconnectTries: cfg.MaxReconnTries,
		})
	case "SDRInterface":
		iface, err = NewSDRInterface(name, cfg.Enabled, SDROptionsFromConfig(cfg))
	case "WebTransportClientInterface":
		iface, err = NewWebTransportClientInterfaceWithRetries(
			name,
			cfg.TargetHost,
			cfg.TargetPort,
			cfg.Path,
			cfg.Enabled,
			cfg.MaxReconnTries,
			WebTransportClientOptions{
				CertFile:      cfg.CertFile,
				KeyFile:       cfg.KeyFile,
				PeerKey:       cfg.PeerKey,
				SNI:           cfg.SNI,
				TransportMode: cfg.TransportMode,
			},
		)
	case "WebTransportServerInterface":
		iface, err = NewWebTransportServerInterface(
			name,
			cfg.Address,
			cfg.Port,
			cfg.Path,
			WebTransportServerOptions{
				CertFile:      cfg.CertFile,
				KeyFile:       cfg.KeyFile,
				PeerKey:       cfg.PeerKey,
				TransportMode: cfg.TransportMode,
			},
		)
	case "DNSRendezvousInterface":
		interval := time.Duration(cfg.ResolveIntervalSec) * time.Second
		listen := cfg.Address
		if cfg.Port != 0 {
			host := cfg.Address
			if host == "" {
				host = "0.0.0.0"
			}
			if !strings.Contains(host, ":") {
				listen = net.JoinHostPort(host, strconv.Itoa(cfg.Port))
			}
		}
		iface, err = NewDNSRendezvousInterface(name, cfg.Enabled, DNSRendezvousOptions{
			Domain:          cfg.Domain,
			ListenAddr:      listen,
			ResolveInterval: interval,
		})
	case "VSOCKClientInterface":
		cid := uint32(1)
		if cfg.ContextID != 0 {
			parsed, perr := ParseVSOCKContextID(cfg.ContextID)
			if perr != nil {
				return nil, perr
			}
			cid = parsed
		}
		iface, err = NewVSOCKClientInterfaceWithRetries(
			name,
			cid,
			uint32(cfg.Port), // #nosec G115
			cfg.Enabled,
			cfg.MaxReconnTries,
		)
	case "VSOCKServerInterface":
		srv, serr := NewVSOCKServerInterface(name, uint32(cfg.Port)) // #nosec G115
		if serr != nil {
			return nil, serr
		}
		if cfg.ContextID != 0 {
			parsed, perr := ParseVSOCKContextID(cfg.ContextID)
			if perr != nil {
				return nil, perr
			}
			srv.SetListenContextID(parsed)
		}
		iface = srv
	case "HTTPSClientInterface":
		lp := time.Duration(cfg.LongPollSec) * time.Second
		iface, err = NewHTTPSClientInterfaceWithRetries(
			name,
			cfg.TargetHost,
			cfg.TargetPort,
			cfg.Enabled,
			cfg.MaxReconnTries,
			HTTPSClientOptions{
				CertFile: cfg.CertFile,
				KeyFile:  cfg.KeyFile,
				PeerKey:  cfg.PeerKey,
				SNI:      cfg.SNI,
				Path:     cfg.Path,
				LongPoll: lp,
			},
		)
	case "HTTPSServerInterface":
		lp := time.Duration(cfg.LongPollSec) * time.Second
		iface, err = NewHTTPSServerInterface(
			name,
			cfg.Address,
			cfg.Port,
			HTTPSServerOptions{
				CertFile: cfg.CertFile,
				KeyFile:  cfg.KeyFile,
				PeerKey:  cfg.PeerKey,
				Path:     cfg.Path,
				LongPoll: lp,
			},
		)
	case "LocalInterface", "LocalServerInterface":
		iface, err = NewLocalFromConfig(name, cfg, ctx)
	default:
		iface, err = loadExternalInterface(name, cfg, ctx)
	}
	if err != nil {
		return nil, err
	}
	ni, ok := iface.(common.NetworkInterface)
	if !ok {
		return nil, fmt.Errorf("interface %q does not implement common.NetworkInterface", name)
	}
	applyModeFromConfig(iface, cfg)
	applyOutgoingFromConfig(iface, cfg)
	if err := ApplyIFACFromConfig(ni, cfg); err != nil {
		return nil, err
	}
	return iface, nil
}

// applyModeFromConfig sets Mode, RecursivePRs, and AnnouncesFromInternal from cfg.
func applyModeFromConfig(iface Interface, cfg *common.InterfaceConfig) {
	if cfg == nil || iface == nil {
		return
	}
	mode := common.ParseInterfaceMode(cfg.Mode)
	afi := announcesFromInternal(cfg)
	if base := baseInterfaceOf(iface); base != nil {
		base.Mode = mode
		base.RecursivePRs = cfg.RecursivePRs
		base.AnnouncesFromInternal = afi
	}
}

// applyOutgoingFromConfig sets the transmit permit from outgoing / selected_outgoing.
func applyOutgoingFromConfig(iface Interface, cfg *common.InterfaceConfig) {
	if cfg == nil || iface == nil || !cfg.OutgoingSet {
		return
	}
	allowed := cfg.Outgoing
	if setter, ok := iface.(common.OutgoingController); ok {
		setter.SetOutgoingAllowed(allowed)
		return
	}
	if base := baseInterfaceOf(iface); base != nil {
		base.ReceiveOnly = !allowed
		return
	}
	if !allowed {
		debug.Log(debug.DebugError, "outgoing=no ignored interface type lacks SetOutgoingAllowed", "type", cfg.Type)
	}
}

func announcesFromInternal(cfg *common.InterfaceConfig) bool {
	if cfg.AnnouncesFromInternalSet {
		return cfg.AnnouncesFromInternal
	}
	return true
}

// baseInterfaceOf returns the embedded *BaseInterface for known concrete types.
func baseInterfaceOf(iface Interface) *BaseInterface {
	switch v := iface.(type) {
	case *UDPInterface:
		return &v.BaseInterface
	case *TCPClientInterface:
		return &v.BaseInterface
	case *TCPServerInterface:
		return &v.BaseInterface
	case *AutoInterface:
		return &v.BaseInterface
	case *I2PInterface:
		return &v.BaseInterface
	case *I2PInterfacePeer:
		return &v.BaseInterface
	case *PipeInterface:
		return &v.BaseInterface
	case *LocalServerInterface:
		return &v.BaseInterface
	case *LocalClientInterface:
		return &v.BaseInterface
	case *WebSocketInterface:
		return &v.BaseInterface
	case *BackboneInterface:
		return &v.BaseInterface
	case *BackboneClientInterface:
		return &v.BaseInterface
	case *QUICClientInterface:
		return &v.BaseInterface
	case *QUICServerInterface:
		return &v.BaseInterface
	case *SerialInterface:
		return &v.BaseInterface
	case *Modem73Interface:
		return &v.BaseInterface
	case *SDRInterface:
		return &v.BaseInterface
	case *WebTransportClientInterface:
		return &v.BaseInterface
	case *WebTransportServerInterface:
		return &v.BaseInterface
	case *DNSRendezvousInterface:
		return &v.BaseInterface
	case *VSOCKClientInterface:
		return &v.BaseInterface
	case *VSOCKServerInterface:
		return &v.BaseInterface
	case *HTTPSClientInterface:
		return &v.BaseInterface
	case *HTTPSServerInterface:
		return &v.BaseInterface
	default:
		return nil
	}
}
