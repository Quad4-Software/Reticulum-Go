// SPDX-License-Identifier: LicenseRef-Reticulum
package gorrcd

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/destination"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/interfaces"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/link"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/rrc"
	"github.com/fxamacker/cbor/v2"
)

var _ rrc.HubPolicy = (*Service)(nil)

type Service struct {
	cfg    Config
	log    *slog.Logger
	hub    *rrc.Hub
	dest   *destination.Destination
	tr     *transport.Transport
	id     *identity.Identity
	sender []byte
	rooms  *RoomRegistry
	trust  *Trust
	stats  *Stats
	res    *Resources
	ifaces []interfaces.Interface

	mu              sync.Mutex
	awaiting        map[ID]time.Time
	lastTrafficBump time.Time
	stop            chan struct{}
	statusRefresh   chan struct{}
	wg              sync.WaitGroup
}

func NewService(cfg Config, log *slog.Logger) *Service {
	s := &Service{
		cfg:           cfg,
		log:           log,
		rooms:         NewRoomRegistry(cfg.RoomRegistryPath, cfg.RoomInviteTimeoutS),
		trust:         NewTrust(),
		stats:         NewStats(),
		awaiting:      make(map[ID]time.Time),
		stop:          make(chan struct{}),
		statusRefresh: make(chan struct{}, 1),
	}
	s.res = NewResources(s)
	return s
}

func (s *Service) Start() error {
	if err := s.trust.Load(s.cfg.TrustedIdentities, s.cfg.BannedIdentities); err != nil {
		return fmt.Errorf("trust lists: %w", err)
	}
	if s.cfg.RoomRegistryPath != "" {
		reg, err := LoadRoomRegistry(s.cfg.RoomRegistryPath)
		if err != nil {
			s.log.Error("room registry load failed", "error", err)
		} else {
			s.rooms.ReplaceAll(reg)
		}
	}
	id, err := identity.FromFile(s.cfg.IdentityPath)
	if err != nil {
		return fmt.Errorf("identity: %w", err)
	}
	s.id = id
	s.sender = append([]byte(nil), id.Hash()...)

	tr, ifaces, err := startTransport(s.cfg, s.log)
	if err != nil {
		return err
	}
	s.tr = tr
	s.ifaces = ifaces

	dest, err := rrc.NewHubDestination(id, tr)
	if err != nil {
		return err
	}
	app, err := cbor.Marshal(map[string]any{"proto": "rrc", "v": 1, "hub": s.cfg.HubName})
	if err == nil {
		dest.SetDefaultAppData(app)
	}
	s.dest = dest

	hub, err := rrc.NewHub(tr, dest, rrc.HubConfig{
		Name:                   s.cfg.HubName,
		Version:                Version,
		Limits:                 s.cfg.HubLimits(),
		IncludeMemberList:      s.cfg.IncludeJoinedMemberList,
		EnableResourceTransfer: s.cfg.EnableResourceTransfer,
		MaxResourceBytes:       s.cfg.MaxResourceBytes,
		Policy:                 s,
		OnInboundBytes: func(n int) {
			s.stats.Inc("pkts_in", 1)
			if n > 0 {
				s.stats.Inc("bytes_in", uint64(n)) // #nosec G115 -- n is packet length
			}
		},
		OnBadPacket: func(error) {
			s.stats.Inc("pkts_bad", 1)
		},
		OnRateLimited: func() {
			s.stats.Inc("rate_limited", 1)
			s.bumpStatus()
		},
		Handlers: rrc.HubHandlers{
			OnHello: func(_ []byte, _ *rrc.HelloBody, _ *rrc.Envelope) {
				s.bumpStatus()
			},
			OnClose: func(_ []byte) {
				s.bumpStatus()
			},
			OnMsg: func(_ []byte, env *rrc.Envelope) {
				if env == nil {
					return
				}
				switch env.Type {
				case rrc.TypeMsg:
					s.stats.Inc("msgs_forwarded", 1)
					s.bumpStatusForTraffic()
				case rrc.TypeNotice:
					s.stats.Inc("notices_forwarded", 1)
				case rrc.TypeAction:
					s.stats.Inc("actions_forwarded", 1)
				}
			},
		},
	})
	if err != nil {
		return err
	}
	s.hub = hub
	hub.Start()

	if s.cfg.AnnounceOnStart {
		s.wg.Add(1)
		go s.announceWhenOnline()
	}
	s.startWorkers()
	s.log.Info("hub running", "dest", fmt.Sprintf("%x", dest.GetHash()), "name", s.cfg.HubName)
	if !s.cfg.ServiceMode {
		printOperatorSummary(buildOperatorSummary(s))
	}
	s.startStatusReporter()
	return s.writeReady()
}

func (s *Service) writeReady() error {
	if s.cfg.ReadyPath == "" || s.dest == nil {
		return nil
	}
	raw, err := json.Marshal(map[string]string{
		"hub_hash": hex.EncodeToString(s.dest.GetHash()),
		"name":     s.cfg.HubName,
		"version":  Version,
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
	if s.hub != nil {
		s.hub.Close()
	}
	s.res.Clear()
	for _, iface := range s.ifaces {
		_ = iface.Stop()
	}
	if s.tr != nil {
		_ = s.tr.Close()
	}
}

func (s *Service) DestinationHash() []byte {
	if s.dest == nil {
		return nil
	}
	return s.dest.GetHash()
}

func (s *Service) startWorkers() {
	if s.cfg.AnnouncePeriodS > 0 {
		s.wg.Add(1)
		go s.announceLoop()
	}
	if s.cfg.PingIntervalS > 0 {
		s.wg.Add(1)
		go s.pingLoop()
	}
	if s.cfg.RoomRegistryPruneIntervalS > 0 && s.cfg.RoomRegistryPruneAfterS > 0 {
		s.wg.Add(1)
		go s.pruneLoop()
	}
	if s.cfg.EnableResourceTransfer {
		s.wg.Add(1)
		go s.resourceLoop()
	}
}

func (s *Service) announceOnce() {
	if s.dest == nil {
		return
	}
	if err := s.dest.Announce(false, nil, nil); err != nil {
		s.log.Warn("announce failed", "error", err)
		return
	}
	s.stats.Inc("announces", 1)
	s.log.Debug("announce sent")
}

// announceOnlineWait bounds how long the startup announce waits for an
// interface to report online. Client interfaces dial asynchronously, so
// announcing the moment Start returns races the connects and fails.
const announceOnlineWait = 30 * time.Second

func (s *Service) announceWhenOnline() {
	defer s.wg.Done()
	deadline := time.NewTimer(announceOnlineWait)
	defer deadline.Stop()
	poll := time.NewTicker(200 * time.Millisecond)
	defer poll.Stop()
	for {
		if s.anyInterfaceOnline() {
			s.announceOnce()
			return
		}
		select {
		case <-s.stop:
			return
		case <-deadline.C:
			s.log.Warn("no interface online yet, announcing anyway")
			s.announceOnce()
			return
		case <-poll.C:
		}
	}
}

func (s *Service) anyInterfaceOnline() bool {
	for _, iface := range s.ifaces {
		if iface != nil && iface.IsOnline() {
			return true
		}
	}
	return false
}

func (s *Service) announceLoop() {
	defer s.wg.Done()
	for {
		period := time.Duration(s.cfg.AnnouncePeriodS * float64(time.Second))
		if period <= 0 {
			period = time.Second
		}
		select {
		case <-s.stop:
			return
		case <-time.After(period):
			if s.cfg.AnnouncePeriodS > 0 {
				s.announceOnce()
			}
		}
	}
}

func (s *Service) pingLoop() {
	defer s.wg.Done()
	for {
		interval := time.Duration(s.cfg.PingIntervalS * float64(time.Second))
		if interval <= 0 {
			interval = time.Second
		}
		select {
		case <-s.stop:
			return
		case <-time.After(interval):
			s.pingOnce()
		}
	}
}

func (s *Service) pingOnce() {
	if s.hub == nil || s.cfg.PingIntervalS <= 0 {
		return
	}
	timeout := s.cfg.PingTimeoutS
	now := time.Now()
	var drop [][]byte
	var ping [][]byte
	s.mu.Lock()
	for _, h := range s.hub.ActivePeerHashes() {
		id, ok := idFrom(h)
		if !ok {
			continue
		}
		if t, ok := s.awaiting[id]; ok && timeout > 0 && now.Sub(t).Seconds() > timeout {
			drop = append(drop, h)
			continue
		}
		if _, ok := s.awaiting[id]; !ok {
			s.awaiting[id] = now
			ping = append(ping, h)
		}
	}
	s.mu.Unlock()
	for _, h := range drop {
		s.hub.Disconnect(h)
	}
	for _, h := range ping {
		if err := s.hub.SendPing(h, float64(now.Unix())); err == nil {
			s.stats.Inc("pings_out", 1)
		}
	}
}

func (s *Service) pruneLoop() {
	defer s.wg.Done()
	for {
		interval := time.Duration(s.cfg.RoomRegistryPruneIntervalS * float64(time.Second))
		if interval <= 0 {
			interval = time.Hour
		}
		select {
		case <-s.stop:
			return
		case <-time.After(interval):
			occupied := map[string]struct{}{}
			if s.hub != nil {
				for _, r := range s.hub.OccupiedRooms() {
					if r.Count > 0 {
						occupied[r.Name] = struct{}{}
					}
				}
			}
			removed := s.rooms.PruneUnused(s.cfg.RoomRegistryPruneAfterS, occupied, s.stats.started)
			for _, name := range removed {
				s.log.Info("pruned unused registered room", "room", name)
			}
			if len(removed) > 0 {
				s.bumpStatus()
			}
		}
	}
}

func (s *Service) resourceLoop() {
	defer s.wg.Done()
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-t.C:
			s.res.Cleanup()
		}
	}
}

func (s *Service) Reload(peer []byte) {
	path := s.cfg.ConfigPath
	if path == "" {
		s.notice(peer, "", "reload failed: config_path not set or missing")
		return
	}
	if _, err := os.Stat(path); err != nil {
		s.notice(peer, "", "reload failed: config_path not set or missing")
		return
	}
	newCfg, err := LoadConfigFile(path)
	if err != nil {
		s.notice(peer, "", "reload failed: config parse error: "+err.Error())
		return
	}
	newCfg.ConfigPath = path
	if err := s.trust.Load(newCfg.TrustedIdentities, newCfg.BannedIdentities); err != nil {
		s.notice(peer, "", "reload failed: identity list parse error: "+err.Error())
		return
	}
	reg, err := LoadRoomRegistry(newCfg.RoomRegistryPath)
	if err != nil {
		s.notice(peer, "", "reload failed: "+err.Error())
		return
	}
	oldTrusted, oldBanned := s.trust.Counts()
	oldRooms := s.rooms.Count()
	s.mu.Lock()
	s.cfg = newCfg
	s.mu.Unlock()
	s.rooms.ReplaceAll(reg)
	s.rooms.SetInviteTimeout(newCfg.RoomInviteTimeoutS)
	if s.hub != nil {
		s.hub.ApplyRuntime(newCfg.HubName, Version, newCfg.IncludeJoinedMemberList, newCfg.EnableResourceTransfer, newCfg.MaxResourceBytes, newCfg.HubLimits())
	}
	newTrusted, newBanned := s.trust.Counts()
	s.notice(peer, "", fmt.Sprintf(
		"reloaded: trusted=%d->%d banned=%d->%d registered_rooms=%d->%d\npolicy: max_nick_bytes=%d",
		oldTrusted, newTrusted, oldBanned, newBanned, oldRooms, s.rooms.Count(), newCfg.MaxNickBytes,
	))
	s.bumpStatus()
}

func (s *Service) OnLink(lnk *link.Link) {
	s.res.OnLink(lnk)
}

func (s *Service) OnIdentified(peer []byte) error {
	id, ok := idFrom(peer)
	if !ok {
		return nil
	}
	if s.trust.IsBanned(id) {
		return errString(rrcdBanned)
	}
	return nil
}

func (s *Service) AfterWelcome(peer []byte) {
	if s.cfg.Greeting != "" {
		s.noticeChunksOrResource(peer, "", s.cfg.Greeting, rrc.ResourceKindMOTD)
	}
	s.bumpStatus()
}

func (s *Service) AllowJoin(peer []byte, room string, body any) error {
	id, ok := idFrom(peer)
	if !ok {
		return errString("invalid peer")
	}
	return s.rooms.AllowJoin(room, id, body, s.trust.IsTrusted(id))
}

func (s *Service) AfterJoin(peer []byte, room string) {
	id, _ := idFrom(peer)
	s.stats.Inc("joins", 1)
	s.rooms.Ensure(room, id, true)
	s.rooms.ConsumeInvite(room, id)
	s.rooms.Touch(room)
	_ = s.rooms.Persist(room)
	s.notice(peer, room, s.rooms.InfoLine(room))
	s.bumpStatus()
}

func (s *Service) AfterPart(peer []byte, room string) {
	s.stats.Inc("parts", 1)
	s.rooms.Touch(room)
	_ = s.rooms.Persist(room)
	s.bumpStatus()
}

func (s *Service) AllowContent(peer []byte, env *rrc.Envelope) error {
	room := rrc.NormalizeRoom(env.Room)
	id, ok := idFrom(peer)
	if !ok {
		return errString("invalid peer")
	}
	isMember := s.hub != nil && s.hub.IsMember(peer, room)
	return s.rooms.AllowContent(room, id, isMember, s.trust.IsTrusted(id))
}

func (s *Service) Intercept(peer []byte, env *rrc.Envelope) bool {
	text, ok := rrc.BodyAsString(env.Body)
	if !ok {
		return false
	}
	text = stringsTrim(text)
	if text == "" || text[0] != '/' {
		return false
	}
	s.handleCommand(peer, env.Room, text)
	return true
}

func (s *Service) OnPong(peer []byte) {
	id, ok := idFrom(peer)
	if !ok {
		return
	}
	s.stats.Inc("pongs_in", 1)
	s.mu.Lock()
	delete(s.awaiting, id)
	s.mu.Unlock()
}

func (s *Service) OnResourceEnvelope(peer []byte, env *rrc.Envelope) error {
	return s.res.Add(peer, env)
}

func (s *Service) snapshotConfig() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

func (s *Service) bumpStatusForTraffic() {
	s.mu.Lock()
	if time.Since(s.lastTrafficBump) < time.Second {
		s.mu.Unlock()
		return
	}
	s.lastTrafficBump = time.Now()
	s.mu.Unlock()
	s.bumpStatus()
}

func stringsTrim(s string) string {
	i := 0
	j := len(s)
	for i < j && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r') {
		i++
	}
	for j > i && (s[j-1] == ' ' || s[j-1] == '\t' || s[j-1] == '\n' || s[j-1] == '\r') {
		j--
	}
	return s[i:j]
}
