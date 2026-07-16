// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package main

import (
	"bufio"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"quad4/msgpack/v5/pkg/msgpack"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/interfaces"
	"quad4/reticulum-go/pkg/link"
	"quad4/reticulum-go/pkg/packet"
	"quad4/reticulum-go/pkg/resource"
	"quad4/reticulum-go/pkg/transport"
)

const (
	APP_NAME    = "example_utilities"
	APP_TIMEOUT = 45 * time.Second
)

var (
	serverMode = flag.Bool("server", false, "run as server")
	servePath  = flag.String("serve", "", "directory to serve files from")
	configPath = flag.String("config", "", "path to alternative Reticulum config directory")
	destHash   = flag.String("destination", "", "hexadecimal hash of the server destination")
	listenPort = flag.Int("listen-port", 4242, "UDP interface listen port")
	targetPort = flag.Int("target-port", 4242, "UDP interface target port")
)

// Global variables for client
var (
	serverFiles     []string
	serverLink      *link.Link
	currentDownload []byte
	currentFilename string
	downloadStarted time.Time
	downloadSize    int64
	transferSize    int64
)

// Global server variables
var (
	serverTransport *transport.Transport
	serveDirectory  string
)

// IncomingLink represents a link from an incoming connection
type IncomingLink struct {
	linkID      []byte
	destination *destination.Destination
	transport   *transport.Transport
}

// Implement basic link interface methods
func (l *IncomingLink) GetLinkID() []byte {
	return l.linkID
}

func (l *IncomingLink) GetDestination() *destination.Destination {
	return l.destination
}

func (l *IncomingLink) GetTransport() *transport.Transport {
	return l.transport
}

func main() {
	flag.Parse()
	debug.Init()

	if *serverMode || *servePath != "" {
		if *servePath == "" {
			debug.Log(debug.DebugCritical, "Server mode requires --serve directory")
			os.Exit(1)
		}
		runServer(*configPath, *servePath)
	} else {
		if *destHash == "" {
			flag.Usage()
			fmt.Println("\nError: destination hash required for client mode")
			fmt.Println("\nUsage:")
			fmt.Println("  Server: filetransfer --server --serve /path/to/files")
			fmt.Println("  Client: filetransfer --destination <hash>")
			os.Exit(1)
		}
		runClient(*destHash, *configPath)
	}
}

// ============================================================
// SERVER FUNCTIONS
// ============================================================

func runServer(configPath string, servePath string) {
	// Verify directory exists
	if _, err := os.Stat(servePath); os.IsNotExist(err) {
		debug.Log(debug.DebugCritical, "The specified directory does not exist", "path", servePath)
		os.Exit(1)
	}

	serveDirectory = servePath

	// Load config
	cfg := loadConfig(configPath)

	// Create transport
	tr := transport.NewTransport(cfg)
	serverTransport = tr

	// Start transport
	if err := tr.Start(); err != nil {
		debug.Log(debug.DebugCritical, "Failed to start transport", "error", err)
		os.Exit(1)
	}

	// Initialize interfaces
	if err := initializeInterfaces(cfg, tr); err != nil {
		debug.Log(debug.DebugCritical, "Failed to initialize interfaces", "error", err)
		os.Exit(1)
	}

	// Create identity
	serverIdentity, err := identity.New()
	if err != nil {
		debug.Log(debug.DebugCritical, "Failed to create identity", "error", err)
		os.Exit(1)
	}

	// Create destination
	dest, err := destination.New(
		serverIdentity,
		destination.In,
		destination.Single,
		APP_NAME,
		tr,
		"filetransfer", "server",
	)
	if err != nil {
		debug.Log(debug.DebugCritical, "Failed to create destination", "error", err)
		os.Exit(1)
	}

	// Set link established callback
	dest.SetLinkEstablishedCallback(func(l interface{}) {
		if lnk, ok := l.(*link.Link); ok {
			debug.Log(debug.DebugVerbose, "Received link request", "id", hex.EncodeToString(lnk.GetLinkID()[:8]))
			handleClientConnected(lnk, servePath)
		} else {
			debug.Log(debug.DebugError, "Invalid link object type", "type", fmt.Sprintf("%T", l))
		}
	})

	// Enable accepting incoming link requests
	dest.AcceptsLinks(true)

	debug.Log(debug.DebugInfo, "File server running", "hash", fmt.Sprintf("%x", dest.GetHash()))
	debug.Log(debug.DebugInfo, "Serve path", "path", servePath)

	// Announce initially
	if err := dest.Announce(false, nil, nil); err != nil {
		debug.Log(debug.DebugError, "Failed to send initial announce", "error", err)
	}

	// Announce loop in a goroutine
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			if err := dest.Announce(false, nil, nil); err != nil {
				debug.Log(debug.DebugError, "Failed to announce", "error", err)
			} else {
				debug.Log(debug.DebugInfo, "Sent periodic announce", "hash", fmt.Sprintf("%x", dest.GetHash()))
			}
		}
	}()

	fmt.Println("Hit enter to manually send an announce (Ctrl-C to quit)")

	// Keep alive
	select {}
}

func handleClientConnected(l *link.Link, servePath string) {
	debug.Log(debug.DebugInfo, "Client connected, sending file list...")

	// Get file list
	files, err := listFiles(servePath)
	if err != nil {
		debug.Log(debug.DebugError, "Error listing files", "error", err)
		return
	}

	// Pack file list
	data, err := msgpack.Marshal(files)
	if err != nil {
		debug.Log(debug.DebugError, "Error packing file list", "error", err)
		return
	}

	// Send file list through the link
	if err := l.SendPacket(data); err != nil {
		debug.Log(debug.DebugError, "Failed to send file list", "error", err)
		return
	}

	debug.Log(debug.DebugInfo, "File list sent, link established successfully!")

	// Register packet callback on the server side to handle client requests
	l.SetPacketCallback(func(data []byte, pkt *packet.Packet) {
		handleClientRequest(data, l, servePath, files)
	})
}

func listFiles(servePath string) ([]string, error) {
	entries, err := os.ReadDir(servePath)
	if err != nil {
		return nil, err
	}

	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			files = append(files, entry.Name())
		}
	}
	return files, nil
}

func handleClientRequest(data []byte, l *link.Link, servePath string, availableFiles []string) {
	filename := string(data)

	// Check if file is in available list
	found := false
	for _, f := range availableFiles {
		if f == filename {
			found = true
			break
		}
	}

	if !found {
		debug.Log(debug.DebugError, "Client requested unknown file", "filename", filename)
		return
	}

	debug.Log(debug.DebugInfo, "Client requested", "filename", filename)

	// Open file
	cleanServePath := filepath.Clean(servePath)
	safeName := filepath.Base(filename)
	filePath := filepath.Join(cleanServePath, safeName)

	// Basic path traversal protection
	if !strings.HasPrefix(filePath, cleanServePath) {
		debug.Log(debug.DebugError, "Invalid file path requested", "filename", filename)
		return
	}

	file, err := os.Open(filePath) // #nosec G304
	if err != nil {
		debug.Log(debug.DebugError, "Error opening file", "error", err)
		return
	}
	defer file.Close()

	// Create resource
	res, err := resource.New(file, true) // true = auto compress
	if err != nil {
		debug.Log(debug.DebugError, "Error creating resource", "error", err)
		return
	}

	debug.Log(debug.DebugInfo, "Starting transfer", "filename", filename, "size", res.GetDataSize())

	// Send resource through the link
	if err := l.SendResource(res); err != nil {
		debug.Log(debug.DebugError, "Failed to send resource", "error", err)
	}
}

// ============================================================
// CLIENT FUNCTIONS
// ============================================================

func runClient(destinationHash string, configPath string) {
	// Parse destination hash
	destLen := (identity.TruncatedHashLength / 8) * 2
	if len(destinationHash) != destLen {
		debug.Log(debug.DebugCritical, "Invalid destination length", "expected", destLen, "got", len(destinationHash))
		os.Exit(1)
	}

	destHashBytes, err := hex.DecodeString(destinationHash)
	if err != nil {
		debug.Log(debug.DebugCritical, "Invalid destination hash", "error", err)
		os.Exit(1)
	}

	// Load config
	cfg := loadConfig(configPath)

	// Create transport
	tr := transport.NewTransport(cfg)

	// Start transport
	if err := tr.Start(); err != nil {
		debug.Log(debug.DebugCritical, "Failed to start transport", "error", err)
		os.Exit(1)
	}

	// Initialize interfaces
	if err := initializeInterfaces(cfg, tr); err != nil {
		debug.Log(debug.DebugCritical, "Failed to initialize interfaces", "error", err)
		os.Exit(1)
	}

	// Seed direct path for local UDP testing
	tr.UpdatePath(destHashBytes, nil, "UDPInterface", 1)
	debug.Log(debug.DebugInfo, "Seeded direct UDP path", "to", destinationHash)

	// Check if we have a path to the destination
	if !tr.HasPath(destHashBytes) {
		debug.Log(debug.DebugInfo, "Destination is not yet known. Requesting path and waiting for announce...")
		if err := tr.RequestPath(destHashBytes, "", nil, false); err != nil {
			debug.Log(debug.DebugError, "Warning: Failed to request path", "error", err)
		}

		timeout := time.After(30 * time.Second)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-timeout:
				debug.Log(debug.DebugCritical, "Timeout waiting for path to destination")
				os.Exit(1)
			case <-ticker.C:
				if tr.HasPath(destHashBytes) {
					goto PathFound
				}
			}
		}
	}

PathFound:
	debug.Log(debug.DebugInfo, "Path to destination is known")

	// Check if we have the identity
	serverIdentity, err := identity.Recall(destHashBytes)
	if err != nil || serverIdentity == nil {
		debug.Log(debug.DebugInfo, "Server identity is not yet known. Waiting for announce...")
		timeout := time.After(60 * time.Second)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-timeout:
				debug.Log(debug.DebugCritical, "Timeout waiting for server identity")
				os.Exit(1)
			case <-ticker.C:
				serverIdentity, err = identity.Recall(destHashBytes)
				if err == nil && serverIdentity != nil {
					goto IdentityFound
				}
			}
		}
	}

IdentityFound:
	debug.Log(debug.DebugInfo, "Server identity is known")

	// Create destination from the announced hash
	// Use FromHash instead of New to preserve the server's destination hash
	serverDestination, err := destination.FromHash(
		destHashBytes,
		serverIdentity,
		destination.Single,
		tr,
	)
	if err != nil {
		debug.Log(debug.DebugCritical, "Failed to create destination", "error", err)
		os.Exit(1)
	}

	debug.Log(debug.DebugInfo, "Establishing link with server...")

	// Create link - need to get the right interface
	// For now, use nil and let the link figure it out
	lnk := link.NewLink(serverDestination, tr, nil, func(l *link.Link) {
		debug.Log(debug.DebugInfo, "Link established with server")
		debug.Log(debug.DebugInfo, "Waiting for filelist...")
	}, func(l *link.Link) {
		debug.Log(debug.DebugInfo, "Link closed, exiting")
		os.Exit(0)
	})

	serverLink = lnk

	// Set callbacks
	lnk.SetPacketCallback(func(data []byte, pkt *packet.Packet) {
		if pkt.Context == packet.ContextNone {
			handleFileListReceived(data)
		} else if pkt.Context == packet.ContextResource {
			handleFileDataReceived(data)
		}
	})

	// Set resource strategy to accept all
	if err := lnk.SetResourceStrategy(link.AcceptAll); err != nil {
		debug.Log(debug.DebugError, "Warning: Failed to set resource strategy", "error", err)
	}

	// Establish the link
	if err := lnk.Establish(); err != nil {
		debug.Log(debug.DebugCritical, "Failed to establish link", "error", err)
		os.Exit(1)
	}

	// Wait for file list
	timeout := time.After(APP_TIMEOUT)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for len(serverFiles) == 0 {
		select {
		case <-timeout:
			debug.Log(debug.DebugCritical, "Timeout waiting for file list")
			os.Exit(1)
		case <-ticker.C:
			// Continue waiting
		}
	}

	debug.Log(debug.DebugInfo, "Ready!")
	time.Sleep(500 * time.Millisecond)

	// Run menu
	runMenu()
}

func handleFileListReceived(data []byte) {
	var fileList []string
	if err := msgpack.Unmarshal(data, &fileList); err != nil {
		debug.Log(debug.DebugError, "Error unpacking file list", "error", err)
		if serverLink != nil {
			serverLink.Teardown()
		}
		return
	}

	for _, file := range fileList {
		found := false
		for _, existing := range serverFiles {
			if existing == file {
				found = true
				break
			}
		}
		if !found {
			serverFiles = append(serverFiles, file)
		}
	}

	debug.Log(debug.DebugInfo, "Received file list", "count", len(serverFiles))
}

func handleFileDataReceived(data []byte) {
	if len(data) == 0 {
		finishDownload()
		return
	}

	if downloadStarted.IsZero() {
		downloadStarted = time.Now()
		currentDownload = make([]byte, 0)
		debug.Log(debug.DebugInfo, "Download started", "filename", currentFilename)
	}

	currentDownload = append(currentDownload, data...)
	transferSize = int64(len(currentDownload))

	// In this simple example, we don't know the final size, so we just log progress
	fmt.Printf("\rDownloaded: %s   ", formatSize(transferSize))
}

func finishDownload() {
	if downloadStarted.IsZero() {
		return
	}

	downloadTime := time.Since(downloadStarted)
	fmt.Printf("\rDownloaded: %s (Completed in %v)\n", formatSize(transferSize), downloadTime)

	// Save file
	baseName := filepath.Base(currentFilename)
	if baseName == "." || baseName == string(os.PathSeparator) {
		baseName = "downloaded_file"
	}

	savedFilename := filepath.Join(".", baseName)
	counter := 0
	for {
		candidate := savedFilename
		if counter > 0 {
			candidate = filepath.Join(".", fmt.Sprintf("%s.%d", baseName, counter))
		}
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			savedFilename = candidate
			break
		}
		counter++
	}

	file, err := os.Create(savedFilename) // #nosec G304
	if err != nil {
		debug.Log(debug.DebugError, "Error creating file", "error", err)
		resetDownload()
		return
	}
	defer file.Close()

	if _, err := file.Write(currentDownload); err != nil {
		debug.Log(debug.DebugError, "Error writing file", "error", err)
		resetDownload()
		return
	}

	// Print statistics
	hours, rem := divmod(int(downloadTime.Seconds()), 3600)
	minutes, seconds := divmod(rem, 60)
	timeString := fmt.Sprintf("%02d:%02d:%05.2f", hours, minutes, float64(seconds))

	fmt.Println("\n--- Statistics -----")
	fmt.Printf("\tTime taken       : %s\n", timeString)
	fmt.Printf("\tFile size        : %s\n", formatSize(int64(len(currentDownload))))
	fmt.Printf("\tData transferred : %s\n", formatSize(transferSize))
	fmt.Printf("\tTransfer rate    : %s/s\n", formatSize(int64(float64(transferSize)/downloadTime.Seconds())))
	fmt.Printf("\nFile saved as: %s\n", savedFilename)
	fmt.Println("\nPress enter to continue...")

	resetDownload()
}

func resetDownload() {
	currentDownload = nil
	downloadStarted = time.Time{}
	transferSize = 0
}

func runMenu() {
	scanner := bufio.NewScanner(os.Stdin)

	for {
		printMenu()
		fmt.Print("> ")

		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())

		if input == "q" || input == "quit" || input == "exit" {
			debug.Log(debug.DebugInfo, "Exiting...")
			if serverLink != nil {
				serverLink.Teardown()
			}
			os.Exit(0)
		}

		// Try as filename
		found := false
		for _, file := range serverFiles {
			if file == input {
				downloadFile(input)
				found = true
				break
			}
		}

		if !found {
			// Try as index
			if idx, err := strconv.Atoi(input); err == nil {
				if idx >= 0 && idx < len(serverFiles) {
					downloadFile(serverFiles[idx])
				}
			}
		}
	}
}

func printMenu() {
	fmt.Println("\n" + strings.Repeat("=", 50))
	fmt.Println("Files on server:")
	for i, file := range serverFiles {
		fmt.Printf("  (%d)\t%s\n", i, file)
	}
	fmt.Println(strings.Repeat("=", 50))
	fmt.Println("Enter filename or number to download, or 'q' to quit")
}

func downloadFile(filename string) {
	currentFilename = filename
	downloadStarted = time.Time{}

	fmt.Printf("\nRequesting \"%s\" from server...\n", filename)

	// Send request packet
	if serverLink != nil {
		if err := serverLink.SendPacket([]byte(filename)); err != nil {
			debug.Log(debug.DebugError, "Error sending request", "error", err)
		}
	}
}

// ============================================================
// UTILITY FUNCTIONS
// ============================================================

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

func loadConfig(configpath string) *common.ReticulumConfig {
	cfg := common.DefaultConfig()

	// Add default interface if none configured
	if len(cfg.Interfaces) == 0 {
		cfg.Interfaces = make(map[string]*common.InterfaceConfig)

		// For local testing, always set target to enable bidirectional communication
		// Server talks to client port, client talks to server port
		targetHost := fmt.Sprintf("127.0.0.1:%d", *targetPort)

		cfg.Interfaces["UDPInterface"] = &common.InterfaceConfig{
			Type:       "UDPInterface",
			Enabled:    true,
			Address:    fmt.Sprintf("127.0.0.1:%d", *listenPort),
			TargetHost: targetHost,
			Name:       "UDPInterface",
		}
	}

	return cfg
}

func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func divmod(numerator, denominator int) (quotient, remainder int) {
	quotient = numerator / denominator
	remainder = numerator % denominator
	return
}
