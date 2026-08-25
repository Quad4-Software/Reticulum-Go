// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package cli

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"quad4/reticulum-go/pkg/packet"
)

func RunDump(args []string, opt ...Options) int {
	stdout, stderr := cliIO(opt)
	fs := flag.NewFlagSet("rgodump", flag.ContinueOnError)
	fs.SetOutput(stderr)
	pcapPath := fs.String("pcap", "", "classic pcap file (UDP payloads decoded as RNS)")
	hexArg := fs.String("hex", "", "single packet as hex (with or without spaces)")
	pretty := fs.Bool("pretty", false, "indent JSON objects (default is JSONL)")
	limit := fs.Int("n", 0, "max packets to emit (0 means no limit)")
	bindFlagUsage(fs, "rgodump - decode RNS packets",
		"Decode packets from hex, files, pcap, or stdin as JSON/JSONL.",
		[]helpLine{
			{Cmd: "rgodump -hex <bytes>"},
			{Cmd: "rgodump -pcap file.pcap"},
			{Cmd: "rgodump <hexfile>"},
			{Cmd: "rgodump < stdin hex lines"},
			{Cmd: "reticulum-go dump [flags] ..."},
		},
		"rgodump -hex deadbeef",
		"rgodump -pcap capture.pcap -pretty",
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	var frames []packet.Frame
	switch {
	case *pcapPath != "":
		f, err := os.Open(*pcapPath)
		if err != nil {
			diagErr(stderr, "pcap open", err)
			return 1
		}
		defer f.Close()
		caps, err := packet.ReadPCAPUDPPayloads(f)
		if err != nil {
			diagErr(stderr, "pcap read", err)
			return 1
		}
		for _, c := range caps {
			fr := packet.DecodeFrame(c.Payload)
			frames = append(frames, fr)
		}
	case *hexArg != "":
		raw, err := decodeHexBlob(*hexArg)
		if err != nil {
			diagErr(stderr, "hex", err)
			return 1
		}
		frames = append(frames, packet.DecodeFrame(raw))
	case fs.NArg() > 0:
		for _, p := range fs.Args() {
			b, err := os.ReadFile(p) // #nosec G304 -- operator-chosen input path
			if err != nil {
				fmt.Fprintf(stderr, "read %s: %v\n", p, err)
				return 1
			}
			if looksLikePCAP(b) {
				caps, err := packet.ReadPCAPUDPPayloads(bytes.NewReader(b))
				if err != nil {
					fmt.Fprintf(stderr, "pcap %s: %v\n", p, err)
					return 1
				}
				for _, c := range caps {
					frames = append(frames, packet.DecodeFrame(c.Payload))
				}
				continue
			}
			lines := strings.SplitSeq(string(b), "\n")
			for line := range lines {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				raw, err := decodeHexBlob(line)
				if err != nil {
					fmt.Fprintf(stderr, "hex line: %v\n", err)
					return 1
				}
				frames = append(frames, packet.DecodeFrame(raw))
			}
		}
	default:
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) != 0 {
			usageErr(stderr, "rgodump -hex <bytes> | rgodump -pcap file.pcap | rgodump <hexfile> | rgodump < stdin hex lines")
			return 2
		}
		br := bufio.NewReader(os.Stdin)
		peek, _ := br.Peek(4)
		if len(peek) >= 4 && looksLikePCAP(peek) {
			all, err := io.ReadAll(br)
			if err != nil {
				diagErr(stderr, "stdin", err)
				return 1
			}
			caps, err := packet.ReadPCAPUDPPayloads(bytes.NewReader(all))
			if err != nil {
				fmt.Fprintf(stderr, "pcap stdin: %v\n", err)
				return 1
			}
			for _, c := range caps {
				frames = append(frames, packet.DecodeFrame(c.Payload))
			}
		} else {
			sc := bufio.NewScanner(br)
			sc.Buffer(make([]byte, 0, 64*1024), 4<<20)
			for sc.Scan() {
				line := strings.TrimSpace(sc.Text())
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				raw, err := decodeHexBlob(line)
				if err != nil {
					fmt.Fprintf(stderr, "hex line: %v\n", err)
					return 1
				}
				frames = append(frames, packet.DecodeFrame(raw))
			}
			if err := sc.Err(); err != nil {
				diagErr(stderr, "stdin", err)
				return 1
			}
		}
	}

	if *limit > 0 && len(frames) > *limit {
		frames = frames[:*limit]
	}

	enc := json.NewEncoder(stdout)
	if *pretty {
		enc.SetIndent("", "  ")
	}
	for i, fr := range frames {
		rec := map[string]any{
			"ts":    time.Now().UTC().Format(time.RFC3339Nano),
			"src":   "rgodump",
			"event": "packet",
			"index": i,
			"frame": fr,
		}
		if err := enc.Encode(rec); err != nil {
			diagErr(stderr, "encode", err)
			return 1
		}
	}
	return 0
}

func decodeHexBlob(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, ":", "")
	s = strings.ReplaceAll(s, "\t", "")
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		s = s[2:]
	}
	if len(s)%2 != 0 {
		return nil, fmt.Errorf("odd hex length")
	}
	return hex.DecodeString(s)
}

func looksLikePCAP(b []byte) bool {
	if len(b) < 4 {
		return false
	}
	m := uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
	switch m {
	case 0xa1b2c3d4, 0xa1b23c4d, 0xd4c3b2a1, 0x4d3cb2a1:
		return true
	default:
		return false
	}
}
