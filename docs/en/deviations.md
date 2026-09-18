# Intentional deviations from Python RNS

This page lists every place Reticulum-Go deliberately differs from the
Python reference implementation, and why. Wire-visible behavior is intended
to match upstream. When it does not, that is a bug: file an issue.

Three categories exist:

- **Local hardening bounds**: caps on memory or state that upstream leaves
  unbounded. They change nothing on the wire for conforming peers.
- **Local-only extensions**: features that never appear on a shared medium
  or in packets a Python peer would parse differently.
- **Wire-visible extensions**: additions a Python peer can observe. These
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

## Local-only extensions

| Feature | Notes |
|---------|-------|
| Node lifecycle API (OnNetworkAvailable, ReloadInterfaces, control API) | Embedder surface. No wire change |
| QUIC, WebSocket, WebTransport, VSOCK, HTTPS long-poll, DNSRendezvous interfaces | New underlay endpoints. Packet bytes on the medium are unchanged HDLC/framing |
| Landlock + seccomp sandbox | Local hardening |
| `pkg/health` counters, `reticulum-go slow` | Observe-only diagnostics |
| RHB1 hardware-bound identity descriptor | Local storage format. Python reads only the 64-byte software layout |

## Wire-visible extensions

| Feature | Wire-visible? | Upstream behavior |
|---------|---------------|-------------------|
| TRANSPORT_IMPL announces as `reticulum-go` + build version | Yes | Honest identification. Upstream announces `rns` |
| OP_PAGE 0xF1 provisional operator page field | Yes | Ignored by current Python RNS |
| rngit `/media` blob route | Yes | Not present in Python rngit. A Reticulum-Go extension |
| SDR burst modem | Yes | Lab/testing only, not air-compatible with Modem73/RNode |

## What is NOT here

No changes to packet layout, crypto primitives, signature inputs, link
handshake, resource hashmap semantics, announce format, LXMF payloads, or
stamp workblock construction. If a difference in those areas is found, it
is a bug.
