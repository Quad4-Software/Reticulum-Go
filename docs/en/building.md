# Building and testing

This repo vendors modules and sets `GOFLAGS=-mod=vendor` in Make and Task. Prefer those wrappers, or export the same flags for plain `go`.

```bash
export GOFLAGS=-mod=vendor
export GOPROXY=off
export GOSUMDB=off
```

If your Task binary is named `go-task`:

```bash
alias task='go-task'
```

## Everyday targets

| Action | Make | Task | Plain go |
| --- | --- | --- | --- |
| Format | `make fmt` | `task fmt` | `go fmt ./...` |
| Vet | `make vet` | `task vet` | `go vet ./...` |
| Short tests | `make test-short` | `task test-short` | `go test -short -count=1 ./...` |
| All tests | `make test` | `task test` | `go test -count=1 ./...` |
| Check | `make check` | `task check` | vet + short tests (Task also runs revive and gosec) |
| Clean | `make clean` | `task clean` | `go clean` and remove `bin/` artifacts |

```bash
make help
task --list
```

## Daemons and libraries

| Action | Make | Task | Manual |
| --- | --- | --- | --- |
| gorrcd | `make gorrcd` | `task gorrcd` | `sh scripts/ci/build-gorrcd.sh "$(go env GOOS)" "$(go env GOARCH)" bin` |
| golxmd | `make golxmd` | `task golxmd` | `sh scripts/ci/build-golxmd.sh "$(go env GOOS)" "$(go env GOARCH)" bin` |
| Both daemons | `make daemons` | | same as above for each |
| librrc | `make librrc` | `task build-librrc` | `sh scripts/ci/build-librrc.sh "$(go env GOOS)" "$(go env GOARCH)" bin` |
| libmf | `make libmf` | `task build-libmf` | `sh scripts/ci/build-libmf.sh "$(go env GOOS)" "$(go env GOARCH)" bin` |
| liblxmf | `make liblxmf` | `task build-liblxmf` | `sh scripts/ci/build-liblxmf.sh "$(go env GOOS)" "$(go env GOARCH)" bin` |
| liblxst | `make liblxst` | `task build-liblxst` | `sh scripts/ci/build-liblxst.sh "$(go env GOOS)" "$(go env GOARCH)" bin` |
| All libs | `make libs` | | |
| Install daemons | `make install` | | copies into `PREFIX` (default `/usr/local`) |
| Install libs | `make install-libs` | | shared libs + headers |

Cross-compile with Make:

```bash
make gorrcd GOOS=linux GOARCH=arm64
make install-golxmd PREFIX=$HOME/.local
```

Run a built binary:

```bash
./bin/gorrcd-$(go env GOOS)-$(go env GOARCH)
./bin/golxmd-$(go env GOOS)-$(go env GOARCH)
```

## Package tests

| Package | Task | Plain go |
| --- | --- | --- |
| mf two-way | `task test:messenger` | `go test -v -run TestMessenger_TwoWayCommunication ./pkg/mf/...` |
| lxmf | `task test:lxmf` | `go test -v -count=1 ./pkg/lxmf/...` |
| lxmf interop | `task test:lxmf:interop` | `go test -v -count=1 -timeout 15m -run TestInterop ./pkg/lxmf/...` |
| rrc | `task test:rrc` | `go test -v -count=1 ./pkg/rrc/...` |
| rrc interop | `task test:rrc:interop` | needs RRC-ref + uv |
| lxst | `task test:lxst` | `go test -v -count=1 ./pkg/lxst/...` |
| lxst interop | `task test:lxst:interop` | needs `pip install lxst==0.5.4` |
| rnv | `task test:rnv` | `go test -v -count=1 ./pkg/rnv/...` |
| rnv short | `task test:rnv:short` | `go test -short -v -count=1 ./pkg/rnv/...` |
| rnv live | `task test:rnv:live` | live UDP + optional Python hub |
| Live Go mesh | `task test:live` | hub/client mesh over UDP |
| Race | `task test-race` | `go test -race -v ./...` |
| Coverage | `task coverage` | writes `coverage.out` |
| Bench | `task bench` | `go test -bench=. -benchmem -count=3 -run=^$ ./pkg/...` |
| Fuzz | `task fuzz` | `bash scripts/run-fuzz.sh` |

Optional tools used by Task only: revive (`task lint`), gosec (`task scan`).

## Examples

```bash
task example            # examples/example.go
task example:messenger  # examples/messenger.go
task example:lxmf
task example:lxst
task example:lxst:call

go run ./examples/example.go
go run ./examples/messenger.go
```

## Dependencies

```bash
task deps       # refresh vendor/
task mod-tidy
task mod-verify
make test       # uses vendor via GOFLAGS
```

## Bindings and codec

```bash
task build-liblxst
task test-codec
task test-bindings
task test-c
task test-cpp
task test-python
task test-rust
task test-java
task test-lua
```

## Phone CLI (CGO)

```bash
task rnphone
./bin/rnphone-$(go env GOOS)-$(go env GOARCH)
```
