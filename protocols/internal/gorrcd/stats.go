// SPDX-License-Identifier: LicenseRef-Reticulum
package gorrcd

import (
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/rrc"
)

type Stats struct {
	started      time.Time
	bytesIn      atomic.Uint64
	bytesOut     atomic.Uint64
	pktsIn       atomic.Uint64
	pktsBad      atomic.Uint64
	rateLimited  atomic.Uint64
	errorsSent   atomic.Uint64
	joins        atomic.Uint64
	parts        atomic.Uint64
	msgsFwd      atomic.Uint64
	noticesFwd   atomic.Uint64
	actionsFwd   atomic.Uint64
	pingsIn      atomic.Uint64
	pongsIn      atomic.Uint64
	pingsOut     atomic.Uint64
	pongsOut     atomic.Uint64
	announces    atomic.Uint64
	resSent      atomic.Uint64
	resRecv      atomic.Uint64
	resRej       atomic.Uint64
	resBytesSent atomic.Uint64
	resBytesRecv atomic.Uint64
}

func NewStats() *Stats {
	return &Stats{started: time.Now()}
}

func (s *Stats) Inc(key string, n uint64) {
	if n == 0 {
		n = 1
	}
	switch key {
	case "bytes_in":
		s.bytesIn.Add(n)
	case "bytes_out":
		s.bytesOut.Add(n)
	case "pkts_in":
		s.pktsIn.Add(n)
	case "pkts_bad":
		s.pktsBad.Add(n)
	case "rate_limited":
		s.rateLimited.Add(n)
	case "errors_sent":
		s.errorsSent.Add(n)
	case "joins":
		s.joins.Add(n)
	case "parts":
		s.parts.Add(n)
	case "msgs_forwarded":
		s.msgsFwd.Add(n)
	case "notices_forwarded":
		s.noticesFwd.Add(n)
	case "actions_forwarded":
		s.actionsFwd.Add(n)
	case "pings_in":
		s.pingsIn.Add(n)
	case "pongs_in":
		s.pongsIn.Add(n)
	case "pings_out":
		s.pingsOut.Add(n)
	case "pongs_out":
		s.pongsOut.Add(n)
	case "announces":
		s.announces.Add(n)
	case "resources_sent":
		s.resSent.Add(n)
	case "resources_received":
		s.resRecv.Add(n)
	case "resources_rejected":
		s.resRej.Add(n)
	case "resource_bytes_sent":
		s.resBytesSent.Add(n)
	case "resource_bytes_received":
		s.resBytesRecv.Add(n)
	}
}

func (s *Stats) Format(hub *rrc.Hub, cfg Config, trust *Trust, rooms *RoomRegistry) string {
	uptime := time.Since(s.started).Seconds()
	trusted, banned := trust.Counts()
	occupied := hub.OccupiedRooms()
	sort.Slice(occupied, func(i, j int) bool {
		if occupied[i].Count != occupied[j].Count {
			return occupied[i].Count > occupied[j].Count
		}
		return occupied[i].Name < occupied[j].Name
	})
	top := occupied
	if len(top) > 5 {
		top = top[:5]
	}
	var topS []string
	for _, r := range top {
		topS = append(topS, fmt.Sprintf("%s:%d", r.Name, r.Count))
	}
	memberships := 0
	for _, r := range occupied {
		memberships += r.Count
	}
	var b strings.Builder
	fmt.Fprintf(&b, "gorrcd %s stats\n", Version)
	fmt.Fprintf(&b, "uptime_s=%.1f\n", uptime)
	fmt.Fprintf(&b, "clients_total=%d clients_identified=%d clients_welcomed=%d\n",
		hub.PeerCount(), hub.PeerCount(), hub.WelcomedCount())
	fmt.Fprintf(&b, "rooms=%d memberships=%d\n", len(occupied), memberships)
	if len(topS) > 0 {
		fmt.Fprintf(&b, "top_rooms=%s\n", strings.Join(topS, ", "))
	}
	fmt.Fprintf(&b, "trust: trusted=%d banned=%d\n", trusted, banned)
	fmt.Fprintf(&b, "limits: rate_limit_msgs_per_minute=%d max_rooms_per_session=%d max_room_name_bytes=%d max_nick_bytes=%d\n",
		cfg.RateLimitMsgsPerMinute, cfg.MaxRoomsPerSession, cfg.MaxRoomNameBytes, cfg.MaxNickBytes)
	fmt.Fprintf(&b, "features: ping_interval_s=%v ping_timeout_s=%v announce_on_start=%v announce_period_s=%v\n",
		cfg.PingIntervalS, cfg.PingTimeoutS, cfg.AnnounceOnStart, cfg.AnnouncePeriodS)
	fmt.Fprintf(&b, "io: pkts_in=%d pkts_bad=%d bytes_in=%d bytes_out=%d\n",
		s.pktsIn.Load(), s.pktsBad.Load(), s.bytesIn.Load(), s.bytesOut.Load())
	fmt.Fprintf(&b, "events: joins=%d parts=%d msgs_fwd=%d notices_fwd=%d actions_fwd=%d errors_sent=%d rate_limited=%d\n",
		s.joins.Load(), s.parts.Load(), s.msgsFwd.Load(), s.noticesFwd.Load(), s.actionsFwd.Load(), s.errorsSent.Load(), s.rateLimited.Load())
	fmt.Fprintf(&b, "pings: in=%d out=%d pongs: in=%d out=%d\n",
		s.pingsIn.Load(), s.pingsOut.Load(), s.pongsIn.Load(), s.pongsOut.Load())
	fmt.Fprintf(&b, "resources: sent=%d received=%d rejected=%d bytes_sent=%d bytes_received=%d\n",
		s.resSent.Load(), s.resRecv.Load(), s.resRej.Load(), s.resBytesSent.Load(), s.resBytesRecv.Load())
	_ = rooms
	return b.String()
}
