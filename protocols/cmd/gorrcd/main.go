// SPDX-License-Identifier: LicenseRef-Reticulum
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Quad4-Software/reticulum-go-protocols/internal/gorrcd"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	fs := flag.NewFlagSet("gorrcd", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", gorrcd.DefaultConfigPath(), "path to gorrcd.toml")
	configDir := fs.String("configdir", "", "Reticulum-Go config directory (default ~/.reticulum-go)")
	identityPath := fs.String("identity", gorrcd.DefaultIdentityPath(), "hub identity file")
	roomsPath := fs.String("room-registry", gorrcd.DefaultRoomsPath(), "rooms.toml path")
	noAnnounce := fs.Bool("no-announce", false, "disable announce on start")
	announcePeriod := fs.Float64("announce-period", -1, "periodic announce interval seconds")
	hubName := fs.String("hub-name", "", "hub name in WELCOME")
	greeting := fs.String("greeting", "", "MOTD NOTICE after WELCOME")
	includeMembers := fs.Bool("include-joined-member-list", false, "include member list in JOINED")
	maxRooms := fs.Uint64("max-rooms", 0, "max rooms per session")
	maxNick := fs.Uint64("max-nick-bytes", 0, "max nickname UTF-8 bytes")
	maxRoomName := fs.Uint64("max-room-name-bytes", 0, "max room name UTF-8 bytes")
	rateLimit := fs.Uint64("rate-limit-msgs-per-minute", 0, "per-link message rate limit")
	maxBody := fs.Uint64("max-msg-body-bytes", 0, "max message body UTF-8 bytes")
	pingInterval := fs.Float64("ping-interval", -1, "hub PING interval seconds")
	pingTimeout := fs.Float64("ping-timeout", -1, "close if PONG is late")
	logLevel := fs.String("log-level", "", "DEBUG, INFO, WARNING, ERROR")
	logFile := fs.String("log-file", "", "log file path")
	udpListen := fs.String("udp-listen", "", "UDP listen host:port (skips AutoInterface)")
	udpForward := fs.String("udp-forward", "", "UDP forward host:port")
	readyFile := fs.String("ready-file", "", "write hub hash JSON when ready")
	serviceMode := fs.Bool("service", false, "headless mode without operator banner or live status")
	showVersion := fs.Bool("version", false, "print version and exit")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *showVersion {
		fmt.Println(gorrcd.VersionLine())
		return 0
	}

	created, err := gorrcd.FirstRun(*configPath, *identityPath, *roomsPath, *configDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gorrcd: first run: %v\n", err)
		return 1
	}
	if created {
		gorrcd.PrintFirstRunNotice(*configPath, *identityPath, *roomsPath, gorrcd.ResolveRNSConfigDir(*configDir))
		return 0
	}

	cfg, err := gorrcd.LoadConfigFile(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gorrcd: load config: %v\n", err)
		return 1
	}
	cfg.ConfigPath = *configPath
	if *configDir != "" {
		cfg.ConfigDir = *configDir
	}
	if *identityPath != gorrcd.DefaultIdentityPath() {
		cfg.IdentityPath = *identityPath
	}
	if *roomsPath != gorrcd.DefaultRoomsPath() {
		cfg.RoomRegistryPath = *roomsPath
	}
	if *noAnnounce {
		cfg.AnnounceOnStart = false
	}
	if *announcePeriod >= 0 {
		cfg.AnnouncePeriodS = *announcePeriod
	}
	if *hubName != "" {
		cfg.HubName = *hubName
	}
	if *greeting != "" {
		cfg.Greeting = *greeting
	}
	if *includeMembers {
		cfg.IncludeJoinedMemberList = true
	}
	if *maxRooms > 0 {
		cfg.MaxRoomsPerSession = *maxRooms
	}
	if *maxNick > 0 {
		cfg.MaxNickBytes = *maxNick
	}
	if *maxRoomName > 0 {
		cfg.MaxRoomNameBytes = *maxRoomName
	}
	if *rateLimit > 0 {
		cfg.RateLimitMsgsPerMinute = *rateLimit
	}
	if *maxBody > 0 {
		cfg.MaxMsgBodyBytes = *maxBody
	}
	if *pingInterval >= 0 {
		cfg.PingIntervalS = *pingInterval
	}
	if *pingTimeout >= 0 {
		cfg.PingTimeoutS = *pingTimeout
	}
	if *logLevel != "" {
		cfg.LogLevel = *logLevel
	}
	if *logFile != "" {
		cfg.LogFile = *logFile
	}
	if *udpListen != "" {
		cfg.UDPListen = *udpListen
	}
	if *udpForward != "" {
		cfg.UDPForward = *udpForward
	}
	if *readyFile != "" {
		cfg.ReadyPath = *readyFile
	}
	if *serviceMode {
		cfg.ServiceMode = true
	}

	log, closer, err := gorrcd.ConfigureLogging(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gorrcd: logging: %v\n", err)
		return 1
	}
	if closer != nil {
		defer closer.Close()
	}

	svc := gorrcd.NewService(cfg, log)
	if err := svc.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "gorrcd: start: %v\n", err)
		return 1
	}
	defer svc.Stop()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	return 0
}
