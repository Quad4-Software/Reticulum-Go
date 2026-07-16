// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package health

// Kind identifies a local mesh health counter.
type Kind uint8

const (
	KindIFACFail Kind = iota
	KindHMACFail
	KindUnpackFail
	KindPaddingFail
	KindAnnounceSigFail
	KindProofFail
	KindLRProofHopMismatch
	KindRequestSkewReject
	KindBlackholeHit
	KindLinkStaleClose
	KindKeepaliveTimeout
	KindResourceStall
	KindNetmonFlap
	KindRxOK
	KindAnnounceOK
	kindCount
)

// String returns the wire and RPC name for k.
func (k Kind) String() string {
	switch k {
	case KindIFACFail:
		return "ifac_fail"
	case KindHMACFail:
		return "hmac_fail"
	case KindUnpackFail:
		return "unpack_fail"
	case KindPaddingFail:
		return "padding_fail"
	case KindAnnounceSigFail:
		return "announce_sig_fail"
	case KindProofFail:
		return "proof_fail"
	case KindLRProofHopMismatch:
		return "lrproof_hop_mismatch"
	case KindRequestSkewReject:
		return "request_skew_reject"
	case KindBlackholeHit:
		return "blackhole_hit"
	case KindLinkStaleClose:
		return "link_stale_close"
	case KindKeepaliveTimeout:
		return "keepalive_timeout"
	case KindResourceStall:
		return "resource_stall"
	case KindNetmonFlap:
		return "netmon_flap"
	case KindRxOK:
		return "rx_ok"
	case KindAnnounceOK:
		return "announce_ok"
	default:
		return "unknown"
	}
}
