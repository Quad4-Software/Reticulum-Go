# Reticulum-Go

Go implementation of the [Reticulum Network Stack](https://github.com/markqvist/Reticulum). It strengthens existing networks and brings Reticulum to more devices. It is not a replacement for the Python reference.

Available on rngit: NomadNet node `132f67e79d9b24aad014e93015fb858f:/page/index.mu`

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
- Native OS sandbox
- Vendored dependencies for offline source builds

Status detail: [docs/en/overview.md](docs/en/overview.md) and [COMPATIBILITY.md](COMPATIBILITY.md).

## Install / Build from source

Requires Go 1.27.1 or later. Dependencies are vendored (`GOFLAGS=-mod=vendor`).

Install a release binary (and optional init units):

```bash
curl -fsSL https://raw.githubusercontent.com/Quad4-Software/Reticulum-Go/master/install.sh | sh
curl -fsSL https://raw.githubusercontent.com/Quad4-Software/Reticulum-Go/master/install.sh | sh -s -- --source
```

From a git checkout, use Make, Task, or plain Go:

```bash
# Make
make build
make install                 # PREFIX=/usr/local (binary, tool symlinks, man pages)
make install-service         # INIT=auto|systemd|openrc|runit|dinit|all
make test

# Task (go-task on some distros: alias task='go-task')
task build
task install
task test

# Manual
mkdir -p bin
CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/reticulum-go ./cmd/reticulum-go
CGO_ENABLED=0 go install -ldflags="-s -w" ./cmd/reticulum-go
go test -v ./...
go run ./cmd/reticulum-go
```

Packages: `make package-deb`, `make package-rpm`, `make package-arch` (output in `dist/`). Arch/CachyOS: [quad4-arch](https://github.com/Quad4-Software/quad4-arch).

More targets, cross-compiles, WASM, and librns: [docs/en/getting-started.md](docs/en/getting-started.md) and [docs/en/development-and-testing.md](docs/en/development-and-testing.md).

## Docs

- English docs: [docs/en/](docs/en/README.md)
- Getting started: [docs/en/getting-started.md](docs/en/getting-started.md)
- API and examples: [docs/en/api-reference.md](docs/en/api-reference.md), [docs/en/examples.md](docs/en/examples.md)
- Reticulum manual (protocol/API authority): https://reticulum.network/manual/reference.html

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Branch from `dev`. Prefer `make check` / `task check` before opening a PR.

## Security

Report issues privately (see [SECURITY.md](SECURITY.md)). Sandbox is on by default (`enable_sandbox = no` to disable). Release assets carry cosign attestations (`cosign.pub`).

## License

Apache License 2.0. See [LICENSE](LICENSE).

Credit: [Mark Qvist](https://github.com/markqvist) for the reference Reticulum Network Stack.
