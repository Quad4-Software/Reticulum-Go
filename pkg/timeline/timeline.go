// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// Package timeline documents the cross-stack INTEROP_EVENT convention.
// Implementations emit the same event names so Go, Python, MeshChat, and
// NomadNet peers can share one timeline format.
package timeline

// Event names shared across stacks (stderr prefix INTEROP_EVENT).
const (
	EventReady     = "ready"
	EventPathWait  = "path_wait"
	EventPathOK    = "path_ok"
	EventPathReq   = "path_req"
	EventPathResp  = "path_resp"
	EventNode      = "node"
	EventLinkUp    = "link_up"
	EventLinkOK    = "link_ok"
	EventRequestOK = "request_ok"
	EventFail      = "fail"
	EventSpawn     = "spawn"
)

// Kind values for fail events (and optional classification elsewhere).
const (
	KindSpawn    = "spawn"
	KindReady    = "ready"
	KindAnnounce = "announce"
	KindPath     = "path"
	KindIdentity = "identity"
	KindLink     = "link"
	KindRequest  = "request"
	KindTimeout  = "timeout"
	KindHarness  = "harness"
)

// StderrPrefix is written before one JSON object on stderr.
const StderrPrefix = "INTEROP_EVENT "

// SpecVersion is the timeline schema version for docs and emitters.
const SpecVersion = 1
