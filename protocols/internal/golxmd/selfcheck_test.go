// SPDX-License-Identifier: LicenseRef-Reticulum
package golxmd

import (
	"testing"
)

func TestRunSelfCheck(t *testing.T) {
	home := t.TempDir()
	results := RunSelfCheck(SelfCheckOptions{Home: home})
	if !SelfCheckPassed(results) {
		t.Fatalf("self-check failed: %+v", results)
	}
}

func TestRunSelfCheckUDP(t *testing.T) {
	if testing.Short() {
		t.Skip("udp self-check skipped in -short mode")
	}
	home := t.TempDir()
	results := RunSelfCheck(SelfCheckOptions{
		Home:       home,
		UDPListen:  "127.0.0.1:43300",
		UDPForward: "127.0.0.1:43301",
	})
	if !SelfCheckPassed(results) {
		t.Fatalf("self-check failed: %+v", results)
	}
}

func TestResolveRNSConfigDir(t *testing.T) {
	if got := ResolveRNSConfigDir(""); got == "" {
		t.Fatal("expected default RNS config dir")
	}
	if got := ResolveRNSConfigDir("/tmp/custom-rns"); got != "/tmp/custom-rns" {
		t.Fatalf("override = %q", got)
	}
}

func TestDefaultRNSConfigDir(t *testing.T) {
	dir := DefaultRNSConfigDir()
	if dir == "" {
		t.Fatal("empty default RNS config dir")
	}
}
