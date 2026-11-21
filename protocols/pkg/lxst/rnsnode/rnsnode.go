// SPDX-License-Identifier: LicenseRef-Reticulum
package rnsnode

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/interfaces"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
)

const DefaultLocalPort = 37428
const DefaultUDPPort = 4242

const (
	KindUDP    = "udp"
	KindLocal  = "local"
	KindConfig = "config"

	recallPollInterval = 100 * time.Millisecond
)

func DefaultConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".reticulum-go")
}

// Options selects how RGESP attaches to Reticulum.
type Options struct {
	Kind       string
	ListenPort int
	TargetPort int
	LocalPort  int
	ConfigDir  string
	Persist    bool
}

// Session is a running transport.
type Session struct {
	Transport *transport.Transport
}

// Start brings up Reticulum for a phone process.
// KindUDP is an isolated hop for tests. KindLocal attaches to a shared instance.
// KindConfig loads a reticulum config directory and registers interfaces from it.
func Start(opts Options) (*Session, error) {
	if opts.Kind == "" {
		opts.Kind = KindUDP
	}
	if opts.LocalPort <= 0 {
		opts.LocalPort = DefaultLocalPort
	}
	if opts.ListenPort <= 0 {
		opts.ListenPort = DefaultUDPPort
	}
	if opts.TargetPort <= 0 {
		opts.TargetPort = opts.ListenPort
	}
	switch opts.Kind {
	case KindConfig:
		return startFromConfig(opts)
	case KindLocal:
		return startLocal(opts)
	case KindUDP:
		return startUDP(opts)
	default:
		return nil, fmt.Errorf("unknown interface kind %q", opts.Kind)
	}
}

func startFromConfig(opts Options) (*Session, error) {
	dir := opts.ConfigDir
	if dir == "" {
		dir = DefaultConfigDir()
	}
	if _, err := EnsureDefaultConfig(dir); err != nil {
		return nil, err
	}
	cfg, err := LoadOfficialDir(dir)
	if err != nil {
		return nil, err
	}
	if opts.Persist {
		cfg.InMemoryPathTable = false
		cfg.InMemoryKnownDestinations = false
	}
	if cfg.ConfigPath == "" && dir != "" {
		cfg.ConfigPath = filepath.Join(dir, "config")
	}
	t, err := startTransport(cfg)
	if err != nil {
		return nil, err
	}
	for name, ic := range cfg.Interfaces {
		if ic == nil || !ic.Enabled {
			continue
		}
		iface, err := interfaces.NewFromConfig(name, ic)
		if err != nil {
			if cfg.PanicOnInterfaceErr {
				return nil, fmt.Errorf("interface %s: %w", name, err)
			}
			continue
		}
		enableDuplex(iface)
		if err := registerIface(t, iface); err != nil {
			return nil, err
		}
		if err := iface.Start(); err != nil {
			if cfg.PanicOnInterfaceErr {
				return nil, fmt.Errorf("start %s: %w", name, err)
			}
		}
	}
	return &Session{Transport: t}, nil
}

func startLocal(opts Options) (*Session, error) {
	cfg := common.DefaultConfig()
	cfg.ShareInstance = false
	cfg.EnableTransport = false
	cfg.SharedInstancePort = opts.LocalPort
	cfg.InMemoryPathTable = !opts.Persist
	cfg.InMemoryKnownDestinations = !opts.Persist
	if opts.Persist && opts.ConfigDir != "" {
		cfg.ConfigPath = filepath.Join(opts.ConfigDir, "config")
	}
	t, err := startTransport(cfg)
	if err != nil {
		return nil, err
	}
	iface, err := interfaces.NewLocalClientInterface(opts.LocalPort, "", false, nil)
	if err != nil {
		return nil, err
	}
	iface.In = true
	iface.Out = true
	if err := registerIface(t, iface); err != nil {
		return nil, err
	}
	if err := iface.Start(); err != nil {
		return nil, err
	}
	if !iface.IsOnline() {
		return nil, fmt.Errorf("local shared instance at 127.0.0.1:%d is offline", opts.LocalPort)
	}
	return &Session{Transport: t}, nil
}

func startUDP(opts Options) (*Session, error) {
	cfg := common.DefaultConfig()
	cfg.ShareInstance = false
	cfg.InMemoryPathTable = true
	cfg.InMemoryKnownDestinations = true
	t, err := startTransport(cfg)
	if err != nil {
		return nil, err
	}
	addr := fmt.Sprintf("127.0.0.1:%d", opts.ListenPort)
	target := fmt.Sprintf("127.0.0.1:%d", opts.TargetPort)
	iface, err := interfaces.NewUDPInterface("UDPInterface", addr, target, true)
	if err != nil {
		return nil, err
	}
	iface.In = true
	iface.Out = true
	if err := registerIface(t, iface); err != nil {
		return nil, err
	}
	if err := iface.Start(); err != nil {
		return nil, err
	}
	return &Session{Transport: t}, nil
}

func startTransport(cfg *common.ReticulumConfig) (*transport.Transport, error) {
	t := transport.NewTransport(cfg)
	if err := t.Start(); err != nil {
		return nil, err
	}
	_ = t.InitializePathRequestHandler()
	return t, nil
}

func registerIface(t *transport.Transport, iface interfaces.Interface) error {
	if t == nil || iface == nil {
		return fmt.Errorf("missing transport or interface")
	}
	name := iface.GetName()
	if err := t.RegisterInterface(name, iface); err != nil {
		return err
	}
	markSharedInstanceClient(t, iface)
	return nil
}

func markSharedInstanceClient(t *transport.Transport, iface interfaces.Interface) {
	lc, ok := iface.(*interfaces.LocalClientInterface)
	if !ok || !lc.IsSharedInstanceClient() {
		return
	}
	t.SetConnectedToSharedInstance(true)
	lc.SetDisconnectHooks(
		func() { t.SetConnectedToSharedInstance(false) },
		func() { t.SetConnectedToSharedInstance(true) },
	)
}

type pathTransport interface {
	HasPath([]byte) bool
	RequestPath([]byte, string, []byte, bool) error
}

func WaitRecall(t pathTransport, hashes [][]byte, timeout time.Duration) (*identity.Identity, error) {
	return WaitRecallContext(context.Background(), t, hashes, timeout)
}

func WaitRecallContext(ctx context.Context, t pathTransport, hashes [][]byte, timeout time.Duration) (*identity.Identity, error) {
	if t == nil {
		return nil, fmt.Errorf("missing transport")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for _, h := range hashes {
		if len(h) == 0 {
			continue
		}
		if !t.HasPath(h) {
			_ = t.RequestPath(h, "", nil, false)
		}
	}
	deadline := time.Now().Add(timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	timer := time.NewTimer(recallPollInterval)
	defer timer.Stop()
	for {
		for _, h := range hashes {
			if len(h) == 0 {
				continue
			}
			if remote, err := identity.Recall(h); err == nil {
				return remote, nil
			}
		}
		if !time.Now().Before(deadline) {
			break
		}
		select {
		case <-ctx.Done():
			if ctx.Err() == context.Canceled {
				return nil, ctx.Err()
			}
			return nil, fmt.Errorf("could not recall identity")
		case <-timer.C:
			timer.Reset(recallPollInterval)
		}
	}
	return nil, fmt.Errorf("could not recall identity")
}

func enableDuplex(iface interfaces.Interface) {
	switch v := iface.(type) {
	case *interfaces.UDPInterface:
		v.In = true
		v.Out = true
	case *interfaces.TCPClientInterface:
		v.In = true
		v.Out = true
	case *interfaces.TCPServerInterface:
		v.In = true
		v.Out = true
	case *interfaces.AutoInterface:
		v.In = true
		v.Out = true
	}
}
