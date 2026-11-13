# Intentional deviations from Python RNS

This page lists every place Reticulum-Go deliberately differs from the
Python reference implementation, and why. Wire-visible behavior is intended
to match upstream. When it does not, that is a bug: file an issue.

Three categories exist:

- Local hardening bounds: caps on memory or state that upstream leaves
  unbounded. They change nothing on the wire for conforming peers.
- Local-only extensions: features that never appear on a shared medium
  or in packets a Python peer would parse differently.
- Wire-visible extensions: additions a Python peer can observe. These
  are additive and ignored by upstream where noted.

## Local hardening bounds

| Bound | Value | Rationale |
|-------|-------|-----------|
| Split-resource assemblies per link | 4 | Assemblies retain attacker data. Upstream has no cap |
| Split-resource assembly size | 1 GiB hard ceiling | Declared `d` is attacker-controlled until verified |
| Split-resource segments | Sequential index only, replay rejected | Upstream assumes in-order arrival. Enforcing it kills memory amplification |
| RNode packet queue | 256 packets per direction | Unbounded queue grew under sustained RX flood |
| RawChannelReader unread buffer | 8 MiB | Unreadable buffer grew without limit |
| Channel receive window | Drop sequences more than WINDOW_MAX (48) ahead | Same check upstream applies. It also bounds ring residency |
| msgpack map keys | Non-comparable keys rejected | Go panics on unhashable keys where Python tolerates them |
| Per-packet panic recovery | Worker recovers and continues | A malformed packet must not kill the process |
| `Destination.Announce` burst cap | 8 announces per 10s per destination | Python has no equivalent. Excess announces return `ErrDestAnnounceThrottled`. Reduces own-originated wire traffic only |
| `RequestPath` per-destination throttle | 20s minimum interval (`PATH_REQUEST_MI`) | Python `request_path()` has no throttle; it only gates automated callers. Go applies it to the public API too, so a second call within 20s returns `ErrPathRequestThrottled`. `AwaitPath` retries on the same interval until its window expires where Python sends exactly one request |
| Announce egress rate keying | Per (interface, destination) | Python keys `announce_rate_table` globally per destination using the ingress interface's target. With the shared 3600s/5/0 defaults a multi-interface Go node can rebroadcast a given destination roughly once per interface per window instead of once total. Both bound floods; Go's bound is N times looser on N interfaces |
| Unsigned DATA `PATH_RESPONSE` | Dropped before delivery | Path responses are verified announces only. Python would hand a plain-destination DATA packet with this context to the app callback. Go ignores it so forged path responses cannot poison the path table |

## Local-only extensions

| Feature | Notes |
|---------|-------|
| Node lifecycle API (OnNetworkAvailable, ReloadInterfaces, control API) | Embedder surface. No wire change |
| QUIC, WebSocket, WebTransport, VSOCK, HTTPS long-poll, DNSRendezvous interfaces | New underlay endpoints. Packet bytes on the medium are unchanged HDLC/framing |
| Landlock + seccomp sandbox | Local hardening |
| `pkg/health` counters, `reticulum-go slow` | Observe-only diagnostics |
| RHB1 hardware-bound identity descriptor | Local storage format. Python reads only the 64-byte software layout |
| RNE1 passphrase-encrypted identity file | Local storage format. Argon2id + XChaCha20-Poly1305 envelope around the standard 64-byte blob. Opt-in, decrypts back to plaintext. No wire change |

## Wire-visible extensions

| Feature | Wire-visible? | Upstream behavior |
|---------|---------------|-------------------|
| TRANSPORT_IMPL announces as `reticulum-go` + build version | Yes | Honest identification. Upstream announces `rns` |
| OP_PAGE 0xF1 provisional operator page field | Yes | Ignored by current Python RNS |
| rngit `/media` blob route | Yes | Not present in Python rngit. A Reticulum-Go extension |
| SDR burst modem | Yes | Lab/testing only, not air-compatible with Modem73/RNode |
| Bare HT1 LINKREQUEST bridging | Yes | Python drops link requests for non-local destinations. When this node holds a known path it re-originates the request on the path interface (HeaderType2-wrapped for multi-hop paths), so link establishment can succeed through a Go mesh where a pure-Python mesh fails. Bounded by packet-hash dedup and MaxHops |

## What is NOT here

No changes to packet layout, crypto primitives, signature inputs, link
handshake, resource hashmap semantics, announce format, LXMF payloads, or
stamp workblock construction. If a difference in those areas is found, it
is a bug.
