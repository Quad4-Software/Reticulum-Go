// SPDX-License-Identifier: LicenseRef-Reticulum
package compare_test

import (
	"bufio"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/destination"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/interfaces"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
	"github.com/Quad4-Software/reticulum-go-protocols/internal/lxsttest"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/audio/opus"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/call"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/proto"
)

type stats struct {
	N    int   `json:"n"`
	Min  int64 `json:"min"`
	P50  int64 `json:"p50"`
	P95  int64 `json:"p95"`
	P99  int64 `json:"p99"`
	Max  int64 `json:"max"`
	Mean int64 `json:"mean"`
}

type callSample struct {
	DialToActiveNs    int64 `json:"dial_to_active_ns"`
	RingingToActiveNs int64 `json:"ringing_to_active_ns"`
	ActiveToFirstNs   int64 `json:"active_to_first_frame_ns"`
}

type callGroup struct {
	DialToActiveMs    stats        `json:"dial_to_active_ms"`
	RingingToActiveMs stats        `json:"ringing_to_active_ms"`
	ActiveToFirstMs   stats        `json:"active_to_first_frame_ms"`
	Samples           []callSample `json:"samples"`
}

type report struct {
	When   string                      `json:"when"`
	Goos   string                      `json:"goos"`
	Goarch string                      `json:"goarch"`
	GOMAX  int                         `json:"gomaxprocs"`
	CGO    string                      `json:"cgo"`
	OpusGo string                      `json:"opus_go"`
	Python string                      `json:"python"`
	CPU    map[string]map[string]stats `json:"cpu_ns"`
	Calls  map[string]callGroup        `json:"calls_ms"`
	Method []string                    `json:"method"`
}

type peerEvent struct {
	Event       string `json:"event"`
	Identity    string `json:"identity"`
	Destination string `json:"destination"`
	Error       string `json:"error"`
	N           int    `json:"n"`
	Tns         int64  `json:"t_ns"`
}

func TestLXSTGoPythonLatencyCompare(t *testing.T) {
	if testing.Short() && os.Getenv("LXST_COMPARE") != "1" {
		t.Skip("set LXST_COMPARE=1 or run without -short")
	}
	py := lxsttest.Python(t)
	root := lxsttest.RepoRoot(t)

	rep := report{
		When:   time.Now().UTC().Format(time.RFC3339),
		Goos:   runtime.GOOS,
		Goarch: runtime.GOARCH,
		GOMAX:  runtime.GOMAXPROCS(0),
		CGO:    os.Getenv("CGO_ENABLED"),
		Python: py,
		CPU:    map[string]map[string]stats{},
		Calls:  map[string]callGroup{},
		Method: []string{
			"CPU uses monotonic clocks (Go time.Since, Python time.perf_counter_ns)",
			"Same payloads: signalling [available, ringing], 80-byte opus-like frame",
			"Call timer starts after path exists, excludes process and announce setup",
			"Go Dial-to-Active is OnAnswered, not Dial return (Dial polls every 20ms)",
			"Auto-answer delay is 0, Go Answer runs from OnRinging with no extra sleep",
			"UDP trials use 127.0.0.1, loopback trials use an in-process paired interface",
			"first-frame is OnFrame / python frame event after Established",
			"Identify runs on Available, the 250ms retry is not on the happy path",
		},
	}
	if rep.CGO == "" {
		rep.CGO = fmt.Sprintf("%d", detectCGOOpus())
	}

	const cpuN = 20000
	const cpuWarm = 3000
	const opusN = 400
	pyCPU := runPythonCPU(t, py, root, cpuN, cpuWarm, opusN)
	goCPU, opusKind := runGoCPU(t, cpuN, cpuWarm, opusN)
	rep.OpusGo = opusKind
	for name, gs := range goCPU {
		rep.CPU[name] = map[string]stats{"go": gs}
		if ps, ok := pyCPU[name]; ok {
			rep.CPU[name]["python"] = ps
		}
	}

	rep.Calls["go_go_loopback"] = measureGoGoLoopback(t, 2, 12)
	rep.Calls["go_go_udp"] = measureGoGoUDP(t, 1, 8)
	rep.Calls["python_python_udp"] = measurePythonPythonUDP(t, py, root, 1, 6)
	rep.Calls["go_dials_python"] = measureGoDialsPython(t, py, root, 1, 6)
	rep.Calls["python_dials_go"] = measurePythonDialsGo(t, py, root, 1, 6)

	out, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(os.TempDir(), "lxst-latency-compare.json")
	if err := os.WriteFile(path, out, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s", path)
	t.Logf("%s", out)
}

func detectCGOOpus() int {
	enc, err := opus.NewEncoderConfig(opus.EncoderConfig{
		SampleRate: 24000, Channels: 1, Bitrate: 8000, FrameSamples: 1440, Voip: true,
	})
	if err != nil {
		return 0
	}
	defer enc.Close()
	pcm := make([]int16, 1440)
	b, err := enc.Encode(pcm)
	if err != nil || len(b) >= 2 && b[0] == 'S' && b[1] == 'T' {
		return 0
	}
	return 1
}

func runPythonCPU(t *testing.T, py, root string, n, warmup, opusN int) map[string]stats {
	t.Helper()
	script := filepath.Join(root, "testdata", "lxst", "lxst_speed.py")
	cmd := exec.Command(py, script, fmt.Sprintf("%d", n), fmt.Sprintf("%d", warmup), fmt.Sprintf("%d", opusN))
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("lxst_speed.py: %v\n%s", err, out)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(out, &raw); err != nil {
		t.Fatalf("python cpu json: %v\n%s", err, out)
	}
	keys := []string{
		"pack_signalling_ns", "unpack_signalling_ns",
		"pack_frame_ns", "unpack_frame_ns", "dest_hash_ns",
		"opus_encode_ns", "opus_decode_ns",
	}
	got := map[string]stats{}
	for _, k := range keys {
		var s stats
		if err := json.Unmarshal(raw[k], &s); err != nil {
			continue
		}
		got[k] = s
	}
	return got
}

func runGoCPU(t *testing.T, n, warmup, opusN int) (map[string]stats, string) {
	t.Helper()
	sigs := []int{proto.StatusAvailable, proto.StatusRinging}
	packedSig, err := proto.PackSignalling(sigs)
	if err != nil {
		t.Fatal(err)
	}
	payload := make([]byte, 80)
	for i := range payload {
		payload[i] = byte(i)
	}
	packedFrame, err := proto.PackFrame(proto.CodecOpus, payload)
	if err != nil {
		t.Fatal(err)
	}
	ident := make([]byte, 16)
	for i := range ident {
		ident[i] = byte(i + 1)
	}
	hot := make([]byte, 0, 128)

	out := map[string]stats{
		"pack_signalling_ns": benchNS(n, warmup, func() {
			_, _ = proto.PackSignalling(sigs)
		}),
		"unpack_signalling_ns": benchNS(n, warmup, func() {
			_, _ = proto.Unpack(packedSig)
		}),
		"pack_frame_ns": benchNS(n, warmup, func() {
			_, _ = proto.PackFrame(proto.CodecOpus, payload)
		}),
		"pack_frame_into_ns": benchNS(n, warmup, func() {
			var e error
			hot, e = proto.PackFrameInto(hot[:0], proto.CodecOpus, payload)
			if e != nil {
				panic(e)
			}
		}),
		"unpack_frame_ns": benchNS(n, warmup, func() {
			_, _ = proto.Unpack(packedFrame)
		}),
		"dest_hash_ns": benchNS(n, warmup, func() {
			_ = proto.TelephonyHash(ident)
		}),
	}

	kind := "unavailable"
	enc, err := opus.NewEncoderConfig(opus.EncoderConfig{
		SampleRate: 24000, Channels: 1, Bitrate: 8000, FrameSamples: 1440, Voip: true,
	})
	if err != nil {
		out["opus_encode_ns"] = stats{}
		out["opus_decode_ns"] = stats{}
		return out, kind
	}
	defer enc.Close()
	pcm := make([]int16, 1440)
	for i := range pcm {
		pcm[i] = int16(1000 * math.Sin(2*math.Pi*440*float64(i)/24000))
	}
	encoded, err := enc.Encode(pcm)
	if err != nil {
		out["opus_encode_ns"] = stats{}
		out["opus_decode_ns"] = stats{}
		return out, "encode_failed"
	}
	if len(encoded) >= 2 && encoded[0] == 'S' && encoded[1] == 'T' {
		kind = "stub"
	} else {
		kind = "cgo"
	}
	params := proto.ProfileParams(proto.DefaultProfile)
	dec, err := opus.NewDecoderConfig(opus.DecoderConfig{
		SampleRate:   proto.PlaybackSampleRate,
		Channels:     1,
		FrameSamples: params.PlaybackFrameSamples(),
	})
	if err != nil {
		out["opus_encode_ns"] = benchNS(opusN, opusN/10, func() { _, _ = enc.Encode(pcm) })
		out["opus_decode_ns"] = stats{}
		return out, kind
	}
	defer dec.Close()
	out["opus_encode_ns"] = benchNS(opusN, opusN/10, func() { _, _ = enc.Encode(pcm) })
	out["opus_decode_ns"] = benchNS(opusN, opusN/10, func() { _, _ = dec.Decode(encoded) })
	return out, kind
}

func benchNS(n, warmup int, fn func()) stats {
	for range warmup {
		fn()
	}
	s := make([]int64, n)
	for i := range n {
		t0 := time.Now()
		fn()
		s[i] = time.Since(t0).Nanoseconds()
	}
	return summarize(s)
}

func summarize(samples []int64) stats {
	if len(samples) == 0 {
		return stats{}
	}
	cp := append([]int64(nil), samples...)
	for i := 1; i < len(cp); i++ {
		v := cp[i]
		j := i
		for j > 0 && cp[j-1] > v {
			cp[j] = cp[j-1]
			j--
		}
		cp[j] = v
	}
	var sum int64
	for _, v := range cp {
		sum += v
	}
	return stats{
		N:    len(cp),
		Min:  cp[0],
		P50:  percentile(cp, 50),
		P95:  percentile(cp, 95),
		P99:  percentile(cp, 99),
		Max:  cp[len(cp)-1],
		Mean: sum / int64(len(cp)),
	}
}

func percentile(sorted []int64, p float64) int64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := max(int(math.Round(p/100*float64(len(sorted)-1))), 0)
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func nsToMsStats(ns []int64) stats {
	ms := make([]int64, len(ns))
	for i, v := range ns {
		ms[i] = (v + 500000) / 1000000
	}
	return summarize(ms)
}

func groupFromSamples(samples []callSample) callGroup {
	dial := make([]int64, 0, len(samples))
	ring := make([]int64, 0, len(samples))
	first := make([]int64, 0, len(samples))
	for _, s := range samples {
		if s.DialToActiveNs > 0 {
			dial = append(dial, s.DialToActiveNs)
		}
		if s.RingingToActiveNs > 0 {
			ring = append(ring, s.RingingToActiveNs)
		}
		if s.ActiveToFirstNs > 0 {
			first = append(first, s.ActiveToFirstNs)
		}
	}
	return callGroup{
		DialToActiveMs:    nsToMsStats(dial),
		RingingToActiveMs: nsToMsStats(ring),
		ActiveToFirstMs:   nsToMsStats(first),
		Samples:           samples,
	}
}

type pairIface struct {
	common.BaseInterface
	peer *pairIface
}

func newPairIface(name string) *pairIface {
	p := &pairIface{BaseInterface: common.NewBaseInterface(name, common.IFTypeAuto, true)}
	p.MTU = common.DefaultMTU
	p.Bitrate = 1_000_000
	p.In = true
	p.Out = true
	p.Enable()
	return p
}

func (p *pairIface) Send(data []byte, _ string) error {
	if p.peer == nil {
		return nil
	}
	cp := append([]byte(nil), data...)
	go p.peer.ProcessIncoming(cp)
	return nil
}

func (p *pairIface) ProcessOutgoing(data []byte) error { return p.Send(data, "") }

func isolatedConfig(t *testing.T) *common.ReticulumConfig {
	t.Helper()
	cfg := common.DefaultConfig()
	cfg.ShareInstance = false
	cfg.InMemoryPathTable = true
	cfg.InMemoryKnownDestinations = true
	cfg.ConfigPath = t.TempDir() + "/config"
	return cfg
}

func measureGoGoLoopback(t *testing.T, warmup, n int) callGroup {
	t.Helper()
	tA := transport.NewTransport(isolatedConfig(t))
	tB := transport.NewTransport(isolatedConfig(t))
	if err := tA.Start(); err != nil {
		t.Fatal(err)
	}
	if err := tB.Start(); err != nil {
		t.Fatal(err)
	}
	ifA := newPairIface("a")
	ifB := newPairIface("b")
	ifA.peer = ifB
	ifB.peer = ifA
	if err := tA.RegisterInterface("a", ifA); err != nil {
		t.Fatal(err)
	}
	if err := tB.RegisterInterface("b", ifB); err != nil {
		t.Fatal(err)
	}
	idA, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	idB, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	destB, err := destination.New(idB, destination.In, destination.Single, proto.AppName, tB, proto.AspectName)
	if err != nil {
		t.Fatal(err)
	}
	destB.AcceptsLinks(true)
	sb := call.NewSwitchboard(tB, call.Config{
		Identity: idB,
		UseAudio: false,
		Events:   call.Events{OnRinging: autoAnswerIncoming},
	}, nil)
	sb.Bind(destB)
	_ = destB.Announce(false, nil, nil)
	time.Sleep(80 * time.Millisecond)

	var samples []callSample
	for i := 0; i < warmup+n; i++ {
		s, err := oneGoDial(tA, idA, idB, sb)
		if err != nil {
			waitCallIdle(nil, sb)
			s, err = oneGoDial(tA, idA, idB, sb)
		}
		if err != nil {
			t.Fatalf("loopback trial %d: %v", i, err)
		}
		if i >= warmup {
			samples = append(samples, s)
		}
	}
	return groupFromSamples(samples)
}

func measureGoGoUDP(t *testing.T, warmup, n int) callGroup {
	t.Helper()
	portA := freeUDP(t)
	portB := freeUDP(t)
	tA, idA := startGoUDP(t, portA, portB)
	tB, idB := startGoUDP(t, portB, portA)
	destB, err := destination.New(idB, destination.In, destination.Single, proto.AppName, tB, proto.AspectName)
	if err != nil {
		t.Fatal(err)
	}
	destB.AcceptsLinks(true)
	sb := call.NewSwitchboard(tB, call.Config{
		Identity: idB,
		UseAudio: false,
		Events:   call.Events{OnRinging: autoAnswerIncoming},
	}, nil)
	sb.Bind(destB)
	for range 8 {
		_ = destB.Announce(false, nil, nil)
		time.Sleep(80 * time.Millisecond)
	}
	if err := waitPath(tA, destB.GetHash(), 15*time.Second); err != nil {
		t.Fatal(err)
	}
	var samples []callSample
	for i := 0; i < warmup+n; i++ {
		s, err := oneGoDial(tA, idA, idB, sb)
		if err != nil {
			waitCallIdle(nil, sb)
			s, err = oneGoDial(tA, idA, idB, sb)
		}
		if err != nil {
			t.Fatalf("udp trial %d: %v", i, err)
		}
		if i >= warmup {
			samples = append(samples, s)
		}
	}
	return groupFromSamples(samples)
}

func autoAnswerIncoming(c *call.Call) {
	if c == nil || !c.Incoming() {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		_ = c.Answer(ctx)
	}()
}

func waitCallIdle(c *call.Call, sb *call.Switchboard) {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if (c == nil || c.State() == call.StateEnded || c.State() == call.StateIdle) && (sb == nil || sb.Active() == nil) {
			time.Sleep(40 * time.Millisecond)
			if sb == nil || sb.Active() == nil {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func oneGoDial(tA *transport.Transport, idA, idB *identity.Identity, sb *call.Switchboard) (callSample, error) {
	waitCallIdle(nil, sb)
	var sample callSample
	var tRing, tActive time.Time
	first := make(chan time.Time, 1)
	answered := make(chan time.Time, 1)
	caller := call.NewCall(tA, call.Config{
		Identity:    idA,
		UseAudio:    false,
		ConnectTime: 8 * time.Second,
		WaitTime:    12 * time.Second,
		Events: call.Events{
			OnRinging: func(*call.Call) { tRing = time.Now() },
			OnAnswered: func(*call.Call) {
				now := time.Now()
				tActive = now
				select {
				case answered <- now:
				default:
				}
			},
			OnFrame: func(pcm []int16) {
				if len(pcm) == 0 {
					return
				}
				select {
				case first <- time.Now():
				default:
				}
			},
		},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	t0 := time.Now()
	if err := caller.Dial(ctx, idB); err != nil {
		_ = caller.Hangup("fail")
		waitCallIdle(caller, sb)
		return sample, err
	}
	select {
	case ta := <-answered:
		tActive = ta
	default:
		if tActive.IsZero() {
			tActive = time.Now()
		}
	}
	sample.DialToActiveNs = tActive.Sub(t0).Nanoseconds()
	if !tRing.IsZero() {
		sample.RingingToActiveNs = tActive.Sub(tRing).Nanoseconds()
	}
	select {
	case tf := <-first:
		dt := tf.Sub(tActive).Nanoseconds()
		if dt > 0 {
			sample.ActiveToFirstNs = dt
		}
	case <-time.After(4 * time.Second):
	}
	_ = caller.Hangup("done")
	waitCallIdle(caller, sb)
	return sample, nil
}

func measureGoDialsPython(t *testing.T, py, root string, warmup, n int) callGroup {
	t.Helper()
	var samples []callSample
	for i := 0; i < warmup+n; i++ {
		s, err := oneGoDialsPython(t, py, root)
		if err != nil {
			t.Fatalf("go dials python trial %d: %v", i, err)
		}
		if i >= warmup {
			samples = append(samples, s)
		}
	}
	return groupFromSamples(samples)
}

func oneGoDialsPython(t *testing.T, py, root string) (callSample, error) {
	t.Helper()
	var sample callSample
	goPort := freeUDP(t)
	pyPort := freeUDP(t)
	cfgDir := t.TempDir()
	peer := filepath.Join(root, "testdata", "lxst", "lxst_peer.py")
	cmd := exec.Command(py, peer,
		"--configdir", cfgDir,
		"--listen-port", fmt.Sprintf("%d", pyPort),
		"--target-port", fmt.Sprintf("%d", goPort),
		"--mode", "listen",
		"--auto-answer", "0",
		"--frames", "8",
		"--name", fmt.Sprintf("pylisten%d", pyPort),
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return sample, err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return sample, err
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()
	events := bufio.NewScanner(stdout)
	ready, err := waitPeerEvent(events, "ready", 12*time.Second)
	if err != nil {
		return sample, err
	}
	tr, id := startGoUDP(t, goPort, pyPort)
	remoteHash, err := hex.DecodeString(ready.Identity)
	if err != nil {
		return sample, err
	}
	destHash := proto.TelephonyHash(remoteHash)
	if err := waitPath(tr, destHash, 15*time.Second); err != nil {
		return sample, err
	}
	remote, err := identity.Recall(destHash)
	if err != nil {
		return sample, err
	}
	var tRing, tActive time.Time
	first := make(chan time.Time, 1)
	answered := make(chan time.Time, 1)
	caller := call.NewCall(tr, call.Config{
		Identity:    id,
		UseAudio:    false,
		ConnectTime: 12 * time.Second,
		WaitTime:    18 * time.Second,
		Events: call.Events{
			OnRinging: func(*call.Call) { tRing = time.Now() },
			OnAnswered: func(*call.Call) {
				now := time.Now()
				tActive = now
				select {
				case answered <- now:
				default:
				}
			},
			OnFrame: func(pcm []int16) {
				if len(pcm) == 0 {
					return
				}
				select {
				case first <- time.Now():
				default:
				}
			},
		},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	t0 := time.Now()
	if err := caller.Dial(ctx, remote); err != nil {
		_ = caller.Hangup("fail")
		return sample, err
	}
	select {
	case ta := <-answered:
		tActive = ta
	default:
		if tActive.IsZero() {
			tActive = time.Now()
		}
	}
	sample.DialToActiveNs = tActive.Sub(t0).Nanoseconds()
	if !tRing.IsZero() {
		sample.RingingToActiveNs = tActive.Sub(tRing).Nanoseconds()
	}
	select {
	case tf := <-first:
		dt := tf.Sub(tActive).Nanoseconds()
		if dt > 0 {
			sample.ActiveToFirstNs = dt
		}
	case <-time.After(5 * time.Second):
	}
	_ = caller.Hangup("done")
	return sample, nil
}

func measurePythonDialsGo(t *testing.T, py, root string, warmup, n int) callGroup {
	t.Helper()
	var samples []callSample
	for i := 0; i < warmup+n; i++ {
		s, err := onePythonDialsGo(t, py, root)
		if err != nil {
			t.Fatalf("python dials go trial %d: %v", i, err)
		}
		if i >= warmup {
			samples = append(samples, s)
		}
	}
	return groupFromSamples(samples)
}

func onePythonDialsGo(t *testing.T, py, root string) (callSample, error) {
	t.Helper()
	var sample callSample
	goPort := freeUDP(t)
	pyPort := freeUDP(t)
	tr, id := startGoUDP(t, goPort, pyPort)
	dest, err := destination.New(id, destination.In, destination.Single, proto.AppName, tr, proto.AspectName)
	if err != nil {
		return sample, err
	}
	dest.AcceptsLinks(true)
	first := make(chan time.Time, 1)
	answered := make(chan time.Time, 1)
	var tRing time.Time
	sb := call.NewSwitchboard(tr, call.Config{
		Identity: id,
		UseAudio: false,
		Events: call.Events{
			OnRinging: func(c *call.Call) {
				tRing = time.Now()
				if !c.Incoming() {
					return
				}
				go func() {
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					_ = c.Answer(ctx)
				}()
			},
			OnAnswered: func(*call.Call) {
				select {
				case answered <- time.Now():
				default:
				}
			},
			OnFrame: func(pcm []int16) {
				if len(pcm) == 0 {
					return
				}
				select {
				case first <- time.Now():
				default:
				}
			},
		},
	}, nil)
	sb.Bind(dest)

	cfgDir := t.TempDir()
	peer := filepath.Join(root, "testdata", "lxst", "lxst_peer.py")
	cmd := exec.Command(py, peer,
		"--configdir", cfgDir,
		"--listen-port", fmt.Sprintf("%d", pyPort),
		"--target-port", fmt.Sprintf("%d", goPort),
		"--mode", "dial",
		"--dial", fmt.Sprintf("%x", id.Hash()),
		"--frames", "8",
		"--name", fmt.Sprintf("pydial%d", pyPort),
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return sample, err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return sample, err
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()
	events := bufio.NewScanner(stdout)
	if _, err := waitPeerEvent(events, "ready", 12*time.Second); err != nil {
		return sample, err
	}
	for range 10 {
		_ = dest.Announce(false, nil, nil)
		time.Sleep(100 * time.Millisecond)
	}
	var tDialStart, tEstablished, tFirstFrame int64
	done := make(chan struct{})
	go func() {
		defer close(done)
		for events.Scan() {
			var ev peerEvent
			if json.Unmarshal(events.Bytes(), &ev) != nil {
				continue
			}
			switch ev.Event {
			case "dial_start":
				if tDialStart == 0 {
					tDialStart = ev.Tns
				}
			case "established":
				if tEstablished == 0 {
					tEstablished = ev.Tns
				}
			case "frame":
				if tFirstFrame == 0 {
					tFirstFrame = ev.Tns
				}
			}
		}
	}()
	select {
	case tAns := <-answered:
		if !tRing.IsZero() {
			sample.RingingToActiveNs = tAns.Sub(tRing).Nanoseconds()
		}
		select {
		case tf := <-first:
			sample.ActiveToFirstNs = tf.Sub(tAns).Nanoseconds()
		case <-time.After(5 * time.Second):
		}
	case <-time.After(20 * time.Second):
		return sample, fmt.Errorf("python did not establish")
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}
	if tDialStart > 0 && tEstablished > tDialStart {
		sample.DialToActiveNs = tEstablished - tDialStart
	}
	if sample.ActiveToFirstNs == 0 && tEstablished > 0 && tFirstFrame > tEstablished {
		sample.ActiveToFirstNs = tFirstFrame - tEstablished
	}
	if c := sb.Active(); c != nil {
		_ = c.Hangup("done")
	}
	return sample, nil
}

func measurePythonPythonUDP(t *testing.T, py, root string, warmup, n int) callGroup {
	t.Helper()
	var samples []callSample
	for i := 0; i < warmup+n; i++ {
		s, err := onePythonPython(t, py, root)
		if err != nil {
			t.Fatalf("python python trial %d: %v", i, err)
		}
		if i >= warmup {
			samples = append(samples, s)
		}
	}
	return groupFromSamples(samples)
}

func onePythonPython(t *testing.T, py, root string) (callSample, error) {
	t.Helper()
	var sample callSample
	listenPort := freeUDP(t)
	dialPort := freeUDP(t)
	listenDir := t.TempDir()
	dialDir := t.TempDir()
	peer := filepath.Join(root, "testdata", "lxst", "lxst_peer.py")

	listen := exec.Command(py, peer,
		"--configdir", listenDir,
		"--listen-port", fmt.Sprintf("%d", listenPort),
		"--target-port", fmt.Sprintf("%d", dialPort),
		"--mode", "listen",
		"--auto-answer", "0",
		"--frames", "8",
		"--name", fmt.Sprintf("pypyl%d", listenPort),
	)
	lout, err := listen.StdoutPipe()
	if err != nil {
		return sample, err
	}
	listen.Stderr = os.Stderr
	if err := listen.Start(); err != nil {
		return sample, err
	}
	defer func() {
		_ = listen.Process.Kill()
		_, _ = listen.Process.Wait()
	}()
	lscan := bufio.NewScanner(lout)
	ready, err := waitPeerEvent(lscan, "ready", 12*time.Second)
	if err != nil {
		return sample, err
	}

	dial := exec.Command(py, peer,
		"--configdir", dialDir,
		"--listen-port", fmt.Sprintf("%d", dialPort),
		"--target-port", fmt.Sprintf("%d", listenPort),
		"--mode", "dial",
		"--dial", ready.Identity,
		"--frames", "8",
		"--name", fmt.Sprintf("pypyd%d", dialPort),
	)
	dout, err := dial.StdoutPipe()
	if err != nil {
		return sample, err
	}
	dial.Stderr = os.Stderr
	if err := dial.Start(); err != nil {
		return sample, err
	}
	defer func() {
		_ = dial.Process.Kill()
		_, _ = dial.Process.Wait()
	}()
	dscan := bufio.NewScanner(dout)
	if _, err := waitPeerEvent(dscan, "ready", 12*time.Second); err != nil {
		return sample, err
	}
	var tDial, tRing, tEst, tFrame int64
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if !dscan.Scan() {
			break
		}
		var ev peerEvent
		if json.Unmarshal(dscan.Bytes(), &ev) != nil {
			continue
		}
		switch ev.Event {
		case "dial_start":
			tDial = ev.Tns
		case "ringing":
			if tRing == 0 {
				tRing = ev.Tns
			}
		case "established":
			tEst = ev.Tns
		case "frame":
			if tFrame == 0 {
				tFrame = ev.Tns
			}
		case "error":
			return sample, fmt.Errorf("python: %s", ev.Error)
		}
		if tDial > 0 && tEst > 0 && tFrame > 0 {
			break
		}
	}
	if tDial == 0 || tEst == 0 {
		return sample, fmt.Errorf("python-python missing dial_start or established")
	}
	sample.DialToActiveNs = tEst - tDial
	if tRing > 0 && tEst > tRing {
		sample.RingingToActiveNs = tEst - tRing
	}
	if tFrame > tEst {
		sample.ActiveToFirstNs = tFrame - tEst
	}
	return sample, nil
}

func startGoUDP(t *testing.T, listenPort, targetPort int) (*transport.Transport, *identity.Identity) {
	t.Helper()
	cfg := isolatedConfig(t)
	tr := transport.NewTransport(cfg)
	if err := tr.Start(); err != nil {
		t.Fatal(err)
	}
	addr := fmt.Sprintf("127.0.0.1:%d", listenPort)
	target := fmt.Sprintf("127.0.0.1:%d", targetPort)
	iface, err := interfaces.NewUDPInterface("UDP", addr, target, true)
	if err != nil {
		t.Fatal(err)
	}
	iface.In = true
	iface.Out = true
	if err := tr.RegisterInterface("UDP", iface); err != nil {
		t.Fatal(err)
	}
	if err := iface.Start(); err != nil {
		t.Fatal(err)
	}
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	return tr, id
}

func waitPath(tr *transport.Transport, hash []byte, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	_ = tr.RequestPath(hash, "", nil, false)
	for time.Now().Before(deadline) {
		if tr.HasPath(hash) {
			return nil
		}
		time.Sleep(80 * time.Millisecond)
		_ = tr.RequestPath(hash, "", nil, false)
	}
	return fmt.Errorf("no path to %x", hash)
}

func waitPeerEvent(sc *bufio.Scanner, name string, timeout time.Duration) (peerEvent, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !sc.Scan() {
			if err := sc.Err(); err != nil {
				return peerEvent{}, err
			}
			return peerEvent{}, fmt.Errorf("python stdout closed waiting for %s", name)
		}
		var ev peerEvent
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
			continue
		}
		if ev.Event == "error" {
			return peerEvent{}, fmt.Errorf("python error: %s", ev.Error)
		}
		if ev.Event == name {
			return ev, nil
		}
	}
	return peerEvent{}, fmt.Errorf("timeout waiting for python event %s", name)
}

func freeUDP(t *testing.T) int {
	t.Helper()
	c, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	return c.LocalAddr().(*net.UDPAddr).Port
}
