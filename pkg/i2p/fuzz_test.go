// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package i2p

import (
	"strings"
	"testing"
)

func FuzzParseSAMMessage(f *testing.F) {
	f.Add("STREAM STATUS RESULT=OK\n")
	f.Add("STREAM STATUS RESULT=TIMEOUT\n")
	f.Add(`STREAM STATUS RESULT=CANT_REACH_PEER MESSAGE="LeaseSet not found"`)
	f.Add("SESSION STATUS RESULT=OK DESTINATION=abc\n")
	f.Add("DEST REPLY PUB=x PRIV=y\n")
	f.Add("")
	f.Add("oneword")
	f.Add(strings.Repeat("A", 4096))
	f.Fuzz(func(t *testing.T, line string) {
		m, err := parseMessage(line)
		if err != nil {
			return
		}
		_ = m.OK()
		_ = m.ResultError()
		_ = m.Opts["RESULT"]
		_ = m.Opts["MESSAGE"]
	})
}

func FuzzResolveDestination(f *testing.F) {
	f.Add("")
	f.Add("example.i2p")
	f.Add("abcdefghijklmnopqrstuvwxyz234567abcdefghijklmnopqrst.b32.i2p")
	f.Add("abcdefghijklmnopqrstuvwxyz234567abcdefghijklmnopqrst")
	f.Add(strings.Repeat("A", 520))
	f.Add("not-a-dest")
	f.Fuzz(func(t *testing.T, name string) {
		lookups := 0
		_, err := ResolveDestination(name, func(string) (string, error) {
			lookups++
			return "lookup", nil
		})
		_ = err
		lower := strings.ToLower(strings.TrimSpace(name))
		if validB32.MatchString(lower) && lookups != 0 {
			t.Fatalf("b32 must not invoke lookup name=%q", name)
		}
	})
}
