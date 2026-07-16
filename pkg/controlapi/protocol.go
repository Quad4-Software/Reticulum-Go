// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package controlapi

import (
	"encoding/json"
	"net/http"
)

// errorResponse is the JSON body returned for any non-2xx HTTP response.
type errorResponse struct {
	Error string `json:"error"`
}

// healthResponse is the body of GET /v1/health.
type healthResponse struct {
	Status          string  `json:"status"`
	TransportID     string  `json:"transport_id,omitempty"`
	TransportUptime float64 `json:"transport_uptime_seconds"`
}

// interfaceStatJSON mirrors the subset of transport.InterfaceStat exposed
// over the control API.
type interfaceStatJSON struct {
	Name              string  `json:"name"`
	Type              string  `json:"type"`
	Status            bool    `json:"status"`
	RXBytes           uint64  `json:"rx_bytes"`
	TXBytes           uint64  `json:"tx_bytes"`
	Bitrate           int64   `json:"bitrate"`
	IFACFail          uint64  `json:"ifac_fail"`
	HMACFail          uint64  `json:"hmac_fail"`
	AnnounceSigFail   uint64  `json:"announce_sig_fail"`
	UnpackFail        uint64  `json:"unpack_fail"`
	IntegrityFailRate float64 `json:"integrity_fail_rate"`
	StaleCloses       uint64  `json:"stale_closes"`
	LinkStaleClose    uint64  `json:"link_stale_close"`
	KeepaliveTimeout  uint64  `json:"keepalive_timeout"`
}

// statusResponse is the body of GET /v1/status.
type statusResponse struct {
	TransportID string              `json:"transport_id"`
	Interfaces  []interfaceStatJSON `json:"interfaces"`
}

// pathTableEntryJSON mirrors transport.PathTableEntry with hex-encoded hashes.
type pathTableEntryJSON struct {
	Hash      string  `json:"hash"`
	Via       string  `json:"via"`
	Hops      uint8   `json:"hops"`
	Expires   float64 `json:"expires"`
	Interface string  `json:"interface"`
}

// createSessionRequest is the body of POST /v1/sessions. IdentityPath, when
// set, is a server-local filesystem path used to load or create a
// persistent identity. When empty a new in-memory identity is generated and

// discarded on session close.
type createSessionRequest struct {
	IdentityPath string `json:"identity_path,omitempty"`
}

// createSessionResponse is the body returned by POST /v1/sessions.
type createSessionResponse struct {
	SessionID    string `json:"session_id"`
	IdentityHash string `json:"identity_hash"`
}

// registerDestinationRequest is the body of
// POST /v1/sessions/{id}/destinations. When AcceptsLinks is true, remote
// peers may open links to this destination. Establishment and teardown are

// reported as link.established/link.failed/link.closed events on every
// WebSocket connection attached to the session.
type registerDestinationRequest struct {
	AppName      string   `json:"app_name"`
	Aspects      []string `json:"aspects,omitempty"`
	AcceptsLinks bool     `json:"accepts_links,omitempty"`
}

// registerDestinationResponse is returned by destination registration.
type registerDestinationResponse struct {
	DestinationHash string `json:"destination_hash"`
}

// registerRequestHandlerRequest is the body of
// POST /v1/sessions/{id}/destinations/{hash}/requests. It registers a path
// that, when a peer sends a link request to it, is bridged to the session's
// WebSocket connections as a request.incoming event. The application

// answers with a request.respond command. Allow is one of "all" (default),
// "none", or "list". AllowedIdentities is required hex identity hashes when

// Allow is "list".
type registerRequestHandlerRequest struct {
	Path              string   `json:"path"`
	Allow             string   `json:"allow,omitempty"`
	AllowedIdentities []string `json:"allowed_identities,omitempty"`
}

// announceRequest is the body of
// POST /v1/sessions/{id}/destinations/{hash}/announce. AppData, when set, is
// base64-encoded and becomes the destination's default app data before
// announcing.
type announceRequest struct {
	AppData string `json:"app_data,omitempty"`
}

// pathRequestRequest is the body of POST /v1/sessions/{id}/path/request.
type pathRequestRequest struct {
	DestinationHash string `json:"destination_hash"`
}

// wsCommandEnvelope is decoded first to dispatch an inbound WebSocket
// message on its type field before decoding the full command.
type wsCommandEnvelope struct {
	Type string `json:"type"`
}

// subscribeAnnouncesCommand subscribes the connection to announceEvent
// pushes. Filter is accepted for forward compatibility but is not yet used
// to narrow delivery. Every announce the node receives is currently

// forwarded to every subscriber.
type subscribeAnnouncesCommand struct {
	Type   string `json:"type"`
	Filter string `json:"filter,omitempty"`
}

// announceEvent is pushed to subscribed WebSocket clients for every
// announce the node's transport receives.
type announceEvent struct {
	Type            string `json:"type"`
	DestinationHash string `json:"destination_hash"`
	IdentityHash    string `json:"identity_hash,omitempty"`
	AppData         string `json:"app_data,omitempty"`
	Hops            uint8  `json:"hops"`
}

// linkOpenCommand requests an outbound link to a destination the node has
// already learned about via an announce (see identity.Recall). The result
// arrives as a linkEstablishedEvent or linkFailedEvent.
type linkOpenCommand struct {
	Type            string `json:"type"`
	DestinationHash string `json:"destination_hash"`
}

// linkSendCommand sends data over an already-established link.
type linkSendCommand struct {
	Type   string `json:"type"`
	LinkID string `json:"link_id"`
	Data   string `json:"data"`
}

// linkCloseCommand tears down an established link.
type linkCloseCommand struct {
	Type   string `json:"type"`
	LinkID string `json:"link_id"`
}

// requestRespondCommand answers a pending requestIncomingEvent.
type requestRespondCommand struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id"`
	Data      string `json:"data,omitempty"`
}

// linkEstablishedEvent reports a link (outbound or inbound) becoming
// active and ready for link.send.
type linkEstablishedEvent struct {
	Type       string `json:"type"`
	LinkID     string `json:"link_id"`
	RemoteHash string `json:"remote_hash,omitempty"`
}

// linkFailedEvent reports an outbound link.open that never became active.
type linkFailedEvent struct {
	Type            string `json:"type"`
	LinkID          string `json:"link_id,omitempty"`
	DestinationHash string `json:"destination_hash,omitempty"`
	Error           string `json:"error,omitempty"`
}

// linkDataEvent carries data received over an established link.
type linkDataEvent struct {
	Type   string `json:"type"`
	LinkID string `json:"link_id"`
	Data   string `json:"data"`
}

// linkClosedEvent reports a previously-active link tearing down.
type linkClosedEvent struct {
	Type   string `json:"type"`
	LinkID string `json:"link_id"`
}

// requestIncomingEvent reports a peer request arriving on a path
// registered via POST .../destinations/{hash}/requests. The application
// must reply with a requestRespondCommand carrying the same RequestID.
type requestIncomingEvent struct {
	Type               string `json:"type"`
	DestinationHash    string `json:"destination_hash"`
	LinkID             string `json:"link_id"`
	RequestID          string `json:"request_id"`
	Path               string `json:"path"`
	Data               string `json:"data,omitempty"`
	RemoteIdentityHash string `json:"remote_identity_hash,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}
