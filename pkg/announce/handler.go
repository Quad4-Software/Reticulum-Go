// SPDX-License-Identifier: 0BSD
// Copyright (c) 2024-2026 Sudo-Ivan / Quad4.io
package announce

type Handler interface {
	AspectFilter() []string
	ReceivedAnnounce(destHash []byte, identity interface{}, appData []byte, hops uint8) error
	ReceivePathResponses() bool
}
