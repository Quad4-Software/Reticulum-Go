// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// Package controlapi implements a localhost-only JSON control plane for a
// running Reticulum-Go node. It lets applications written in any language
// use destinations, announces, links, and request/response without
// embedding the Go transport stack themselves.
//
// # Wire protocol
//
// All routes are versioned under /v1 and require a bearer token equal to the
// node's hex-encoded rpc_key:
//
//	Authorization: Bearer <hex rpc_key>
//
// HTTP routes:
//
//	GET    /v1/health                                          node status
//	GET    /v1/status                                           interface stats
//	GET    /v1/paths                                             path table
//	POST   /v1/sessions                                          create a session (identity)
//	DELETE /v1/sessions/{id}                                     tear down a session
//	POST   /v1/sessions/{id}/destinations                        register a destination
//	POST   /v1/sessions/{id}/destinations/{hash}/announce        send an announce
//	POST   /v1/sessions/{id}/destinations/{hash}/requests        bridge a request path to the WebSocket
//	POST   /v1/sessions/{id}/path/request                        request a path
//	GET    /v1/sessions/{id}/events                              WebSocket event stream
//
// A session owns one identity, the destinations registered under it, and
// the links it has opened or accepted. Binary values (hashes, app data,
// link payloads) are hex- or base64-encoded as noted on each
// request/response type in protocol.go.
//
// # Events
//
// The /v1/sessions/{id}/events WebSocket pushes JSON events (announceEvent,
// linkEstablishedEvent, linkFailedEvent, linkDataEvent, linkClosedEvent,
// requestIncomingEvent) and accepts JSON commands (subscribeAnnouncesCommand,
// linkOpenCommand, linkSendCommand, linkCloseCommand,
// requestRespondCommand). See protocol.go for the full set.
//
// Links: registering a destination with accepts_links opens it to inbound
// links; link.open initiates an outbound link to a destination the node
// has already learned about via an announce. Both report
// linkEstablishedEvent once active, linkDataEvent for each link.send from
// the peer, and linkClosedEvent on teardown; an outbound link.open that
// never becomes active reports linkFailedEvent instead.
//
// Requests: registering a path via POST .../destinations/{hash}/requests
// bridges it to requestIncomingEvent/requestRespondCommand. The handler
// blocks the underlying link goroutine until the application answers with
// request.respond or a fixed timeout elapses, so it should be answered
// promptly.
//
// # Scope
//
// The server binds to 127.0.0.1 by default and is disabled unless
// enable_control_api is set in the node configuration. It is an application
// contract over the transport, not a mirror of the internal transport API.
package controlapi
