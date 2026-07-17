// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// Package controlapi implements a localhost-only JSON control plane for a
// running Reticulum-Go node. It lets applications written in any language
// use destinations, announces, links, request/response, identify, and
// minimal link resources without embedding the Go transport stack.
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
//	DELETE /v1/sessions/{id}/destinations/{hash}/requests        deregister a request path (?path=)
//	POST   /v1/sessions/{id}/path/request                        request a path
//	GET    /v1/sessions/{id}/events                              WebSocket event stream
//
// A session owns one identity, the destinations registered under it, and
// the links it has opened or accepted. Binary values (hashes, app data,
// link payloads) are hex- or base64-encoded as noted on each
// request/response type in protocol.go.
//
// # Events and commands
//
// The events WebSocket pushes announce, link.established, link.failed,
// link.data, link.closed, link.remote_identified, request.incoming,
// request.response, request.failed, resource.started, resource.concluded,
// and command.error. It accepts subscribe_announces, link.open, link.send,
// link.close, link.request, link.send_resource, link.identify, and
// request.respond (optional filename for NomadNet file replies).
//
// subscribe_announces.filter must be empty (all announces) or an exact
// 16-byte destination hash as hex.
//
// # Scope
//
// The server binds to 127.0.0.1 by default and is disabled unless
// enable_control_api is set in the node configuration. It is an application
// contract over the transport, not a mirror of channels, buffers, or
// mesh-admin RPC. Large resources over base64 WebSocket frames are
// impractical. Prefer rncp or in-process librns for bulk transfers.
//
// Do not treat this API as a required remote control plane for a product.
// It fronts one local node. Application traffic should use destinations and
// links between peers. See docs/en/control-api.md (Architecture notes) and
// reticulum.network/manual/zen.html for upstream design intent.
package controlapi
