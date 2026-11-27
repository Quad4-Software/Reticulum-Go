// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/link"
)

const (
	// propagationRequestTimeout bounds each /get request round trip.
	propagationRequestTimeout = 60 * time.Second
	// requestPollInterval is the receipt poll step while awaiting a
	// response.
	requestPollInterval = 10 * time.Millisecond
	// propagationIdentifyRetries bounds list-request retries while the
	// identify packet settles on the remote link.
	propagationIdentifyRetries    = 5
	propagationIdentifyRetryDelay = 200 * time.Millisecond
)

// propagationError carries a wire error code returned by the node.
type propagationError struct{ code byte }

func (e *propagationError) Error() string {
	return fmt.Sprintf("lxmf: propagation get rejected: error 0x%02x", e.code)
}

// FetchAllMessages matches upstream PR_ALL_MESSAGES.
const FetchAllMessages = 0

// DeliveryLimitKBDefault matches upstream LXMRouter.DELIVERY_LIMIT, the
// per-request transfer cap clients send to propagation nodes.
const DeliveryLimitKBDefault = 1000

// FetchPropagated retrieves messages waiting on a propagation node for this
// messenger's delivery destination, matching upstream
// request_messages_from_propagation_node, message_list_response and
// message_get_response. It returns the number of messages delivered to the
// inbound handler. Received transient ids are sent back to the node so it
// can purge them from its store. A transferLimitKB of zero or less sends the
// upstream default delivery limit. On the wire a literal zero means no
// transfer at all.
func (m *Messenger) FetchPropagated(propNodeHash []byte, maxMessages int, transferLimitKB float64) (int, error) {
	if m == nil || m.transport == nil {
		return 0, errors.New("lxmf: messenger not initialized")
	}
	if transferLimitKB <= 0 {
		transferLimitKB = DeliveryLimitKBDefault
	}
	lnk, err := m.ensurePropagationLink(propNodeHash)
	if err != nil {
		return 0, err
	}

	// Upstream identifies on the link so the node can authorize the
	// request and purge the right delivery destination. Inbound packets
	// are processed concurrently, so a request can reach the node before
	// the identify lands. Retry the list request on NO_IDENTITY.
	var list []any
	for attempt := 0; ; attempt++ {
		if id := m.dest.GetIdentity(); id != nil {
			_ = lnk.Identify(id)
		}
		list, err = m.propagationGetRequest(lnk, []any{nil, nil})
		var perr *propagationError
		if errors.As(err, &perr) && perr.code == PeerErrorNoIdentity && attempt < propagationIdentifyRetries {
			time.Sleep(propagationIdentifyRetryDelay)
			continue
		}
		break
	}
	if err != nil {
		m.clearPropagationLink()
		return 0, err
	}
	if len(list) == 0 {
		return 0, nil
	}

	// Upstream always sends lists for the want and have fields, even
	// when empty, so they are built as []any rather than nil slices.
	m.mu.RLock()
	retain := m.retainSyncedOnNode
	m.mu.RUnlock()
	wants := []any{}
	haves := []any{}
	for _, item := range list {
		tid, ok := item.([]byte)
		if !ok || len(tid) == 0 {
			continue
		}
		if m.hasDelivered(tid) {
			if !retain {
				haves = append(haves, tid)
			}
			continue
		}
		if maxMessages <= 0 || len(wants) < maxMessages {
			wants = append(wants, tid)
		}
	}

	msgs, err := m.propagationGetRequest(lnk, []any{wants, haves, transferLimitKB})
	if err != nil {
		// Upstream tears down the link on NO_IDENTITY and NO_ACCESS.
		m.clearPropagationLink()
		return 0, err
	}

	received := 0
	receivedIDs := make([][]byte, 0, len(msgs))
	for _, item := range msgs {
		data, ok := item.([]byte)
		if !ok {
			continue
		}
		sum := sha256.Sum256(data)
		receivedIDs = append(receivedIDs, sum[:])
		if len(data) <= Overhead {
			continue
		}
		m.deliverPropagated(data)
		received++
	}

	// Upstream returns the received transient ids so the node purges
	// them from its store. The response is ignored.
	if len(receivedIDs) > 0 {
		_, _ = lnk.Request(PathMessageGet, []any{nil, receivedIDs}, propagationRequestTimeout)
	}
	return received, nil
}

// propagationGetRequest sends one /get request and waits for the decoded
// response, mirroring the upstream request receipt flow.
func (m *Messenger) propagationGetRequest(lnk *link.Link, payload []any) ([]any, error) {
	receipt, err := lnk.Request(PathMessageGet, payload, propagationRequestTimeout)
	if err != nil {
		return nil, fmt.Errorf("propagation get request: %w", err)
	}

	deadline := time.Now().Add(propagationRequestTimeout + 5*time.Second)
	for !receipt.Concluded() {
		if time.Now().After(deadline) {
			return nil, errors.New("lxmf: propagation get request timed out")
		}
		time.Sleep(requestPollInterval)
	}
	if receipt.GetStatus() != link.StatusActive {
		return nil, errors.New("lxmf: propagation get request failed")
	}

	resp := receipt.GetResponseValue()
	if code, ok := responseErrorCode(resp); ok {
		return nil, &propagationError{code: code}
	}
	list, ok := resp.([]any)
	if !ok {
		if resp == nil {
			// A nil response is a valid empty result upstream.
			return nil, nil
		}
		return nil, fmt.Errorf("lxmf: invalid propagation get response type %T", resp)
	}
	return list, nil
}

// deliverPropagated decrypts a stored wire payload and hands the inner
// plaintext to the inbound handler, matching upstream lxmf_propagation
// local delivery.
func (m *Messenger) deliverPropagated(lxmfData []byte) {
	if len(lxmfData) < DestinationLength {
		return
	}
	if !bytes.Equal(lxmfData[:DestinationLength], m.DestinationHash()) {
		return
	}
	plaintext, err := m.decryptInbound(lxmfData[DestinationLength:])
	if err != nil {
		m.receiveError(fmt.Errorf("propagated decrypt: %w", err))
		return
	}
	m.noteDelivered(lxmfData)
	m.onPacket(plaintext, nil)
}
