# CLI utilities

| Field | Value |
|-------|-------|
| Document version | 1.0 |
| Last updated | 2026-07-09 |
| Author | Ivan |

Go-native tools that speak the same shared-instance msgpack RPC and identity file formats as Python `rnstatus`, `rnid`, and `rnprobe`. They are not Python clones. Build them with:

```bash
make build-utils
```

Binaries land in `bin/rgostatus`, `bin/rgoid`, and `bin/rgoprobe`.

| Tool | Python counterpart | Role |
|------|--------------------|------|
| `rgostatus` | `rnstatus` | Interface and transport status over shared-instance RPC |
| `rgoid` | `rnid` | Identity generate, hash, `.rsg` / `.rsm` / `.rfe` |
| `rgoprobe` | `rnprobe` | Path wait, encrypted probe, RTT |

Library code lives in `pkg/rnsutil`.

## Shared-instance RPC (required for rgostatus)

`rgostatus` does not start a network stack. It dials a running shared instance (Python `rnsd` or `reticulum-go`) and calls `get: interface_stats` over the same multiprocessing.connection + msgpack protocol Python uses.

### Why connection refused is common

Three separate issues usually stack:

1. **Wrong config directory.** Python uses `~/.reticulum`. Go defaults to `~/.reticulum-go`. Point `-config` at the directory of the daemon you are querying.
2. **Linux Python defaults to Unix RPC.** Without `shared_instance_type = tcp`, `rnsd` listens on an abstract Unix socket (`@rns/<instance_name>/rpc`), not `127.0.0.1:37429`. Go tools dial TCP by default when the config says `tcp` or when the type is unset in Go configs.
3. **Daemon not sharing.** The process that owns interfaces must have `share_instance = yes` and be running.

### Working config for Python rnsd + Go tools

Add the same block to **both** `~/.reticulum/config` and `~/.reticulum-go/config` when you want either path to reach the same daemon:

```ini
[reticulum]
share_instance = yes
instance_name = default
shared_instance_type = tcp
shared_instance_port = 37428
instance_control_port = 37429
rpc_key = <64 hex characters>
```

Generate a key once:

```bash
python3 -c 'import secrets; print(secrets.token_hex(32))'
```

Restart `rnsd` after editing `~/.reticulum/config` so it binds TCP `127.0.0.1:37429`.

Then:

```bash
./bin/rgostatus -config ~/.reticulum
./bin/rgostatus -config ~/.reticulum -json
./bin/rgostatus -config ~/.reticulum -l -s announce
```

If both configs share ports and `rpc_key`, `-config ~/.reticulum-go` also works against a Python `rnsd` that was started with the matching `~/.reticulum` settings.

### Auth key rules

| Config | Authkey used |
|--------|----------------|
| `rpc_key` set (64 hex chars) | That exact 32-byte key |
| `rpc_key` empty | SHA-256 of the daemon `storage/transport_identity` private key |

Go and Python must agree. Prefer an explicit shared `rpc_key` when mixing stacks so you do not depend on identical transport identity files.

### Query a Go daemon instead

Run `reticulum-go` with `share_instance = yes` and `shared_instance_type = tcp` under `~/.reticulum-go`, then:

```bash
./bin/rgostatus -config ~/.reticulum-go -json
```

Only one process should own the shared instance ports at a time.

## rgostatus

```bash
rgostatus [flags]
```

| Flag | Meaning |
|------|---------|
| `-config dir` | Config directory (default: `~/.reticulum-go`) |
| `-json` | Emit JSON (bytes as hex, same field names as Python where populated) |
| `-a` | Include all interfaces (less filtering of local/client peers) |
| `-n substr` | Filter interface names |
| `-l` | Include link count |
| `-s key` | Sort by `rate`, `rx`, `tx`, `rxs`, `txs`, `traffic`, `announce`, `arx`, `atx`, `prx`, `ptx`, `held` |
| `-r` | Sort ascending (default descending) |
| `-timeout dur` | RPC timeout (default 10s) |

JSON includes per-interface announce and path-request frequencies, held announces, burst flags, and traffic counters when the daemon provides them.

## rgoid

Identity and signing tool. Files are wire-compatible with Python:

| Extension | Format |
|-----------|--------|
| `.rid` | 64 raw bytes (X25519 private + Ed25519 seed) |
| `.rsg` | 64-byte Ed25519 signature + msgpack envelope (`hashtype`, `hash`, `meta`) |
| `.rsm` | Same as `.rsg` with embedded `message` |
| `.rfe` | Chunked identity encrypt (same token layout as Python) |

Examples:

```bash
./bin/rgoid -g ~/.reticulum-go/id.rid -p
./bin/rgoid -i ~/.reticulum-go/id.rid -H rns.id
./bin/rgoid -i id.rid -s file.bin -f
./bin/rgoid -i id.rid -V file.bin
./bin/rgoid -i id.rid -S "hello" -w note -f
./bin/rgoid -V note.rsm
./bin/rgoid -i id.rid -e secret.txt -f
./bin/rgoid -i id.rid -d secret.txt.rfe -f
```

Go-signed `.rsg` / `.rsm` / `.rfe` validate with Python `rnid`, and the reverse also works.

## rgoprobe

```bash
rgoprobe [flags] <full_name> <destination_hash_hex>
```

Attaches as a shared-instance client (or starts local transport), waits for a path, sends encrypted probes, and prints RTT. Example:

```bash
./bin/rgoprobe -config ~/.reticulum -n 3 -v app.aspect aabbccddeeff00112233445566778899
```

## Troubleshooting

| Symptom | Fix |
|---------|-----|
| `dial tcp 127.0.0.1:37429: connection refused` | Start the daemon. Set `shared_instance_type = tcp`. Restart after config change. Use `-config` for that daemon's config dir. |
| `rpc auth` failure | Align `rpc_key`, or use the same `storage/transport_identity` when keys are derived. |
| Empty or missing announce rates from Python | Field is present but may be `0` until traffic accumulates. Sorting and JSON keys still work. |
| Top-level `rxb`/`txb` are `0` while interfaces show traffic | Python aggregate totals often omit some parent interfaces. Prefer per-interface counters. |
| Identity load log lines on stderr | Harmless debug from loading `transport_identity` for derived auth when resolving keys. Prefer explicit `rpc_key` to avoid that path when possible. |

## Related documents

- [Configuration](configuration.md) for `share_instance`, ports, and `rpc_key`
- [Compatibility](compatibility.md) for Python utility parity
- [Getting started](getting-started.md) for first daemon run
- [Package map](package-map.md) for `pkg/rnsutil`
