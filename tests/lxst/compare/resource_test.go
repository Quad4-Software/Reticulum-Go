// SPDX-License-Identifier: LicenseRef-Reticulum
package compare_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/destination"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
	"github.com/Quad4-Software/reticulum-go-protocols/internal/lxsttest"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/audio/opus"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/call"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/proto"
)

type procSnap struct {
	RSSKb int64 `json:"rss_kb"`
	Ticks int64 `json:"cpu_ticks"`
}

type memSnap struct {
	HeapAllocKb  uint64  `json:"heap_alloc_kb"`
	HeapSysKb    uint64  `json:"heap_sys_kb"`
	HeapInuseKb  uint64  `json:"heap_inuse_kb"`
	StackInuseKb uint64  `json:"stack_inuse_kb"`
	NumGC        uint32  `json:"num_gc"`
	PauseTotalMs float64 `json:"pause_total_ms"`
	Goroutines   int     `json:"goroutines"`
}

type loadReport struct {
	When    string  `json:"when"`
	HoldSec float64 `json:"hold_sec"`
	ClkTck  int64   `json:"clk_tck"`
	GoIdle  struct {
		RSSKb       int64  `json:"rss_kb"`
		HeapAllocKb uint64 `json:"heap_alloc_kb"`
		Goroutines  int    `json:"goroutines"`
	} `json:"go_idle"`
	GoCall struct {
		RSSKb         int64   `json:"rss_kb_peak"`
		RSSKbMedian   int64   `json:"rss_kb_median"`
		HeapAllocKb   uint64  `json:"heap_alloc_kb"`
		HeapAfterGCKb uint64  `json:"heap_after_gc_kb"`
		CPUPercent    float64 `json:"cpu_percent"`
		Goroutines    int     `json:"goroutines"`
		GCDelta       uint32  `json:"gc_count"`
		PauseMs       float64 `json:"gc_pause_ms"`
		SentPerSec    float64 `json:"sent_frames_per_sec"`
		RecvPerSec    float64 `json:"recv_frames_per_sec"`
		FrameIntMs    stats   `json:"frame_interval_ms"`
		PackedBytes   int     `json:"packed_frame_bytes"`
		BitrateBps    float64 `json:"est_bitrate_bps"`
		DurationSec   float64 `json:"duration_sec"`
		CallerSent    uint64  `json:"caller_sent"`
		CallerRecv    uint64  `json:"caller_recv"`
		CalleeSent    uint64  `json:"callee_sent"`
		CalleeRecv    uint64  `json:"callee_recv"`
	} `json:"go_call"`
	PythonIdle struct {
		RSSKb int64 `json:"rss_kb"`
	} `json:"python_idle"`
	PythonCall struct {
		ListenRSSKb int64   `json:"listen_rss_kb_peak"`
		DialRSSKb   int64   `json:"dial_rss_kb_peak"`
		PairRSSKb   int64   `json:"pair_rss_kb_peak"`
		CPUPercent  float64 `json:"cpu_percent_sum"`
		ListenCPU   float64 `json:"listen_cpu_percent"`
		DialCPU     float64 `json:"dial_cpu_percent"`
		FrameIntMs  stats   `json:"frame_interval_ms"`
		DurationSec float64 `json:"duration_sec"`
		Frames      int     `json:"frames"`
	} `json:"python_call"`
	Method []string `json:"method"`
}

func TestLXSTResourceCompare(t *testing.T) {
	if testing.Short() && os.Getenv("LXST_COMPARE") != "1" {
		t.Skip("set LXST_COMPARE=1 or run without -short")
	}
	py := lxsttest.Python(t)
	root := lxsttest.RepoRoot(t)
	const hold = 5 * time.Second

	rep := loadReport{
		When:    time.Now().UTC().Format(time.RFC3339),
		HoldSec: hold.Seconds(),
		ClkTck:  clkTck(),
		Method: []string{
			"Go idle and call run in one test process hosting both caller and callee on a paired interface",
			"Python idle is one lxst_peer listener. Python call is two processes on UDP 127.0.0.1",
			"RSS from /proc/pid/status VmRSS. CPU from /proc/pid/stat utime+stime over the hold window",
			"Go heap from runtime.MemStats without a forced GC during the call. heap_after_gc is a GC at the end",
			"Frame interval is OnFrame / python frame event spacing after Established, in milliseconds",
			"Bitrate estimate is recv frames per sec times packed mq Opus frame size",
		},
	}

	idleRSS, idleHeap, idleG := measureGoIdle(t)
	rep.GoIdle.RSSKb = idleRSS
	rep.GoIdle.HeapAllocKb = idleHeap
	rep.GoIdle.Goroutines = idleG

	measureGoCallLoad(t, hold, &rep)

	rep.PythonIdle.RSSKb = measurePythonIdle(t, py, root)
	measurePythonCallLoad(t, py, root, hold, &rep)

	out, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(os.TempDir(), "lxst-resource-compare.json")
	if err := os.WriteFile(path, out, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s", path)
	t.Logf("%s", out)
}

func measureGoIdle(t *testing.T) (rssKb int64, heapKb uint64, goroutines int) {
	t.Helper()
	tr := transport.NewTransport(isolatedConfig(t))
	if err := tr.Start(); err != nil {
		t.Fatal(err)
	}
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	dest, err := destination.New(id, destination.In, destination.Single, proto.AppName, tr, proto.AspectName)
	if err != nil {
		t.Fatal(err)
	}
	dest.AcceptsLinks(true)
	sb := call.NewSwitchboard(tr, call.Config{Identity: id, UseAudio: false}, nil)
	sb.Bind(dest)
	_ = dest.Announce(false, nil, nil)
	runtime.GC()
	var samples []int64
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if s, err := readProc(os.Getpid()); err == nil {
			samples = append(samples, s.RSSKb)
		}
		time.Sleep(200 * time.Millisecond)
	}
	ms := readMem()
	rssKb = medianInt(samples)
	heapKb = ms.HeapAllocKb
	goroutines = runtime.NumGoroutine()
	_ = tr.Close()
	return rssKb, heapKb, goroutines
}

func measureGoCallLoad(t *testing.T, hold time.Duration, rep *loadReport) {
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
	var callee *call.Call
	sb := call.NewSwitchboard(tB, call.Config{
		Identity: idB,
		UseAudio: false,
		Events: call.Events{
			OnRinging:  autoAnswerIncoming,
			OnAnswered: func(c *call.Call) { callee = c },
		},
	}, nil)
	sb.Bind(destB)
	_ = destB.Announce(false, nil, nil)
	time.Sleep(80 * time.Millisecond)

	var ivMu sync.Mutex
	var intervals []int64
	var lastFrame time.Time
	caller := call.NewCall(tA, call.Config{
		Identity: idA,
		UseAudio: false,
		Events: call.Events{
			OnFrame: func(pcm []int16) {
				if len(pcm) == 0 {
					return
				}
				now := time.Now()
				ivMu.Lock()
				if !lastFrame.IsZero() {
					intervals = append(intervals, now.Sub(lastFrame).Milliseconds())
				}
				lastFrame = now
				ivMu.Unlock()
			},
		},
	})
	ctx, cancel := context.WithTimeout(context.Background(), hold+15*time.Second)
	defer cancel()
	if err := caller.Dial(ctx, idB); err != nil {
		t.Fatalf("dial: %v", err)
	}

	packed := packedFrameSize(t)
	startCPU, _ := readProc(os.Getpid())
	startMem := readMem()
	t0 := time.Now()
	sent0 := caller.SentFrames()
	recv0 := caller.RecvFrames()
	var rss []int64
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for time.Now().Before(t0.Add(hold)) {
		<-ticker.C
		if s, err := readProc(os.Getpid()); err == nil {
			rss = append(rss, s.RSSKb)
		}
	}
	elapsed := time.Since(t0).Seconds()
	endCPU, _ := readProc(os.Getpid())
	liveMem := readMem()
	runtime.GC()
	afterGC := readMem()
	sent := caller.SentFrames() - sent0
	recv := caller.RecvFrames() - recv0
	var calSent, calRecv uint64
	if callee != nil {
		calSent = callee.SentFrames()
		calRecv = callee.RecvFrames()
	}
	_ = caller.Hangup("done")
	ivMu.Lock()
	ivCopy := append([]int64(nil), intervals...)
	ivMu.Unlock()
	rep.GoCall.RSSKb = maxInt(rss)
	rep.GoCall.RSSKbMedian = medianInt(rss)
	rep.GoCall.HeapAllocKb = liveMem.HeapAllocKb
	rep.GoCall.HeapAfterGCKb = afterGC.HeapAllocKb
	rep.GoCall.CPUPercent = cpuPercent(startCPU.Ticks, endCPU.Ticks, elapsed, rep.ClkTck)
	rep.GoCall.Goroutines = runtime.NumGoroutine()
	rep.GoCall.GCDelta = liveMem.NumGC - startMem.NumGC
	rep.GoCall.PauseMs = liveMem.PauseTotalMs - startMem.PauseTotalMs
	rep.GoCall.SentPerSec = float64(sent) / elapsed
	rep.GoCall.RecvPerSec = float64(recv) / elapsed
	rep.GoCall.FrameIntMs = summarize(ivCopy)
	rep.GoCall.PackedBytes = packed
	rep.GoCall.BitrateBps = (float64(recv) / elapsed) * float64(packed) * 8
	rep.GoCall.DurationSec = elapsed
	rep.GoCall.CallerSent = sent
	rep.GoCall.CallerRecv = recv
	rep.GoCall.CalleeSent = calSent
	rep.GoCall.CalleeRecv = calRecv
	_ = tA.Close()
	_ = tB.Close()
}

func measurePythonIdle(t *testing.T, py, root string) int64 {
	t.Helper()
	listenPort := freeUDP(t)
	dialPort := freeUDP(t)
	cmd, scan := startPeer(t, py, root, listenPort, dialPort, "listen", "", "pyidle", 4)
	defer stopCmd(cmd)
	if _, err := waitPeerEvent(scan, "ready", 12*time.Second); err != nil {
		t.Fatal(err)
	}
	var samples []int64
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if s, err := readProc(cmd.Process.Pid); err == nil {
			samples = append(samples, s.RSSKb)
		}
		time.Sleep(200 * time.Millisecond)
	}
	return medianInt(samples)
}

func measurePythonCallLoad(t *testing.T, py, root string, hold time.Duration, rep *loadReport) {
	t.Helper()
	listenPort := freeUDP(t)
	dialPort := freeUDP(t)
	frames := int(hold/time.Millisecond)/60 + 20
	listen, lscan := startPeer(t, py, root, listenPort, dialPort, "listen", "", "pyresl", frames)
	defer stopCmd(listen)
	ready, err := waitPeerEvent(lscan, "ready", 12*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	go drainPeer(lscan)

	dial, dscan := startPeer(t, py, root, dialPort, listenPort, "dial", ready.Identity, "pyresd", frames)
	defer stopCmd(dial)
	if _, err := waitPeerEvent(dscan, "ready", 12*time.Second); err != nil {
		t.Fatal(err)
	}

	var frameMu sync.Mutex
	var frameTimes []int64
	established := make(chan struct{}, 1)
	go func() {
		for dscan.Scan() {
			var ev peerEvent
			if json.Unmarshal(dscan.Bytes(), &ev) != nil {
				continue
			}
			switch ev.Event {
			case "established":
				select {
				case established <- struct{}{}:
				default:
				}
			case "frame":
				if ev.Tns > 0 {
					frameMu.Lock()
					frameTimes = append(frameTimes, ev.Tns)
					frameMu.Unlock()
				}
			}
		}
	}()
	select {
	case <-established:
	case <-time.After(20 * time.Second):
		t.Fatal("python-python did not establish")
	}

	t0 := time.Now()
	l0, _ := readProc(listen.Process.Pid)
	d0, _ := readProc(dial.Process.Pid)
	var lRSS, dRSS []int64
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for time.Now().Before(t0.Add(hold)) {
		<-ticker.C
		if s, err := readProc(listen.Process.Pid); err == nil {
			lRSS = append(lRSS, s.RSSKb)
		}
		if s, err := readProc(dial.Process.Pid); err == nil {
			dRSS = append(dRSS, s.RSSKb)
		}
	}
	elapsed := time.Since(t0).Seconds()
	l1, _ := readProc(listen.Process.Pid)
	d1, _ := readProc(dial.Process.Pid)

	frameMu.Lock()
	times := append([]int64(nil), frameTimes...)
	frameMu.Unlock()
	var gaps []int64
	for i := 1; i < len(times); i++ {
		gaps = append(gaps, (times[i]-times[i-1])/1_000_000)
	}
	rep.PythonCall.ListenRSSKb = maxInt(lRSS)
	rep.PythonCall.DialRSSKb = maxInt(dRSS)
	rep.PythonCall.PairRSSKb = rep.PythonCall.ListenRSSKb + rep.PythonCall.DialRSSKb
	rep.PythonCall.ListenCPU = cpuPercent(l0.Ticks, l1.Ticks, elapsed, rep.ClkTck)
	rep.PythonCall.DialCPU = cpuPercent(d0.Ticks, d1.Ticks, elapsed, rep.ClkTck)
	rep.PythonCall.CPUPercent = rep.PythonCall.ListenCPU + rep.PythonCall.DialCPU
	rep.PythonCall.FrameIntMs = summarize(gaps)
	rep.PythonCall.DurationSec = elapsed
	rep.PythonCall.Frames = len(times)
}

func startPeer(t *testing.T, py, root string, listenPort, targetPort int, mode, dial, name string, frames int) (*exec.Cmd, *bufio.Scanner) {
	t.Helper()
	peer := filepath.Join(root, "testdata", "lxst", "lxst_peer.py")
	args := []string{
		peer,
		"--configdir", t.TempDir(),
		"--listen-port", fmt.Sprintf("%d", listenPort),
		"--target-port", fmt.Sprintf("%d", targetPort),
		"--mode", mode,
		"--auto-answer", "0",
		"--frames", fmt.Sprintf("%d", frames),
		"--name", fmt.Sprintf("%s%d", name, listenPort),
	}
	if mode == "dial" {
		args = append(args, "--dial", dial)
	}
	cmd := exec.Command(py, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	return cmd, bufio.NewScanner(stdout)
}

func stopCmd(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
	_, _ = cmd.Process.Wait()
}

func drainPeer(sc *bufio.Scanner) {
	for sc.Scan() {
	}
}

func packedFrameSize(t *testing.T) int {
	t.Helper()
	enc, err := opus.NewEncoderConfig(opus.EncoderConfig{
		SampleRate: 24000, Channels: 1, Bitrate: 8000, FrameSamples: 1440, Voip: true,
	})
	if err != nil {
		raw, err := proto.PackFrame(proto.CodecOpus, make([]byte, 80))
		if err != nil {
			t.Fatal(err)
		}
		return len(raw)
	}
	defer enc.Close()
	pcm := make([]int16, 1440)
	const hz = 1000.0
	const rate = 24000.0
	for i := range pcm {
		pcm[i] = int16(math.Sin(2*math.Pi*hz*float64(i)/rate) * 16000)
	}
	encoded, err := enc.Encode(pcm)
	if err != nil || len(encoded) == 0 {
		raw, err := proto.PackFrame(proto.CodecOpus, make([]byte, 80))
		if err != nil {
			t.Fatal(err)
		}
		return len(raw)
	}
	raw, err := proto.PackFrame(proto.CodecOpus, encoded)
	if err != nil {
		t.Fatal(err)
	}
	return len(raw)
}

func readMem() memSnap {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return memSnap{
		HeapAllocKb:  m.HeapAlloc / 1024,
		HeapSysKb:    m.HeapSys / 1024,
		HeapInuseKb:  m.HeapInuse / 1024,
		StackInuseKb: m.StackInuse / 1024,
		NumGC:        m.NumGC,
		PauseTotalMs: float64(m.PauseTotalNs) / 1e6,
		Goroutines:   runtime.NumGoroutine(),
	}
}

func readProc(pid int) (procSnap, error) {
	status, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return procSnap{}, err
	}
	var rss int64
	for line := range strings.SplitSeq(string(status), "\n") {
		if strings.HasPrefix(line, "VmRSS:") {
			f := strings.Fields(line)
			if len(f) >= 2 {
				rss, _ = strconv.ParseInt(f[1], 10, 64)
			}
			break
		}
	}
	stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return procSnap{RSSKb: rss}, err
	}
	idx := bytes.LastIndexByte(stat, ')')
	if idx < 0 || idx+2 >= len(stat) {
		return procSnap{RSSKb: rss}, fmt.Errorf("stat parse")
	}
	fields := strings.Fields(string(stat[idx+2:]))
	if len(fields) < 13 {
		return procSnap{RSSKb: rss}, fmt.Errorf("stat fields")
	}
	utime, _ := strconv.ParseInt(fields[11], 10, 64)
	stime, _ := strconv.ParseInt(fields[12], 10, 64)
	return procSnap{RSSKb: rss, Ticks: utime + stime}, nil
}

func clkTck() int64 {
	return 100
}

func cpuPercent(startTicks, endTicks int64, elapsedSec float64, hz int64) float64 {
	if elapsedSec <= 0 || hz <= 0 {
		return 0
	}
	delta := endTicks - startTicks
	if delta < 0 {
		return 0
	}
	return 100 * float64(delta) / (float64(hz) * elapsedSec)
}

func medianInt(v []int64) int64 {
	if len(v) == 0 {
		return 0
	}
	s := summarize(v)
	return s.P50
}

func maxInt(v []int64) int64 {
	if len(v) == 0 {
		return 0
	}
	m := v[0]
	for _, x := range v[1:] {
		if x > m {
			m = x
		}
	}
	return m
}
