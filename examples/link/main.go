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
	"strings"
	"syscall"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/interfaces"
	"quad4/reticulum-go/pkg/link"
	"quad4/reticulum-go/pkg/packet"
	"quad4/reticulum-go/pkg/transport"
)

// Link shows establishing a bidirectional link and exchanging text.

const APP_NAME = "example_utilities"

var (
	isServer   = flag.Bool("server", false, "wait for incoming link requests from clients")
	configPath = flag.String("config", "", "path to alternative Reticulum config directory")
	destHash   = flag.String("destination", "", "hexadecimal hash of the server destination")
	listenPort = flag.Int("listen-port", 4242, "UDP interface listen port")
	targetPort = flag.Int("target-port", 4242, "UDP interface target port for client")
)

var latestClientLink *link.Link
var serverLink *link.Link

func main() {
	flag.Parse()
	debug.Init()

	if *isServer {
		if err := server(*configPath); err != nil {
			debug.Log(debug.DebugCritical, "Server error", "error", err)
			os.Exit(1)
		}
	} else {
		if *destHash == "" {
			flag.Usage()
			fmt.Fprintf(os.Stderr, "\nError: destination hash required for client mode\n")
			os.Exit(1)
		}
		if err := client(*destHash, *configPath); err != nil {
			debug.Log(debug.DebugCritical, "Client error", "error", err)
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

	// Single destinations can accept encrypted link requests.
	serverDestination, err := destination.New(
		serverIdentity,
		destination.In,
		destination.Single,
		APP_NAME,
		t,
		"linkexample",
	)
	if err != nil {
		return fmt.Errorf("failed to create destination: %w", err)
	}

	serverDestination.SetLinkEstablishedCallback(func(l interface{}) {
		if linkObj, ok := l.(*link.Link); ok {
			clientConnected(linkObj)
		}
	})

	if err := t.Start(); err != nil {
		return fmt.Errorf("failed to start transport: %w", err)
	}

	if err := initializeInterfaces(cfg, t); err != nil {
		return fmt.Errorf("failed to initialize interfaces: %w", err)
	}

	debug.Log(
		debug.DebugInfo,
		"Link server waiting for connections",
		"hash", fmt.Sprintf("%x", serverDestination.GetHash()),
	)
	debug.Log(debug.DebugInfo, "Press enter to announce, Ctrl-C to quit")

	serverLoop(serverDestination)

	return nil
}

func serverLoop(dest *destination.Destination) {
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

// clientConnected is invoked when a peer opens a link to this server.
func clientConnected(l *link.Link) {
	debug.Log(debug.DebugInfo, "Client connected")
	latestClientLink = l
	l.SetLinkClosedCallback(clientDisconnected)
	l.SetPacketCallback(serverPacketReceived)
}

func clientDisconnected(l *link.Link) {
	debug.Log(debug.DebugInfo, "Client disconnected")
}

func serverPacketReceived(data []byte, pkt *packet.Packet) {
	// Replies go to the most recent connected client link.
	text := string(data)
	debug.Log(debug.DebugInfo, "Received data on the link", "text", text)

	replyText := fmt.Sprintf("I received \"%s\" over the link", text)
	replyData := []byte(replyText)

	if latestClientLink != nil && latestClientLink.IsActive() {
		if err := latestClientLink.SendPacket(replyData); err != nil {
			debug.Log(debug.DebugError, "Failed to send reply", "error", err)
		}
	}
}

// ========== Client Part ==========

func client(destinationHexHash string, configpath string) error {
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

	if ifaceCfg, ok := cfg.Interfaces["UDPInterface"]; ok {
		if ifaceCfg.TargetHost == "" {
			ifaceCfg.TargetHost = fmt.Sprintf("127.0.0.1:%d", *targetPort)
		}
	}

	t := transport.NewTransport(cfg)

	if err := t.Start(); err != nil {
		return fmt.Errorf("failed to start transport: %w", err)
	}

	if err := initializeInterfaces(cfg, t); err != nil {
		return fmt.Errorf("failed to initialize interfaces: %w", err)
	}

	if !t.HasPath(destinationHash) {
		debug.Log(debug.DebugInfo, "No path yet, requesting and waiting for announce")
		if err := t.RequestPath(destinationHash, "", nil, false); err != nil {
			return fmt.Errorf("failed to request path: %w", err)
		}

		timeout := time.After(30 * time.Second)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		pathFound := false
		for !pathFound {
			select {
			case <-timeout:
				return fmt.Errorf("timeout waiting for path to destination")
			case <-ticker.C:
				if t.HasPath(destinationHash) {
					pathFound = true
				}
			}
		}
	}

	serverIdentity, err := identity.Recall(destinationHash)
	if err != nil {
		return fmt.Errorf("failed to recall identity: %w", err)
	}

	debug.Log(debug.DebugInfo, "Opening link to server")

	serverDestination, err := destination.New(
		serverIdentity,
		destination.Out,
		destination.Single,
		APP_NAME,
		t,
		"linkexample",
	)
	if err != nil {
		return fmt.Errorf("failed to create destination: %w", err)
	}

	l := link.NewLink(serverDestination, t, nil, func(lnk *link.Link) {
		linkEstablished(lnk)
	}, linkClosed)

	l.SetPacketCallback(clientPacketReceived)

	if err := l.Establish(); err != nil {
		return fmt.Errorf("failed to establish link: %w", err)
	}

	l.Start()

	clientLoop()

	return nil
}

func clientLoop() {
	for serverLink == nil {
		time.Sleep(100 * time.Millisecond)
	}

	debug.Log(debug.DebugInfo, "Link up. Type text to send, or quit to exit")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	inputChan := make(chan string)
	go func() {
		reader := bufio.NewReader(os.Stdin)
		for {
			fmt.Print("> ")
			text, _ := reader.ReadString('\n')
			inputChan <- strings.TrimSpace(text)
		}
	}()

	for {
		select {
		case <-sigChan:
			debug.Log(debug.DebugInfo, "Shutting down...")
			serverLink.Teardown()
			return
		case text := <-inputChan:
			if text == "quit" || text == "q" || text == "exit" {
				serverLink.Teardown()
				return
			}

			if text != "" {
				data := []byte(text)

				const estimatedMDU = 400

				if len(data) <= estimatedMDU {
					if err := serverLink.SendPacket(data); err != nil {
						debug.Log(debug.DebugError, "Failed to send packet", "error", err)
					}
				} else {
					debug.Log(debug.DebugError,
						"Cannot send this packet, the data size exceeds the link packet MDU",
						"size", len(data),
						"mdu", estimatedMDU,
					)
				}
			}
		}
	}
}

func linkEstablished(l *link.Link) {
	serverLink = l
	debug.Log(debug.DebugInfo, "Link established with server")
}

func linkClosed(l *link.Link) {
	debug.Log(debug.DebugInfo, "Link closed")
	time.Sleep(1500 * time.Millisecond)
	os.Exit(0)
}

func clientPacketReceived(data []byte, pkt *packet.Packet) {
	text := string(data)
	debug.Log(debug.DebugInfo, "Received data on the link", "text", text)
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
