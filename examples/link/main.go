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

// This RNS example demonstrates how to set up a link to
// a destination, and pass data back and forth over it.

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
	// Initialize config
	cfg := loadConfig(configpath)

	// Initialize Reticulum
	t := transport.NewTransport(cfg)

	// Randomly create a new identity for our link example
	serverIdentity, err := identity.NewIdentity()
	if err != nil {
		return fmt.Errorf("failed to create server identity: %w", err)
	}

	// We create a destination that clients can connect to. We
	// want clients to create links to this destination, so we
	// need to create a "single" destination type.
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

	// We configure a function that will get called every time
	// a new client creates a link to this destination.
	serverDestination.SetLinkEstablishedCallback(func(l interface{}) {
		if linkObj, ok := l.(*link.Link); ok {
			clientConnected(linkObj)
		}
	})

	// Start transport
	if err := t.Start(); err != nil {
		return fmt.Errorf("failed to start transport: %w", err)
	}

	// Initialize interfaces
	if err := initializeInterfaces(cfg, t); err != nil {
		return fmt.Errorf("failed to initialize interfaces: %w", err)
	}

	debug.Log(
		debug.DebugInfo,
		"Link example running, waiting for a connection.",
		"hash", fmt.Sprintf("%x", serverDestination.GetHash()),
	)
	debug.Log(debug.DebugInfo, "Hit enter to manually send an announce (Ctrl-C to quit)")

	// Everything's ready!
	// Let's wait for client requests or user input
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

// When a client establishes a link to our server
// destination, this function will be called with
// a reference to the link.
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
	// When data is received over any active link,
	// it will all be directed to the last client
	// that connected.
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
	// We need a binary representation of the destination
	// hash that was entered on the command line
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

	// Initialize config
	cfg := loadConfig(configpath)

	// For local client/server testing, we must set a target for the client.
	if ifaceCfg, ok := cfg.Interfaces["UDPInterface"]; ok {
		if ifaceCfg.TargetHost == "" {
			ifaceCfg.TargetHost = fmt.Sprintf("127.0.0.1:%d", *targetPort)
		}
	}

	// Initialize Reticulum
	t := transport.NewTransport(cfg)

	// Start transport
	if err := t.Start(); err != nil {
		return fmt.Errorf("failed to start transport: %w", err)
	}

	// Initialize interfaces
	if err := initializeInterfaces(cfg, t); err != nil {
		return fmt.Errorf("failed to initialize interfaces: %w", err)
	}

	// Check if we know a path to the destination
	if !t.HasPath(destinationHash) {
		debug.Log(debug.DebugInfo, "Destination is not yet known. Requesting path and waiting for announce to arrive...")
		// RequestPath(destinationHash, onInterface, tag, recursive)
		if err := t.RequestPath(destinationHash, "", nil, false); err != nil {
			return fmt.Errorf("failed to request path: %w", err)
		}

		// Wait for path
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

	// Recall the server identity
	serverIdentity, err := identity.Recall(destinationHash)
	if err != nil {
		return fmt.Errorf("failed to recall identity: %w", err)
	}

	// Inform the user that we'll begin connecting
	debug.Log(debug.DebugInfo, "Establishing link with server...")

	// When the server identity is known, we set up a destination
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

	// And create a link
	l := link.NewLink(serverDestination, t, nil, func(lnk *link.Link) {
		linkEstablished(lnk)
	}, linkClosed)

	// We set a callback that will get executed
	// every time a packet is received over the link
	l.SetPacketCallback(clientPacketReceived)

	// Establish the link
	if err := l.Establish(); err != nil {
		return fmt.Errorf("failed to establish link: %w", err)
	}

	// Start the link watchdog
	l.Start()

	// Everything is set up, so let's enter a loop
	// for the user to interact with the example
	clientLoop()

	return nil
}

func clientLoop() {
	// Wait for the link to become active
	for serverLink == nil {
		time.Sleep(100 * time.Millisecond)
	}

	debug.Log(debug.DebugInfo, "Link established with server, enter some text to send, or \"quit\" to quit")

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
			// Check if we should quit the example
			if text == "quit" || text == "q" || text == "exit" {
				serverLink.Teardown()
				return
			}

			// If not empty, send the entered text over the link
			if text != "" {
				data := []byte(text)

				// Check MDU (Maximum Data Unit) for the link
				// For now we'll use a conservative estimate
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

// This function is called when a link has been established with the server
func linkEstablished(l *link.Link) {
	// We store a reference to the link instance for later use
	serverLink = l

	// Inform the user that the server is connected
	debug.Log(debug.DebugInfo, "Link established with server")
}

// When a link is closed, we'll inform the user, and exit the program
func linkClosed(l *link.Link) {
	debug.Log(debug.DebugInfo, "Link closed")
	time.Sleep(1500 * time.Millisecond)
	os.Exit(0)
}

// When a packet is received over the link, we simply print out the data.
func clientPacketReceived(data []byte, pkt *packet.Packet) {
	text := string(data)
	debug.Log(debug.DebugInfo, "Received data on the link", "text", text)
}

// ========== Helper Functions ==========

func loadConfig(configpath string) *common.ReticulumConfig {
	cfg := common.DefaultConfig()

	// Add default interface if none configured
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
