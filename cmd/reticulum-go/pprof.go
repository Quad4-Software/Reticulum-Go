// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package main

import (
	"net"
	"net/http"
	_ "net/http/pprof" // #nosec G108 -- opt-in via RETICULUM_PPROF_ADDR, localhost bind only
	"os"
	"strings"
	"time"

	"quad4/reticulum-go/pkg/debug"
)

// startPprofIfRequested binds a localhost pprof HTTP server when
// RETICULUM_PPROF_ADDR is set (for example 127.0.0.1:6060). Call before
// sandbox.Apply so the listen stays allowed under Landlock/seccomp.
func startPprofIfRequested() {
	addr := strings.TrimSpace(os.Getenv("RETICULUM_PPROF_ADDR"))
	if addr == "" {
		return
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		debug.Log(debug.DebugError, "pprof listen failed", "addr", addr, "error", err)
		return
	}
	srv := &http.Server{
		Handler:           http.DefaultServeMux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		debug.Log(debug.DebugInfo, "pprof listening", "addr", ln.Addr().String())
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			debug.Log(debug.DebugError, "pprof server stopped", "error", err)
		}
	}()
}
