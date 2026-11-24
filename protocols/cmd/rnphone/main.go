// SPDX-License-Identifier: LicenseRef-Reticulum
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/debug"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/destination"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/internal/lxstcli"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxst/audio/io"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxst/audio/opus"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxst/call"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxst/hardware"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxst/history"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxst/phonebook"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxst/proto"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxst/rnsnode"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxst/sandbox"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxst/sounds"
)

const (
	version             = "0.5.4"
	interactiveRingTime = 30 * time.Second
	interactiveWaitTime = 60 * time.Second
)

var (
	configDir  = flag.String("config", "", "rnphone config directory")
	rnsConfig  = flag.String("rnsconfig", "", "reticulum config directory")
	service    = flag.Bool("s", false, "run as service without stdin")
	listDev    = flag.Bool("l", false, "list audio devices")
	showVer    = flag.Bool("version", false, "print version")
	ifaceKind  = flag.String("if", "local", "interface: local, udp, or config")
	listenPort = flag.Int("listen-port", rnsnode.DefaultUDPPort, "UDP listen port")
	targetPort = flag.Int("target-port", rnsnode.DefaultUDPPort, "UDP target port")
	localPort  = flag.Int("local-port", rnsnode.DefaultLocalPort, "shared instance TCP port")
	noAudio    = flag.Bool("no-audio", false, "do not open an audio device")
	profile    = flag.String("profile", "mq", "call profile")
	callMode   = flag.String("call-mode", "full", "call mode: full, half")
	rateLimit  = flag.Bool("rate-limit", false, "enable per-peer incoming call rate limit")
)

func main() {
	flag.BoolVar(service, "service", false, "run as service without stdin")
	flag.BoolVar(listDev, "list-devices", false, "list audio devices")
	flag.Parse()
	if *showVer {
		fmt.Println("rnphone", version)
		return
	}
	if *listDev {
		listDevices()
		return
	}
	debug.Init()

	dir, err := resolveConfigDir(*configDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config dir: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(filepath.Join(dir, "storage"), appcli.DirMode); err != nil {
		fmt.Fprintf(os.Stderr, "storage: %v\n", err)
		os.Exit(1)
	}
	if err := sounds.Install(dir); err != nil {
		fmt.Fprintf(os.Stderr, "sounds: %v\n", err)
		os.Exit(1)
	}

	kind := *ifaceKind
	if *rnsConfig != "" && kind == rnsnode.KindLocal {
		kind = rnsnode.KindConfig
	}
	rw := []string{dir, os.TempDir()}
	if *rnsConfig != "" {
		rw = append(rw, *rnsConfig)
	} else if kind == rnsnode.KindConfig {
		rw = append(rw, rnsnode.DefaultConfigDir())
	}
	if _, err := sandbox.Apply(sandbox.Paths{ReadWrite: rw}); err != nil {
		fmt.Fprintf(os.Stderr, "sandbox: %v\n", err)
		os.Exit(1)
	}

	book := phonebook.New()
	iniPath := filepath.Join(dir, "config")
	var ini phonebook.Config
	if _, err := os.Stat(iniPath); err == nil {
		ini, err = phonebook.LoadINI(iniPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "config: %v\n", err)
			os.Exit(1)
		}
		if err := phonebook.ApplyPolicy(book, ini); err != nil {
			fmt.Fprintf(os.Stderr, "phonebook: %v\n", err)
			os.Exit(1)
		}
	}

	id, err := appcli.LoadOrCreateIdentity(filepath.Join(dir, "identity"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "identity: %v\n", err)
		os.Exit(1)
	}

	rns, err := rnsnode.Start(rnsnode.Options{
		Kind:       kind,
		ListenPort: *listenPort,
		TargetPort: *targetPort,
		LocalPort:  *localPort,
		ConfigDir:  *rnsConfig,
		Persist:    kind == rnsnode.KindConfig,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "transport: %v\n", err)
		os.Exit(1)
	}

	dest, err := destination.New(id, destination.In, destination.Single, proto.AppName, rns.Transport, proto.AspectName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "destination: %v\n", err)
		os.Exit(1)
	}

	modeName := *callMode
	modeID, ok := proto.LookupMode(modeName)
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown call mode %q\n", modeName)
		os.Exit(1)
	}
	profileID, ok := proto.LookupProfile(*profile)
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown profile %q\n", *profile)
		os.Exit(1)
	}

	ui := &session{
		book:     book,
		log:      history.New(filepath.Join(dir, "storage", "calls.jsonl")),
		id:       id,
		dest:     dest,
		trans:    rns.Transport,
		wait:     interactiveWaitTime,
		callMode: modeID,
	}
	cfg := call.DefaultConfig()
	cfg.Identity = id
	cfg.Events = ui.events()
	cfg.UseAudio = !*noAudio
	cfg.DuplexIO = true
	if cfg.UseAudio && !opus.Native() {
		fmt.Fprintf(os.Stderr, "rnphone: native Opus is required for audio (build with CGO)\n")
		os.Exit(1)
	}
	cfg.Profile = profileID
	cfg.Mode = ui.callMode
	cfg.RingTime = interactiveRingTime
	cfg.WaitTime = interactiveWaitTime
	cfg.AllowPolicy = book.Policy()
	cfg.Allowed = book.AllowedHashes()
	cfg.Blocked = book.BlockedHashes()
	cfg.AllowFunc = book.IsAllowed
	if ini.Ringtone != "" {
		cfg.RingtonePath = filepath.Join(dir, ini.Ringtone)
	}
	cfg.Speaker = ini.Speaker
	cfg.Microphone = ini.Microphone
	cfg.Ringer = ini.Ringer
	if ini.AutoAnswer != "" {
		if d, err := time.ParseDuration(ini.AutoAnswer); err == nil {
			cfg.AutoAnswer = d
		}
	}
	ui.wait = cfg.WaitTime

	phone := call.NewPhone(rns.Transport, dest, cfg)
	if *rateLimit {
		phone.Switchboard().SetRateLimiter(call.NewRateLimiter(0, 0))
	}
	phone.StartAnnounce(context.Background())
	ui.phone = phone
	keypad := hardware.NewKeypad()
	lcd := hardware.NewLCD()
	if hw := ini.Raw["hardware"]; hw != nil {
		if err := keypad.Enable(hw["keypad"]); err != nil {
			fmt.Fprintf(os.Stderr, "hardware keypad: %v\n", err)
		}
		if err := lcd.Enable(hw["display"]); err != nil {
			fmt.Fprintf(os.Stderr, "hardware display: %v\n", err)
		}
	}
	_ = keypad
	_ = lcd

	fmt.Printf("identity %x\n", id.Hash())
	fmt.Printf("listening on %x\n", dest.GetHash())
	fmt.Printf("default mode %s profile %s\n", proto.ModeName(ui.callMode), *profile)

	if *service {
		appcli.WaitSignal()
		phone.Stop()
		return
	}
	ui.run()
	phone.Stop()
}

func listDevices() {
	if _, err := sandbox.Apply(sandbox.Paths{}); err != nil {
		fmt.Fprintf(os.Stderr, "sandbox: %v\n", err)
		os.Exit(1)
	}
	devs, err := io.ListDevices()
	if err != nil {
		fmt.Fprintf(os.Stderr, "devices: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Output")
	for _, d := range devs {
		if !d.Capture {
			fmt.Println(" ", d.Name)
		}
	}
	fmt.Println("Input")
	for _, d := range devs {
		if d.Capture {
			fmt.Println(" ", d.Name)
		}
	}
}
