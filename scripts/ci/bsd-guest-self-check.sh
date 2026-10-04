#!/bin/sh
# Guest-side BSD self-check for vmactions CI and local anyvm.
# Expects go on PATH and the repository as cwd.
set -eu

if [ -x /usr/local/go/bin/go ]; then
	PATH="/usr/local/go/bin:$PATH"
	export PATH
fi

export GOFLAGS="${GOFLAGS:--mod=vendor}"
export GOPROXY="${GOPROXY:-off}"
export GOTOOLCHAIN="${GOTOOLCHAIN:-local}"
export CGO_ENABLED="${CGO_ENABLED:-0}"

go version
go test -buildvcs=false -short -count=1 -timeout 10m ./pkg/selfcheck/ ./pkg/sandbox/ ./pkg/protect/
go test -buildvcs=false -short -count=1 -timeout 10m ./pkg/transport/ -run 'Protect|HandlePacket'
mkdir -p bin
go build -buildvcs=false -ldflags="-s -w" -o bin/reticulum-go ./cmd/reticulum-go
./bin/reticulum-go self-check --binary "$(pwd)/bin/reticulum-go" --json --full
