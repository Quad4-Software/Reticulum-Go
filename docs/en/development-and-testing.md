# Development and testing

## Development environment

Requirements:

- Go 1.26.5 or later
- Make or Task
- revive linter for `make lint`
- Python 3 for crossref vector generation (optional)

Clone the repository. Dependencies are in `vendor/`. No network fetch is needed for ordinary builds.

## Code quality commands

```bash
make fmt
make vet
make lint
make test-short
make vulncheck
make check
```

`make check` runs fmt, vet, lint, test-short, and vulncheck in sequence.

### Linting

revive with project config:

```bash
revive -config revive.toml -formatter friendly ./pkg/* ./cmd/* ./internal/*
```

### Race detector

```bash
make test-race
```

Transport and interface packages include dedicated race and deadlock regression tests (`interface_stress_race_test.go`, `pipe_race_test.go`, `pipe_deadlock_test.go`).

### Coverage

```bash
make coverage
```

Opens HTML coverage from `coverage.out`.

### Benchmarks

```bash
make bench
task test-bench-gate
```

Loopback link throughput smoke (RNS Speedtest-style liveness floor):

```bash
task test-link-speed
reticulum-go speedtest
```

Nightly `sim-heavy` also runs `test-link-speed`.

## Project layout for contributors

| Path | Purpose |
|------|---------|
| `pkg/` | Public library code |
| `cmd/` | Binaries and e2e tests |
| `internal/` | Daemon-only code |
| `tests/crossref/` | Python vector parity |
| `tests/interop/` | Live Go/Python tests |
| `scripts/ci/` | CI install and release scripts |
| `.github/workflows/` | GitHub Actions |

Follow existing naming, error wrapping, and SPDX headers in each file.

## Testing layers

| Layer | How to run | What it covers |
|-------|------------|----------------|
| Unit | `task test` / `task test-short` | Package `*_test.go` |
| Property | `task test-property` | `*_pbt_test.go` and embedded `quad4/pbt` / `testing/quick` |
| Mutation | `task test-mutation` | gomutant on cryptography, packet, announce, destination, identity, ifac |
| Chaos | `task test-chaos` / `task test-soak` / `task test-soak-protect` | `TestSimChaos*` / `TestLinkChaos*` / `TestIfaceChaos*` plus soak, including dos_protection flood soak |
| Oracle | `task test-oracle` | Crossref vectors, health `TransportOracle` deltas, adversarial corpus |
| Smoke | `task test-binary-smoke`, binding smokes | Binary `--version`/`--help`, CLI dump via `Main`, librns smoke examples |
| Acceptance | `task test-acceptance` | librns SCAFFOLD minimum, control API acceptance |
| E2E | `task test-e2e` | Daemon reload, UDP path e2e, transport `TestE2E_*` |
| Black box | `task test-blackbox` | CLI `Main` / `rgodump` surface, control API HTTP acceptance |
| Interop | `RUN_LIVE_INTEROP=1 go test ./tests/interop/...` | Live Go↔Python (optional locally), including dos_protection live suites |

### Unit tests

Standard `go test` in each package. Run all:

```bash
go test -v ./...
```

Short mode skips long tests:

```bash
go test -short -v ./...
```

### Property-based tests

Files named `*_pbt_test.go` use `quad4/pbt` for generative testing (cryptography, packet, buffer, rate, resource, announce, link, identity). Some properties still live beside unit tests in the same package.

```bash
task test-property
```

### Mutation tests

In-repo `tools/gomutant` flips same-width operators and re-runs package tests.

```bash
task test-mutation
```

Default packages: cryptography, packet, announce, destination, identity, ifac. Override with `MUTATION_PACKAGES` / `MUTATION_THRESHOLD`.

### Fuzz tests

Files named `*_fuzz_test.go` cover packet, link, ifac, blackhole, discovery, health counters (`pkg/health`), pipe HDLC framing (`pkg/interfaces/pipe_fuzz_test.go`), and librns (`pkg/librns`).

### Chaos and fault injection

Seeded loss, reorder, corruption, and flap tests across layers:

| Prefix | Package | Focus |
|--------|---------|-------|
| `TestSimChaos*` | `pkg/transport` | Multi-node path and announce under delay/loss/corrupt/reorder/flap (health oracle deltas on corrupt) |
| `TestLinkChaos*` | `pkg/link` | Establish, packet under loss/reorder, resource under capped drop, mid-session flap, goroutine budget |
| `TestIfaceChaos*` | `pkg/interfaces` | TCP HDLC corrupt/reorder, Local corrupt resync, Pipe respawn, Backbone reconnect |

```bash
task test-chaos
task test-soak
task test-soak-protect
```

Chaos suites are in-process Go only (sim pipes / HDLC fixtures). Live Go↔Python loss, reorder, and mid-session flap are not covered yet. Healthy-path cross-stack coverage lives under `tests/interop/` (`RUN_LIVE_INTEROP=1`).

### dos_protection tests

`pkg/protect` and interface hooks cover false positives, false negatives, auto learn/persist/relearn, and live sockets.

| Suite | Location | Focus |
|-------|----------|-------|
| Unit / FP-FN | `pkg/protect/false_positive_test.go` | Quiet traffic, bursty legit, multi-iface isolation, oscillation, poisoned warmup, drift without false block |
| Replay traces | `pkg/protect/replay_trace_test.go` | Mesh-like timelines, flood inject and recovery, auto learn then block |
| Property / fuzz | `pkg/protect/property_test.go`, `fuzz_test.go` | Detect never blocks, prevent after threshold, mode round-trip |
| Soak | `task test-soak-protect` | Bounded flood heap and goroutine budgets |
| Live UDP/TCP | `pkg/interfaces/protect_*_live_test.go` | Real loopback sockets, optional non-loopback NIC, TCP accept storms |
| Live interop | `tests/interop/dos_protect_live_test.go` | `RUN_LIVE_INTEROP=1`: quiet budget, UDP flood shed, auto learn on live UDP, transport path, public mesh peer dials |

```bash
go test ./pkg/protect -short
go test ./pkg/interfaces -run 'LiveUDP|LiveTCP|Protect'
task test-soak-protect
RUN_LIVE_INTEROP=1 go test ./tests/interop -run 'DoSProtect|DoSProtection'
```

Config and threat scope: [Configuration](configuration.md#dos_protection-go-only), [Security](security.md#dos-protection-local-idsips).

### Test oracles

| Oracle | Location |
|--------|----------|
| Python crossref vectors | `tests/crossref/` |
| Health counter deltas | `pkg/health.TransportOracle` / `OracleSnapshot.Delta` |
| Handshake / adversarial frames | `pkg/packet/testdata/` |
| IFAC goldens | `pkg/ifac` unit tests |
| Sim path/hop asserts | `pkg/transport/sim_assertions_test.go` |

```bash
task test-oracle
```

### Smoke, acceptance, e2e, black box

```bash
task test-binary-smoke
task test-acceptance
task test-e2e
task test-blackbox
```

Notable additions: `pkg/cli` dump smoke via `Main`, `pkg/librns.TestAcceptanceScaffoldMinimum`, `pkg/node.TestUDPPathE2E`.

### Crossref tests

Location: `tests/crossref/`

Purpose: byte-level parity with Python reference output.

Workflow:

```bash
./tests/crossref/run_crossref.sh generate   # requires Python reference
./tests/crossref/run_crossref.sh test
./tests/crossref/run_crossref.sh all
```

Vectors are JSON with format version 5 (`generate_vectors.py`). The reference tree is fetched via rngit in `run_crossref.sh`.

Coverage includes identity, HKDF, HMAC, packet wire, announces, encryption, links, resources, channel envelopes, buffers, path requests.

Handshake decode trees for porters also live in `pkg/packet/testdata/handshake_vectors.json` (see [packet-debug.md](packet-debug.md)).

### Packet debug tools

- Wireshark Lua dissector: `tools/wireshark/rns.lua`
- `reticulum-go dump` / rgodump for hex and pcap
- `reticulum-go snapshot` / rgosnap for path and health JSON
- Timeline convention: [interop-timeline.md](interop-timeline.md)

```bash
go test ./pkg/packet/ -run 'TestHandshakeVector|TestTshark|TestDecode|TestPCAP'
go test ./pkg/health/ -run TestDrop
go test ./pkg/cli/ -run TestRunDump
```

### Interop tests

Location: `tests/interop/`

Live tests pair a Go process with Python helpers under `tests/interop/py/`.

Enable:

```bash
RUN_LIVE_INTEROP=1 go test -v ./tests/interop/...
```

Optional Python interpreter (prefer a venv with `rns==1.3.9`):

```bash
sh scripts/ci/setup-venv-pip.sh 'rns==1.3.9'
PYTHON_INTEROP=.venv/bin/python RUN_LIVE_INTEROP=1 go test -v ./tests/interop/...
```

#### Debug harness

NomadNet relay, pageserver, and shared helpers use `tests/interop/harness/`.

| Variable | Behavior |
|----------|----------|
| `INTEROP_ARTIFACTS=1` | Always keep artifact dirs (default: only when the test fails) |
| `INTEROP_EVENTS=1` | Force event logging (also on when artifacts are enabled) |
| `INTEROP_ARTIFACT_ROOT` | Parent directory for durable artifact folders |

On failure (or with `INTEROP_ARTIFACTS=1`) the test logs the artifact path and the last events. Typical files:

| File | Contents |
|------|----------|
| `events.jsonl` | One JSON object per line (ts, src, event, kind, detail) |
| `stderr.txt` | Captured Python stderr (human RNS logs plus `INTEROP_EVENT` lines) |
| `env.json` | Selected interop env keys |

Python peers emit `INTEROP_EVENT {...}` on stderr via `tests/interop/py/interop_events.py`. Stdout tokens such as `READY` and `REQUEST_OK` are unchanged.

Example:

```bash
INTEROP_ARTIFACTS=1 INTEROP_ARTIFACT_ROOT=/tmp/rns-interop \
  RUN_LIVE_INTEROP=1 go test -v ./tests/interop/ -run 'TestLiveNomadNetLinkThroughGoRelay|TestLiveInteropPythonPageServerLargePageRequest'
```

| Test file | Topic |
|-----------|-------|
| `link_live_test.go` | Link sessions, resources, binary burst echo, Go→Python request |
| `channel_buffer_live_test.go` | Channel messages and buffer streams |
| `rncp_blackhole_live_test.go` | rncp file transfer and blackhole LINKIDENTIFY |
| `auto_live_test.go` | AutoInterface |
| `ifac_live_test.go` | IFAC |
| `transport_path_live_test.go` | Path requests |
| `transport_relay_live_test.go` | Transport relay |
| `backbone_live_test.go` | Backbone |
| `quic_live_test.go` | QUIC Go-Go echo (no Python peer) |
| `pipe_live_test.go` | PipeInterface with Python echo |
| `modem73_live_test.go` | Modem73Interface (fake KISS/control, optional modem73 binary audio loop via `MODEM73_BIN` + pactl) |
| `sdr_live_test.go` | SDRInterface mock exchange (`RUN_LIVE_SDR=1`) |
| `shared_rpc_live_test.go` | Shared-instance RPC (TCP and Unix on Linux) |
| `pageserver_live_test.go` | Pageserver example |
| `nomadnet_crawl_live_test.go` | Nomadnet crawl |
| `nomadnet_relay_live_test.go` | NomadNet through Go mesh relay |
| `path_cp_live_test.go` | path and rgocp utilities |
| `i2p_live_test.go` | I2PInterface Go and Python (needs SAM) |
| `directory_outgoing_live_test.go` | Public directory clearnet TCP peers |

### Package-specific live tests

| Package | Env var |
|---------|---------|
| `pkg/i2p`, `pkg/interfaces` I2P | `RUN_LIVE_I2P=1` (optional `I2P_SAM_ADDRESS`, `I2P_DIRECTORY_URL`) |
| `pkg/blackhole` | `RUN_PY_INTEROP=1` |
| `pkg/discovery` | `RUN_PY_INTEROP=1` |

### End-to-end daemon tests

`cmd/reticulum-go/` contains `controlapi_e2e_test.go`, `reload_e2e_test.go`, and related tests.

### Host self-check

`reticulum-go self-check` is a host OS preflight. It validates that platform features work on the machine under test (crypto, identity file backend, sandbox, securemem, loopback interfaces, daemon with sandbox, shared-instance RPC, and on Unix a SIGHUP config/interface reload).

```bash
make test-self-check
# or
task test-self-check
# or
./bin/reticulum-go self-check --json --full
```

Flags:

| Flag | Behavior |
|------|----------|
| `--json` | Machine-readable report |
| `--quick` | Core and platform only (no loopback or daemon) |
| `--full` | Also probe QUIC, HTTPS, VSOCK, Pipe, and Serial |
| `--interop` | Optional external tools (crossref vectors, Python RNS, binding CLIs) |
| `--strict` | Treat warnings as failures |
| `--binary PATH` | Binary used for CLI and daemon checks |

Environment:

| Variable | Behavior |
|----------|----------|
| `RETICULUM_SELF_CHECK=1` | Used by CI wrappers that invoke the same checklist |
| `RETICULUM_SELF_CHECK_INTEROP=1` | Enables the interop tier |
| `RETICULUM_TEST_KEYRING=1` | Require Linux keyring round-trip (fail if unavailable) |

Exit code is non-zero on any fail result. With `--strict`, warnings also fail.

Daemon checks include Control API health with sandbox enabled, shared-instance GetInterfaceStats RPC, and (except Windows and FreeBSD CapEnter) SIGHUP reload of a UDP interface.

CI runs self-check on Linux (amd64 and arm64), macOS, Windows, FreeBSD, and OpenBSD. Extra Linux arches (`386`, arm GOARM=6, riscv64, ppc64le, ppc64) run via `qemu-user-static` (`task test-self-check-386`, `test-self-check-arm`, `test-self-check-riscv64`, `test-self-check-ppc64le`, `test-self-check-ppc64`). Android emulator self-check is a separate workflow (`selfcheck-android.yml`) on schedule or workflow_dispatch.

NetBSD is not in CI. Run `reticulum-go self-check` manually on that host.

## Vendoring

Ordinary builds use vendored modules. Refresh after dependency changes:

```bash
task vendor-sync
```

Requires `LIBS_ROOT` pointing at the Reticulum-Go-Projects sibling tree for replace directives. Commit `go.mod`, `go.sum`, and `vendor/` after refresh.

Day-to-day clones only need `vendor/` to build offline.

## CI overview

GitHub Actions workflows in `.github/workflows/`:

| Workflow | Role |
|----------|------|
| ci.yml | Build, test, reproducibility, OS self-check (Linux amd64/arm64 plus 386/arm/riscv64/ppc64le/ppc64 via qemu-user, macOS, Windows, FreeBSD, OpenBSD) |
| selfcheck-android.yml | Android emulator self-check (nightly / manual) |
| security.yml | Gosec, govulncheck, Trivy, SBOM dispatch |
| publish.yml | Tagged releases, cosign attestations |

CI uses Go 1.26.5 via `actions/setup-go` in `.github/actions/setup-ci` with `GOTOOLCHAIN=local` and vendored modules.

## Cross-compilation

```bash
make build-linux
make build-windows
make build-darwin
make build-all
```

Legacy Windows uses go-legacy-win7 (`make build-windows-legacy`).

## WebAssembly development

```bash
task build-wasm
task test-wasm
```

Requires Task. See [Embedding and WebAssembly](embedding-and-wasm.md).

## librns shared library

```bash
task build-librns
make -C bindings/c/examples/smoke
./bindings/c/examples/smoke/librns-smoke
```

Needs a C toolchain and CGO. Daemon builds stay `CGO_ENABLED=0`. See [librns](librns.md).

## Odin bindings

```bash
task build-librns
task test-odin
```

Requires the Odin compiler on `PATH` (CI installs a pinned monthly release via `scripts/ci/setup-odin.sh`, job `Odin bindings`). Package lives under `bindings/odin`. See [librns](librns.md#odin-bindings).

## Zig bindings

```bash
task build-librns
task test-zig
```

Requires Zig 0.16.0 or later on `PATH` (CI installs a pinned release via `scripts/ci/setup-zig.sh`, job `Zig bindings`). Package lives under `bindings/zig`. See [librns](librns.md#zig-bindings).

## C++ bindings

```bash
task build-librns
task test-cpp
```

Requires CMake and a C++17 compiler on `PATH` (CI job `C++ bindings`). Package lives under `bindings/cpp`. See [librns](librns.md#c-bindings).

## Dart bindings

```bash
task build-librns
task test-dart
```

Requires the Dart SDK on `PATH` (CI pins `3.11.4`, job `Dart bindings`) and CGO for `librns.so` FFI smoke tests. Package and examples live under `bindings/dart`. See [librns Dart FFI](librns.md#dart-ffi-bindings) and [Control API](control-api.md#dart-and-flutter).

## Rust bindings

```bash
task build-librns
task test-rust
```

Requires `cargo` on `PATH` (CI job `Rust bindings`). Package and examples live under `bindings/rust`.

## Python bindings

```bash
task build-librns
task test-python
```

Requires `python3` on `PATH` (CI job `Python bindings`). Package and examples live under `bindings/python`.

## Lua bindings

```bash
task build-librns
task test-lua
```

Requires LuaJIT on `PATH` (CI job `Lua bindings`). Package and examples live under `bindings/lua`.

## Swift bindings

```bash
task build-librns
task test-swift
```

Requires `swift` on `PATH` (CI pins `6.0.3` via `scripts/ci/setup-swift.sh`, job `Swift bindings`). Package and examples live under `bindings/swift`.

## Java bindings

```bash
task build-librns
task test-java
```

Requires `javac` on `PATH` (CI uses Temurin 17, job `Java bindings`). Package and examples live under `bindings/java`. JNA is fetched on first build.

## Kotlin bindings

```bash
task build-librns
task test-kotlin
```

Requires `kotlinc` and `javac` on `PATH` (CI pins Kotlin `2.1.10` via `scripts/ci/setup-kotlin.sh`, job `Kotlin bindings`). Package and examples live under `bindings/kotlin` and depend on `bindings/java`.

## C ABI examples

```bash
task build-librns
task test-c
```

Builds and runs `bindings/c/examples` (smoke plus page-fetch/pageserver compile). Binding CI jobs also run `make -C bindings/<lang> examples`.

## Adding a change safely

1. Write or extend unit tests in the affected package
2. If wire format changes, update crossref vectors and Python reference together
3. Run `make check` locally
4. For protocol behavior, add or extend interop test when feasible
5. Update [Compatibility](compatibility.md) and [COMPATIBILITY.md](../../COMPATIBILITY.md) if parity status changes

## Related documents

- [Security](security.md) for CI scan detail
- [Package map](package-map.md)
- [Examples](examples.md)
