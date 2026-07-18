// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package controlapi

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
)

// requestResponseTimeout bounds how long a request.incoming bridge blocks
// waiting for the application's request.respond command. Tests may shorten it.
var requestResponseTimeout = 30 * time.Second

func (s *Server) handleRegisterDestination(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.session(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	var req registerDestinationRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		if isBodyTooLarge(err) {
			return
		}
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.AppName == "" {
		writeError(w, http.StatusBadRequest, "app_name is required")
		return
	}

	dest, err := destination.New(sess.identity, destination.In, destination.Single, req.AppName, s.transport, req.Aspects...)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("register destination: %v", err))
		return
	}

	hashHex := hex.EncodeToString(dest.GetHash())
	sess.addDestination(hashHex, dest)

	if req.AcceptsLinks {
		wireInboundLinks(sess, dest)
	} else {
		debug.Log(debug.DebugInfo, common.MsgControlAPINoAcceptsLinks, "hash", hashHex)
	}

	writeJSON(w, http.StatusCreated, registerDestinationResponse{DestinationHash: hashHex})
}

// handleRegisterRequestHandler processes
// POST /v1/sessions/{id}/destinations/{hash}/requests, bridging the
// destination's request handler for the given path to request.incoming and
// request.respond WebSocket messages.
func (s *Server) handleRegisterRequestHandler(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.session(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	destHashHex := r.PathValue("hash")
	dest, ok := sess.destination(destHashHex)
	if !ok {
		writeError(w, http.StatusNotFound, "destination not found")
		return
	}

	var req registerRequestHandlerRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		if isBodyTooLarge(err) {
			return
		}
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}

	allow, allowedList, err := parseAllowMode(req.Allow, req.AllowedIdentities)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := wireRequestHandler(sess, dest, destHashHex, req.Path, allow, allowedList); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("register request handler: %v", err))
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func parseAllowMode(allow string, identities []string) (byte, [][]byte, error) {
	switch allow {
	case "", "all":
		return destination.AllowAll, nil, nil
	case "none":
		return destination.AllowNone, nil, nil
	case "list":
		if len(identities) == 0 {
			return 0, nil, errors.New(`allowed_identities is required when allow is "list"`)
		}
		list := make([][]byte, 0, len(identities))
		for _, h := range identities {
			b, err := hex.DecodeString(h)
			if err != nil {
				return 0, nil, fmt.Errorf("invalid allowed identity hash %q", h)
			}
			list = append(list, b)
		}
		return destination.AllowList, list, nil
	default:
		return 0, nil, fmt.Errorf("invalid allow mode %q", allow)
	}
}

// wireRequestHandler registers path on dest, forwarding every matching
// incoming request to the session's WebSocket clients as a
// requestIncomingEvent and blocking the calling link goroutine until a
// matching request.respond command arrives or requestResponseTimeout
// elapses.
func wireRequestHandler(sess *session, dest *destination.Destination, destHashHex, path string, allow byte, allowedList [][]byte) error {
	return dest.RegisterRequestHandlerAny(path, func(p string, data []byte, requestID []byte, linkID []byte, remoteIdentity *identity.Identity, requestedAt int64) any {
		requestIDHex := hex.EncodeToString(requestID)
		evt := requestIncomingEvent{
			Type:            "request.incoming",
			DestinationHash: destHashHex,
			LinkID:          hex.EncodeToString(linkID),
			RequestID:       requestIDHex,
			Path:            p,
			Data:            base64.StdEncoding.EncodeToString(data),
		}
		if remoteIdentity != nil {
			evt.RemoteIdentityHash = remoteIdentity.GetHexHash()
		}

		ch := sess.awaitResponse(requestIDHex)
		sess.broadcast(evt)

		select {
		case resp := <-ch:
			return resp
		case <-time.After(requestResponseTimeout):
			// Ownership protocol: whoever removes the map entry owns the
			// outcome. If deliverResponse already took it, wait for the
			// buffered send instead of returning nil while the WS client
			// was told the respond succeeded.
			if !sess.forgetResponse(requestIDHex) {
				return <-ch
			}
			debug.Log(debug.DebugError, "controlapi: request.respond timed out", "path", p, "request_id", requestIDHex)
			return nil
		}
	}, allow, allowedList)
}

// handleRequestRespond processes a request.respond command, delivering its
// data to the goroutine blocked in wireRequestHandler for the same
// request_id. When Filename is set the payload is [filename, bytes].
func (c *wsClient) handleRequestRespond(raw []byte) {
	var cmd requestRespondCommand
	if err := json.Unmarshal(raw, &cmd); err != nil {
		c.send(commandErrorEvent{Type: "command.error", Command: "request.respond", Error: "invalid command json"})
		return
	}
	var data []byte
	if cmd.Data != "" {
		decoded, err := base64.StdEncoding.DecodeString(cmd.Data)
		if err != nil {
			c.send(commandErrorEvent{Type: "command.error", Command: "request.respond", Error: "data must be base64"})
			return
		}
		data = decoded
	}
	var payload any = data
	if cmd.Filename != "" {
		payload = []any{cmd.Filename, data}
	}
	if !c.session.deliverResponse(cmd.RequestID, payload) {
		c.send(commandErrorEvent{Type: "command.error", Command: "request.respond", Error: "unknown or expired request_id"})
	}
}

func (s *Server) handleDeregisterRequestHandler(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.session(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	destHashHex := r.PathValue("hash")
	dest, ok := sess.destination(destHashHex)
	if !ok {
		writeError(w, http.StatusNotFound, "destination not found")
		return
	}
	path := r.URL.Query().Get("path")
	if path == "" {
		writeError(w, http.StatusBadRequest, "path query parameter is required")
		return
	}
	if !dest.DeregisterRequestHandler(path) {
		writeError(w, http.StatusNotFound, "request handler not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAnnounce(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.session(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	dest, ok := sess.destination(r.PathValue("hash"))
	if !ok {
		writeError(w, http.StatusNotFound, "destination not found")
		return
	}

	var req announceRequest
	if err := decodeJSONBody(w, r, &req); err != nil && !errors.Is(err, io.EOF) {
		if isBodyTooLarge(err) {
			return
		}
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.AppData != "" {
		appData, err := base64.StdEncoding.DecodeString(req.AppData)
		if err != nil {
			writeError(w, http.StatusBadRequest, "app_data must be base64-encoded")
			return
		}
		// Announce app_data must fit packet.MTU after fixed fields.
		// HEADER1 without ratchet leaves 333 bytes, with ratchet 301.
		// Allow the no-ratchet budget and let CreatePacket reject if a
		// ratchet is attached and the payload no longer fits.
		const maxAnnounceAppData = 333
		if len(appData) > maxAnnounceAppData {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("app_data exceeds announce MTU budget (%d > %d)", len(appData), maxAnnounceAppData))
			return
		}
		dest.SetDefaultAppData(appData)
	}

	if err := dest.Announce(false, nil, nil); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("announce: %v", err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// announceBridge forwards every announce the transport receives to
// WebSocket clients subscribed via subscribe_announces. It registers with
// AspectFilter "*" so no announces are filtered before reaching sessions.
type announceBridge struct {
	server *Server
}

func (a *announceBridge) AspectFilter() []string { return []string{"*"} }

func (a *announceBridge) ReceivePathResponses() bool { return true }

func (a *announceBridge) ReceivedAnnounce(destHash []byte, announcedIdentity any, appData []byte, hops uint8) error {
	evt := announceEvent{
		Type:            "announce",
		DestinationHash: hex.EncodeToString(destHash),
		AppData:         base64.StdEncoding.EncodeToString(appData),
		Hops:            hops,
	}
	if id, ok := announcedIdentity.(*identity.Identity); ok {
		evt.IdentityHash = id.GetHexHash()
	}
	a.server.broadcastAnnounce(evt)
	return nil
}
