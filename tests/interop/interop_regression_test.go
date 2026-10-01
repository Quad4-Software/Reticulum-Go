// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package interop

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestNomadNetPageExpectMatchesIndexMu keeps the Python pageserver live expect
// string aligned with examples/pageserver/pages/index.mu (previously expected
// "Reticulum-Go Node" while the page only contained "librns via Reticulum-Go").
func TestNomadNetPageExpectMatchesIndexMu(t *testing.T) {
	root := repoRoot(t)
	pagePath := filepath.Join(root, "examples", "pageserver", "pages", "index.mu")
	page, err := os.ReadFile(pagePath)
	if err != nil {
		t.Fatal(err)
	}
	const expect = "librns via Reticulum-Go"
	if !bytes.Contains(page, []byte(expect)) {
		t.Fatalf("%s missing %q", pagePath, expect)
	}
	livePath := filepath.Join(root, "tests", "interop", "pageserver_live_test.go")
	live, err := os.ReadFile(livePath)
	if err != nil {
		t.Fatal(err)
	}
	needle := `"/page/index.mu", "` + expect + `"`
	if !bytes.Contains(live, []byte(needle)) {
		t.Fatalf("%s must pass expect %q for /page/index.mu", livePath, expect)
	}
}

func TestRNS154InteropPins(t *testing.T) {
	root := repoRoot(t)
	crossref := filepath.Join(root, "tests", "crossref", "run_crossref.sh")
	body, err := os.ReadFile(crossref)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte(`RNS_REF_TAG="${RNS_REF_TAG:-1.5.5}"`)) {
		t.Fatal("crossref must default RNS_REF_TAG to 1.5.5")
	}
	ci := filepath.Join(root, ".github", "workflows", "ci.yml")
	ciBody, err := os.ReadFile(ci)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(ciBody, []byte(`rns==1.5.5`)) {
		t.Fatal("CI must install rns==1.5.5")
	}
	disc := filepath.Join(root, "pkg", "discovery", "discovery.go")
	discBody, err := os.ReadFile(disc)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(discBody, []byte("DefaultStampValue = 16")) {
		t.Fatal("discovery DefaultStampValue must be 16 for RNS 1.4.0+")
	}
	if !bytes.Contains(discBody, []byte("FieldTransportImpl   byte = 0xFD")) {
		t.Fatal("discovery must define TRANSPORT_IMPL 0xFD (RNS 1.5.1+)")
	}
	if !bytes.Contains(discBody, []byte("FieldTransportVers   byte = 0xFC")) {
		t.Fatal("discovery must define TRANSPORT_VERS 0xFC (RNS 1.5.1+)")
	}
	queues := filepath.Join(root, "pkg", "transport", "inbound_queues.go")
	qBody, err := os.ReadFile(queues)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{
		"defaultInboundDataQueueLen     = 1024",
		"defaultInboundAnnounceQueueLen = 128",
		"defaultInboundPRQueueLen       = 128",
		"defaultInboundILQueueLen       = 8",
	} {
		if !bytes.Contains(qBody, []byte(n)) {
			t.Fatalf("inbound queues missing %q (RNS 1.5.1+ defaults)", n)
		}
	}
}

// TestRNS155InteropPins locks the RNS 1.5.5 surface: the manage RPC path,
// autoconnect criteria, sequential naming, and the new config keys.
func TestRNS155InteropPins(t *testing.T) {
	root := repoRoot(t)

	rpcBody, err := os.ReadFile(filepath.Join(root, "pkg", "sharedinstance", "rpc.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"attach_interface", "detach_interface", "reload_interface"} {
		if !bytes.Contains(rpcBody, []byte(`"`+action+`"`)) {
			t.Fatalf("sharedinstance RPC missing manage action %q", action)
		}
	}
	if !bytes.Contains(rpcBody, []byte(`call["manage"]`)) {
		t.Fatal("sharedinstance RPC must serve the manage path")
	}

	cfgBody, err := os.ReadFile(filepath.Join(root, "pkg", "reticulumconfig", "config.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"enable_interface_management", "autoconnect_unverified_implementations"} {
		if !bytes.Contains(cfgBody, []byte(`"`+key+`"`)) {
			t.Fatalf("reticulumconfig missing %q", key)
		}
	}

	acBody, err := os.ReadFile(filepath.Join(root, "pkg", "node", "autoconnect.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"autoconnectQualified", "autoconnectInterfaceName"} {
		if !bytes.Contains(acBody, []byte(s)) {
			t.Fatalf("autoconnect missing %s (RNS 1.5.5 criteria)", s)
		}
	}
	discBody, err := os.ReadFile(filepath.Join(root, "pkg", "discovery", "discovery.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(discBody, []byte("AutoconnectMinVersions")) {
		t.Fatal("discovery must carry the autoconnect minimum-version table")
	}

	statusBody, err := os.ReadFile(filepath.Join(root, "pkg", "cli", "status.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, flag := range []string{`"attach"`, `"detach"`, `"reload"`, `"show-stale"`, `"show-unknown"`} {
		if !bytes.Contains(statusBody, []byte(flag)) {
			t.Fatalf("rgostatus missing %s flag", flag)
		}
	}
}

// TestHealthObservabilityPins locks the announce reject-reason counters and
// endpoint-health fields on the interface_stats surface. These are Go-only
// keys: extra msgpack fields are ignored by Python readers, so interop is
// unaffected, but removing them would silently break operator tooling.
func TestHealthObservabilityPins(t *testing.T) {
	root := repoRoot(t)

	statBody, err := os.ReadFile(filepath.Join(root, "pkg", "transport", "rpc_api.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{
		"announce_malformed", "announce_dest_type", "announce_blackholed",
		"announce_key_mismatch", "announce_max_hops", "announce_held",
		"announce_suppressed",
		"endpoint_dial_failures", "endpoint_flaps", "endpoint_quarantined",
		"endpoint_quarantine_s",
	} {
		if !bytes.Contains(statBody, []byte(`msgpack:"`+f+`"`)) {
			t.Fatalf("InterfaceStat missing %s field", f)
		}
	}

	kindBody, err := os.ReadFile(filepath.Join(root, "pkg", "health", "kind.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"announce_malformed", "announce_held", "announce_suppressed"} {
		if !bytes.Contains(kindBody, []byte(`"`+k+`"`)) {
			t.Fatalf("health kind missing %q", k)
		}
	}

	epBody, err := os.ReadFile(filepath.Join(root, "pkg", "interfaces", "endpoint_health.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(epBody, []byte("DefaultEndpointTracker")) {
		t.Fatal("interfaces must share the DefaultEndpointTracker")
	}
	rcBody, err := os.ReadFile(filepath.Join(root, "pkg", "interfaces", "reconnect.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(rcBody, []byte("dialTracked")) {
		t.Fatal("reconnect driver must route dials through dialTracked")
	}
}

// TestPageRequestHelperWaitsForClientExit locks the double-request UDP bind fix:
// the second pageserver_client must not start until the first has exited.
func TestPageRequestHelperWaitsForClientExit(t *testing.T) {
	livePath := filepath.Join(repoRoot(t), "tests", "interop", "pageserver_live_test.go")
	live, err := os.ReadFile(livePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(live, []byte("probe.Done()")) {
		t.Fatal("runPythonPageRequest must wait on probe.Done() before returning")
	}
}

func TestWaitStdoutTokenSkipsNoise(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	_, _ = w.WriteString("[Error] SAM API went offline\n")
	_, _ = w.WriteString("ONLINE\n")
	_ = w.Close()
	got, err := waitStdoutToken(context.Background(), bufio.NewReader(r), "ONLINE", 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if got != "ONLINE" {
		t.Fatalf("got %q", got)
	}
}

func TestWaitStdoutTokenPrefix(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	_, _ = w.WriteString("noise\n")
	_, _ = w.WriteString("B32=abcdefghijklmnopqrstuvwxyz234567abcdefghijklmnopqrst\n")
	_ = w.Close()
	got, err := waitStdoutToken(context.Background(), bufio.NewReader(r), "B32=", 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "B32=") {
		t.Fatalf("got %q", got)
	}
}

// TestI2PPeerWriteConfigCreatesParentDir locks the FileNotFoundError fix when
// INTEROP_CONFIG_DIR points at a nested path that does not exist yet.
func TestI2PPeerWriteConfigCreatesParentDir(t *testing.T) {
	script := filepath.Join(repoRoot(t), "tests", "interop", "py", "i2p_peer.py")
	body, err := os.ReadFile(script)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte("os.makedirs(cfg_dir, exist_ok=True)")) {
		t.Fatal("i2p_peer.write_config must os.makedirs(cfg_dir) before writing config")
	}
	if !bytes.Contains(body, []byte("RNS.logdest = RNS.LOG_FILE")) {
		t.Fatal("i2p_peer must log to file so READY/ONLINE tokens are not mixed with RNS logs")
	}
}

func TestI2PLiveTestsCreateConfigDirs(t *testing.T) {
	path := filepath.Join(repoRoot(t), "tests", "interop", "i2p_live_test.go")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte(`os.MkdirAll(pyCfg`)) {
		t.Fatal("i2p live tests must MkdirAll Python config dirs before spawn")
	}
	if !bytes.Contains(body, []byte("stopPythonI2P")) {
		t.Fatal("i2p live tests must soft-stop Python peers (avoid SAM thrash on Kill)")
	}
}

func TestCLIInteropHelpersAlwaysRebuildBins(t *testing.T) {
	rnxPath := filepath.Join(repoRoot(t), "tests", "interop", "rnx_live_test.go")
	rnx, err := os.ReadFile(rnxPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(rnx, []byte("func ensureRgox")) {
		t.Fatal("missing ensureRgox")
	}
	// Stale bin/rgox that forced ShareInstance=true broke UDP interop. ensureRgox
	// must always rebuild, not only when the binary is missing.
	if bytes.Contains(rnx, []byte("if _, err := os.Stat(bin); err != nil")) {
		t.Fatal("ensureRgox must always go build (not Stat-gated)")
	}
	rgoshPath := filepath.Join(repoRoot(t), "tests", "interop", "rgosh_live_test.go")
	rgosh, err := os.ReadFile(rgoshPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(rgosh, []byte("func ensureRgosh")) {
		t.Fatal("missing ensureRgosh")
	}
	if bytes.Contains(rgosh, []byte("if _, err := os.Stat(bin); err != nil")) {
		t.Fatal("ensureRgosh must always go build (not Stat-gated)")
	}
	cpPath := filepath.Join(repoRoot(t), "tests", "interop", "path_cp_live_test.go")
	cp, err := os.ReadFile(cpPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(cp, []byte(`build := exec.Command("go", "build", "-o", rgocp`)) {
		t.Fatal("rgocp live test must always go build before use")
	}
}

// TestTransportParityConstantsMatchPython locks safe deferred parity values
// against Python RNS Transport.py (AP/Roaming TTL, hashlist size, link timeout).
func TestTransportParityConstantsMatchPython(t *testing.T) {
	root := repoRoot(t)
	constPath := filepath.Join(root, "pkg", "transport", "constants.go")
	body, err := os.ReadFile(constPath)
	if err != nil {
		t.Fatal(err)
	}
	needles := []string{
		"APPathTime = 24 * time.Hour",
		"RoamingPathTime = 6 * time.Hour",
		"HashlistMaxSize = 1_000_000",
		"LinkTimeout = time.Duration(StaleTime*5/4) * time.Second",
	}
	for _, n := range needles {
		if !bytes.Contains(body, []byte(n)) {
			t.Fatalf("%s missing %q", constPath, n)
		}
	}
	lifetimePath := filepath.Join(root, "pkg", "transport", "path_lifetime.go")
	if _, err := os.Stat(lifetimePath); err != nil {
		t.Fatal("missing path_lifetime.go for AP/Roaming TTL")
	}
	hashPath := filepath.Join(root, "pkg", "transport", "packet_hashlist.go")
	if _, err := os.Stat(hashPath); err != nil {
		t.Fatal("missing packet_hashlist.go for duplicate filter")
	}
	mtuPath := filepath.Join(root, "pkg", "transport", "lr_mtu.go")
	if _, err := os.Stat(mtuPath); err != nil {
		t.Fatal("missing lr_mtu.go for relayed LR MTU clamp")
	}
	expiryPath := filepath.Join(root, "pkg", "transport", "link_expiry.go")
	if _, err := os.Stat(expiryPath); err != nil {
		t.Fatal("missing link_expiry.go for link proof-timeout rediscovery")
	}
}
