# Python reference compatibility

This document ties Reticulum-Go to the [Reticulum Network Stack API reference](https://reticulum.network/manual/reference.html) and the Python `rns` package ([Reticulum](https://github.com/markqvist/Reticulum)).

## Protocol constants

The following match the documented `RNS.Identity` / `RNS.Reticulum` values used on the wire:

| Item | Value |
|------|--------|
| Curve | Curve25519 (X25519 + Ed25519) |
| `KEYSIZE` | 512 bits (256-bit encryption key + 256-bit signing key) |
| `TRUNCATED_HASHLENGTH` | 128 bits (identity and destination addressing) |
| `RATCHETSIZE` | 256 bits |
| `RATCHET_EXPIRY` | 2 592 000 seconds (30 days) |
| Default MTU | 500 bytes (`pkg/packet.MTU`) |

Changing MTU or hash widths breaks interoperability with other RNS stacks; the reference manual treats MTU as a global network invariant.

## Automated checks

### Cross-reference vectors (`tests/crossref`)

Golden tests compare Go output to values produced by the Python reference (cryptography, packets, announcements, links, resources, and related structures). The JSON fixtures are generated from the reference (`tests/crossref/generate_vectors.py`); CI runs `task test-crossref` with a generate step where applicable.

### Live interop (`tests/interop`)

End-to-end checks run Go against a real `rns` process over UDP loopback on the same machine:

- Set `RUN_LIVE_INTEROP=1` and install `rns` in a virtualenv (recommended: `python3 -m venv .venv && .venv/bin/pip install rns`).
- Point tests at that interpreter: `export PYTHON_INTEROP="$PWD/.venv/bin/python"`.
- Run with a generous timeout, e.g. `go test -timeout 600s ./tests/interop/...`.

The scripts under `tests/interop/*.py` use the same app name and aspect as the Go tests (`interop_pygo` / `linksvc`). Large-resource tests are stress cases; if the subprocess or interface stalls, retry or increase `--timeout` before treating it as a protocol bug.

## Behavioural parity notes

Incoming resource handling follows the reference implementation: a short random prefix is prepended to the plaintext before link encryption, while the advertisement carries a separate random value used in the SHA-256 content hash; the two are not required to be equal. Cache-request packets on links are sent unencrypted; resource completion uses a proof packet (`RESOURCE_PRF`) as in Python `Resource.prove` / `validate_proof`.
