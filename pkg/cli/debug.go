// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"runtime"
	"time"

	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/rnsutil"
)

// RunDebug prints effective runtime diagnostics via shared-instance RPC.
func RunDebug(args []string, opt ...Options) int {
	stdout, stderr := cliIO(opt)
	fs := flag.NewFlagSet("debug", flag.ContinueOnError)
	fs.SetOutput(stderr)
	configDir := fs.String("config", "", "path to config directory")
	jsonOut := fs.Bool("json", false, "emit JSON")
	rates := fs.Bool("rates", false, "include rate table")
	timeout := fs.Duration("timeout", 10*time.Second, "RPC timeout")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	cfg, err := rnsutil.LoadConfigDir(*configDir)
	if err != nil {
		fmt.Fprintf(stderr, "config: %v\n", err)
		return 1
	}

	type dump struct {
		ConfigPath   string  `json:"config_path"`
		LogLevel     int     `json:"log_level"`
		DebugLevel   int     `json:"debug_level"`
		GOOS         string  `json:"goos"`
		GOARCH       string  `json:"goarch"`
		RPCAddr      string  `json:"rpc_addr,omitempty"`
		TransportID  string  `json:"transport_id,omitempty"`
		UptimeSec    float64 `json:"uptime_sec,omitempty"`
		InterfaceN   int     `json:"interfaces,omitempty"`
		RateTableLen int     `json:"rate_table_len,omitempty"`
		Error        string  `json:"error,omitempty"`
	}

	out := dump{
		ConfigPath: cfg.ConfigPath,
		LogLevel:   cfg.LogLevel,
		DebugLevel: debug.GetDebugLevel(),
		GOOS:       runtime.GOOS,
		GOARCH:     runtime.GOARCH,
	}

	client, err := rnsutil.DialRPC(cfg, nil)
	if err != nil {
		out.Error = err.Error()
		if *jsonOut {
			enc := json.NewEncoder(stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(out)
			return 1
		}
		fmt.Fprintf(stderr, "rpc: %v\n", err)
		fmt.Fprintf(stdout, "config_path=%s log_level=%d goos=%s goarch=%s\n",
			out.ConfigPath, out.LogLevel, out.GOOS, out.GOARCH)
		return 1
	}
	client.SetTimeout(*timeout)
	out.RPCAddr = client.Addr()

	stats, err := client.GetInterfaceStats()
	if err != nil {
		out.Error = err.Error()
	} else {
		out.InterfaceN = len(stats.Interfaces)
		out.UptimeSec = stats.TransportUptime
		out.TransportID = rnsutil.PrettyHex(stats.TransportID)
	}

	if *rates {
		var raw any
		if err := client.Call(map[string]any{"get": "rate_table"}, &raw); err == nil {
			switch v := raw.(type) {
			case []any:
				out.RateTableLen = len(v)
			case []map[string]any:
				out.RateTableLen = len(v)
			}
		}
	}

	if *jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(out); err != nil {
			fmt.Fprintf(stderr, "json: %v\n", err)
			return 1
		}
		return 0
	}

	fmt.Fprintf(stdout, "config_path  : %s\n", out.ConfigPath)
	fmt.Fprintf(stdout, "log_level    : %d\n", out.LogLevel)
	fmt.Fprintf(stdout, "debug_level  : %d\n", out.DebugLevel)
	fmt.Fprintf(stdout, "platform     : %s/%s\n", out.GOOS, out.GOARCH)
	fmt.Fprintf(stdout, "rpc_addr     : %s\n", out.RPCAddr)
	if out.Error != "" {
		fmt.Fprintf(stdout, "rpc_error    : %s\n", out.Error)
		return 1
	}
	fmt.Fprintf(stdout, "transport_id : %s\n", out.TransportID)
	fmt.Fprintf(stdout, "uptime_sec   : %.1f\n", out.UptimeSec)
	fmt.Fprintf(stdout, "interfaces   : %d\n", out.InterfaceN)
	if *rates {
		fmt.Fprintf(stdout, "rate_table   : %d entries\n", out.RateTableLen)
	}
	return 0
}
