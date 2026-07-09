# Development and testing

| Field | Value |
|-------|-------|
| Document version | 1.0 |
| Last updated | 2026-07-07 |
| Author | Ivan |

## Development environment

Requirements:

- Go 1.26.4 or later
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
```

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

Files named `*_pbt_test.go` use `quad4/pbt` for generative testing (cryptography, packet, buffer, rate, resource).

### Fuzz tests

Files named `*_fuzz_test.go` cover packet, link, ifac, blackhole, discovery, pipe HDLC framing (`pkg/interfaces/pipe_fuzz_test.go`), and librns (`pkg/librns`).

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

### Interop tests

Location: `tests/interop/`

Live tests pair a Go process with Python helpers under `tests/interop/py/`.

Enable:

```bash
RUN_LIVE_INTEROP=1 go test -v ./tests/interop/...
```

| Test file | Topic |
|-----------|-------|
| `link_live_test.go` | Link sessions |
| `auto_live_test.go` | AutoInterface |
| `ifac_live_test.go` | IFAC |
| `transport_path_live_test.go` | Path requests |
| `transport_relay_live_test.go` | Transport relay |
| `backbone_live_test.go` | Backbone |
| `quic_live_test.go` | QUIC Go-Go echo (no Python peer) |
| `pipe_live_test.go` | PipeInterface with Python echo |
| `shared_rpc_live_test.go` | Shared-instance RPC |
| `pageserver_live_test.go` | Pageserver example |
| `nomadnet_crawl_live_test.go` | Nomadnet crawl |

### Package-specific live tests

| Package | Env var |
|---------|---------|
| `pkg/i2p`, `pkg/interfaces` I2P | `RUN_LIVE_I2P=1` |
| `pkg/blackhole` | `RUN_PY_INTEROP=1` |
| `pkg/discovery` | `RUN_PY_INTEROP=1` |

### End-to-end daemon tests

`cmd/reticulum-go/` contains `controlapi_e2e_test.go`, `reload_e2e_test.go`, and related tests.

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
| ci.yml | Build, test, reproducibility |
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
make -C examples/librns-smoke
./examples/librns-smoke/librns-smoke
```

Needs a C toolchain and CGO. Daemon builds stay `CGO_ENABLED=0`. See [librns](librns.md).

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
