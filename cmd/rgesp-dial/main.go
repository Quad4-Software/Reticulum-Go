// SPDX-License-Identifier: LicenseRef-Reticulum
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/debug"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/destination"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
	"github.com/Quad4-Software/reticulum-go-protocols/internal/lxstcli"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/call"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/proto"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/rnsnode"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/sandbox"
)

var (
	serverMode   = flag.Bool("server", false, "listen for incoming calls")
	destHex      = flag.String("dest", "", "identity or destination hash to dial")
	listenPort   = flag.Int("listen-port", rnsnode.DefaultUDPPort, "UDP listen port")
	targetPort   = flag.Int("target-port", rnsnode.DefaultUDPPort, "UDP target port")
	ifaceKind    = flag.String("if", rnsnode.KindConfig, "reticulum attach: config, local, or udp")
	localPort    = flag.Int("local-port", rnsnode.DefaultLocalPort, "shared instance TCP port")
	identityPath = flag.String("identity", "", "identity file, default ~/.config/rgesp-dial/identity")
	noAudio      = flag.Bool("no-audio", false, "send silence instead of capturing a device")
	profileName  = flag.String("profile", "mq", "call profile: ulbw vlbw lbw mq hq shq ll ull")
	configPath   = flag.String("config", "", "optional key = value file")
	rnsConfig    = flag.String("rnsconfig", "", "reticulum config directory, default ~/.reticulum-go")
	autoAnswer   = flag.Duration("auto-answer", 0, "auto-answer delay for incoming calls, 0 is off")
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: rgesp-dial [hash]\n\n")
		fmt.Fprintf(os.Stderr, "  rgesp-dial HASH     dial that identity or dest hash\n")
		fmt.Fprintf(os.Stderr, "  rgesp-dial          listen for incoming calls\n\n")
		fmt.Fprintf(os.Stderr, "Uses ~/.reticulum-go by default. Flags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if err := applyOptionalConfig(); err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}
	debug.Init()
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func run() error {
	*destHex = destFromArgs(*destHex, flag.Args())
	if *destHex == "" {
		*serverMode = true
	}

	if *identityPath == "" {
		path, err := defaultIdentityPath()
		if err != nil {
			return fmt.Errorf("identity: %w", err)
		}
		*identityPath = path
	}
	if err := os.MkdirAll(filepath.Dir(*identityPath), appcli.DirMode); err != nil && filepath.Dir(*identityPath) != "." && filepath.Dir(*identityPath) != "" {
		return fmt.Errorf("identity: %w", err)
	}

	rw := []string{filepath.Dir(*identityPath), os.TempDir()}
	cfgDir := *rnsConfig
	if cfgDir == "" && (*ifaceKind == "" || *ifaceKind == rnsnode.KindConfig) {
		cfgDir = rnsnode.DefaultConfigDir()
	}
	if cfgDir != "" {
		rw = append(rw, cfgDir)
	}
	if _, err := sandbox.Apply(sandbox.Paths{ReadWrite: rw}); err != nil {
		return fmt.Errorf("sandbox: %w", err)
	}

	id, err := appcli.LoadOrCreateIdentity(*identityPath)
	if err != nil {
		return fmt.Errorf("identity: %w", err)
	}

	t, err := startTransport()
	if err != nil {
		return fmt.Errorf("transport: %w", err)
	}

	dest, err := destination.New(id, destination.In, destination.Single, proto.AppName, t, proto.AspectName)
	if err != nil {
		return fmt.Errorf("destination: %w", err)
	}
	dest.AcceptsLinks(true)

	profileID, ok := proto.LookupProfile(*profileName)
	if !ok {
		return fmt.Errorf("unknown profile %q", *profileName)
	}

	cfg := call.Config{
		Identity:   id,
		Events:     dialEvents(),
		UseAudio:   !*noAudio,
		DuplexIO:   true,
		Profile:    profileID,
		Mode:       proto.DefaultMode,
		AutoAnswer: *autoAnswer,
	}

	sb := call.NewSwitchboard(t, cfg, nil)
	sb.Bind(dest)

	if err := dest.Announce(false, nil, nil); err != nil {
		fmt.Fprintf(os.Stderr, "announce: %v\n", err)
	}

	fmt.Printf("identity %x\n", id.Hash())
	fmt.Printf("listening on %x\n", dest.GetHash())

	if *serverMode {
		appcli.WaitSignal()
		return nil
	}

	fmt.Fprintf(os.Stderr, "looking up %s\n", *destHex)
	remote, err := resolveRemote(t, *destHex)
	if err != nil {
		return fmt.Errorf("resolve: %w", err)
	}
	fmt.Fprintf(os.Stderr, "resolve: %x\n", remote.Hash())

	caller := call.NewCall(t, cfg)
	if !sb.Occupy(caller) {
		return fmt.Errorf("already in a call")
	}
	ctx, cancel := context.WithTimeout(context.Background(), call.DefaultWaitTime)
	defer cancel()
	if err := caller.Dial(ctx, remote); err != nil {
		sb.Release(caller)
		return fmt.Errorf("dial: %w", err)
	}
	fmt.Println("call active")
	appcli.WaitSignal()
	_ = caller.Hangup("user hangup")
	return nil
}

func applyOptionalConfig() error {
	if env := os.Getenv("RGESP_CONFIG"); *configPath == "" && env != "" {
		*configPath = env
	}
	if *configPath == "" {
		return nil
	}
	vals, err := loadConfigFile(*configPath)
	if err != nil {
		return err
	}
	set := map[string]bool{}
	flag.Visit(func(f *flag.Flag) {
		set[f.Name] = true
	})
	return applyConfigMap(vals, set)
}

func dialEvents() call.Events {
	return call.Events{
		OnRinging: func(c *call.Call) {
			fmt.Println("ringing")
			if c.Incoming() {
				fmt.Println("fingerprint", call.Fingerprint(c.RemoteIdentity()))
			}
		},
		OnAnswered: func(*call.Call) {
			fmt.Println("answered")
		},
		OnBusy: func(*call.Call) {
			fmt.Println("busy")
		},
		OnRejected: func(*call.Call) {
			fmt.Println("rejected")
		},
		OnEnded: func(_ *call.Call, reason string) {
			fmt.Printf("ended: %s\n", reason)
		},
	}
}

func startTransport() (*transport.Transport, error) {
	kind := *ifaceKind
	if kind == "" {
		kind = rnsnode.KindConfig
	}
	cfgDir := *rnsConfig
	if kind == rnsnode.KindConfig && cfgDir == "" {
		cfgDir = rnsnode.DefaultConfigDir()
	}
	sess, err := rnsnode.Start(rnsnode.Options{
		Kind:       kind,
		ListenPort: *listenPort,
		TargetPort: *targetPort,
		LocalPort:  *localPort,
		ConfigDir:  cfgDir,
		Persist:    kind == rnsnode.KindConfig,
	})
	if err != nil {
		return nil, err
	}
	return sess.Transport, nil
}
