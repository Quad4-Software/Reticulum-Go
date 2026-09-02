// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package protect

import "quad4/reticulum-go/pkg/health"

// Reason identifies why a protect trip fired.
type Reason int

const (
	ReasonNone Reason = iota
	ReasonPPS
	ReasonBPS
	ReasonHandler
	ReasonConn
	ReasonResource
	ReasonMemory
	ReasonCrypto
	ReasonHandshake
	ReasonCoolDown
	reasonCount
)

// String returns the stable reason token used in warnings and logs.
func (r Reason) String() string {
	switch r {
	case ReasonPPS:
		return "pps"
	case ReasonBPS:
		return "bps"
	case ReasonHandler:
		return "handler"
	case ReasonConn:
		return "conn"
	case ReasonResource:
		return "resource"
	case ReasonMemory:
		return "memory"
	case ReasonCrypto:
		return "crypto"
	case ReasonHandshake:
		return "handshake"
	case ReasonCoolDown:
		return "cooldown"
	default:
		return "none"
	}
}

// HealthKind maps a trip reason to a health counter kind.
func (r Reason) HealthKind() health.Kind {
	switch r {
	case ReasonPPS:
		return health.KindDoSPPS
	case ReasonBPS:
		return health.KindDoSBPS
	case ReasonHandler:
		return health.KindDoSHandler
	case ReasonConn:
		return health.KindDoSConn
	case ReasonResource:
		return health.KindDoSResource
	case ReasonMemory:
		return health.KindDoSMemory
	case ReasonCrypto:
		return health.KindDoSCrypto
	case ReasonHandshake:
		return health.KindDoSHandshake
	case ReasonCoolDown:
		return health.KindDoSCoolDown
	default:
		return health.KindDoSPPS
	}
}
