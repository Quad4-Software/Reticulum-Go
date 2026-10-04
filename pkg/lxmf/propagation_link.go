// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/destination"
	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/Reticulum-Go/pkg/link"
	"github.com/Quad4-Software/Reticulum-Go/pkg/packet"
	"github.com/Quad4-Software/Reticulum-Go/pkg/resource"
	"github.com/Quad4-Software/msgpack/v5/pkg/msgpack"
)

const (
	propagationLinkAttemptTimeout = 45 * time.Second
	pathWaitForPropagation        = 60 * time.Second
	propagationStatusInterval     = 3 * time.Second
)

func (m *Messenger) clearPropagationLink() {
	m.propLinkMu.Lock()
	lnk := m.propLink
	m.propLink = nil
	m.propLinkNode = nil
	m.propLinkMu.Unlock()
	if lnk != nil {
		// Teardown invokes the closed callback synchronously, and the
		// callback takes propLinkMu, so it cannot run under the lock.
		lnk.Teardown()
	}
}

func (m *Messenger) ensurePropagationLink(propNodeHash []byte) (*link.Link, error) {
	if m == nil || m.transport == nil {
		return nil, errors.New("lxmf: messenger not initialized")
	}
	if len(propNodeHash) != DestinationLength {
		return nil, fmt.Errorf("propagation node: %w", ErrInvalidHashLength)
	}

	m.propLinkMu.Lock()
	if m.propLink != nil && m.propLink.IsActive() && bytes.Equal(m.propLinkNode, propNodeHash) {
		lnk := m.propLink
		m.propLinkMu.Unlock()
		Verbose("propagation reusing active link", "node", hex.EncodeToString(propNodeHash))
		return lnk, nil
	}
	stale := m.propLink
	m.propLink = nil
	m.propLinkNode = nil
	m.propLinkMu.Unlock()
	if stale != nil {
		stale.Teardown()
	}

	return m.establishPropagationLink(propNodeHash)
}

func (m *Messenger) establishPropagationLink(propNodeHash []byte) (*link.Link, error) {
	nodeHex := hex.EncodeToString(propNodeHash)
	hops := m.transport.HopsTo(propNodeHash)
	Info("propagation link start", "node", nodeHex, "hops", hops, "has_path", m.transport.HasPath(propNodeHash))

	if !m.transport.HasPath(propNodeHash) {
		Info("propagation requesting path", "node", nodeHex)
		m.requestPath(propNodeHash, true)
	}

	// Upstream bounds path waits by max(timeout, path_request_wait) so a
	// slow medium gets its full round trip before the next attempt.
	pathWait := max(pathWaitForPropagation, m.pathRequestWait())
	pathDeadline := time.Now().Add(pathWait)
	lastPathLog := time.Time{}
	for !m.transport.HasPath(propNodeHash) {
		if time.Now().After(pathDeadline) {
			return nil, fmt.Errorf("propagation node %s: no path within %s", nodeHex, pathWait)
		}
		if time.Since(lastPathLog) >= propagationStatusInterval {
			Verbose("propagation waiting for path", "node", nodeHex, "hops", m.transport.HopsTo(propNodeHash))
			lastPathLog = time.Now()
		}
		time.Sleep(pathPollInterval)
	}
	Info("propagation path ready", "node", nodeHex, "hops", m.transport.HopsTo(propNodeHash))

	pnIdentity, err := identity.Recall(propNodeHash)
	if err != nil {
		return nil, fmt.Errorf("propagation node identity: %w", err)
	}
	if pnIdentity == nil {
		return nil, fmt.Errorf("propagation node %s: %w", nodeHex, ErrDestinationUnknown)
	}

	destOut, err := destination.FromHash(propNodeHash, pnIdentity, destination.Single, m.transport)
	if err != nil {
		return nil, fmt.Errorf("propagation destination: %w", err)
	}

	lnk := link.NewLink(destOut, m.transport, nil, nil, func(closed *link.Link) {
		m.propLinkMu.Lock()
		if m.propLink == closed {
			m.propLink = nil
			m.propLinkNode = nil
		}
		m.propLinkMu.Unlock()
		Verbose("propagation link closed", "node", nodeHex)
	})
	lnk.SetPacketCallback(m.onPropagationSignalling)

	Info("propagation sending link request", "node", nodeHex)
	if err := lnk.Establish(); err != nil {
		return nil, fmt.Errorf("propagation link request: %w", err)
	}
	lnk.Start()

	timeout := propagationLinkAttemptTimeout
	if hops > 0 {
		hopTimeout := time.Duration(link.EstablishmentTimeoutPerHop)*time.Second*time.Duration(hops) + 5*time.Second
		if hopTimeout < timeout {
			timeout = hopTimeout
		}
	}

	waitDeadline := time.Now().Add(timeout)
	lastStatusLog := time.Time{}
	linkID := lnk.GetLinkID()
	Info("propagation waiting for link active", "node", nodeHex, "link_id", hex.EncodeToString(linkID), "timeout", timeout.String())

	for !lnk.IsActive() {
		if time.Now().After(waitDeadline) {
			status := lnk.GetStatus()
			lnk.Teardown()
			return nil, fmt.Errorf("propagation link establishment timeout on %s (status=%d, timeout=%s)", nodeHex, status, timeout)
		}
		if time.Since(lastStatusLog) >= propagationStatusInterval {
			Verbose("propagation link pending",
				"node", nodeHex,
				"link_id", hex.EncodeToString(linkID),
				"status", lnk.GetStatus(),
				"elapsed", time.Since(waitDeadline.Add(-timeout)).Round(time.Second).String(),
			)
			lastStatusLog = time.Now()
		}
		time.Sleep(pathPollInterval)
	}

	Info("propagation link active", "node", nodeHex, "link_id", hex.EncodeToString(linkID), "rtt", lnk.RTT())

	m.propLinkMu.Lock()
	m.propLink = lnk
	m.propLinkNode = append([]byte(nil), propNodeHash...)
	m.propLinkMu.Unlock()
	return lnk, nil
}

func (m *Messenger) onPropagationSignalling(data []byte, _ *packet.Packet) {
	var payload []any
	if err := msgpack.Unmarshal(data, &payload); err != nil || len(payload) < 1 {
		return
	}
	code, ok := signalByte(payload[0])
	if !ok || code != PeerErrorInvalidStamp {
		return
	}
	Warning("propagation upload rejected: invalid stamp")
	m.clearPropagationLink()
}

func signalByte(v any) (byte, bool) {
	if b, ok := v.([]byte); ok && len(b) == 1 {
		return b[0], true
	}
	if n, ok := asInt64(v); ok && n >= 0 && n <= 0xff {
		return byte(n), true
	}
	return 0, false
}

func (m *Messenger) sendPropagationPayload(lnk *link.Link, payload []byte) error {
	if lnk == nil {
		return errors.New("lxmf: nil propagation link")
	}
	if len(payload) == 0 {
		return errors.New("lxmf: empty propagation payload")
	}

	mode := "packet"
	if len(payload) > LinkPacketMaxContent {
		mode = "resource"
	}
	Info("propagation uploading", "bytes", len(payload), "mode", mode)

	if len(payload) <= LinkPacketMaxContent {
		if err := lnk.SendPacket(payload); err != nil {
			return fmt.Errorf("propagation link packet: %w", err)
		}
		Verbose("propagation packet sent", "bytes", len(payload))
		return nil
	}

	res, err := resource.New(payload, true)
	if err != nil {
		return fmt.Errorf("propagation resource: %w", err)
	}
	// SendResource blocks until the receiver proof arrives, so a nil return
	// means the transfer completed. The resource status field is only updated
	// for inbound transfers, so it must not be polled here.
	if err := lnk.SendResource(res); err != nil {
		return fmt.Errorf("propagation link resource: %w", err)
	}
	Info("propagation resource complete", "bytes", len(payload))
	return nil
}
