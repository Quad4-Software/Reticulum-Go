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

	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
)

// requestResponseTimeout bounds how long a request.incoming bridge blocks
// waiting for the application's request.respond command.
const requestResponseTimeout = 30 * time.Second

func (s *Server) handleRegisterDestination(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.session(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	var req registerDestinationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.AppName == "" {
		writeError(w, http.StatusBadRequest, "app_name is required")
		return
	}

	dest, err := destination.New(sess.identity, destination.In, destination.Single, req.AppName, s.transport, req.Aspects...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("register destination: %v", err))
		return
	}

	hashHex := hex.EncodeToString(dest.GetHash())
	sess.addDestination(hashHex, dest)

	if req.AcceptsLinks {
		wireInboundLinks(sess, dest)
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
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
	return dest.RegisterRequestHandler(path, func(p string, data []byte, requestID []byte, linkID []byte, remoteIdentity *identity.Identity, requestedAt int64) []byte {
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
			sess.forgetResponse(requestIDHex)
			debug.Log(debug.DebugError, "controlapi: request.respond timed out", "path", p, "request_id", requestIDHex)
			return nil
		}
	}, allow, allowedList)
}

// handleRequestRespond processes a request.respond command, delivering its
// data to the goroutine blocked in wireRequestHandler for the same
// request_id.
func (c *wsClient) handleRequestRespond(raw []byte) {
	var cmd requestRespondCommand
	if err := json.Unmarshal(raw, &cmd); err != nil {
		debug.Log(debug.DebugError, "controlapi: invalid request.respond command", "error", err)
		return
	}
	var data []byte
	if cmd.Data != "" {
		decoded, err := base64.StdEncoding.DecodeString(cmd.Data)
		if err != nil {
			debug.Log(debug.DebugError, "controlapi: request.respond data is not base64", "request_id", cmd.RequestID)
			return
		}
		data = decoded
	}
	if !c.session.deliverResponse(cmd.RequestID, data) {
		debug.Log(debug.DebugError, "controlapi: request.respond for unknown or expired request_id", "request_id", cmd.RequestID)
	}
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.AppData != "" {
		appData, err := base64.StdEncoding.DecodeString(req.AppData)
		if err != nil {
			writeError(w, http.StatusBadRequest, "app_data must be base64-encoded")
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
