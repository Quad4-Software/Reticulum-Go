// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"bytes"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/debug"
	"github.com/Quad4-Software/Reticulum-Go/pkg/health"
	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/Reticulum-Go/pkg/packet"
	"github.com/Quad4-Software/Reticulum-Go/pkg/resource"
	"github.com/Quad4-Software/msgpack/v5/pkg/msgpack"
)

func (l *Link) Request(path string, data any, timeout time.Duration) (*RequestReceipt, error) {
	return l.RequestLimited(path, data, timeout, 0)
}

// RequestLimited sends a request with an optional max accepted response size
// in bytes (RNS 1.4.1 max_response_size). Zero means unlimited.
func (l *Link) RequestLimited(path string, data any, timeout time.Duration, maxResponseSize int) (*RequestReceipt, error) {
	l.mutex.Lock()
	if l.status.Load() != int32(StatusActive) {
		l.mutex.Unlock()
		return nil, common.ErrLinkNotActive
	}

	pathHash := identity.TruncatedHash([]byte(path))
	requestData := []any{time.Now().Unix(), pathHash, data}
	packedRequest, err := msgpack.Marshal(requestData)
	if err != nil {
		l.mutex.Unlock()
		return nil, fmt.Errorf("failed to pack request: %w", err)
	}

	if timeout <= 0 {
		timeout = time.Duration(l.rtt*TrafficTimeoutFactor*float64(time.Second)) + time.Duration(resource.ResponseMaxGraceTime*1.125*float64(time.Second))
	}

	if len(packedRequest) <= l.mdu {
		reqPkt := &packet.Packet{
			HeaderType:      packet.HeaderType1,
			PacketType:      packet.PacketTypeData,
			TransportType:   0,
			Context:         packet.ContextRequest,
			ContextFlag:     packet.FlagUnset,
			Hops:            0,
			DestinationType: DestTypeLink,
			DestinationHash: l.linkID,
			Data:            packedRequest,
			CreateReceipt:   false,
		}

		if err := reqPkt.Pack(); err != nil {
			l.mutex.Unlock()
			return nil, err
		}

		encrypted, err := l.encryptLocked(packedRequest)
		if err != nil {
			l.mutex.Unlock()
			return nil, err
		}

		reqPkt.Data = encrypted
		reqPkt.Packed = false
		if err := reqPkt.Pack(); err != nil {
			l.mutex.Unlock()
			return nil, err
		}

		requestID := reqPkt.TruncatedHash()
		receipt := &RequestReceipt{
			link:            l,
			requestID:       requestID,
			pathHash:        pathHash,
			status:          StatusPending,
			sentAt:          time.Now(),
			timeout:         timeout,
			maxResponseSize: maxResponseSize,
		}

		if err := l.registerPendingRequest(receipt); err != nil {
			l.mutex.Unlock()
			return nil, err
		}
		l.mutex.Unlock()

		debug.Log(debug.DebugVerbose, "Sending request", "path", path, "request_id", fmt.Sprintf("%x", requestID))
		if err := l.transport.SendPacket(reqPkt); err != nil {
			l.failPendingRequest(receipt)
			return nil, fmt.Errorf("failed to send request: %w", err)
		}

		go receipt.startTimeout()

		return receipt, nil
	}
	l.mutex.Unlock()

	// Oversized requests are transferred as a resource.
	requestID := identity.TruncatedHash(packedRequest)
	res, err := resource.New(packedRequest, false)
	if err != nil {
		return nil, fmt.Errorf("failed to create request resource: %w", err)
	}
	res.SetRequestID(requestID)
	res.SetIsResponse(false)

	receipt := &RequestReceipt{
		link:            l,
		requestID:       requestID,
		pathHash:        pathHash,
		status:          StatusPending,
		sentAt:          time.Now(),
		timeout:         timeout,
		maxResponseSize: maxResponseSize,
	}

	if err := l.registerPendingRequest(receipt); err != nil {
		return nil, err
	}

	debug.Log(debug.DebugVerbose, "Sending request as resource", "path", path, "request_id", fmt.Sprintf("%x", requestID), "packed_len", len(packedRequest))
	go func() {
		if err := l.SendResource(res); err != nil {
			debug.Log(debug.DebugError, "Failed to send request resource", "request_id", fmt.Sprintf("%x", requestID), "error", err)
			l.failPendingRequest(receipt)
			return
		}
		// Match Python RequestReceipt: the response window starts when the
		// request resource concludes (DELIVERED), so a slow upload does not
		// consume the response timeout.
		go receipt.startTimeout()
	}()

	return receipt, nil
}
func (l *Link) registerPendingRequest(receipt *RequestReceipt) error {
	if l == nil || receipt == nil {
		return common.ErrLinkRequestBusy
	}
	l.requestMutex.Lock()
	defer l.requestMutex.Unlock()
	if len(l.pendingRequests) >= MaxPendingRequests {
		debug.Log(debug.DebugWarning, "Link request rejected, too many in flight",
			"pending", len(l.pendingRequests),
			"max", MaxPendingRequests,
			"hint", "wait for receipts, do not loop Request")
		return common.ErrLinkRequestBusy
	}
	if len(receipt.pathHash) > 0 {
		for _, pending := range l.pendingRequests {
			if pending != nil && bytes.Equal(pending.pathHash, receipt.pathHash) {
				debug.Log(debug.DebugWarning, "Link request rejected, duplicate path in flight",
					"hint", "wait for the receipt")
				return common.ErrLinkRequestDuplicate
			}
		}
	}
	l.pendingRequests = append(l.pendingRequests, receipt)
	return nil
}

type RequestReceipt struct {
	link            *Link
	mutex           sync.RWMutex
	requestID       []byte
	pathHash        []byte
	status          byte
	sentAt          time.Time
	receivedAt      time.Time
	response        []byte
	responseValue   any
	metadata        map[string]any
	timeout         time.Duration
	bytesReceived   int64
	totalBytes      int64
	maxResponseSize int
	responseCb      func(*RequestReceipt)
	failedCb        func(*RequestReceipt)
	progressCb      func(*RequestReceipt)
}

func (r *RequestReceipt) GetRequestID() []byte {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return append([]byte{}, r.requestID...)
}
func (r *RequestReceipt) GetStatus() byte {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return r.status
}
func (r *RequestReceipt) GetResponse() []byte {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	if r.response == nil {
		return nil
	}
	return append([]byte{}, r.response...)
}

// GetResponseValue returns the decoded response payload as any (bytes, bool,
// int, or other msgpack value). Used by fetch_file status codes from rncp.
func (r *RequestReceipt) GetResponseValue() any {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	if r.responseValue != nil {
		return r.responseValue
	}
	if r.response != nil {
		return append([]byte{}, r.response...)
	}
	return nil
}

// GetMetadata returns the metadata attached to a response delivered as a
// resource transfer (e.g. a file's name in nomadnetwork /file/ requests).
// It returns nil if the response carried no metadata.
func (r *RequestReceipt) GetMetadata() map[string]any {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return r.metadata
}

// Progress returns how many bytes of the response have arrived so far and
// the total number of bytes expected, for responses delivered as a resource
// transfer (e.g. large /file/ downloads). total is 0 until the resource
// advertisement carrying the transfer size has been received. Both values
// are 0 for responses that never go through a resource transfer.
func (r *RequestReceipt) Progress() (received int64, total int64) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return r.bytesReceived, r.totalBytes
}
func (r *RequestReceipt) GetResponseTime() float64 {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	if r.receivedAt.IsZero() {
		return 0.0
	}
	return r.receivedAt.Sub(r.sentAt).Seconds()
}
func (r *RequestReceipt) Concluded() bool {
	status := r.GetStatus()
	return status == StatusActive || status == StatusFailed
}
func (l *Link) removePendingRequest(req *RequestReceipt) {
	l.requestMutex.Lock()
	for i, pending := range l.pendingRequests {
		if pending == req {
			l.pendingRequests = append(l.pendingRequests[:i], l.pendingRequests[i+1:]...)
			break
		}
	}
	l.requestMutex.Unlock()
}

// failPendingRequest moves a pending or receiving receipt to FAILED, drops
// it from pendingRequests, and fires the failed callback. Idempotent; a
// concluded receipt is left alone so a late failure cannot resurrect it or
// double-fire callbacks.
func (l *Link) failPendingRequest(req *RequestReceipt) {
	req.mutex.Lock()
	if req.status != StatusPending && req.status != StatusReceiving {
		req.mutex.Unlock()
		return
	}
	req.status = StatusFailed
	cb := req.failedCb
	req.mutex.Unlock()
	l.removePendingRequest(req)
	if cb != nil {
		go cb(req)
	}
}
func (r *RequestReceipt) startTimeout() {
	timer := time.NewTimer(r.timeout)
	<-timer.C
	timer.Stop()
	var unboundSince time.Time
	for {
		r.mutex.RLock()
		status := r.status
		r.mutex.RUnlock()
		if status == StatusPending {
			// Deadline hit while still waiting for a response to start. If a
			// response resource was advertised but never progressed, cancel
			// it like Python response_resource_progress does for a FAILED
			// receipt.
			r.link.abortResponseResourceFor(r)
			r.link.failPendingRequest(r)
			return
		}
		if status != StatusReceiving {
			return
		}
		// A response resource is transferring. Python suspends the request
		// timeout in RECEIVING and lets the resource watchdog bound stalls;
		// abort paths fail this receipt. Bound only the orphaned case where
		// no live transfer claims the receipt (e.g. a peer that abandons a
		// split transfer between segments).
		r.link.incomingMu.Lock()
		bound := r.link.incomingRx != nil && r.link.incomingRx.request == r
		r.link.incomingMu.Unlock()
		if bound {
			unboundSince = time.Time{}
		} else if unboundSince.IsZero() {
			unboundSince = time.Now()
		} else if time.Since(unboundSince) > r.timeout {
			r.link.failPendingRequest(r)
			return
		}
		time.Sleep(incomingResourceRetryInterval)
	}
}
func (r *RequestReceipt) SetResponseCallback(cb func(*RequestReceipt)) {
	r.mutex.Lock()
	prev := r.responseCb
	r.responseCb = cb
	status := r.status
	r.mutex.Unlock()
	// Late-fire once when attaching after the response already landed.
	if status == StatusActive && cb != nil && prev == nil {
		go cb(r)
	}
}
func (r *RequestReceipt) SetFailedCallback(cb func(*RequestReceipt)) {
	r.mutex.Lock()
	prev := r.failedCb
	r.failedCb = cb
	status := r.status
	r.mutex.Unlock()
	if status == StatusFailed && cb != nil && prev == nil {
		go cb(r)
	}
}
func (r *RequestReceipt) SetProgressCallback(cb func(*RequestReceipt)) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.progressCb = cb
}
func (l *Link) handleRequest(plaintext []byte, requestID []byte) error {
	if l.destination == nil {
		return errors.New("no destination for request handling")
	}
	if maxSize, ok := l.destination.MaxRequestSize(); ok && len(plaintext) > maxSize {
		debug.Log(debug.DebugVerbose, "Ignored request with excessive size",
			"bytes", len(plaintext), "max", maxSize)
		return nil
	}

	var requestedAt time.Time
	var pathHash []byte
	var requestPayload []byte
	requestedAt, pathHash, requestPayload, err := unpackLinkRequest(plaintext)
	if err != nil {
		return err
	}
	if !requestTimestampValid(requestedAt, time.Now()) {
		debug.Log(debug.DebugVerbose, "Rejecting request with stale requested_at",
			"requested_at", requestedAt.Unix(),
			"request_id", fmt.Sprintf("%x", requestID))
		health.Inc(l.attachedIfaceName(), health.KindRequestSkewReject)
		return nil
	}

	debug.Log(debug.DebugVerbose, "Handling request", "path_hash", fmt.Sprintf("%x", pathHash), "request_id", fmt.Sprintf("%x", requestID))

	if l.destination != nil {
		handler := l.destination.GetRequestHandler(pathHash)
		if handler != nil {
			response := handler(pathHash, requestPayload, requestID, l.linkID, l.remoteIdentity, requestedAt)
			if response != nil {
				return l.sendResponse(requestID, response)
			}
		} else {
			debug.Log(debug.DebugVerbose, "No handler found for path", "path_hash", fmt.Sprintf("%x", pathHash))
		}
	}

	return nil
}
func (l *Link) handleResponse(plaintext []byte) error {
	var responseData []any
	if err := msgpack.Unmarshal(plaintext, &responseData); err != nil {
		return fmt.Errorf("failed to unpack response: %w", err)
	}

	if len(responseData) < MinResponseDataLen {
		return errors.New("invalid response format")
	}

	requestIDRaw, ok := responseData[0].([]byte)
	if !ok {
		return errors.New("invalid response format: request id is not bytes")
	}
	requestID := requestIDRaw
	responseValue := responseData[1]
	var responsePayload []byte
	switch p := responseValue.(type) {
	case []byte:
		responsePayload = p
	case string:
		responsePayload = []byte(p)
	case nil, bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		// Status codes (rncp fetch_file returns False / 0xF0 / True).
	default:
		if packed, err := msgpack.Marshal(responseValue); err == nil {
			responsePayload = packed
		}
	}

	l.requestMutex.RLock()
	var matched *RequestReceipt
	for _, req := range l.pendingRequests {
		if string(req.requestID) == string(requestID) {
			matched = req
			break
		}
	}
	l.requestMutex.RUnlock()

	if matched == nil {
		return nil
	}
	if matched.maxResponseSize > 0 && len(responsePayload) > matched.maxResponseSize {
		debug.Log(debug.DebugVerbose, "Rejected response with excessive size",
			"bytes", len(responsePayload), "max", matched.maxResponseSize)
		l.failPendingRequest(matched)
		return nil
	}

	matched.mutex.Lock()
	if matched.status != StatusPending && matched.status != StatusReceiving {
		matched.mutex.Unlock()
		return nil
	}
	matched.status = StatusActive
	matched.response = responsePayload
	matched.responseValue = responseValue
	matched.receivedAt = time.Now()
	matched.bytesReceived = int64(len(responsePayload))
	matched.totalBytes = int64(len(responsePayload))
	cb := matched.responseCb
	matched.mutex.Unlock()

	l.removePendingRequest(matched)
	if cb != nil {
		go cb(matched)
	}

	return nil
}

// FileResponse sends raw bytes as a response resource with optional metadata,
// matching Python RNS Link file tuple responses used by rngit fetch.
type FileResponse struct {
	Data           []byte
	MetadataPacked []byte
	AutoCompress   bool
}

func (l *Link) sendResponse(requestID []byte, response any) error {
	if fr, ok := response.(FileResponse); ok {
		res, err := resource.New(fr.Data, fr.AutoCompress)
		if err != nil {
			return fmt.Errorf("failed to create file response resource: %w", err)
		}
		if len(fr.MetadataPacked) > 0 {
			if err := res.SetMetadataPacked(fr.MetadataPacked); err != nil {
				return err
			}
		}
		res.SetRequestID(requestID)
		res.SetIsResponse(true)
		go func() {
			if err := l.SendResource(res); err != nil {
				debug.Log(debug.DebugError, "Failed to send file response resource", "request_id", fmt.Sprintf("%x", requestID), "error", err)
			}
		}()
		return nil
	}

	responseData := []any{requestID, response}
	packedResponse, err := msgpack.Marshal(responseData)
	if err != nil {
		return fmt.Errorf("failed to pack response: %w", err)
	}

	l.mutex.RLock()
	mdu := l.mdu
	l.mutex.RUnlock()

	if len(packedResponse) <= mdu {
		encrypted, err := l.encrypt(packedResponse)
		if err != nil {
			return err
		}

		respPkt := &packet.Packet{
			HeaderType:      packet.HeaderType1,
			PacketType:      packet.PacketTypeData,
			TransportType:   0,
			Context:         packet.ContextResponse,
			ContextFlag:     packet.FlagUnset,
			Hops:            0,
			DestinationType: DestTypeLink,
			DestinationHash: l.linkID,
			Data:            encrypted,
			CreateReceipt:   false,
		}

		if err := respPkt.Pack(); err != nil {
			return err
		}

		l.recordOutboundData()

		debug.Log(debug.DebugVerbose, "Sending response", "request_id", fmt.Sprintf("%x", requestID), "response_len", len(encrypted))
		return l.transport.SendPacket(respPkt)
	}

	res, err := resource.New(packedResponse, false)
	if err != nil {
		return fmt.Errorf("failed to create response resource: %w", err)
	}
	res.SetRequestID(requestID)
	res.SetIsResponse(true)

	debug.Log(debug.DebugVerbose, "Sending response as resource", "request_id", fmt.Sprintf("%x", requestID), "packed_len", len(packedResponse), "mdu", mdu)
	go func() {
		if err := l.SendResource(res); err != nil {
			debug.Log(debug.DebugError, "Failed to send response resource", "request_id", fmt.Sprintf("%x", requestID), "error", err)
		}
	}()
	return nil
}
