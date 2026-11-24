// SPDX-License-Identifier: LicenseRef-Reticulum
package rrc

import (
	"bytes"
	"errors"
	"fmt"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/internal/testconfig"
	"strings"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/link"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
)

func TestDialTimeoutNoPath(t *testing.T) {
	cfg := testconfig.New()
	cfg.ShareInstance = false
	cfg.InMemoryPathTable = true
	cfg.InMemoryKnownDestinations = true
	cfg.ConfigPath = t.TempDir() + "/config"
	tr := transport.NewTransport(cfg)
	if err := tr.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tr.Close() })
	id, err := identity.NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	_, err = Dial(tr, id, bytes.Repeat([]byte{0xaa}, IdentityLength), ClientConfig{DialTimeout: 300 * time.Millisecond})
	if !errors.Is(err, ErrDialTimeout) {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(err.Error(), "path") {
		t.Fatalf("err = %v", err)
	}
}

func TestDialHashGrouped(t *testing.T) {
	if testing.Short() {
		t.Skip("live dial skipped in -short")
	}
	m := newTestMesh(t, 43110, HubConfig{Limits: HubLimits{RateLimitMsgsPerMinute: 60}})
	c, err := DialHash(m.trA, m.idA, FormatHash(m.hubHash), ClientConfig{DialTimeout: 10 * time.Second, WelcomeTimeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	if c.State() != ClientActive {
		t.Fatalf("state %v", c.State())
	}
}

func TestDialWelcomeBodyInvalid(t *testing.T) {
	if testing.Short() {
		t.Skip("live dial skipped in -short")
	}
	cfgH := testconfig.New()
	cfgA := testconfig.New()
	cfgH.ShareInstance = false
	cfgA.ShareInstance = false
	cfgH.InMemoryPathTable = true
	cfgA.InMemoryPathTable = true
	cfgH.InMemoryKnownDestinations = true
	cfgA.InMemoryKnownDestinations = true
	cfgH.ConfigPath = t.TempDir() + "/h"
	cfgA.ConfigPath = t.TempDir() + "/a"
	trH := transport.NewTransport(cfgH)
	trA := transport.NewTransport(cfgA)
	if err := trH.Start(); err != nil {
		t.Fatal(err)
	}
	if err := trA.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = trH.Close()
		_ = trA.Close()
	})
	base := 43120
	ifaceH := startTestUDP(t, "WH", fmt.Sprintf("127.0.0.1:%d", base), fmt.Sprintf("127.0.0.1:%d", base+1), trH)
	ifaceA := startTestUDP(t, "WA", fmt.Sprintf("127.0.0.1:%d", base+1), fmt.Sprintf("127.0.0.1:%d", base), trA)
	t.Cleanup(func() {
		_ = ifaceH.Stop()
		_ = ifaceA.Stop()
	})
	idH, err := identity.NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	idA, err := identity.NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	dest, err := NewHubDestination(idH, trH)
	if err != nil {
		t.Fatal(err)
	}
	dest.SetLinkEstablishedCallback(func(v any) {
		lnk, ok := v.(*link.Link)
		if !ok || lnk == nil {
			return
		}
		lnk.Start()
		sess := newSession(lnk, idH.Hash(), false, nil, nil)
		env, envErr := NewEnvelope(TypeWelcome, idH.Hash())
		if envErr != nil {
			return
		}
		env.Body = "nope"
		env.HasBody = true
		_ = sess.sendEnvelope(env)
	})
	if err := dest.Announce(false, nil, nil); err != nil {
		t.Fatal(err)
	}
	hubHash := dest.GetHash()
	identity.Remember(nil, hubHash, idH.GetPublicKey(), nil)
	deadline := time.Now().Add(rrcPathWait)
	for !trA.HasPath(hubHash) {
		if time.Now().After(deadline) {
			t.Fatal("path timeout")
		}
		_ = dest.Announce(false, nil, nil)
		time.Sleep(40 * time.Millisecond)
	}
	_, err = Dial(trA, idA, hubHash, ClientConfig{DialTimeout: 10 * time.Second, WelcomeTimeout: 10 * time.Second})
	if !errors.Is(err, ErrInvalidEnvelope) {
		t.Fatalf("err = %v", err)
	}
}
