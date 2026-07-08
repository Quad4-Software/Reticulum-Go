// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package i2p

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"quad4/reticulum-go/pkg/cryptography"
	"quad4/reticulum-go/pkg/debug"
)

// Controller manages SAM tunnels with bounded setup timeouts and clean shutdown.
type Controller struct {
	client      *Client
	storagePath string
	mu          sync.Mutex
	clientTuns  map[string]*ClientTunnel
	serverTuns  map[string]*ServerTunnel
	ctx         context.Context
	cancel      context.CancelFunc
}

func NewController(storagePath, samAddress string) *Controller {
	if storagePath != "" {
		_ = os.MkdirAll(storagePath, 0o700)
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Controller{
		client:      NewClient(samAddress),
		storagePath: storagePath,
		clientTuns:  make(map[string]*ClientTunnel),
		serverTuns:  make(map[string]*ServerTunnel),
		ctx:         ctx,
		cancel:      cancel,
	}
}

func (c *Controller) Stop() {
	c.cancel()
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, t := range c.clientTuns {
		t.Stop()
	}
	for _, t := range c.serverTuns {
		t.Stop()
	}
	c.clientTuns = make(map[string]*ClientTunnel)
	c.serverTuns = make(map[string]*ServerTunnel)
}

func (c *Controller) FreePort() (int, error) {
	return FreePort()
}

// StartClientTunnel brings up or returns an existing client tunnel to dest.
func (c *Controller) StartClientTunnel(dest string, localPort int) (*ClientTunnel, error) {
	c.mu.Lock()
	if t, ok := c.clientTuns[dest]; ok {
		st := t.Status()
		if st.SetupRan && !st.SetupFailed {
			c.mu.Unlock()
			return t, nil
		}
		t.Stop()
		delete(c.clientTuns, dest)
	}
	c.mu.Unlock()

	tun, err := NewClientTunnel(c.client, dest, localPort)
	if err != nil {
		return nil, err
	}
	if err := tun.Run(c.ctx); err != nil {
		return nil, err
	}
	st := tun.Status()
	for !st.SetupRan && c.ctx.Err() == nil {
		time.Sleep(50 * time.Millisecond)
		st = tun.Status()
	}
	if st.SetupFailed {
		return nil, st.Err
	}
	c.mu.Lock()
	c.clientTuns[dest] = tun
	c.mu.Unlock()
	return tun, nil
}

// StartServerTunnel publishes a local service on I2P using a persistent destination key.
func (c *Controller) StartServerTunnel(ifaceName string, transportID []byte, localPort int) (*ServerTunnel, *Destination, error) {
	dest, err := c.loadOrCreateDestination(ifaceName, transportID)
	if err != nil {
		return nil, nil, err
	}
	key := dest.Base32()
	c.mu.Lock()
	if t, ok := c.serverTuns[key]; ok {
		st := t.Status()
		if st.SetupRan && !st.SetupFailed {
			c.mu.Unlock()
			return t, dest, nil
		}
		t.Stop()
		delete(c.serverTuns, key)
	}
	c.mu.Unlock()

	tun, err := NewServerTunnel(c.client, dest, localPort)
	if err != nil {
		return nil, nil, err
	}
	if err := tun.Run(c.ctx); err != nil {
		return nil, nil, err
	}
	st := tun.Status()
	for !st.SetupRan && c.ctx.Err() == nil {
		time.Sleep(50 * time.Millisecond)
		st = tun.Status()
	}
	if st.SetupFailed {
		return nil, nil, st.Err
	}
	c.mu.Lock()
	c.serverTuns[key] = tun
	c.mu.Unlock()
	debug.Log(debug.DebugInfo, "I2P server endpoint ready", "b32", dest.Base32()+".b32.i2p")
	return tun, dest, nil
}

func (c *Controller) loadOrCreateDestination(ifaceName string, transportID []byte) (*Destination, error) {
	nameHash := cryptography.Hash(cryptography.Hash([]byte(ifaceName)))
	oldFile := filepath.Join(c.storagePath, hex.EncodeToString(nameHash)+".i2p")
	newMaterial := append(append([]byte(nil), nameHash...), cryptography.Hash(transportID)...)
	newHash := cryptography.Hash(newMaterial)
	newFile := filepath.Join(c.storagePath, hex.EncodeToString(newHash)+".i2p")

	keyFile := newFile
	if _, err := os.Stat(oldFile); err == nil {
		keyFile = oldFile
	}
	if data, err := os.ReadFile(keyFile); err == nil { // #nosec G304 -- keyFile is filepath.Join(storagePath, hex hash)
		return NewDestinationFromPrivateB64(string(data))
	}
	ctx, cancel := context.WithTimeout(c.ctx, defaultSetupTimeout)
	defer cancel()
	dest, err := c.client.DestGenerate(ctx)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(keyFile, []byte(dest.PrivateKeyB64()), 0o600); err != nil {
		return nil, fmt.Errorf("i2p: persist destination: %w", err)
	}
	return dest, nil
}

func GenerateSessionID() string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	out := make([]byte, 8)
	for i := range out {
		out[i] = letters[int(b[i])%len(letters)]
	}
	return "reticulum-" + string(out)
}
