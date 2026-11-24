// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/link"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/packet"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/resource"
)

// Receive implements inbound delivery for the lxmf.delivery destination.
func (r *Router) Receive(pkt *packet.Packet, iface common.NetworkInterface) bool {
	if pkt == nil || r.deliveryDest == nil {
		return false
	}
	if pkt.PacketType == packet.PacketTypeLinkReq {
		return r.deliveryDest.Receive(pkt, iface)
	}

	plaintext, err := r.decryptDelivery(pkt.Data)
	if err != nil {
		Warning("router delivery decrypt failed", "error", err)
		return false
	}
	_ = sendDeliveryProof(r.deliveryDest, pkt, iface)

	method := MethodOpportunistic
	lxmfData := plaintext
	if pkt.DestinationType != packet.DestinationLink {
		lxmfData = append(append([]byte(nil), r.deliveryDest.GetHash()...), plaintext...)
	} else {
		method = MethodDirect
	}
	r.handleDeliveryPayload(lxmfData, method, false, false)
	return true
}

func (r *Router) deliveryLinkEstablished(v any) {
	lnk, ok := v.(*link.Link)
	if !ok || lnk == nil {
		return
	}
	_ = lnk.SetResourceStrategy(link.AcceptApp)
	lnk.SetPacketCallback(func(data []byte, _ *packet.Packet) {
		r.handleDeliveryPayload(data, MethodDirect, false, false)
	})
	lnk.SetResourceCallback(func(adv any) bool {
		return r.deliveryResourceAdvertised(adv)
	})
	lnk.SetResourceConcludedCallback(func(res any) {
		r.deliveryResourceConcluded(res)
	})
	linkKey := hex.EncodeToString(lnk.GetLinkID())
	r.linksMu.Lock()
	r.directLinks[linkKey] = lnk
	r.linksMu.Unlock()
	lnk.SetLinkClosedCallback(func(closed *link.Link) {
		r.linksMu.Lock()
		delete(r.directLinks, linkKey)
		r.linksMu.Unlock()
	})
}

func (r *Router) deliveryResourceAdvertised(adv any) bool {
	var size int64
	switch a := adv.(type) {
	case *resource.ResourceAdvertisement:
		if a != nil {
			size = a.DataSize
		}
	default:
	}
	limit := int64(r.cfg.LXMF.DeliveryTransferMaxAcceptedSize * 1000)
	if limit > 0 && size > limit {
		return false
	}
	return true
}

func (r *Router) deliveryResourceConcluded(res any) {
	data := extractResourceData(res)
	if len(data) == 0 {
		return
	}
	r.handleDeliveryPayload(data, MethodDirect, false, false)
}

func (r *Router) handleDeliveryPayload(lxmfData []byte, method byte, noStampEnforcement, allowDuplicate bool) {
	defer func() {
		if rec := recover(); rec != nil {
			Error("router delivery panic", "error", rec)
		}
	}()

	if r.deliveryDest == nil {
		return
	}
	if len(lxmfData) < 2*DestinationLength+SignatureLength {
		return
	}

	localHash := r.deliveryDest.GetHash()
	inner := lxmfData[DestinationLength:]
	msg, err := UnpackFromBytes(localHash, inner, RecallSource)
	if err != nil && msg == nil {
		Warning("router delivery unpack failed", "error", err)
		return
	}
	if msg == nil {
		return
	}

	msg.Incoming = true
	msg.Method = method

	// Upstream remembers tickets carried in FIELD_TICKET before the
	// ignored check so valid tickets are kept even when the source is
	// subsequently ignored.
	rememberTicketField(r.tickets, msg)

	r.mu.RLock()
	stampCost := r.inboundStampCost
	enforce := r.enforceStamps
	_, ignored := r.ignoredList[peerKey(msg.SourceHash)]
	r.mu.RUnlock()

	if ignored {
		return
	}

	if stampCost != nil && *stampCost > 0 {
		ok, err := msg.ValidateStamp(*stampCost, r.tickets.inboundTickets(msg.SourceHash))
		if err == nil {
			msg.StampValid = ok
		}
		if !msg.StampValid && enforce && !noStampEnforcement {
			Warning("dropping message with invalid stamp", "hash", hex.EncodeToString(msg.Hash))
			return
		}
	}

	if !allowDuplicate && r.hasMessage(msg.Hash) {
		return
	}

	r.deliveredMu.Lock()
	r.locallyDelivered[peerKey(msg.Hash)] = float64(time.Now().Unix())
	r.deliveredMu.Unlock()

	var savedPath string
	if r.messagesDir != "" {
		if err := os.MkdirAll(r.messagesDir, 0o700); err == nil {
			path, err := WriteToDirectory(msg, r.messagesDir)
			if err != nil {
				Warning("write inbound message failed", "error", err)
			} else {
				savedPath = path
			}
		}
	}

	if r.onInbound != "" && savedPath != "" {
		cmd := exec.Command(r.onInbound, savedPath) // #nosec G204 -- operator configured hook
		_ = cmd.Run()
	}

	Info("router delivered message", "hash", hex.EncodeToString(msg.Hash), "method", method)
}

func (r *Router) decryptDelivery(ciphertext []byte) ([]byte, error) {
	if r.deliveryDest == nil {
		return nil, errors.New("lxmf: no delivery destination")
	}
	id := r.deliveryDest.GetIdentity()
	if id == nil {
		return nil, errors.New("no identity available for decryption")
	}
	ratchets := r.deliveryDest.GetRatchets()
	if idRatchets := id.GetRatchets(); len(idRatchets) > 0 {
		ratchets = append(ratchets, idRatchets...)
	}
	receiver := &common.RatchetIDReceiver{}
	return id.Decrypt(ciphertext, ratchets, false, receiver)
}

func extractResourceData(res any) []byte {
	switch v := res.(type) {
	case []byte:
		return v
	case link.IncomingResource:
		return v.Data
	default:
		return nil
	}
}

func resourceAdvertisedSize(adv any) int64 {
	switch a := adv.(type) {
	case *resource.ResourceAdvertisement:
		if a != nil {
			return a.DataSize
		}
	}
	return 0
}
