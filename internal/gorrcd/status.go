// SPDX-License-Identifier: LicenseRef-Reticulum
package gorrcd

import (
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/interfaces"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/rrc"
)

// OperatorSummary is a concise operator-facing status snapshot.
type OperatorSummary struct {
	VersionLine   string
	HubName       string
	Home          string
	ConfigPath    string
	IdentityPath  string
	RoomsPath     string
	RNSConfigDir  string
	LogFile       string
	Transport     string
	Address       string
	HubHash       string
	PeerCount     int
	WelcomedCount int
	RoomCount     int
	Memberships   int
	RegistryRooms int
	MsgsForwarded uint64
	Joins         uint64
	RateLimited   uint64
	UptimeSeconds float64
}

func buildOperatorSummary(s *Service) OperatorSummary {
	cfg := s.snapshotConfig()
	o := OperatorSummary{
		VersionLine:   VersionLine(),
		HubName:       cfg.HubName,
		Home:          DefaultHome(),
		ConfigPath:    expandPath(cfg.ConfigPath),
		IdentityPath:  expandPath(cfg.IdentityPath),
		RoomsPath:     expandPath(cfg.RoomRegistryPath),
		RNSConfigDir:  ResolveRNSConfigDir(cfg.ConfigDir),
		LogFile:       expandPath(cfg.LogFile),
		RegistryRooms: s.rooms.Count(),
	}
	o.Transport, o.Address = transportSummary(cfg, s.ifaces)
	if s.dest != nil {
		o.HubHash = formatHubHash(s.dest.GetHash())
	}
	if s.hub != nil {
		o.PeerCount = s.hub.PeerCount()
		o.WelcomedCount = s.hub.WelcomedCount()
		occupied := s.hub.OccupiedRooms()
		o.RoomCount = len(occupied)
		for _, r := range occupied {
			o.Memberships += r.Count
		}
	}
	if s.stats != nil {
		o.UptimeSeconds = time.Since(s.stats.started).Seconds()
		o.MsgsForwarded = s.stats.msgsFwd.Load()
		o.Joins = s.stats.joins.Load()
		o.RateLimited = s.stats.rateLimited.Load()
	}
	return o
}

func transportSummary(cfg Config, ifaces []interfaces.Interface) (label, address string) {
	if cfg.UDPListen != "" {
		label = "UDPInterface"
		fwd := udpAddr(cfg.UDPForward)
		if fwd == "" {
			fwd = "(none)"
		}
		address = fmt.Sprintf("%s -> %s", udpAddr(cfg.UDPListen), fwd)
		return label, address
	}
	if len(ifaces) == 0 {
		return "none", ""
	}
	label = ifaces[0].GetName()
	switch label {
	case "Local shared instance", "LocalInterface":
		address = "reticulum shared instance"
	case "AutoInterface":
		address = "multicast UDP"
	default:
		address = label
	}
	return label, address
}

func formatHubHash(hash []byte) string {
	if len(hash) == 0 {
		return ""
	}
	return prettyHubHash(hex.EncodeToString(hash))
}

func normalizeHubHashHex(s string) string {
	s = stringsTrimHash(s)
	s = strings.ReplaceAll(s, ".", "")
	s = strings.ReplaceAll(s, ":", "")
	return strings.ToLower(s)
}

func prettyHubHash(s string) string {
	s = normalizeHubHashHex(s)
	if len(s) != 2*rrc.IdentityLength {
		return stringsTrimHash(s)
	}
	return s
}

func stringsTrimHash(s string) string {
	i := 0
	j := len(s)
	for i < j && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	for j > i && (s[j-1] == ' ' || s[j-1] == '\t') {
		j--
	}
	return s[i:j]
}

func prettyTime(seconds float64) string {
	if seconds < 0 {
		seconds = 0
	}
	s := int(seconds)
	if s < 60 {
		return fmt.Sprintf("%ds", s)
	}
	m := s / 60
	s %= 60
	if m < 60 {
		return fmt.Sprintf("%dm%ds", m, s)
	}
	h := m / 60
	m %= 60
	return fmt.Sprintf("%dh%dm", h, m)
}

func printOperatorSummary(o OperatorSummary) {
	opConsole.println()
	opConsole.printf("%s %s", statusBold("gorrcd"), o.VersionLine)
	if o.HubName != "" {
		opConsole.printf(" (%s)", o.HubName)
	}
	opConsole.println()
	printKV("Home", o.Home)
	printKV("Config", o.ConfigPath)
	printKV("Identity", o.IdentityPath)
	if o.RoomsPath != "" {
		printKV("Rooms", o.RoomsPath)
	}
	if o.RNSConfigDir != "" {
		printKV("RNS config", o.RNSConfigDir)
	}
	if o.LogFile != "" {
		printKV("Log", o.LogFile)
	}
	printKV("Transport", o.Transport)
	if o.Address != "" {
		printKV("Address", o.Address)
	}
	if o.HubHash != "" {
		printKV("Hub", o.HubHash)
	} else {
		printKV("Hub", statusWarn("not ready"))
	}
	printKV("Status", statusOK("running"))

	printSection("Clients")
	printKV("Connected", fmt.Sprintf("%d", o.PeerCount))
	printKV("Welcomed", fmt.Sprintf("%d", o.WelcomedCount))

	printSection("Rooms")
	printKV("Active", fmt.Sprintf("%d (%d memberships)", o.RoomCount, o.Memberships))
	printKV("Registered", fmt.Sprintf("%d", o.RegistryRooms))
	if o.UptimeSeconds > 0 {
		printKV("Uptime", prettyTime(o.UptimeSeconds))
	}
	opConsole.println()
}

func formatOperatorStatusLine(o OperatorSummary) string {
	ts := time.Now().Format("15:04:05")
	line := fmt.Sprintf("[%s] clients %d (%d welcomed)", ts, o.PeerCount, o.WelcomedCount)
	line += fmt.Sprintf(" | rooms %d (%d members)", o.RoomCount, o.Memberships)
	if o.MsgsForwarded > 0 {
		line += fmt.Sprintf(" | msgs %d", o.MsgsForwarded)
	}
	if o.RateLimited > 0 {
		line += fmt.Sprintf(" | rate-limited %d", o.RateLimited)
	}
	if o.UptimeSeconds > 0 {
		line += fmt.Sprintf(" | uptime %s", prettyTime(o.UptimeSeconds))
	}
	return line
}

func printOperatorStatusLine(o OperatorSummary) {
	writeLiveStatusLine(statusLabel(formatOperatorStatusLine(o)))
}

func (s *Service) bumpStatus() {
	if s == nil || s.statusRefresh == nil {
		return
	}
	select {
	case s.statusRefresh <- struct{}{}:
	default:
	}
}

func (s *Service) startStatusReporter() {
	if s.cfg.ServiceMode || !s.cfg.LogConsole {
		return
	}
	s.wg.Go(func() {
		defer clearLiveStatusLine()
		t := time.NewTicker(statusInterval(opConsole.isLive()))
		defer t.Stop()
		printOperatorStatusLine(buildOperatorSummary(s))
		for {
			select {
			case <-s.stop:
				return
			case <-s.statusRefresh:
				printOperatorStatusLine(buildOperatorSummary(s))
			case <-t.C:
				printOperatorStatusLine(buildOperatorSummary(s))
			}
		}
	})
}
