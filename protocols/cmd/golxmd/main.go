// SPDX-License-Identifier: LicenseRef-Reticulum
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Quad4-Software/reticulum-go-protocols/internal/golxmd"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxmf"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	fs := flag.NewFlagSet("golxmd", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	configDir := fs.String("config", "", "path to golxmd config directory")
	rnsConfig := fs.String("rnsconfig", "", "path to Reticulum config directory (default ~/.reticulum-go)")
	propagationNode := fs.Bool("propagation-node", false, "run an LXMF propagation node")
	onInbound := fs.String("on-inbound", "", "executable to run when a message is received")
	verbose := fs.Int("v", 0, "increase log verbosity")
	quiet := fs.Int("q", 0, "decrease log verbosity")
	serviceMode := fs.Bool("service", false, "suppress console output (logs still go to file)")
	selfCheck := fs.Bool("self-check", false, "verify home directories and storage read/write")
	logFile := fs.String("log-file", "", "log file path (default ~/.golxmd/logfile)")
	noLogFile := fs.Bool("no-log-file", false, "disable file logging")
	showStatus := fs.Bool("status", false, "display node status")
	showPeers := fs.Bool("peers", false, "display peered nodes")
	syncPeer := fs.String("sync", "", "request sync with the specified peer")
	unpeer := fs.String("break", "", "break peering with the specified peer")
	timeout := fs.Float64("timeout", 0, "timeout in seconds for query operations")
	remote := fs.String("remote", "", "remote propagation node destination hash")
	identityPath := fs.String("identity", "", "path to identity used for remote requests")
	exampleConfig := fs.Bool("exampleconfig", false, "print verbose configuration example and exit")
	showVersion := fs.Bool("version", false, "print version and exit")
	udpListen := fs.String("udp-listen", "", "UDP listen host:port (skips AutoInterface)")
	udpForward := fs.String("udp-forward", "", "UDP forward host:port")
	readyFile := fs.String("ready-file", "", "write ready JSON when daemon is running")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *showVersion {
		fmt.Println(golxmd.Version)
		return 0
	}
	if *exampleConfig {
		fmt.Print(lxmf.DefaultConfigText)
		return 0
	}

	home := golxmd.DefaultHome()
	if *configDir != "" {
		home = *configDir
	}
	cfgPath := filepath.Join(home, "config")
	identPath := filepath.Join(home, "identity")
	if *identityPath != "" {
		identPath = *identityPath
	}
	storageDir := filepath.Join(home, "storage")
	messagesDir := filepath.Join(storageDir, "messages")

	if *selfCheck {
		results := golxmd.RunSelfCheck(golxmd.SelfCheckOptions{
			Home:         home,
			RNSConfigDir: *rnsConfig,
			UDPListen:    *udpListen,
			UDPForward:   *udpForward,
		})
		golxmd.PrintSelfCheckResults(results)
		if !golxmd.SelfCheckPassed(results) {
			return 1
		}
		return 0
	}

	if *showStatus || *showPeers || *syncPeer != "" || *unpeer != "" {
		to := 5 * time.Second
		if *timeout > 0 {
			to = time.Duration(*timeout * float64(time.Second))
		}
		remoteOpts := golxmd.RemoteOptions{
			ConfigDir:    home,
			RNSConfigDir: golxmd.ResolveRNSConfigDir(*rnsConfig),
			IdentityPath: identPath,
			RemoteHash:   *remote,
			Timeout:      to,
			Verbosity:    *verbose,
			Quietness:    *quiet,
		}
		if *syncPeer != "" {
			if err := golxmd.RequestSync(*syncPeer, remoteOpts); err != nil {
				fmt.Fprintf(os.Stderr, "golxmd: %v\n", err)
				return exitCodeForRemoteErr(err)
			}
			fmt.Printf("Sync requested for peer %s\n", *syncPeer)
			return 0
		}
		if *unpeer != "" {
			if err := golxmd.RequestUnpeer(*unpeer, remoteOpts); err != nil {
				fmt.Fprintf(os.Stderr, "golxmd: %v\n", err)
				return exitCodeForRemoteErr(err)
			}
			fmt.Printf("Broke peering with %s\n", *unpeer)
			return 0
		}
		stats, err := golxmd.QueryStatus(remoteOpts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "golxmd: %v\n", err)
			return 1
		}
		golxmd.PrintStatus(stats, *showStatus, *showPeers)
		return 0
	}

	created, err := golxmd.FirstRun(home, cfgPath, identPath, storageDir, messagesDir, *rnsConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "golxmd: first run: %v\n", err)
		return 1
	}
	if created {
		golxmd.PrintFirstRunNotice(cfgPath, identPath, storageDir, golxmd.ResolveRNSConfigDir(*rnsConfig))
		return 0
	}

	lxmfCfg, err := lxmf.LoadConfig(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "golxmd: load config: %v\n", err)
		return 1
	}
	if *propagationNode {
		lxmfCfg.Propagation.EnableNode = true
	}
	if *onInbound != "" {
		lxmfCfg.LXMF.OnInbound = *onInbound
	}

	logPath := golxmd.DefaultLogPath()
	if *logFile != "" {
		logPath = *logFile
	}
	if *noLogFile {
		logPath = ""
	}

	consoleLogs := *verbose > 0 && !*serviceMode
	log, closer, err := golxmd.ConfigureLogging(golxmd.LogConfig{
		Level:     lxmfCfg.Logging.Level,
		Console:   consoleLogs,
		File:      logPath,
		Verbosity: *verbose,
		Quietness: *quiet,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "golxmd: logging: %v\n", err)
		return 1
	}
	if closer != nil {
		defer closer.Close()
	}

	readyPath := golxmd.DefaultReadyPath()
	if *readyFile != "" {
		readyPath = *readyFile
	}

	svc := golxmd.NewService(golxmd.Config{
		HomeDir:          home,
		ConfigPath:       cfgPath,
		IdentityPath:     identPath,
		StorageDir:       storageDir,
		MessagesDir:      messagesDir,
		IgnoredPath:      filepath.Join(home, "ignored"),
		AllowedPath:      filepath.Join(home, "allowed"),
		ReadyPath:        readyPath,
		RNSConfigDir:     golxmd.ResolveRNSConfigDir(*rnsConfig),
		LogFile:          logPath,
		ServiceMode:      *serviceMode,
		UDPListen:        *udpListen,
		UDPForward:       *udpForward,
		ForcePropagation: *propagationNode,
		Verbosity:        *verbose,
		Quietness:        *quiet,
		OnInbound:        *onInbound,
	}, lxmfCfg, log)

	if err := svc.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "golxmd: start: %v\n", err)
		return 1
	}
	defer svc.Stop()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	return 0
}

func exitCodeForRemoteErr(err error) int {
	msg := err.Error()
	switch {
	case msg == "remote received no identity":
		return 203
	case msg == "access denied":
		return 204
	case msg == "invalid data received by remote":
		return 205
	case msg == "the requested peer was not found":
		return 206
	default:
		return 1
	}
}
