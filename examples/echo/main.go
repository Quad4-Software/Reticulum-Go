// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package main

import (
	"bufio"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/interfaces"
	"quad4/reticulum-go/pkg/packet"
	"quad4/reticulum-go/pkg/transport"
)

// Echo is a small client/server demo. The client sends a
// packet and the server answers with a proof of receipt.

const APP_NAME = "example_utilities"

var (
	isServer       = flag.Bool("server", false, "run as server")
	configPath     = flag.String("config", "", "path to alternative Reticulum config directory")
	destHashStr    = flag.String("destination", "", "hexadecimal hash of the server destination")
	timeoutSeconds = flag.Float64("timeout", 0, "set a reply timeout in seconds")
	listenPort     = flag.Int("listen-port", 4242, "UDP interface listen port")
	targetPort     = flag.Int("target-port", 4242, "UDP interface target port for client")
)

var reticulum *ReticulumInstance

type ReticulumInstance struct {
	transport   *transport.Transport
	identity    *identity.Identity
	destination *destination.Destination
}

func main() {
	flag.Parse()
	debug.Init()

	if *isServer {
		if err := server(*configPath); err != nil {
			debug.GetLogger().Error("Server error", "error", err)
			os.Exit(1)
		}
	} else {
		if *destHashStr == "" {
			flag.Usage()
			fmt.Fprintf(os.Stderr, "\nError: destination hash required for client mode\n")
			os.Exit(1)
		}
		if err := client(*destHashStr, *configPath, *timeoutSeconds); err != nil {
			debug.GetLogger().Error("Client error", "error", err)
			os.Exit(1)
		}
	}
}

// ========== Server Part ==========

func server(configpath string) error {
	cfg := loadConfig(configpath)
	t := transport.NewTransport(cfg)

	serverIdentity, err := identity.NewIdentity()
	if err != nil {
		return fmt.Errorf("failed to create server identity: %w", err)
	}

	// Single destination so requests are encrypted to this identity only.
	echoDestination, err := destination.New(
		serverIdentity,
		destination.In,
		destination.Single,
		APP_NAME,
		t,
		"echo",
		"request",
	)
	if err != nil {
		return fmt.Errorf("failed to create destination: %w", err)
	}

	// ProveAll makes Transport return a proof for every inbound packet.
	echoDestination.SetProofStrategy(destination.ProveAll)

	echoDestination.SetPacketCallback(func(data []byte, iface common.NetworkInterface) {
		serverCallback(data, iface)
	})

	reticulum = &ReticulumInstance{
		transport:   t,
		identity:    serverIdentity,
		destination: echoDestination,
	}

	if err := t.Start(); err != nil {
		return fmt.Errorf("failed to start transport: %w", err)
	}

	if err := initializeInterfaces(cfg, t); err != nil {
		return fmt.Errorf("failed to initialize interfaces: %w", err)
	}

	debug.Log(debug.DebugInfo,
		"Echo server ready. Press enter to announce, Ctrl-C to quit",
		"hash", fmt.Sprintf("%x", echoDestination.GetHash()),
	)

	announceLoop(echoDestination)

	return nil
}

func serverCallback(data []byte, iface common.NetworkInterface) {
	debug.Log(debug.DebugInfo, "Received packet from echo client, proof sent")
	debug.Log(debug.DebugVerbose, "Received packet",
		"bytes", len(data),
		"interface", iface.GetName(),
	)
}

func announceLoop(dest *destination.Destination) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	inputChan := make(chan struct{})
	go func() {
		reader := bufio.NewReader(os.Stdin)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			if len(line) > 0 {
				inputChan <- struct{}{}
			}
		}
	}()

	for {
		select {
		case <-sigChan:
			debug.Log(debug.DebugInfo, "Shutting down...")
			return
		case <-inputChan:
			if err := dest.Announce(false, nil, nil); err != nil {
				debug.Log(debug.DebugError, "Failed to send announce", "error", err)
			} else {
				debug.Log(debug.DebugInfo, "Sent announce", "hash", fmt.Sprintf("%x", dest.GetHash()))
			}
		}
	}
}

// ========== Client Part ==========

func client(destinationHexHash string, configpath string, timeout float64) error {
	destLen := (identity.TruncatedHashLength / 8) * 2
	if len(destinationHexHash) != destLen {
		return fmt.Errorf(
			"destination length is invalid, must be %d hexadecimal characters (%d bytes)",
			destLen, destLen/2,
		)
	}

	destinationHash, err := hex.DecodeString(destinationHexHash)
	if err != nil {
		return fmt.Errorf("invalid destination hash: %w", err)
	}

	cfg := loadConfig(configpath)

	// Point UDP at the peer for local loopback tests.
	if ifaceCfg, ok := cfg.Interfaces["UDPInterface"]; ok {
		if ifaceCfg.TargetHost == "" {
			ifaceCfg.TargetHost = fmt.Sprintf("127.0.0.1:%d", *targetPort)
		}
	}

	t := transport.NewTransport(cfg)

	clientIdentity, err := identity.NewIdentity()
	if err != nil {
		return fmt.Errorf("failed to create client identity: %w", err)
	}

	reticulum = &ReticulumInstance{
		transport: t,
		identity:  clientIdentity,
	}

	if err := t.Start(); err != nil {
		return fmt.Errorf("failed to start transport: %w", err)
	}

	if err := initializeInterfaces(cfg, t); err != nil {
		return fmt.Errorf("failed to initialize interfaces: %w", err)
	}

	debug.Log(debug.DebugInfo, "Echo client ready. Press enter to send a request, Ctrl-C to quit",
		"destination", destinationHexHash,
	)

	return clientLoop(destinationHash, t, timeout)
}

func clientLoop(destHash []byte, t *transport.Transport, timeout float64) error {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	inputChan := make(chan struct{})
	go func() {
		reader := bufio.NewReader(os.Stdin)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			if len(line) > 0 {
				inputChan <- struct{}{}
			}
		}
	}()

	for {
		select {
		case <-sigChan:
			debug.Log(debug.DebugInfo, "Shutting down...")
			return nil
		case <-inputChan:
			if !t.HasPath(destHash) {
				debug.Log(debug.DebugInfo, "No path yet, requesting one")
				if err := t.RequestPath(destHash, "", nil, false); err != nil {
					debug.Log(debug.DebugError, "Failed to request path", "error", err)
				}
				debug.Log(debug.DebugInfo, "Press enter again after an announce arrives")
				continue
			}

			serverIdentity, err := identity.Recall(destHash)
			if err != nil {
				debug.Log(debug.DebugError, "Failed to recall identity", "error", err)
				continue
			}

			requestDestination, err := destination.New(
				serverIdentity,
				destination.Out,
				destination.Single,
				APP_NAME,
				t,
				"echo",
				"request",
			)
			if err != nil {
				debug.Log(debug.DebugError, "Failed to create request destination", "error", err)
				continue
			}

			echoRequest := packet.NewPacket(
				destination.Single,
				identity.GetRandomHash(),
				packet.PacketTypeData,
				packet.ContextNone,
				0,
				packet.HeaderType1,
				nil,
				true,
				packet.FlagUnset,
			)
			echoRequest.DestinationHash = requestDestination.GetHash()

			if err := echoRequest.Pack(); err != nil {
				debug.Log(debug.DebugError, "Failed to pack packet", "error", err)
				continue
			}

			if err := t.SendPacket(echoRequest); err != nil {
				debug.Log(debug.DebugError, "Failed to send packet", "error", err)
				continue
			}

			debug.Log(debug.DebugInfo, "Sent echo request", "hash", fmt.Sprintf("%x", requestDestination.GetHash()))

			if timeout > 0 {
				go func() {
					time.Sleep(time.Duration(timeout * float64(time.Second)))
					debug.Log(debug.DebugError, "Request timed out")
				}()
			}
		}
	}
}

// ========== Helper Functions ==========

func loadConfig(configpath string) *common.ReticulumConfig {
	cfg := common.DefaultConfig()

	if len(cfg.Interfaces) == 0 {
		cfg.Interfaces = make(map[string]*common.InterfaceConfig)
		cfg.Interfaces["UDPInterface"] = &common.InterfaceConfig{
			Type:    "UDPInterface",
			Enabled: true,
			Address: fmt.Sprintf("0.0.0.0:%d", *listenPort),
			Name:    "UDPInterface",
		}
	}

	return cfg
}

func initializeInterfaces(cfg *common.ReticulumConfig, t *transport.Transport) error {
	for name, ifaceConfig := range cfg.Interfaces {
		if !ifaceConfig.Enabled {
			continue
		}

		var iface interfaces.Interface
		var err error

		switch ifaceConfig.Type {
		case "UDPInterface":
			iface, err = interfaces.NewUDPInterface(
				name,
				ifaceConfig.Address,
				ifaceConfig.TargetHost,
				ifaceConfig.Enabled,
			)
		case "TCPClientInterface":
			iface, err = interfaces.NewTCPClientInterface(
				name,
				ifaceConfig.TargetHost,
				ifaceConfig.TargetPort,
				ifaceConfig.KISSFraming,
				ifaceConfig.I2PTunneled,
				ifaceConfig.Enabled,
			)
		default:
			debug.Log(debug.DebugError, "Unknown interface type", "type", ifaceConfig.Type)
			continue
		}

		if err != nil {
			debug.Log(debug.DebugError, "Failed to create interface", "name", name, "error", err)
			continue
		}

		iface.SetPacketCallback(func(data []byte, ni common.NetworkInterface) {
			t.HandlePacket(data, ni)
		})

		if err := iface.Start(); err != nil {
			debug.Log(debug.DebugError, "Failed to start interface", "name", name, "error", err)
			continue
		}

		if netIface, ok := iface.(common.NetworkInterface); ok {
			if err := t.RegisterInterface(name, netIface); err != nil {
				debug.Log(debug.DebugError, "Failed to register interface", "error", err)
			}
		}

		debug.Log(debug.DebugInfo, "Interface started successfully", "name", name)
	}

	return nil
}
