#!/bin/sh
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

if [ -d "$HOME/go-sdk/bin" ]; then
	PATH="$HOME/go-sdk/bin:$PATH"
	export PATH
fi

export CGO_ENABLED="${CGO_ENABLED:-1}"
export GOFLAGS="${GOFLAGS:--mod=vendor}"
export GOPROXY="${GOPROXY:-off}"
export GOSUMDB="${GOSUMDB:-off}"
export CGO_CFLAGS="${CGO_CFLAGS:--I${ROOT}/third_party/opus/include -I${ROOT}/third_party/miniaudio -O3}"

if [ -z "${CGO_LDFLAGS:-}" ]; then
	case "$(uname -s)" in
		Linux)
			if cc -dumpmachine 2>/dev/null | grep -q musl; then
				CGO_LDFLAGS="-lm -lpthread"
			else
				CGO_LDFLAGS="-lm -lpthread -ldl"
			fi
			;;
		MINGW*|MSYS*|CYGWIN*)
			CGO_LDFLAGS="-lm"
			export CMAKE_GENERATOR="${CMAKE_GENERATOR:-Ninja}"
			export CC="${CC:-gcc}"
			;;
		*)
			CGO_LDFLAGS="-lm -lpthread"
			;;
	esac
	export CGO_LDFLAGS
fi

sh scripts/vendor-sync.sh

test_exec=""
if [ "${GOARCH:-}" = "arm" ] && command -v qemu-arm-static >/dev/null 2>&1; then
	test_exec="-exec qemu-arm-static"
	if [ -z "${QEMU_LD_PREFIX:-}" ] && [ -d /usr/arm-linux-gnueabihf ]; then
		export QEMU_LD_PREFIX=/usr/arm-linux-gnueabihf
	fi
fi

# shellcheck disable=SC2086
go test -tags lxst_native -count=1 -short $test_exec \
	./pkg/audio/... \
	./pkg/sandbox/ \
	./pkg/proto/ \
	./pkg/media/ \
	./pkg/call/ \
	./pkg/phonebook/ \
	./pkg/history/ \
	./pkg/rnsnode/ \
	./cmd/rgesp-dial/ \
	./cmd/rnphone/

mkdir -p bin
go build -tags lxst_native -o bin/rgesp-dial ./cmd/rgesp-dial
go build -tags lxst_native -o bin/rnphone ./cmd/rnphone
if [ -z "$test_exec" ]; then
	./bin/rnphone -version
else
	qemu-arm-static ./bin/rnphone -version
fi
