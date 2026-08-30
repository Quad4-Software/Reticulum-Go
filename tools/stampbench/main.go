// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// Command stampbench compares Python LXStamper, Go CPU, and Go OpenCL GPU
// stamp generation for the same cost and expand rounds.
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"quad4/reticulum-go/pkg/lxstamper"
)

func main() {
	cost := 12
	rounds := 3
	if len(os.Args) > 1 {
		fmt.Sscanf(os.Args[1], "%d", &cost)
	}
	if len(os.Args) > 2 {
		fmt.Sscanf(os.Args[2], "%d", &rounds)
	}
	const iters = 5
	msg := bytes.Repeat([]byte{0x5A}, 16)
	ctx := context.Background()

	fmt.Printf("stamp cost=%d expand_rounds=%d material_len=%d iters=%d\n", cost, rounds, len(msg), iters)
	if v, n, ok := lxstamper.GPUDeviceInfo(); ok {
		fmt.Printf("OpenCL GPU: %s / %s\n", v, n)
	} else {
		fmt.Printf("OpenCL GPU: not detected (needs NVIDIA/AMD/Intel OpenCL ICD)\n")
	}
	fmt.Printf("preferred backend: %s active=%s\n\n", lxstamper.PreferredStampBackend(), lxstamper.ActiveStampBackend())

	timeGo := func(name string, fn func() error) {
		var total time.Duration
		for i := 0; i < iters; i++ {
			st := time.Now()
			if err := fn(); err != nil {
				fmt.Printf("%-8s FAIL: %v\n", name, err)
				return
			}
			total += time.Since(st)
		}
		fmt.Printf("%-8s n=%d avg=%s\n", name, iters, (total / iters).Round(time.Microsecond))
	}

	timeGo("go-cpu", func() error {
		_, _, err := lxstamper.GenerateStampCPU(ctx, msg, cost, rounds)
		return err
	})
	timeGo("go-gpu", func() error {
		_, _, err := lxstamper.GenerateStampGPU(ctx, msg, cost, rounds)
		return err
	})

	avg, err := runPythonBatch(msg, cost, rounds, iters)
	if err != nil {
		fmt.Printf("%-8s FAIL: %v\n", "python", err)
	} else {
		fmt.Printf("%-8s n=%d avg=%s (single process)\n", "python", iters, avg.Round(time.Microsecond))
	}
}

func runPythonBatch(msg []byte, cost, rounds, iters int) (time.Duration, error) {
	py := os.Getenv("PYTHON_INTEROP")
	if py == "" {
		py = "python3"
	}
	script := `
import os, sys, base64, time
lxmf = os.environ.get("LXMF_PATH", "")
if lxmf:
    sys.path.insert(0, os.path.abspath(lxmf))
from LXMF import LXStamper
material = base64.b64decode(sys.argv[1])
cost = int(sys.argv[2])
rounds = int(sys.argv[3])
iters = int(sys.argv[4])
# warmup
LXStamper.generate_stamp(material, cost, expand_rounds=rounds)
times = []
for _ in range(iters):
    st = time.perf_counter()
    stamp, value = LXStamper.generate_stamp(material, cost, expand_rounds=rounds)
    times.append(time.perf_counter() - st)
    if stamp is None:
        sys.exit(2)
print(sum(times) / len(times))
`
	cmd := exec.Command(py, "-c", script,
		base64.StdEncoding.EncodeToString(msg),
		strconv.Itoa(cost),
		strconv.Itoa(rounds),
		strconv.Itoa(iters),
	)
	cmd.Env = os.Environ()
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	sec, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil {
		return 0, err
	}
	return time.Duration(sec * float64(time.Second)), nil
}
