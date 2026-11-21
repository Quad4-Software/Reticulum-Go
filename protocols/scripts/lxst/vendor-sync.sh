#!/bin/sh
set -eu

RETICULUM_ROOT="${1:-../Reticulum/Reticulum-Go}"
LIBS_ROOT="${2:-../Reticulum-Go-Projects}"
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

if [ "${CI:-}" != "true" ] && [ "${GOPROXY:-}" != "off" ]; then
	go mod tidy
	go mod vendor
else
	echo "skipping go mod tidy and vendor"
fi


OPUS_VERSION="1.6.1"
MINIAUDIO_VERSION="0.11.25"
CODEC2_VERSION="1.2.0"

fetch_archive() {
	url="$1"
	dest="$2"
	if [ -d "$dest" ]; then
		return 0
	fi
	tmp="$(mktemp -d)"
	trap 'rm -rf "$tmp"' EXIT INT TERM
	curl -fsSL "$url" -o "$tmp/archive"
	case "$url" in
		*.tar.gz) tar -xzf "$tmp/archive" -C "$tmp" ;;
		*.zip) unzip -q "$tmp/archive" -d "$tmp" ;;
		*) echo "unsupported archive: $url" >&2; exit 1 ;;
	esac
	mkdir -p "$(dirname "$dest")"
	found=""
	for d in "$tmp"/*; do
		if [ -d "$d" ]; then
			found="$d"
			break
		fi
	done
	if [ -z "$found" ]; then
		echo "empty archive: $url" >&2
		exit 1
	fi
	mv "$found" "$dest"
	trap - EXIT INT TERM
	rm -rf "$tmp"
}

cmake_native() {
	src="$1"
	build="$2"
	prefix="$3"
	extra="$4"
	mkdir -p "$build"
	cd "$build"
	compiler="${CC:-}"
	if [ -n "$compiler" ] && [ "${compiler#* }" != "$compiler" ]; then
		wrap="$ROOT/third_party/.cc-wrap"
		printf '#!/bin/sh\nexec %s "$@"\n' "$compiler" > "$wrap"
		chmod +x "$wrap"
		compiler="$wrap"
	fi
	set -- \
		-DCMAKE_BUILD_TYPE=Release \
		-DBUILD_SHARED_LIBS=OFF \
		-DCMAKE_INSTALL_PREFIX="$prefix"
	if [ -n "${compiler:-}" ]; then
		set -- "$@" "-DCMAKE_C_COMPILER=$compiler"
	fi
	if [ -n "${CFLAGS:-}" ]; then
		set -- "$@" "-DCMAKE_C_FLAGS=${CFLAGS}"
	fi
	if [ -n "${CMAKE_SYSTEM_NAME:-}" ]; then
		set -- "$@" "-DCMAKE_SYSTEM_NAME=$CMAKE_SYSTEM_NAME" -DCMAKE_C_COMPILER_WORKS=1
		set -- "$@" "-DCMAKE_TRY_COMPILE_TARGET_TYPE=STATIC_LIBRARY"
	fi
	if [ -n "${CMAKE_SYSTEM_PROCESSOR:-}" ]; then
		set -- "$@" "-DCMAKE_SYSTEM_PROCESSOR=$CMAKE_SYSTEM_PROCESSOR"
	fi
	# shellcheck disable=SC2086
	cmake "$src" "$@" $extra
	cmake --build . --parallel "$(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 2)"
	cmake --install .
	cd "$ROOT"
}

if [ "${RGESP_REBUILD_LIBS:-}" = "1" ]; then
	rm -rf third_party/opus/build third_party/opus/install third_party/opus/lib
	rm -rf third_party/codec2/build third_party/codec2/install third_party/codec2/lib
fi

fetch_archive \
	"https://github.com/xiph/opus/archive/refs/tags/v${OPUS_VERSION}.tar.gz" \
	"third_party/opus-src"

if [ ! -f third_party/opus/lib/libopus.a ]; then
	rm -rf third_party/opus/build third_party/opus/install
	cp third_party/opus/config.h third_party/opus-src/config.h 2>/dev/null || true
	cmake_native \
		"$ROOT/third_party/opus-src" \
		"$ROOT/third_party/opus/build" \
		"$ROOT/third_party/opus/install" \
		"-DOPUS_BUILD_TESTING=OFF -DOPUS_BUILD_PROGRAMS=OFF"
	mkdir -p third_party/opus/lib third_party/opus/include
	cp third_party/opus/install/lib/libopus.a third_party/opus/lib/
	cp -R third_party/opus/install/include/* third_party/opus/include/
fi

fetch_archive \
	"https://github.com/mackron/miniaudio/archive/refs/tags/${MINIAUDIO_VERSION}.tar.gz" \
	"third_party/miniaudio-src"

if [ ! -f third_party/miniaudio/miniaudio.h ]; then
	mkdir -p third_party/miniaudio
	cp "third_party/miniaudio-src/miniaudio.h" third_party/miniaudio/
fi

fetch_archive \
	"https://github.com/drowe67/codec2/archive/refs/tags/${CODEC2_VERSION}.tar.gz" \
	"third_party/codec2-src"

if [ ! -f third_party/codec2/lib/libcodec2.a ]; then
	rm -rf third_party/codec2/build third_party/codec2/install
	cmake_native \
		"$ROOT/third_party/codec2-src" \
		"$ROOT/third_party/codec2/build" \
		"$ROOT/third_party/codec2/install" \
		"-DUNITTEST=OFF -DINSTALL_EXAMPLES=OFF"
	mkdir -p third_party/codec2/lib third_party/codec2/include
	if [ -f third_party/codec2/install/lib/libcodec2.a ]; then
		cp third_party/codec2/install/lib/libcodec2.a third_party/codec2/lib/
	elif [ -f third_party/codec2/install/lib64/libcodec2.a ]; then
		cp third_party/codec2/install/lib64/libcodec2.a third_party/codec2/lib/
	else
		echo "libcodec2.a missing after install" >&2
		exit 1
	fi
	if [ -d third_party/codec2/install/include/codec2 ]; then
		cp -R third_party/codec2/install/include/codec2 third_party/codec2/include/
	else
		cp -R third_party/codec2/install/include/* third_party/codec2/include/
	fi
fi

echo "vendor sync complete"
