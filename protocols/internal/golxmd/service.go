// SPDX-License-Identifier: LicenseRef-Reticulum
package golxmd

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/interfaces"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxmf"
)

const (
	deferredJobsDelay = 10 * time.Second
	jobsInterval      = 5 * time.Second
)

type Config struct {
	HomeDir          string
	ConfigPath       string
	IdentityPath     string
	StorageDir       string
	MessagesDir      string
	IgnoredPath      string
	AllowedPath      string
	ReadyPath        string
	RNSConfigDir     string
	LogFile          string
	ServiceMode      bool
	UDPListen        string
	UDPForward       string
	ForcePropagation bool
	Verbosity        int
	Quietness        int
	OnInbound        string
}

type Service struct {
	cfg     Config
	lxmfCfg lxmf.Config
	log     *slog.Logger

	router *lxmf.Router
	tr     *transport.Transport
	ifaces []interfaces.Interface

	mu               sync.Mutex
	lastPeerAnnounce time.Time
	lastNodeAnnounce time.Time
	stop             chan struct{}
	wg               sync.WaitGroup
}

func NewService(cfg Config, lxmfCfg lxmf.Config, log *slog.Logger) *Service {
	return &Service{
		cfg:     cfg,
		lxmfCfg: lxmfCfg,
		log:     log,
		stop:    make(chan struct{}),
	}
}

func (s *Service) Start() error {
	id, err := identity.FromFile(s.cfg.IdentityPath)
	if err != nil {
		return fmt.Errorf("identity: %w", err)
	}

	tr, ifaces, err := startTransport(TransportConfig{
		RNSConfigDir: s.cfg.RNSConfigDir,
		UDPListen:    s.cfg.UDPListen,
		UDPForward:   s.cfg.UDPForward,
	}, s.log)
	if err != nil {
		return err
	}
	s.tr = tr
	s.ifaces = ifaces

	onInbound := s.cfg.OnInbound
	if onInbound == "" {
		onInbound = s.lxmfCfg.LXMF.OnInbound
	}

	router, err := lxmf.NewRouter(id, tr, lxmf.RouterOptions{
		Config:      s.lxmfCfg,
		StoragePath: s.cfg.StorageDir,
		MessagesDir: s.cfg.MessagesDir,
		OnInbound:   onInbound,
	})
	if err != nil {
		return fmt.Errorf("router: %w", err)
	}
	s.router = router

	stampCost := s.lxmfCfg.LXMF.StampCost
	var stampCostPtr *int
	if stampCost > 0 {
		stampCostPtr = &stampCost
	}
	if _, err := router.RegisterDelivery(s.lxmfCfg.LXMF.DisplayName, stampCostPtr); err != nil {
		return fmt.Errorf("register delivery: %w", err)
	}

	for _, h := range loadHashList(s.cfg.IgnoredPath) {
		router.IgnoreDestination(h)
	}

	runPN := s.cfg.ForcePropagation || s.lxmfCfg.Propagation.EnableNode
	if runPN {
		if err := router.EnablePropagation(); err != nil {
			return fmt.Errorf("enable propagation: %w", err)
		}
		if s.lxmfCfg.Propagation.AuthRequired {
			for _, h := range loadHashList(s.cfg.AllowedPath) {
				router.AllowDestination(h)
			}
		}
	}

	router.Start()
	s.startAnnounceWorkers()
	if s.cfg.ReadyPath != "" {
		if s.lxmfCfg.LXMF.AnnounceAtStart {
			s.announceDeliveryOnce()
		}
		if runPN && s.lxmfCfg.Propagation.AnnounceAtStart {
			s.announcePropagationOnce()
		}
	}
	s.log.Debug("golxmd running",
		"delivery", fmt.Sprintf("%x", deliveryHash(router)),
		"propagation", fmt.Sprintf("%x", propagationHash(router)),
	)
	printOperatorSummary(buildOperatorSummary(s))
	s.startStatusReporter()
	return s.writeReady()
}

func deliveryHash(r *lxmf.Router) []byte {
	if d := r.DeliveryDestination(); d != nil {
		return d.GetHash()
	}
	return nil
}

func propagationHash(r *lxmf.Router) []byte {
	if d := r.PropagationDestination(); d != nil {
		return d.GetHash()
	}
	return nil
}

func (s *Service) writeReady() error {
	if s.cfg.ReadyPath == "" || s.router == nil {
		return nil
	}
	raw, err := json.Marshal(map[string]string{
		"delivery_hash":    hex.EncodeToString(deliveryHash(s.router)),
		"propagation_hash": hex.EncodeToString(propagationHash(s.router)),
		"version":          Version,
	})
	if err != nil {
		return err
	}
	return atomicWrite(s.cfg.ReadyPath, raw, 0o600)
}

func (s *Service) Stop() {
	select {
	case <-s.stop:
	default:
		close(s.stop)
	}
	s.wg.Wait()
	if s.router != nil {
		s.router.Stop()
	}
	for _, iface := range s.ifaces {
		_ = iface.Stop()
	}
	if s.tr != nil {
		_ = s.tr.Close()
	}
}

func (s *Service) startAnnounceWorkers() {
	now := time.Now()
	s.lastPeerAnnounce = now
	s.lastNodeAnnounce = now

	s.wg.Go(func() {
		select {
		case <-s.stop:
			return
		case <-time.After(deferredJobsDelay):
		}
		if s.lxmfCfg.LXMF.AnnounceAtStart {
			s.announceDeliveryOnce()
		}
		if s.lxmfCfg.Propagation.EnableNode && s.lxmfCfg.Propagation.AnnounceAtStart {
			s.announcePropagationOnce()
		}
		s.lastPeerAnnounce = time.Now()
		s.lastNodeAnnounce = time.Now()
	})

	s.wg.Add(1)
	go s.announceLoop()
}

func (s *Service) announceLoop() {
	defer s.wg.Done()
	t := time.NewTicker(jobsInterval)
	defer t.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-t.C:
			s.tickAnnounces()
		}
	}
}

func (s *Service) tickAnnounces() {
	now := time.Now()
	if s.lxmfCfg.LXMF.AnnounceIntervalMinutes > 0 {
		interval := time.Duration(s.lxmfCfg.LXMF.AnnounceIntervalMinutes) * time.Minute
		if now.Sub(s.lastPeerAnnounce) >= interval {
			s.announceDeliveryOnce()
			s.lastPeerAnnounce = now
		}
	}
	if s.lxmfCfg.Propagation.EnableNode && s.lxmfCfg.Propagation.AnnounceIntervalMinutes > 0 {
		interval := time.Duration(s.lxmfCfg.Propagation.AnnounceIntervalMinutes) * time.Minute
		if now.Sub(s.lastNodeAnnounce) >= interval {
			s.announcePropagationOnce()
			s.lastNodeAnnounce = now
		}
	}
}

func (s *Service) announceDeliveryOnce() {
	if s.router == nil {
		return
	}
	dest := s.router.DeliveryDestination()
	if dest == nil {
		return
	}
	// Upstream announces [display_name, stamp_cost, [SF_COMPRESSION]]
	// where the cost is included only when 0 < cost < 255.
	announceCost := int64(-1)
	if c := s.lxmfCfg.LXMF.StampCost; c > 0 && c < 255 {
		announceCost = int64(c)
	}
	if appData, err := lxmf.EncodeAnnounceAppDataV5WithFeatures(
		s.lxmfCfg.LXMF.DisplayName, announceCost, []byte{lxmf.SFCompression},
	); err == nil {
		dest.SetDefaultAppData(appData)
	}
	if err := dest.Announce(false, nil, nil); err != nil {
		s.log.Warn("delivery announce failed", "error", err)
		return
	}
	s.log.Debug("delivery announce sent")
}

func (s *Service) announcePropagationOnce() {
	if s.router == nil {
		return
	}
	dest := s.router.PropagationDestination()
	if dest == nil {
		return
	}
	nodeName := s.lxmfCfg.Propagation.NodeName
	if nodeName == "" {
		nodeName = "Anonymous Propagation Node"
	}
	transfer := int(s.lxmfCfg.Propagation.PropagationTransferMaxAcceptedKB)
	syncLimit := int(s.lxmfCfg.Propagation.PropagationSyncMaxAcceptedKB)
	nodeState := !s.lxmfCfg.Propagation.FromStaticOnly
	appData, err := lxmf.EncodePNAnnounceAppData(
		time.Now().Unix(),
		nodeState,
		transfer,
		syncLimit,
		s.lxmfCfg.Propagation.PropagationStampCostTarget,
		s.lxmfCfg.Propagation.PropagationStampCostFlexibility,
		s.lxmfCfg.Propagation.PeeringCost,
		nodeName,
	)
	if err != nil {
		s.log.Warn("propagation announce app data failed", "error", err)
		return
	}
	dest.SetDefaultAppData(appData)
	if err := dest.Announce(false, nil, nil); err != nil {
		s.log.Warn("propagation announce failed", "error", err)
		return
	}
	s.log.Debug("propagation announce sent")
}

func loadHashList(path string) [][]byte {
	path = expandPath(path)
	if path == "" {
		return nil
	}
	f, err := os.Open(filepath.Clean(path)) // #nosec G304 -- operator list path
	if err != nil {
		return nil
	}
	defer f.Close()
	var out [][]byte
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if len(line) != 2*lxmf.DestinationLength {
			continue
		}
		h, err := hex.DecodeString(line)
		if err != nil || len(h) != lxmf.DestinationLength {
			continue
		}
		out = append(out, h)
	}
	return out
}
