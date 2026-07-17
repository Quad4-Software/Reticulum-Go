// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package harness

// Kind classifies interop failures for timeline and artifact summaries.
type Kind string

const (
	KindSpawn    Kind = "spawn"
	KindReady    Kind = "ready"
	KindAnnounce Kind = "announce"
	KindPath     Kind = "path"
	KindIdentity Kind = "identity"
	KindLink     Kind = "link"
	KindRequest  Kind = "request"
	KindTimeout  Kind = "timeout"
	KindHarness  Kind = "harness"
)

func (k Kind) String() string {
	if k == "" {
		return string(KindHarness)
	}
	return string(k)
}
