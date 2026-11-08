// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/cryptography"
	"github.com/Quad4-Software/Reticulum-Go/pkg/debug"
	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/Reticulum-Go/pkg/securemem"
	"github.com/Quad4-Software/Reticulum-Go/pkg/stamps"
)

// TeardownStampFailure is the RNS 1.6.0 link teardown reason for a link
// request stamp generation that timed out or was cancelled.
const TeardownStampFailure = 0x04

// linkStampCost resolves the initiator's required link-request stamp cost:
// the announced lr_cost for the destination, or LR_COST when
// ALWAYS_STAMP_LRS is on. stamped is false when no stamping applies
// (no cost, zero cost, or STAMP_LRS off).
func (l *Link) linkStampCost() (cost float64, stamped bool) {
	settings := common.DefaultStampsSettings()
	if l.transport != nil {
		settings = l.transport.GetConfig().StampsSettings()
	}
	cost, ok := identity.RecallLinkCost(l.destination.GetHash())
	if !ok {
		if !settings.AlwaysStampLRs {
			return 0, false
		}
		cost = settings.LRCost
	}
	if cost <= 0 || !settings.StampLRs {
		return 0, false
	}
	return cost, true
}

// stampThenEstablish runs the deferred-key path of Link.establish (Python
// __defer_keys): the sig keypair is generated first, then the ephemeral
// X25519 key is mined as a key-stamp, and only then is the link request
// built, registered, and sent. On timeout or failure the link closes with
// teardown reason STAMP_FAILURE.
func (l *Link) stampThenEstablish(destHash []byte, cost float64) {
	settings := common.DefaultStampsSettings()
	if l.transport != nil {
		settings = l.transport.GetConfig().StampsSettings()
	}
	timeout := settings.LinkStampTimeout
	if timeout <= 0 {
		timeout = 15
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout*float64(time.Second)))
	defer cancel()

	// Ed25519 signalling keypair first; the stamp material binds to it.
	sigPub, sigPriv, err := cryptography.GenerateSigningKeyPair()
	if err != nil {
		l.failStampedEstablish(destHash, fmt.Errorf("signing keypair: %w", err))
		return
	}
	material := append(append([]byte{}, destHash...), sigPub...)

	priv, pub, _, err := stamps.GenerateStampedKey(ctx, material, cost)
	securemem.WipeBytes(material)
	if err != nil {
		securemem.WipeBytes(sigPriv)
		l.failStampedEstablish(destHash, fmt.Errorf("stamp generation: %w", err))
		return
	}

	l.mutex.Lock()
	if l.status.Load() != int32(StatusPending) || l.destination == nil {
		// Closed while stamping: cancel path per Python teardown.
		l.mutex.Unlock()
		securemem.WipeBytes(sigPriv)
		securemem.WipeBytes(priv[:])
		return
	}
	if err := setSecBuf(&l.prv, priv[:]); err != nil {
		l.mutex.Unlock()
		securemem.WipeBytes(sigPriv)
		securemem.WipeBytes(priv[:])
		l.failStampedEstablish(destHash, err)
		return
	}
	securemem.WipeBytes(priv[:])
	l.pub = pub[:]
	if err := setSecBuf(&l.sigPriv, sigPriv); err != nil {
		l.mutex.Unlock()
		securemem.WipeBytes(sigPriv)
		l.failStampedEstablish(destHash, err)
		return
	}
	securemem.WipeBytes(sigPriv)
	l.sigPub = sigPub
	l.stampCost = cost
	l.stampStatus = true

	if err := l.prepareLinkRequestLocked(); err != nil {
		l.requestTime = time.Time{}
		l.transport.EndOutboundEstablish(destHash)
		l.mutex.Unlock()
		l.failStampedEstablish(destHash, err)
		return
	}

	l.transport.RegisterLink(l.linkID, l)
	if l.networkInterface == nil {
		if ifaceName := l.transport.NextHopInterface(destHash); ifaceName != "" {
			if iface, err := l.transport.GetInterface(ifaceName); err == nil {
				l.networkInterface = iface
			}
		}
	}
	if l.networkInterface != nil {
		l.registerLinkPath()
	}
	l.mutex.Unlock()

	if err := l.sendPreparedLinkRequest(); err != nil {
		l.mutex.Lock()
		l.markInitiatorEstablishmentFailedLocked()
		l.mutex.Unlock()
		return
	}
	go l.startWatchdog()
	debug.Log(debug.DebugVerbose, "Stamped link request sent",
		"link_id", fmt.Sprintf("%x", l.linkID), "cost", cost)
}

// failStampedEstablish closes the link with teardown reason STAMP_FAILURE,
// mirrors Python Link.__stamp_failed.
func (l *Link) failStampedEstablish(destHash []byte, err error) {
	debug.Log(debug.DebugWarning, "Link request stamp generation failed",
		"dest_hash", fmt.Sprintf("%x", destHash), "error", err)
	l.mutex.Lock()
	if l.transport != nil {
		l.transport.EndOutboundEstablish(destHash)
	}
	l.mutex.Unlock()
	if l.closeOnce(TeardownStampFailure) {
		if l.closedCallback != nil {
			l.closedCallback(l)
		}
	}
}

// generateStampedKeys is replaced by stampThenEstablish's inline path; kept
// for tests exercising the stamped key fields directly.
func (l *Link) generateStampedKeys(ctx context.Context, material []byte, cost float64) error {
	sigPub, sigPriv, err := cryptography.GenerateSigningKeyPair()
	if err != nil {
		return err
	}
	priv, pub, _, err := stamps.GenerateStampedKey(ctx, material, cost)
	if err != nil {
		securemem.WipeBytes(sigPriv)
		return err
	}
	if err := setSecBuf(&l.prv, priv[:]); err != nil {
		securemem.WipeBytes(priv[:])
		securemem.WipeBytes(sigPriv)
		return err
	}
	securemem.WipeBytes(priv[:])
	l.pub = pub[:]
	if err := setSecBuf(&l.sigPriv, sigPriv); err != nil {
		securemem.WipeBytes(sigPriv)
		return err
	}
	securemem.WipeBytes(sigPriv)
	l.sigPub = sigPub
	return nil
}

// StampCost reports the computed link-request stamp value (responder) or
// the target cost (initiator). ok is false when unset.
func (l *Link) StampCost() (float64, bool) {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	if !l.stampStatus {
		return 0, false
	}
	return l.stampCost, true
}

// StampStatus reports whether stamp validation ran for this link
// (Python Link.stamp_status).
func (l *Link) StampStatus() bool {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	return l.stampStatus
}

// validateLinkRequestStamp mirrors Python's responder-side check:
// value = stamp_value(owner.hash + peer_sig_pub, peer_pub); when
// LR_STAMPS_ENFORCED the stamp must satisfy owner.get_link_cost().
func (l *Link) validateLinkRequestStamp(destHash []byte, linkCost float64, peerPub, peerSigPub []byte) (float64, error) {
	material := append(append([]byte{}, destHash...), peerSigPub...)
	value := stamps.StampValue(material, peerPub)
	settings := common.DefaultStampsSettings()
	if l.transport != nil {
		settings = l.transport.GetConfig().StampsSettings()
	}
	if settings.LREnforced && !stamps.StampValid(peerPub, linkCost, material) {
		return value, errors.New("insufficient link request stamp value")
	}
	return value, nil
}
