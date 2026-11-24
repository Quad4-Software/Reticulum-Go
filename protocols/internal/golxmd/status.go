// SPDX-License-Identifier: LicenseRef-Reticulum
package golxmd

import (
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/interfaces"
)

// OperatorSummary is a concise operator-facing status snapshot.
type OperatorSummary struct {
	Version         string
	Home            string
	LogFile         string
	DisplayName     string
	Transport       string
	Delivery        string
	Propagation     string
	PropagationOn   bool
	DeliveryFiles   int
	StoreCount      int
	StoreBytes      int64
	StoreLimit      int64
	TotalPeers      int
	AlivePeers      int
	SyncingPeers    int
	StaticPeers     int
	StampCost       int
	PeeringCost     int
	Autopeer        bool
	TransferLimitKB float64
	UptimeSeconds   float64
}

func buildOperatorSummary(s *Service) OperatorSummary {
	out := OperatorSummary{
		Version:         Version,
		Home:            s.cfg.HomeDir,
		LogFile:         s.cfg.LogFile,
		DisplayName:     s.lxmfCfg.LXMF.DisplayName,
		Transport:       transportLabel(s.ifaces),
		Delivery:        fmt.Sprintf("%x", deliveryHash(s.router)),
		Propagation:     fmt.Sprintf("%x", propagationHash(s.router)),
		Autopeer:        s.lxmfCfg.Propagation.Autopeer,
		StampCost:       s.lxmfCfg.Propagation.PropagationStampCostTarget,
		PeeringCost:     s.lxmfCfg.Propagation.PeeringCost,
		TransferLimitKB: s.lxmfCfg.Propagation.PropagationTransferMaxAcceptedKB,
	}
	out.PropagationOn = s.cfg.ForcePropagation || s.lxmfCfg.Propagation.EnableNode
	out.DeliveryFiles = countDeliveryMessages(s.cfg.MessagesDir)
	if s.router != nil && out.PropagationOn {
		stats := s.router.PropagationStats()
		if stats != nil {
			out.fillFromPropagationStats(stats)
		}
	}
	return out
}

func (o *OperatorSummary) fillFromPropagationStats(stats map[string]any) {
	ms := asMap(stats["messagestore"])
	o.StoreCount = int(asFloat(ms["count"]))
	o.StoreBytes = int64(asFloat(ms["bytes"]))
	if lim := asFloat(ms["limit"]); lim > 0 {
		o.StoreLimit = int64(lim)
	}
	o.TotalPeers = int(asFloat(stats["total_peers"]))
	o.StaticPeers = int(asFloat(stats["static_peers"]))
	o.AlivePeers = 0
	o.SyncingPeers = 0
	for _, raw := range asMap(stats["peers"]) {
		p := asMap(raw)
		if asBool(p["alive"]) {
			o.AlivePeers++
		}
		if int(asFloat(p["state"])) != 0 {
			o.SyncingPeers++
		}
	}
	o.UptimeSeconds = asFloat(stats["uptime"])
}

func countDeliveryMessages(dir string) int {
	entries, err := os.ReadDir(expandPath(dir))
	if err != nil {
		return 0
	}
	n := 0
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		name := ent.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		n++
	}
	return n
}

func transportLabel(ifaces []interfaces.Interface) string {
	if len(ifaces) == 0 {
		return "none"
	}
	return ifaces[0].GetName()
}

func printOperatorSummary(o OperatorSummary) {
	_, _ = fmt.Fprintln(colorOut())
	_, _ = fmt.Fprintf(colorOut(), "%s %s", statusBold("golxmd"), o.Version)
	if o.DisplayName != "" {
		_, _ = fmt.Fprintf(colorOut(), " (%s)", o.DisplayName)
	}
	_, _ = fmt.Fprintln(colorOut())
	printKV("Home", o.Home)
	if o.LogFile != "" {
		printKV("Log", o.LogFile)
	}
	printKV("Transport", o.Transport)
	if isValidDestHashHex(o.Delivery) {
		printKV("Delivery", prettyHex(o.Delivery))
	} else {
		printKV("Delivery", statusWarn("not registered"))
	}
	if o.PropagationOn {
		if isValidDestHashHex(o.Propagation) {
			printKV("Propagation", prettyHex(o.Propagation))
		} else {
			printKV("Propagation", statusWarn("not ready"))
		}
	} else {
		printKV("Propagation", statusWarn("disabled"))
	}
	printKV("Status", statusOK("running"))

	printSection("Delivery")
	printKV("Inbox files", fmt.Sprintf("%d", o.DeliveryFiles))

	if o.PropagationOn {
		printSection("Propagation store")
		util := 0.0
		if o.StoreLimit > 0 {
			util = math.Round((float64(o.StoreBytes)/float64(o.StoreLimit))*10000) / 100
		}
		limitStr := prettySize(float64(o.StoreLimit))
		if o.StoreLimit == 0 {
			limitStr = "unlimited"
		}
		printKV("Messages", fmt.Sprintf("%d (%s, %.1f%% of %s)", o.StoreCount, prettySize(float64(o.StoreBytes)), util, limitStr))
		printKV("Stamp cost", fmt.Sprintf("%d (peering %d)", o.StampCost, o.PeeringCost))
		printKV("Transfer limit", prettySize(o.TransferLimitKB*1000))

		printSection("Peers")
		autopeer := "off"
		if o.Autopeer {
			autopeer = "on"
		}
		printKV("Autopeer", autopeer)
		printKV("Known", fmt.Sprintf("%d total (%d static)", o.TotalPeers, o.StaticPeers))
		printKV("Reachable", fmt.Sprintf("%d up, %d unreachable", o.AlivePeers, o.TotalPeers-o.AlivePeers))
		if o.SyncingPeers > 0 {
			printKV("Sync", statusLabel(fmt.Sprintf("%d peer(s) syncing", o.SyncingPeers)))
		} else if o.TotalPeers > 0 {
			printKV("Sync", statusOK("idle"))
		} else {
			printKV("Sync", statusWarn("discovering peers"))
		}
		if o.UptimeSeconds > 0 {
			printKV("Uptime", prettyTime(o.UptimeSeconds))
		}
	}
	opConsole.println()
}

func printOperatorStatusLine(o OperatorSummary) {
	ts := time.Now().Format("15:04:05")
	line := fmt.Sprintf("[%s]", ts)
	if o.PropagationOn {
		line += fmt.Sprintf(" store %d msgs (%s)", o.StoreCount, prettySize(float64(o.StoreBytes)))
		line += fmt.Sprintf(" | peers %d (%d up", o.TotalPeers, o.AlivePeers)
		if o.SyncingPeers > 0 {
			line += fmt.Sprintf(", %d syncing", o.SyncingPeers)
		}
		line += ")"
	} else {
		line += fmt.Sprintf(" delivery inbox %d files", o.DeliveryFiles)
	}
	if o.UptimeSeconds > 0 {
		line += fmt.Sprintf(" | uptime %s", prettyTime(o.UptimeSeconds))
	}
	opConsole.updateStatus(statusLabel(line))
}

func (s *Service) startStatusReporter() {
	if s.cfg.ServiceMode {
		return
	}
	s.wg.Go(func() {
		t := time.NewTicker(statusInterval(opConsole.isLive()))
		defer t.Stop()
		for {
			select {
			case <-s.stop:
				return
			case <-t.C:
				printOperatorStatusLine(buildOperatorSummary(s))
			}
		}
	})
}
