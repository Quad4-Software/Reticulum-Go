# reticulum-go-protocols

Go implementations of application protocols that run on [Reticulum-Go](https://git.quad4.io/reticulum/Reticulum-Go). All packages share one transport instance. This is a monorepo.

| Package | What it is | Docs |
| --- | --- | --- |
| [pkg/mf](pkg/mf) | Compact native format: 16-byte sender hash plus UTF-8 text | [docs/en/mf.md](docs/en/mf.md) |
| [pkg/lxmf](pkg/lxmf) | [LXMF](https://github.com/markqvist/LXMF) 1.3.0 pack, stamp, paper URI, delivery | [docs/en/lxmf.md](docs/en/lxmf.md) |
| [pkg/rrc](pkg/rrc) | [RRC](https://rrc.kc1awv.net/) protocol version 1 (spec 0.1.3) hub and client | [docs/en/rrc.md](docs/en/rrc.md) |
| [pkg/lxst](pkg/lxst) | [LXST](https://pypi.org/project/lxst/) 0.6.0 encrypted voice over Reticulum links | [docs/en/lxst.md](docs/en/lxst.md) |
| [pkg/rnv](pkg/rnv) | Reticulum-Go Native Video (concept only): stills, clips, low-rate A/V streams | [docs/en/rnv.md](docs/en/rnv.md) |
| [cmd/gorrcd](cmd/gorrcd) | Go RRC hub daemon | [docs/en/gorrcd.md](docs/en/gorrcd.md) |

Full English index: [docs/en](docs/en).

## Requirements

- Go 1.27.4
- [Reticulum-Go](https://git.quad4.io/reticulum/Reticulum-Go)
- [Make](https://www.gnu.org/software/make/) and/or [Task](https://taskfile.dev/) (optional wrappers)

Optional, for Go/Python interop tests:

- [uv](https://docs.astral.sh/uv/)
- LXMF-ref clone of upstream LXMF
- RRC-ref clone of [rrcd](https://github.com/kc1awv/rrcd)

## Install

The module path is `github.com/Quad4-Software/reticulum-go-protocols`. It is not on the public Go module proxy.

```go
require github.com/Quad4-Software/reticulum-go-protocols v0.0.0

replace github.com/Quad4-Software/reticulum-go-protocols => github.com/Quad4-Software/reticulum-go-protocols master
```

```bash
go mod tidy
```

Pin a commit or tag instead of `master` for a reproducible build. For a local checkout:

```go
replace github.com/Quad4-Software/reticulum-go-protocols => ../reticulum-go-protocols
```

Copy the `replace` lines for `git.quad4.io/reticulum/Reticulum-Go` and the other `quad4/*` modules from this repo's go.mod, or set:

```bash
export GOPRIVATE=quad4/*
```

## Quick start

Make and Task wrap the same scripts. Plain `go` works if you keep vendor flags:

```bash
export GOFLAGS=-mod=vendor
export GOPROXY=off
export GOSUMDB=off
```

```bash
make help
make test-short
make gorrcd
./bin/gorrcd-$(go env GOOS)-$(go env GOARCH)
```

```bash
task --list
task test-short
task gorrcd
```

```bash
go test -short -count=1 ./...
sh scripts/ci/build-gorrcd.sh "$(go env GOOS)" "$(go env GOARCH)" bin
```

| Action | Make | Task | Plain |
| --- | --- | --- | --- |
| Short tests | `make test-short` | `task test-short` | `go test -short -count=1 ./...` |
| All tests | `make test` | `task test` | `go test -count=1 ./...` |
| Vet / fmt | `make vet` / `make fmt` | `task vet` / `task fmt` | `go vet ./...` / `go fmt ./...` |
| Check | `make check` | `task check` | vet + short tests |
| gorrcd | `make gorrcd` | `task gorrcd` | `sh scripts/ci/build-gorrcd.sh …` |
| golxmd | `make golxmd` | `task golxmd` | `sh scripts/ci/build-golxmd.sh …` |
| Shared libs | `make libs` | `task build-lib*` | `sh scripts/ci/build-lib*.sh …` |

Cross-compile and install:

```bash
make gorrcd GOOS=linux GOARCH=arm64
make install PREFIX=$HOME/.local
```

Package APIs, examples, interop notes, and the full command matrix: [docs/en/building.md](docs/en/building.md).

## License

[Reticulum License](LICENSE). Copyright 2026 Quad4.io. Portions derived from the Reticulum Network Stack reference implementation are Copyright (c) 2016-2026 Mark Qvist. See [LEGAL.md](LEGAL.md) for the license lineage.
