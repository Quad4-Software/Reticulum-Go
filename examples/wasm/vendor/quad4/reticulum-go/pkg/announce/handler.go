// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package announce

type Handler interface {
	AspectFilter() []string
	ReceivedAnnounce(destHash []byte, identity any, appData []byte, hops uint8) error
	ReceivePathResponses() bool
}
