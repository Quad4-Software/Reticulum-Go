// SPDX-License-Identifier: 0BSD
// Copyright (c) 2024-2026 Quad4.io
package packet

import (
	"fmt"
	"sync"
	"time"

	"git.quad4.io/Networks/Reticulum-Go/pkg/debug"
	"git.quad4.io/Networks/Reticulum-Go/pkg/identity"
)

// Receipt status and proof lengths.
const (
	ReceiptFailed    = 0x00
	ReceiptSent      = 0x01
	ReceiptDelivered = 0x02
	ReceiptCulled    = 0xFF

	ExplicitLength = (identity.HashLength + identity.SigLength) / 8
	ImplicitLength = identity.SigLength / 8
)

// PacketReceipt tracks delivery status and proof for a sent packet.
type PacketReceipt struct {
	mutex sync.RWMutex

	hash          []byte
	truncatedHash []byte
	sent          bool
	sentAt        time.Time
	proved        bool
	status        byte
	destination   any
	timeout       time.Duration
	concludedAt   time.Time
	proofPacket   *Packet

	deliveryCallback func(*PacketReceipt)
	timeoutCallback  func(*PacketReceipt)

	link             any
	destinationIdent *identity.Identity
	timeoutCheckDone chan bool
}

// NewPacketReceipt creates a receipt for the given packet and starts the timeout watchdog.
func NewPacketReceipt(pkt *Packet) *PacketReceipt {
	hash := pkt.Hash()
	receipt := &PacketReceipt{
		hash:             hash,
		truncatedHash:    pkt.TruncatedHash(),
		sent:             true,
		sentAt:           time.Now(),
		proved:           false,
		status:           ReceiptSent,
		destination:      pkt.Destination,
		timeout:          calculateTimeout(pkt),
		timeoutCheckDone: make(chan bool, 1),
	}

	go receipt.timeoutWatchdog()

	debug.Log(debug.DebugPackets, "Created packet receipt", "hash", fmt.Sprintf("%x", receipt.truncatedHash))
	return receipt
}

func calculateTimeout(pkt *Packet) time.Duration {
	baseTimeout := time.Duration(ReceiptTimeoutBaseSec) * time.Second
	if pkt.Hops > 0 {
		baseTimeout += time.Duration(pkt.Hops) * (time.Duration(ReceiptTimeoutPerHopSec) * time.Second)
	}
	return baseTimeout
}

func (pr *PacketReceipt) GetStatus() byte {
	pr.mutex.RLock()
	defer pr.mutex.RUnlock()
	return pr.status
}

func (pr *PacketReceipt) GetHash() []byte {
	pr.mutex.RLock()
	defer pr.mutex.RUnlock()
	return pr.hash
}

func (pr *PacketReceipt) IsDelivered() bool {
	pr.mutex.RLock()
	defer pr.mutex.RUnlock()
	return pr.status == ReceiptDelivered
}

func (pr *PacketReceipt) IsFailed() bool {
	pr.mutex.RLock()
	defer pr.mutex.RUnlock()
	return pr.status == ReceiptFailed
}

func (pr *PacketReceipt) ValidateProofPacket(proofPacket *Packet) bool {
	if proofPacket.Link != nil {
		return pr.ValidateLinkProof(proofPacket.Data, proofPacket.Link, proofPacket)
	}
	return pr.ValidateProof(proofPacket.Data, proofPacket)
}

func (pr *PacketReceipt) ValidateLinkProof(proof []byte, link any, proofPacket *Packet) bool {
	if len(proof) == ExplicitLength {
		proofHash := proof[:identity.HashLength/8]
		signature := proof[identity.HashLength/8 : identity.HashLength/8+identity.SigLength/8]

		pr.mutex.RLock()
		hashMatch := string(proofHash) == string(pr.hash)
		pr.mutex.RUnlock()

		if !hashMatch {
			return false
		}

		proofValid := pr.validateLinkSignature(signature, link)
		if proofValid {
			pr.mutex.Lock()
			pr.status = ReceiptDelivered
			pr.proved = true
			pr.concludedAt = time.Now()
			pr.proofPacket = proofPacket
			callback := pr.deliveryCallback
			pr.mutex.Unlock()

			if callback != nil {
				go callback(pr)
			}

			debug.Log(debug.DebugPackets, "Link proof validated", "hash", fmt.Sprintf("%x", pr.truncatedHash))
			return true
		}
	} else if len(proof) == ImplicitLength {
		debug.Log(debug.DebugTrace, "Implicit link proof not yet implemented")
	}

	return false
}

func (pr *PacketReceipt) ValidateProof(proof []byte, proofPacket *Packet) bool {
	if len(proof) == ExplicitLength {
		proofHash := proof[:identity.HashLength/8]
		signature := proof[identity.HashLength/8 : identity.HashLength/8+identity.SigLength/8]

		pr.mutex.RLock()
		hashMatch := string(proofHash) == string(pr.hash)
		ident := pr.destinationIdent
		pr.mutex.RUnlock()

		debug.Log(debug.DebugPackets, "Explicit proof validation", "len", len(proof), "hashMatch", hashMatch, "hasIdent", ident != nil)

		if !hashMatch {
			debug.Log(debug.DebugPackets, "Proof hash mismatch")
			return false
		}

		if ident == nil {
			debug.Log(debug.DebugVerbose, "Cannot validate proof without destination identity")
			return false
		}

		proofValid := ident.Verify(pr.hash, signature)
		debug.Log(debug.DebugPackets, "Signature verification result", "valid", proofValid)
		if proofValid {
			pr.mutex.Lock()
			pr.status = ReceiptDelivered
			pr.proved = true
			pr.concludedAt = time.Now()
			pr.proofPacket = proofPacket
			callback := pr.deliveryCallback
			pr.mutex.Unlock()

			if callback != nil {
				go callback(pr)
			}

			debug.Log(debug.DebugPackets, "Proof validated", "hash", fmt.Sprintf("%x", pr.truncatedHash))
			return true
		}
	} else if len(proof) == ImplicitLength {
		signature := proof[:identity.SigLength/8]

		pr.mutex.RLock()
		ident := pr.destinationIdent
		pr.mutex.RUnlock()

		if ident == nil {
			return false
		}

		proofValid := ident.Verify(pr.hash, signature)
		if proofValid {
			pr.mutex.Lock()
			pr.status = ReceiptDelivered
			pr.proved = true
			pr.concludedAt = time.Now()
			pr.proofPacket = proofPacket
			callback := pr.deliveryCallback
			pr.mutex.Unlock()

			if callback != nil {
				go callback(pr)
			}

			debug.Log(debug.DebugPackets, "Implicit proof validated", "hash", fmt.Sprintf("%x", pr.truncatedHash))
			return true
		}
	}

	return false
}

func (pr *PacketReceipt) validateLinkSignature(signature []byte, link any) bool {
	type linkValidator interface {
		Validate(signature, message []byte) bool
	}

	if validator, ok := link.(linkValidator); ok {
		return validator.Validate(signature, pr.hash)
	}

	debug.Log(debug.DebugTrace, "Link does not implement Validate method")
	return false
}

func (pr *PacketReceipt) GetRTT() time.Duration {
	pr.mutex.RLock()
	defer pr.mutex.RUnlock()

	if pr.concludedAt.IsZero() {
		return 0
	}

	return pr.concludedAt.Sub(pr.sentAt)
}

func (pr *PacketReceipt) IsTimedOut() bool {
	pr.mutex.RLock()
	defer pr.mutex.RUnlock()

	return time.Since(pr.sentAt) > pr.timeout
}

func (pr *PacketReceipt) checkTimeout() {
	pr.mutex.Lock()

	if pr.status != ReceiptSent {
		pr.mutex.Unlock()
		return
	}

	if time.Since(pr.sentAt) <= pr.timeout {
		pr.mutex.Unlock()
		return
	}

	if pr.timeout < 0 {
		pr.status = ReceiptCulled
	} else {
		pr.status = ReceiptFailed
	}

	pr.concludedAt = time.Now()
	callback := pr.timeoutCallback
	pr.mutex.Unlock()

	debug.Log(debug.DebugVerbose, "Packet receipt timed out", "hash", fmt.Sprintf("%x", pr.truncatedHash))

	if callback != nil {
		go callback(pr)
	}
}

func (pr *PacketReceipt) timeoutWatchdog() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			pr.checkTimeout()

			pr.mutex.RLock()
			status := pr.status
			pr.mutex.RUnlock()

			if status != ReceiptSent {
				return
			}
		case <-pr.timeoutCheckDone:
			return
		}
	}
}

func (pr *PacketReceipt) SetTimeout(timeout time.Duration) {
	pr.mutex.Lock()
	defer pr.mutex.Unlock()
	pr.timeout = timeout
}

func (pr *PacketReceipt) SetDeliveryCallback(callback func(*PacketReceipt)) {
	pr.mutex.Lock()
	defer pr.mutex.Unlock()
	pr.deliveryCallback = callback
}

func (pr *PacketReceipt) SetTimeoutCallback(callback func(*PacketReceipt)) {
	pr.mutex.Lock()
	defer pr.mutex.Unlock()
	pr.timeoutCallback = callback
}

func (pr *PacketReceipt) SetDestinationIdentity(ident *identity.Identity) {
	pr.mutex.Lock()
	defer pr.mutex.Unlock()
	pr.destinationIdent = ident
}

func (pr *PacketReceipt) SetLink(link any) {
	pr.mutex.Lock()
	defer pr.mutex.Unlock()
	pr.link = link
}

func (pr *PacketReceipt) Cancel() {
	pr.mutex.Lock()
	defer pr.mutex.Unlock()

	if pr.status == ReceiptSent {
		pr.status = ReceiptCulled
		pr.concludedAt = time.Now()
	}

	select {
	case pr.timeoutCheckDone <- true:
	default:
	}
}
