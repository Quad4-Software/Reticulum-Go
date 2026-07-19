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
	KindResourceReq
	KindResourceHMU
	KindResourceComplete
	KindNetmonFlap
	KindRxOK
	KindAnnounceOK
	KindAnnounceDup
	KindPathRespSuppressed
	KindPathReqDup
	KindPathReqNoCache
	KindPathRespQueuedSkip
	KindLinkRelayUnknownIface
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
	case KindResourceReq:
		return "resource_req"
	case KindResourceHMU:
		return "resource_hmu"
	case KindResourceComplete:
		return "resource_complete"
	case KindNetmonFlap:
		return "netmon_flap"
	case KindRxOK:
		return "rx_ok"
	case KindAnnounceOK:
		return "announce_ok"
	case KindAnnounceDup:
		return "announce_dup"
	case KindPathRespSuppressed:
		return "path_resp_suppressed"
	case KindPathReqDup:
		return "path_req_dup"
	case KindPathReqNoCache:
		return "path_req_no_cache"
	case KindPathRespQueuedSkip:
		return "path_resp_queued_skip"
	case KindLinkRelayUnknownIface:
		return "link_relay_unknown_iface"
	default:
		return "unknown"
	}
}
