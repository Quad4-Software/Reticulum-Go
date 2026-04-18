# Compatibility with the Python Reference

This document covers how well Reticulum-Go plays along with the official [Reticulum Network Stack API reference](https://reticulum.network/manual/reference.html) and the original Python `rns` package ([Reticulum](https://github.com/markqvist/Reticulum)). 

## Table

| Component                                        | Reticulum-Go | Notes |
|--------------------------------------------------|:------------:|-------|
| Crypto                                           | Yes          | Curve25519 (X25519 + Ed25519), AES-256-CBC (`MODE_AES256_CBC`, the only currently enabled `Link.ENABLED_MODES` entry in Python `rns`), HMAC-SHA256, HKDF; verified by `tests/crossref` |
| Identity                                         | Yes          | Key generation, recall, sign/verify, encrypt/decrypt, ratchets |
| Destination                                      | Yes          | `SINGLE`, `GROUP`, `PLAIN`, `LINK` types; announce, request handlers, link in/out |
| Packet                                           | Yes          | `HEADER_TYPE_1` and `HEADER_TYPE_2`, all packet types and contexts; byte-for-byte parity in crossref |
| Transport [^transport-scope]                     | Yes          | Path table, announces, `RequestPath`, hops, next-hop, multi-hop `HEADER_TYPE_2` rewrap, link-table forwarding for `DEST_TYPE_LINK` |
| Interfaces                                       | Partial      | UDP, TCP client/server (HDLC framed), AutoInterface (multicast group discovery), WebSocket. Missing: I2P, Backbone, RNode/RNodeMulti, Serial, KISS, AX25KISS, Pipe, Local, Weave |
| Discovery (`RNS.Discovery`, `rnstransport`)      | Partial      | `pkg/discovery` ports the wire constants, the LXStamper proof-of-work (`StampWorkblock`/`StampValue`/`StampValid`/`GenerateStamp`) and the msgpack info-dict / `app_data` flag-byte format. Cross-validated against the Python `RNS.Discovery` and `LXMF.LXStamper` references in `pkg/discovery` interop tests. The high-level `InterfaceAnnouncer` job loop and `BlackholeUpdater` from `RNS/Discovery.py` are not started automatically; callers compose announces with `BuildAppData` and decode with `ValidateAndDecode`. (Unrelated to `AutoInterface` multicast discovery, which is supported.) |
| Blackhole                                        | Partial      | `pkg/blackhole` implements `Transport.blackholed_identities`, on-disk msgpack persistence (matching `RNS.Transport.persist_blackhole`/`reload_blackhole`), expiry sweeping, per-source merge with `MergeRemote`, and `EncodeForRequest` (the payload that `Transport.blackhole_list_handler` returns). `Transport.HandleAnnounce` consults the table and drops announces from blackholed identities. The `/list` request handler over `rnstransport.info.blackhole` requires the RNS Request layer (not yet ported). Round-trip with Python `RNS.vendor.umsgpack` is verified by `pkg/blackhole` Python interop tests |
| IFAC (Interface Access Code)                     | Yes          | `pkg/ifac` ports `RNS.Reticulum.IFAC_SALT`, the HKDF-derived `ifac_identity`, and the byte-level mask/unmask used by `RNS.Transport.transmit`/`inbound`. UDP, TCP client/server and AutoInterface mask outbound packets and unmask inbound packets via `pkg/common.ApplyIFACOutbound`/`ApplyIFACInbound`, including the drop policy for missing or unauthenticated packets. Wire vectors and live UDP loopback against Python `rns` (`tests/interop/ifac_live_test.go`) confirm bidirectional interop and that unauthenticated peers are rejected |
| Link                                             | Yes          | Establish (both directions), RTT, requests/responses, channel and buffer carriage, resource transfer over link |
| Resource                                         | Yes          | Multi-part transfer, hashmaps, `RESOURCE_PRF` proofs, bzip2 compression, accept/reject (`ACCEPT_NONE`) |
| Channel                                          | Yes          | In-link reliable channel; unit tests in `pkg/channel` |
| Buffer                                           | Yes          | Stream buffer over channel; unit tests in `pkg/buffer` |

## Protocol Constants

We use the following constants on the wire to make sure we perfectly match what `RNS.Identity` and `RNS.Reticulum` expect:

| Item | Value |
|------|--------|
| Curve | Curve25519 (X25519 + Ed25519) |
| `KEYSIZE` | 512 bits (256-bit encryption key + 256-bit signing key) |
| `TRUNCATED_HASHLENGTH` | 128 bits (identity and destination addressing) |
| `RATCHETSIZE` | 256 bits |
| `RATCHET_EXPIRY` | 2 592 000 seconds (30 days) |
| Default MTU | 500 bytes (`pkg/packet.MTU`) |

