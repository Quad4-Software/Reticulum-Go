# Reticulum-Go

Go implementation of the [Reticulum Network Stack](https://github.com/markqvist/Reticulum). It strengthens existing networks and brings Reticulum to more devices without needing the pure cryptographic primitives. It is not a replacement for the Python reference.

Important: This project is still considered experimental, may not always be up to date with the Python reference, and not ready to be reliably used, we do welcome testers and feedback.

NomadNet Node (rngit): `132f67e79d9b24aad014e93015fb858f:/page/index.mu`

```bash
git clone rns://06a54b505bb67b25ef3f8097e8001edc/public/Reticulum-Go
```

## Features

- Wire-compatible with Python RNS (links, resources, channels, IFAC, transport)
- Portable cross-platform and 32-bit support
- Single reticulum-go binary (daemon plus status, id, probe, path, cp, and related tools)
- Static, portable builds including legacy Windows via go-legacy-win7 / go-legacy-winxp
- Interfaces: UDP, TCP, Auto, I2P, Backbone, Pipe, Local, Serial, Modem73, SDR, WebSocket, QUIC, WebTransport, DNS rendezvous, VSOCK, HTTPS, and RNode.
- WebAssembly, librns C ABI, and language bindings (see [docs](docs/en/))
- Native OS sandboxing
- Vendored dependencies for offline source builds

Status detail: [docs/en/overview.md](docs/en/overview.md) and [COMPATIBILITY.md](COMPATIBILITY.md).

## Install / Build from source

Requires Go 1.27.4 or later. Dependencies are vendored (`GOFLAGS=-mod=vendor`).

```bash
git clone https://git.quad4.io/reticulum/Reticulum-Go.git
cd Reticulum-Go
make build                   # bin/reticulum-go plus tool symlinks
make install                 # PREFIX=/usr/local (binary, tool symlinks, man pages)
```

Or install straight into your Go binary directory without a checkout:

```bash
go install git.quad4.io/reticulum/Reticulum-Go/cmd/reticulum-go@master
```

`@latest` resolves once a release tag carrying the new module path is published (v1.6.0 onward).

From a git checkout, use Make or plain Go:

```bash
# Make
make build
make install                 # PREFIX=/usr/local (binary, tool symlinks, man pages)
make install-service         # INIT=auto|systemd|openrc|runit|dinit|all
make test

# Manual
mkdir -p bin
CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/reticulum-go ./cmd/reticulum-go
CGO_ENABLED=0 go install -ldflags="-s -w" ./cmd/reticulum-go
go test -v ./...
go run ./cmd/reticulum-go
```

Packages: `make package-deb`, `make package-rpm`, `make package-arch` (output in `dist/`). Arch/CachyOS: [arch](https://github.com/Quad4-Software/arch).

More targets, cross-compiles, WASM, and librns: [docs/en/getting-started.md](docs/en/getting-started.md) and [docs/en/development-and-testing.md](docs/en/development-and-testing.md).

## Repository layout

This repository holds two Go modules:

| Path | Module | Contents |
|------|--------|----------|
| `/` | `git.quad4.io/reticulum/Reticulum-Go` | The RNS stack, CLI utilities, librns |
| `protocols/` | `github.com/Quad4-Software/reticulum-go-protocols` | LXMF, LXST, MF, RRC and their bindings and daemons |

The nested module points at the root module with a `replace` directive, so local builds use the code in this tree. Build and test it with `task protocols-build` and `task protocols-test`.

## Docs

The documentation is Markdown under `docs/en/`. The website that renders it is maintained outside this repository.

- English docs: [docs/en/](docs/en/README.md)
- Getting started: [docs/en/getting-started.md](docs/en/getting-started.md)
- API and examples: [docs/en/api-reference.md](docs/en/api-reference.md), [docs/en/examples.md](docs/en/examples.md)
- Reticulum manual (protocol/API authority): https://reticulum.network/manual/reference.html

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Branch from `dev`. Run `make check` before opening a PR.

## Security

Report issues privately (see [SECURITY.md](SECURITY.md)). Sandbox is on by default (`enable_sandbox = no` to disable). Release assets carry cosign attestations (`cosign.pub`).

## License

[Reticulum License](LICENSE), the same license as the Python reference implementation since RNS 0.9.4. Earlier revisions carried MIT, then 0BSD, then Apache-2.0. [LEGAL.md](LEGAL.md) records the boundary commits.

## Upstream

[Python RNS](https://github.com/markqvist/Reticulum) by [Mark Qvist](https://github.com/markqvist) is the reference implementation and defines the protocol. If you are new to Reticulum, start there. 75% of donations to this project go to Mark Qvist.
