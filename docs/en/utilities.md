# CLI utilities

Go-native tools that speak the same shared-instance msgpack RPC and identity file formats as Python rnstatus, rnid, rnprobe, rnpath, rncp, and rnx. They are not Python clones.

They ship as **subcommands of the single `reticulum-go` binary**:

```bash
make build
./bin/reticulum-go status
./bin/reticulum-go id -h
./bin/reticulum-go probe ...
./bin/reticulum-go path -t
./bin/reticulum-go cp -l
./bin/reticulum-go x -l
./bin/reticulum-go pageserver
```

`make install` also creates legacy symlinks (rgostatus, rgoid, rgoprobe, rgopath, rgocp, rgox, rnx, rgopageserver, rgoslow, rgospeed, rgodump, rgosnap, rgoselfcheck) that invoke the same binary. Man pages: man reticulum-go, man reticulum-go-status, man reticulum-go-speedtest, man reticulum-go-self-check, and so on.

| Tool / subcommand | Python counterpart | Role |
|-------------------|--------------------|------|
| `reticulum-go status` (rgostatus) | rnstatus | Interface and transport status over shared-instance RPC |
| `reticulum-go slow` (rgoslow) | (Go-only) | Bottleneck and local health findings from interface/path stats |
| `reticulum-go id` (rgoid) | rnid | Identity generate, hash, `.rsg` / `.rsm` / `.rfe` |
| `reticulum-go probe` (rgoprobe) | rnprobe | Path wait, encrypted probe, RTT |
| `reticulum-go path` (rgopath) | rnpath | Path table, drop, blackhole, path request |
| `reticulum-go cp` (rgocp) | rncp | File send / listen / fetch over links |
| `reticulum-go x` (rgox, rnx) | rnx | Remote command execution over links (`rnx.execute`) |
| `reticulum-go pageserver` | (example app) | NomadNet-style page and file server |
| `reticulum-go self-check` (rgoselfcheck) | (Go-only) | Host OS preflight for sandbox, crypto, and interfaces |
| `reticulum-go speedtest` (rgospeed) | `Examples/Speedtest.py` | Loopback smoke plus cross-host / docker daemon (`-daemon`, `-iface`) |
| `reticulum-go dump` (rgodump) | (Go-only) | Decode RNS packets from hex or pcap to JSONL |
| `reticulum-go snapshot` (rgosnap) | (Go-only) | Path table, links, and health drop counters as JSON |

Library code lives in `pkg/rnsutil` and `pkg/cli`. Pageserver logic lives in `pkg/pageserver`.

Packet capture and Wireshark docs live in [packet-debug.md](packet-debug.md). The cross-stack `INTEROP_EVENT` timeline is in [interop-timeline.md](interop-timeline.md).

## Shared-instance RPC (required for rgostatus and rgopath table modes)

rgostatus and `rgopath -t` / drop / blackhole modes dial a running shared instance (Python rnsd or `reticulum-go`) over the same multiprocessing.connection + msgpack protocol Python uses.

RPC is fully supported on **both** transports:

| shared_instance_type | Listen / dial address |
|------------------------|------------------------|
| tcp | `127.0.0.1:<instance_control_port>` (default 37429) |
| unix | Abstract socket `@rns/<instance_name>/rpc` (Linux) |
| unset | Platform default: **unix** on Linux, **tcp** elsewhere (matches Python RNS) |

Go implements both server and client for TCP and Unix. When the type is unset, utilities try the platform default first, then the other transport, so stock Linux Python rnsd works without forcing TCP.

### Why connection refused is common

Issues that usually stack:

1. **Wrong config directory.** Python uses `~/.reticulum`. Go defaults to `~/.reticulum-go`. Point `-config` at the directory of the daemon you are querying.
2. **Explicit transport mismatch.** If one side sets `shared_instance_type = tcp` and the other unix, dials miss. Leave the key unset on Linux, or set the same value on both.
3. **Daemon not sharing.** The process that owns interfaces must have `share_instance = yes` and be running.
4. **Auth key mismatch.** Align rpc_key, or share the same derived transport_identity.

### Working config for Python rnsd + Go tools (Unix on Linux)

Stock Linux Python already uses abstract Unix sockets. Point Go tools at that config directory:

```bash
./bin/reticulum-go status -config ~/.reticulum
./bin/reticulum-go status -config ~/.reticulum -json
./bin/reticulum-go path -config ~/.reticulum -t -json
```

Optional shared auth key in **both** configs (recommended when mixing stacks):

```ini
[reticulum]
share_instance = yes
instance_name = default
shared_instance_type = unix
rpc_key = <64 hex characters>
```

Generate a key once:

```bash
python3 -c 'import secrets; print(secrets.token_hex(32))'
```

Restart rnsd after editing rpc_key. Go tools with unset or unix type dial `@rns/default/rpc`.

### TCP RPC (all platforms)

Use TCP when you want the same recipe on every OS, or when mixing with an older Go daemon that bound TCP:

```ini
[reticulum]
share_instance = yes
instance_name = default
shared_instance_type = tcp
shared_instance_port = 37428
instance_control_port = 37429
rpc_key = <64 hex characters>
```

Restart the daemon after editing so it binds TCP `127.0.0.1:37429`.

### Auth key rules

| Config | Authkey used |
|--------|----------------|
| rpc_key set (64 hex chars) | That exact 32-byte key |
| rpc_key empty | SHA-256 of the daemon `storage/transport_identity` private key |

Go and Python must agree. Prefer an explicit shared rpc_key when mixing stacks so you do not depend on identical transport identity files.

### Query a Go daemon instead

Run `reticulum-go` with `share_instance = yes` under `~/.reticulum-go`. Unset type follows the platform default (Unix on Linux):

```bash
./bin/reticulum-go status -config ~/.reticulum-go -json
./bin/reticulum-go path -config ~/.reticulum-go -t
```

Only one process should own the shared instance ports (or Unix RPC name) at a time.

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
| `-s key` | Sort by rate, rx, tx, rxs, txs, traffic, announce, arx, atx, prx, ptx, held |
| `-r` | Sort ascending (default descending) |
| `-timeout dur` | RPC timeout (default 10s) |

JSON includes per-interface announce and path-request frequencies, held announces, burst flags, and traffic counters when the daemon provides them.

Against a Go daemon, human and JSON output also include local mesh health fields when counters are non-zero: ifac_fail, hmac_fail, announce_sig_fail, unpack_fail, integrity_fail_rate, stale_closes, keepalive_timeout, and related totals. Python rnsd does not populate these keys. Missing fields mean zero or unknown, not a protocol error.

## rgoslow

```bash
reticulum-go slow [flags]
# legacy: rgoslow
```

Ranks congestion and local health signals that commonly explain stalled transfers or noisy interfaces. Uses the same shared-instance RPC as status (interface_stats plus path table).

| Flag | Meaning |
|------|---------|
| `-config dir` | Config directory (default: `~/.reticulum-go`) |
| `-json` | Emit full JSON report |
| `-a` | Include all interfaces |
| `-n substr` | Filter interface names |
| `-l` | Include link count |
| `-dest hash` | Focus analysis on a destination (32 hex) |
| `-top n` | Max interfaces to rank (default 12) |
| `-high-hop n` | Hop count treated as high (default 6) |
| `-m` | Continuously refresh |
| `-I dur` | Monitor interval (default 2s) |
| `-timeout dur` | RPC timeout (default 10s) |

Bottleneck findings cover bitrate utilization, announce/PR bursts, held announces, bandwidth gates, socket RTT, and high-hop paths.

When talking to a Go daemon, health findings can also appear:

| Kind | Meaning |
|------|---------|
| integrity_burst | Elevated IFAC/HMAC/unpack fail rate vs accepted frames |
| auth_pressure | Announce signature or link proof rejects clustered on an iface |
| link_degraded | Rising stale closes or keepalive timeouts |
| ingress_pressure | Held announces or burst limiters active |

Counters stay local to the node. slow only observes and scores. It does not change ingress policy or blackhole tables.

## rgospeed

```bash
reticulum-go speedtest [flags]
# legacy: rgospeed
```

Link throughput test modeled on Python `Examples/Speedtest.py`.

| Mode | Command |
|------|---------|
| Loopback smoke (default / CI) | `reticulum-go speedtest` or `-loopback` |
| Server (oneshot) | `reticulum-go speedtest -l` |
| Daemon (VPS / docker) | `reticulum-go speedtest -daemon` |
| Client (cross-host) | `reticulum-go speedtest <server_dest_hash>` |

Destination is `speedtest.server`. Server and client must use the same `-bytes` size. After the transfer the server sends a `SPEEDOK` ack with the confirmed RX count. Networked clients pace sends (100 µs per packet by default) so UDP sockets are not overrun; loopback does not pace.

Every run prints a grep-friendly `speedtest_result ...` line on stdout (visible in `docker logs`). With `-json`, a JSON object follows on stdout as well.

Use a real config with UDP/TCP (or a shared path). share_instance is forced off so the tool owns its interfaces. Python-style forward_ip / forward_port are accepted as aliases for target_host / target_port.

| Flag | Meaning |
|------|---------|
| `-loopback` | In-process pipe (CI liveness, default when no args) |
| `-l` | Listen as server (one client then exit) |
| `-daemon` | Listen forever (implies `-l -m`, default announce every 120s) |
| `-m` | Listen: serve multiple clients |
| `-iface` | all (default) or comma-separated config section names |
| `-p` | Print identity / destination hash and exit |
| `-config dir` | Config directory (default `~/.reticulum-go`) |
| `-identity path` | Persistent identity for listen mode |
| `-bytes n` | Plaintext bytes to transfer (default 2 MiB) |
| `-min-bps n` | Fail below this rate (`0` disables; loopback defaults to 1e6) |
| `-timeout sec` | Overall timeout (default 60) |
| `-announce sec` | Listen announce interval (`0` once, `<0` never) |
| `-json` | Emit JSON after each speedtest_result line |
| `-q` | Quieter debug |

Cross-host example (two machines / configs with a shared path):

```bash
# server
reticulum-go speedtest -daemon -iface tcp -bytes 4194304
# client (paste the 32-hex hash printed by the server)
reticulum-go speedtest -iface tcp -bytes 4194304 <hash_from_server>
```

### Docker VPS reference

Build and run a persistent public TCP listener:

```bash
task docker:build:speedtest
docker run -d --name rgo-speedtest -p 4242:4242 \
  -e SPEEDTEST_IFACE=tcp \
  -v reticulum-go-speedtest:/data \
  reticulum-go-speedtest:latest
docker logs -f rgo-speedtest
```

Env knobs: `SPEEDTEST_IFACE` (all or names), `SPEEDTEST_BYTES`, `SPEEDTEST_ANNOUNCE`, `SPEEDTEST_PORT`, `SPEEDTEST_JSON`, `RNS_CONFIG`. Persist `/data` so the identity (and thus dest hash) stays stable for CI secrets.

CI client config needs a TCPClientInterface to the VPS host:port and the printed dest hash. Treat measured rates as a path floor (runner to VPS), not a lab loopback number.

Nightly CI runs `task test-link-speed` (TestLinkSpeedSmoke) with a 512 KiB loopback cap and a 1 MB/s floor.

## rgoid

Identity and signing tool. Files are wire-compatible with Python:

| Extension | Format |
|-----------|--------|
| `.rid` | 64 raw bytes (X25519 private + Ed25519 seed) |
| `.rsg` | 64-byte Ed25519 signature + msgpack envelope (hashtype, hash, meta) |
| `.rsm` | Same as `.rsg` with embedded message |
| `.rfe` | Chunked identity encrypt (same token layout as Python) |

Examples:

```bash
./bin/reticulum-go id -g ~/.reticulum-go/id.rid -p
./bin/reticulum-go id -i ~/.reticulum-go/id.rid -H rns.id
./bin/reticulum-go id -i id.rid -s file.bin -f
./bin/reticulum-go id -i id.rid -V file.bin
./bin/reticulum-go id -i id.rid -S "hello" -w note -f
./bin/reticulum-go id -i id.rid -S @inventory.txt -w reticulum-go.rsm -f
./bin/reticulum-go id -V note.rsm
./bin/reticulum-go id -i e46112d44649266d71fe2193e00a4710 -V reticulum-go.rsm -extract
./bin/reticulum-go id -i id.rid -e secret.txt -f
./bin/reticulum-go id -i id.rid -d secret.txt.rfe -f
```

`-S @path` reads the message body from a file (needed for large tree inventories). `-extract` prints only the embedded RSM message after a successful verify. `-i` may be a 32-character identity hash when verifying (no private key required).

Go-signed `.rsg` / `.rsm` / `.rfe` validate with Python rnid, and the reverse also works.

## rgoprobe

```bash
rgoprobe [flags] <full_name> <destination_hash_hex>
```

Attaches as a shared-instance client (or starts local transport), waits for a path, sends encrypted probes, and prints RTT. Example:

```bash
./bin/reticulum-go probe -config ~/.reticulum -n 3 -v app.aspect aabbccddeeff00112233445566778899
```

## rgopath

Path table and blackhole management over shared-instance RPC, plus a default path-request mode that attaches like rgoprobe.

```bash
rgopath [flags] [destination_hash]
```

| Flag | Meaning |
|------|---------|
| `-config dir` | Config directory |
| `-t` | Show path table (optional hash filter) |
| `-json` | JSON for `-t` / `-blackholed` |
| `-m N` | Max hops filter for path table |
| `-d` | Drop path to hash |
| `-D` | Drop all paths via transport hash |
| `-q` | Drop announce queues |
| `-w sec` | Path request timeout (default 15) |
| `-blackholed` | List blackholed identities |
| `-blackhole` | Blackhole identity hash |
| `-unblackhole` | Lift blackhole |
| `-for hours` | Blackhole duration (0 = indefinite) |
| `-reason str` | Blackhole reason |
| `-filter substr` | Filter blackhole list lines |

Go extras (compat preserved): `-json` on path and blackhole lists, drop-via count in the success line, clearer path-found summary after a successful request.

Remote rnstransport management modes from Python rnpath are not ported yet.

## rgocp

File transfer over links. Destination name is `rncp.receive` so Go and Python peers interoperate.

```bash
rgocp [flags] <file> <destination_hash>     # send
rgocp -l [flags]                            # listen
rgocp -f -F <remote_path> [flags] <hash>    # fetch
```

| Flag | Meaning |
|------|---------|
| `-config dir` | Config directory |
| `-identity path` | Identity file (default `storage/identities/rncp`) |
| `-l` | Listen for pushes |
| `-f` / `-F path` | Fetch remote file |
| `-a` | Allow unauthenticated senders (listen) |
| `-allowed hash` | Allowed identity (repeatable) |
| `-allow-fetch` | Enable fetch_file requests |
| `-jail dir` | Restrict fetch paths |
| `-save dir` | Save directory for received files |
| `-overwrite` | Overwrite on receive |
| `-no-compress` | Disable auto compression |
| `-announce sec` | Announce interval (`0` once, `<0` never) |
| `-w sec` | Path/link timeout |
| `-s` | Silent progress |
| `-p` | Print identity and destination hash |

Allow lists are loaded from `/etc/rncp/allowed_identities`, `~/.config/rncp/`, `~/.rncp/`, plus Go-specific `~/.config/rgocp/` and `~/.rgocp/`.

Go extras: cleaner progress lines on stderr, `-json` is not used (transfer is binary), unique `.N` rename when not overwriting.

## rgox / rnx

Remote command execution over links. Destination name is `rnx.execute`, request path command (wire-compatible with Python rnx).

```bash
reticulum-go x -l [flags]                         # listen
reticulum-go x [flags] <destination_hash> <cmd>   # execute
reticulum-go x -x [flags] <destination_hash>      # interactive
```

| Flag | Meaning |
|------|---------|
| `-config dir` | Config directory |
| `-i path` | Identity file (default `storage/identities/rnx`) |
| `-l` | Listen for commands |
| `-x` | Interactive REPL |
| `-a hash` | Allowed identity (repeatable, listen) |
| `-n` | Accept from anyone (listen) |
| `-N` | Do not identify to listener |
| `-b` | Skip announce on listen start |
| `-m` | Mirror remote exit code |
| `-d` | Detailed timing/size summary |
| `-w sec` | Path/link/command timeout |
| `-W sec` | Max result download time |
| `--stdin str` | Remote stdin |
| `--stdout N` / `--stderr N` | Max returned bytes |
| `-json` | Structured JSON result (Go) |
| `-p` | Print identity and destination hash |

Allow lists: `/etc/rnx/`, `~/.config/rnx/`, `~/.rnx/`, plus `~/.config/rgox/` and `~/.rgox/`.

Exit codes match Python rnx (241-249 for client failures, `-m` mirrors remote).

## Troubleshooting

| Symptom | Fix |
|---------|-----|
| `dial tcp 127.0.0.1:37429: connection refused` | Start the daemon. If type is explicit tcp, the daemon must also be TCP. With unset type, utilities also try Unix. Use `-config` for that daemon's config dir. |
| `dial unix @rns/...: connection refused` | Daemon is on TCP only, or instance_name differs. Align shared_instance_type and instance_name, or leave type unset so tools try both. |
| `rpc auth` failure | Align rpc_key, or use the same `storage/transport_identity` when keys are derived. |
| Empty or missing announce rates from Python | Field is present but may be `0` until traffic accumulates. Sorting and JSON keys still work. |
| Top-level rxb/txb are `0` while interfaces show traffic | Python aggregate totals often omit some parent interfaces. Prefer per-interface counters. |
| Identity load log lines on stderr | Harmless debug from loading transport_identity for derived auth when resolving keys. Prefer explicit rpc_key to avoid that path when possible. |
| rgocp transfer ignored | Listener needs `-a` or an allow-list entry matching the sender identity hash. Metadata (name) is required on the wire. |

## Debugging

| Tool | Role |
|------|------|
| `reticulum-go status` | Interface stats over shared-instance RPC (`-json`, `-q`), including Go integrity counters when present |
| `reticulum-go slow` | Bottleneck and local health findings (integrity_burst, auth_pressure, link_degraded, …) |
| `reticulum-go path -t` | Path table dump |
| `reticulum-go debug` | Effective config path, log level, platform, RPC reachability (`-rates`, `-json`) |
| `reticulum-go self-check` | Host OS preflight checklist (`--json`, `--quick`, `--full`, `--strict`) |
| `reticulum-go probe` | Connectivity / RTT (`-json`) |
| Control API | HTTP `/v1/health` (liveness), `/v1/status` (iface stats plus integrity fields), `/v1/paths` when `enable_control_api = yes` |
| Daemon `-debug N` | Override config loglevel for one run |

TTY colors (status Up/Down, probe/path/cp/id outcomes, pageserver banner, daemon text log levels) honor `NO_COLOR` (off) and `FORCE_COLOR` / `CLICOLOR_FORCE` (on). JSON output and file logs stay plain.

## Related documents

- [Configuration](configuration.md) for share_instance, ports, and rpc_key
- [Compatibility](compatibility.md) for Python utility parity
- [Getting started](getting-started.md) for first daemon run
- [Package map](package-map.md) for `pkg/rnsutil`
- [Links, channels, and resources](links-channels-and-resources.md) for resource wire details
