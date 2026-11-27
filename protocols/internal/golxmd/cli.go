// SPDX-License-Identifier: LicenseRef-Reticulum
package golxmd

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"git.quad4.io/libraries/msgpack/v5/pkg/msgpack"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/destination"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/interfaces"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/link"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxmf"
)

type RemoteOptions struct {
	ConfigDir    string
	RNSConfigDir string
	IdentityPath string
	RemoteHash   string
	Timeout      time.Duration
	Verbosity    int
	Quietness    int
	UDPListen    string
	UDPForward   string
}

func QueryStatus(opts RemoteOptions) (map[string]any, error) {
	resp, err := controlRequest(opts, lxmf.PathStatsGet, nil)
	if err != nil {
		return nil, err
	}
	m := asMap(resp)
	if len(m) != 0 {
		return m, nil
	}
	return nil, fmt.Errorf("empty response received")
}

func RequestSync(peerHex string, opts RemoteOptions) error {
	peer, err := decodeDestHash(peerHex)
	if err != nil {
		return err
	}
	resp, err := controlRequest(opts, lxmf.PathSyncRequest, peer)
	if err != nil {
		return err
	}
	return checkPeerResponse(resp, "sync")
}

func RequestUnpeer(peerHex string, opts RemoteOptions) error {
	peer, err := decodeDestHash(peerHex)
	if err != nil {
		return err
	}
	resp, err := controlRequest(opts, lxmf.PathUnpeerRequest, peer)
	if err != nil {
		return err
	}
	return checkPeerResponse(resp, "unpeer")
}

func checkPeerResponse(resp any, action string) error {
	switch v := resp.(type) {
	case bool:
		if v {
			return nil
		}
		return fmt.Errorf("%s request rejected", action)
	case int8:
		if v < 0 {
			return fmt.Errorf("%s request rejected", action)
		}
		return peerErrorFromCode(byte(v), action)
	case int64:
		if v < 0 || v > 255 {
			return fmt.Errorf("%s request rejected", action)
		}
		return peerErrorFromCode(byte(v), action)
	case uint8:
		return peerErrorFromCode(v, action)
	case []byte:
		if len(v) == 1 {
			return peerErrorFromCode(v[0], action)
		}
		return nil
	default:
		if resp == nil {
			return fmt.Errorf("empty response received")
		}
		return nil
	}
}

func peerErrorFromCode(code byte, action string) error {
	switch code {
	case lxmf.PeerErrorNoIdentity:
		return fmt.Errorf("remote received no identity")
	case lxmf.PeerErrorNoAccess:
		return fmt.Errorf("access denied")
	case lxmf.PeerErrorInvalidData:
		return fmt.Errorf("invalid data received by remote")
	case lxmf.PeerErrorNotFound:
		return fmt.Errorf("the requested peer was not found")
	case lxmf.PeerErrorTimeout:
		return fmt.Errorf("%s request timed out", action)
	default:
		return fmt.Errorf("%s request failed (code 0x%02x)", action, code)
	}
}

func controlRequest(opts RemoteOptions, path string, data []byte) (any, error) {
	if opts.Timeout <= 0 {
		opts.Timeout = 15 * time.Second
	}
	id, tr, cleanup, err := remoteTransport(opts)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	remoteID, err := resolveRemoteIdentity(opts, tr)
	if err != nil {
		return nil, err
	}

	controlDest, err := destination.New(remoteID, destination.Out, destination.Single, lxmf.AppName, tr, "propagation", "control")
	if err != nil {
		return nil, err
	}
	controlHash := controlDest.GetHash()

	deadline := time.Now().Add(opts.Timeout)
	if !tr.HasPath(controlHash) {
		if err := tr.RequestPath(controlHash, "", nil, true); err != nil {
			return nil, err
		}
		lastRequest := time.Now()
		for !tr.HasPath(controlHash) {
			if time.Now().After(deadline) {
				return nil, fmt.Errorf("path to remote control destination timed out")
			}
			// Re-request on a bounded interval. The first request can be
			// lost when the peer interface was not up yet, and client
			// initiated requests carry no minimum interval in python.
			if time.Since(lastRequest) >= 2*time.Second {
				_ = tr.RequestPath(controlHash, "", randomPathTag(), true)
				lastRequest = time.Now()
			}
			time.Sleep(100 * time.Millisecond)
		}
	}

	lnk := link.NewLink(controlDest, tr, nil, nil, nil)
	if err := lnk.Establish(); err != nil {
		return nil, fmt.Errorf("link request: %w", err)
	}
	lnk.Start()

	for !lnk.IsActive() {
		if time.Now().After(deadline) {
			lnk.Teardown()
			return nil, fmt.Errorf("link establishment timed out")
		}
		time.Sleep(100 * time.Millisecond)
	}

	if err := lnk.Identify(id); err != nil {
		lnk.Teardown()
		return nil, fmt.Errorf("link identify: %w", err)
	}

	receipt, err := lnk.Request(path, data, opts.Timeout)
	if err != nil {
		lnk.Teardown()
		return nil, err
	}
	for !receipt.Concluded() {
		if time.Now().After(deadline) {
			lnk.Teardown()
			return nil, fmt.Errorf("request timed out")
		}
		time.Sleep(100 * time.Millisecond)
	}
	lnk.Teardown()

	if receipt.GetStatus() == link.StatusFailed {
		return nil, fmt.Errorf("request failed")
	}
	val := receipt.GetResponseValue()
	if val == nil {
		raw := receipt.GetResponse()
		if len(raw) == 0 {
			return nil, nil
		}
		var decoded any
		if err := msgpack.Unmarshal(raw, &decoded); err == nil {
			return decoded, nil
		}
		return raw, nil
	}
	return val, nil
}

func remoteTransport(opts RemoteOptions) (*identity.Identity, *transport.Transport, func(), error) {
	identityPath := opts.IdentityPath
	if identityPath == "" {
		home := DefaultHome()
		if opts.ConfigDir != "" {
			home = expandPath(opts.ConfigDir)
		}
		identityPath = filepathJoin(home, "identity")
	}
	id, err := identity.FromFile(identityPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("identity: %w", err)
	}

	logCfg := LogConfig{Level: lxmf.LogWarning, Console: false}
	log, closer, err := ConfigureLogging(logCfg)
	if err != nil {
		return nil, nil, nil, err
	}
	_ = log

	tr, ifaces, err := startTransport(TransportConfig{
		RNSConfigDir: ResolveRNSConfigDir(opts.RNSConfigDir),
		UDPListen:    opts.UDPListen,
		UDPForward:   opts.UDPForward,
	}, log)
	if err == nil {
		waitInterfacesOnline(ifaces, 8*time.Second)
	}
	if err != nil {
		if closer != nil {
			_ = closer.Close()
		}
		return nil, nil, nil, err
	}
	cleanup := func() {
		for _, iface := range ifaces {
			_ = iface.Stop()
		}
		_ = tr.Close()
		if closer != nil {
			_ = closer.Close()
		}
	}
	return id, tr, cleanup, nil
}

func resolveRemoteIdentity(opts RemoteOptions, tr *transport.Transport) (*identity.Identity, error) {
	if opts.RemoteHash == "" {
		id, err := identity.FromFile(opts.IdentityPath)
		if err != nil {
			home := DefaultHome()
			if opts.ConfigDir != "" {
				home = expandPath(opts.ConfigDir)
			}
			id, err = identity.FromFile(filepathJoin(home, "identity"))
		}
		return id, err
	}
	destHash, err := decodeDestHash(opts.RemoteHash)
	if err != nil {
		return nil, err
	}
	if id, err := identity.Recall(destHash); err == nil && id != nil {
		return id, nil
	}
	deadline := time.Now().Add(opts.Timeout)
	lastRequest := time.Now().Add(-3 * time.Second)
	if !tr.HasPath(destHash) {
		if err := tr.RequestPath(destHash, "", randomPathTag(), true); err != nil {
			return nil, err
		}
		lastRequest = time.Now()
	}
	for {
		if id, err := identity.Recall(destHash); err == nil && id != nil {
			return id, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("could not resolve remote identity")
		}
		if !tr.HasPath(destHash) && time.Since(lastRequest) >= 2*time.Second {
			// Explicit tag bypasses the automated request throttle,
			// matching python where client initiated path requests are
			// not rate limited.
			_ = tr.RequestPath(destHash, "", randomPathTag(), true)
			lastRequest = time.Now()
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func waitInterfacesOnline(ifaces []interfaces.Interface, maxWait time.Duration) {
	if len(ifaces) == 0 {
		return
	}
	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		for _, ie := range ifaces {
			if ni, ok := ie.(common.NetworkInterface); ok && ni.IsOnline() {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// randomPathTag returns a fresh random path request tag. Python generates a
// new random tag for every client initiated path request.
func randomPathTag() []byte {
	tag := make([]byte, 16)
	if _, err := rand.Read(tag); err != nil {
		return nil
	}
	return tag
}

func decodeDestHash(s string) ([]byte, error) {
	s = normalizeDestHashHex(s)
	if len(s) != 2*lxmf.DestinationLength {
		return nil, fmt.Errorf("destination hash length must be %d characters", 2*lxmf.DestinationLength)
	}
	h, err := hex.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("invalid destination hash: %w", err)
	}
	if len(h) != lxmf.DestinationLength {
		return nil, fmt.Errorf("destination hash length must be %d characters", 2*lxmf.DestinationLength)
	}
	return h, nil
}

func normalizeDestHashHex(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ".", "")
	s = strings.ReplaceAll(s, ":", "")
	return strings.ToLower(s)
}

func isValidDestHashHex(s string) bool {
	s = normalizeDestHashHex(s)
	if len(s) != 2*lxmf.DestinationLength {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

func filepathJoin(a, b string) string {
	if strings.HasSuffix(a, string(os.PathSeparator)) {
		return a + b
	}
	return a + string(os.PathSeparator) + b
}

func PrintStatus(stats map[string]any, showStatus, showPeers bool) {
	if stats == nil {
		_, _ = fmt.Fprintln(colorOut(), statusFail("Empty response received"))
		return
	}
	destHash := asString(stats["destination_hash"])
	uptime := asFloat(stats["uptime"])
	printStatusHeader(destHash, uptime)

	if showStatus {
		printStatusDetails(stats)
	}
	if showPeers {
		if !showStatus {
			_, _ = fmt.Fprintln(colorOut())
		}
		printPeerDetails(stats)
	}
}

func printStatusDetails(s map[string]any) {
	ms := asMap(s["messagestore"])
	msBytes := asFloat(ms["bytes"])
	msLimit := asFloat(ms["limit"])
	mutil := 0.0
	if msLimit > 0 {
		mutil = math.Round((msBytes/msLimit)*10000) / 100
	}
	who := "all nodes"
	if asBool(s["from_static_only"]) {
		who = "static peers only"
	}

	peers := asMap(s["peers"])
	available := 0
	unreachable := 0
	peeredIncoming := 0.0
	peeredOutgoing := 0.0
	peeredRx := 0.0
	peeredTx := 0.0
	for _, raw := range peers {
		p := asMap(raw)
		pm := asMap(p["messages"])
		peeredIncoming += asFloat(pm["incoming"])
		peeredOutgoing += asFloat(pm["outgoing"])
		peeredRx += asFloat(p["rx_bytes"])
		peeredTx += asFloat(p["tx_bytes"])
		if asBool(p["alive"]) {
			available++
		} else {
			unreachable++
		}
	}
	clients := asMap(s["clients"])
	upi := asFloat(s["unpeered_propagation_incoming"])
	uprx := asFloat(s["unpeered_propagation_rx_bytes"])
	cprr := asFloat(clients["client_propagation_messages_received"])
	cprs := asFloat(clients["client_propagation_messages_served"])
	totalIncoming := peeredIncoming + upi + cprr
	totalRx := peeredRx + uprx
	df := 0.0
	if totalIncoming != 0 {
		df = math.Round((peeredOutgoing/totalIncoming)*100) / 100
	}

	printSection("Message store")
	printKV("Messages", fmt.Sprintf("%.0f", asFloat(ms["count"])))
	printKV("Size", fmt.Sprintf("%s (%.2f%% of %s)", prettySize(msBytes), mutil, prettySize(msLimit)))
	printKV("Stamp cost", fmt.Sprintf("%.0f (flex %.0f)", asFloat(s["target_stamp_cost"]), asFloat(s["stamp_cost_flexibility"])))
	printKV("Peering cost", fmt.Sprintf("%.0f (max remote %.0f)", asFloat(s["peering_cost"]), asFloat(s["max_peering_cost"])))
	printKV("Accepting from", who)
	printKV("Limits", fmt.Sprintf("%s message, %s sync",
		prettySize(asFloat(s["propagation_limit"])*1000), prettySize(asFloat(s["sync_limit"])*1000)))

	printSection("Peers")
	printKV("Total", fmt.Sprintf("%.0f (limit %.0f)", asFloat(s["total_peers"]), asFloat(s["max_peers"])))
	printKV("Discovered", fmt.Sprintf("%.0f", asFloat(s["discovered_peers"])))
	printKV("Static", fmt.Sprintf("%.0f", asFloat(s["static_peers"])))
	printKV("Reachable", fmt.Sprintf("%d available, %d unreachable", available, unreachable))

	printSection("Traffic")
	printKV("Received", fmt.Sprintf("%.0f messages (%s)", totalIncoming, prettySize(totalRx)))
	printKV("From peers", fmt.Sprintf("%.0f messages (%s)", peeredIncoming, prettySize(peeredRx)))
	printKV("From unpeered", fmt.Sprintf("%.0f messages (%s)", upi, prettySize(uprx)))
	printKV("To peers", fmt.Sprintf("%.0f messages (%s)", peeredOutgoing, prettySize(peeredTx)))
	printKV("Client RX", fmt.Sprintf("%.0f propagation messages", cprr))
	printKV("Client TX", fmt.Sprintf("%.0f propagation messages", cprs))
	printKV("Distribution", fmt.Sprintf("%.2f", df))
	_, _ = fmt.Fprintln(colorOut())
}

func printPeerDetails(s map[string]any) {
	peers := asMap(s["peers"])
	for peerID, raw := range peers {
		p := asMap(raw)
		ind := "  "
		t := "Unknown peer    "
		switch asString(p["type"]) {
		case "static":
			t = "Static peer     "
		case "discovered":
			t = "Discovered peer "
		}
		status := peerStatusLabel(asBool(p["alive"]))
		hops := int(asFloat(p["network_distance"]))
		hs := "hops unknown"
		if hops != 0xff {
			if hops == 1 {
				hs = "1 hop away"
			} else {
				hs = fmt.Sprintf("%d hops away", hops)
			}
		}
		lastHeard := asFloat(p["last_heard"])
		heardAgo := math.Max(float64(time.Now().Unix()-int64(lastHeard)), 0)
		pm := asMap(p["messages"])
		pk := p["peering_key"]
		pkStr := "Not generated"
		if pk != nil {
			pkStr = fmt.Sprintf("Generated, value is %v", pk)
		}
		pc := p["peering_cost"]
		psc := p["target_stamp_cost"]
		psf := p["stamp_cost_flexibility"]
		if pc == nil {
			pc = "unknown"
		}
		if psc == nil {
			psc = "unknown"
		}
		if psf == nil {
			psf = "unknown"
		}
		ls := "never synced"
		if asFloat(p["last_sync_attempt"]) != 0 {
			lsa := asFloat(p["last_sync_attempt"])
			ls = fmt.Sprintf("last synced %s ago", prettyTime(math.Max(float64(time.Now().Unix()-int64(lsa)), 0)))
		}
		_, _ = fmt.Fprintf(colorOut(), "%s%s%s\n", ind, t, prettyHex(peerID))
		name := asString(p["name"])
		if name != "" {
			printKV("Name", truncateName(name, 45))
		}
		_, _ = fmt.Fprintf(colorOut(), "%s%s %s, %s, last heard %s ago\n",
			ind+ind, statusLabel("Status:"), status, hs, prettyTime(float64(heardAgo)))
		_, _ = fmt.Fprintf(colorOut(), "%s%s Propagation %v (flex %v), peering %v\n",
			ind+ind, statusLabel("Costs:"), psc, psf, pc)
		_, _ = fmt.Fprintf(colorOut(), "%s%s %s\n", ind+ind, statusLabel("Sync key:"), pkStr)
		_, _ = fmt.Fprintf(colorOut(), "%s%s %s STR, %s LER\n",
			ind+ind, statusLabel("Speeds:"), prettySpeed(asFloat(p["str"])), prettySpeed(asFloat(p["ler"])))
		tl := p["transfer_limit"]
		sl := p["sync_limit"]
		tlStr := "Unknown"
		slStr := "unknown"
		if tl != nil {
			tlStr = prettySize(asFloat(tl) * 1000)
		}
		if sl != nil {
			slStr = prettySize(asFloat(sl) * 1000)
		}
		ar := math.Round(asFloat(p["acceptance_rate"])*10000) / 100
		_, _ = fmt.Fprintf(colorOut(), "%s%s %s message limit, %s sync limit\n", ind+ind, statusLabel("Limits:"), tlStr, slStr)
		_, _ = fmt.Fprintf(colorOut(), "%s%s %.0f offered, %.0f outgoing, %.0f incoming, %.2f%% acceptance\n",
			ind+ind, statusLabel("Messages:"), asFloat(pm["offered"]), asFloat(pm["outgoing"]), asFloat(pm["incoming"]), ar)
		_, _ = fmt.Fprintf(colorOut(), "%s%s %s received, %s sent\n",
			ind+ind, statusLabel("Traffic:"), prettySize(asFloat(p["rx_bytes"])), prettySize(asFloat(p["tx_bytes"])))
		uh := asFloat(pm["unhandled"])
		ms := ""
		if uh != 1 {
			ms = "s"
		}
		_, _ = fmt.Fprintf(colorOut(), "%s%s %.0f unhandled message%s, %s\n", ind+ind, statusLabel("Sync:"), uh, ms, ls)
		_, _ = fmt.Fprintln(colorOut())
	}
}

func sanitizeName(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "\x00", ""))
}

func asMap(v any) map[string]any {
	out := make(map[string]any)
	switch m := v.(type) {
	case map[string]any:
		for k, val := range m {
			out[normalizeDecodedKey(k)] = val
		}
	case map[any]any:
		for k, val := range m {
			out[normalizeDecodedKey(k)] = val
		}
	}
	return out
}

func normalizeDecodedKey(v any) string {
	switch k := v.(type) {
	case string:
		if len(k) == lxmf.DestinationLength {
			return hex.EncodeToString([]byte(k))
		}
		return k
	case []byte:
		if len(k) == lxmf.DestinationLength {
			return hex.EncodeToString(k)
		}
		return string(k)
	case [16]byte:
		return hex.EncodeToString(k[:])
	default:
		return fmt.Sprint(v)
	}
}

func asFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case int32:
		return float64(n)
	case int16:
		return float64(n)
	case int8:
		return float64(n)
	case uint:
		return float64(n)
	case uint64:
		return float64(n)
	case uint8:
		return float64(n)
	case uint16:
		return float64(n)
	case uint32:
		return float64(n)
	case []byte:
		return float64(len(n))
	default:
		return 0
	}
}

func asBool(v any) bool {
	switch b := v.(type) {
	case bool:
		return b
	default:
		return false
	}
}

func asString(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case []byte:
		return hex.EncodeToString(s)
	default:
		return fmt.Sprint(v)
	}
}

func prettyHex(s string) string {
	s = normalizeDestHashHex(s)
	if len(s) != 2*lxmf.DestinationLength {
		return strings.TrimSpace(s)
	}
	return s
}

func prettySize(n float64) string {
	if n < 0 {
		n = 0
	}
	units := []string{"B", "KB", "MB", "GB", "TB"}
	i := 0
	for n >= 1000 && i < len(units)-1 {
		n /= 1000
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%.0f %s", n, units[i])
	}
	return fmt.Sprintf("%.2f %s", n, units[i])
}

func prettyTime(seconds float64) string {
	if seconds < 0 {
		seconds = 0
	}
	s := int(seconds)
	if s < 60 {
		return fmt.Sprintf("%ds", s)
	}
	if s < 3600 {
		return fmt.Sprintf("%dm", s/60)
	}
	if s < 86400 {
		return fmt.Sprintf("%dh", s/3600)
	}
	return fmt.Sprintf("%dd", s/86400)
}

func prettySpeed(bps float64) string {
	if bps <= 0 {
		return "0 B/s"
	}
	return prettySize(bps) + "/s"
}
