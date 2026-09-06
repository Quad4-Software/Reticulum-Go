#!/bin/sh
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io
#
# Install reticulum-go from a GitHub release binary or by building source.
#
# Curl:
#   curl -fsSL https://raw.githubusercontent.com/Quad4-Software/Reticulum-Go/master/install.sh | sh
#   curl -fsSL https://raw.githubusercontent.com/Quad4-Software/Reticulum-Go/master/install.sh | sh -s -- --dry-run
#   curl -fsSL https://raw.githubusercontent.com/Quad4-Software/Reticulum-Go/master/install.sh | sh -s -- --source
#
# Checkout:
#   ./install.sh
#   ./install.sh --dry-run
#   ./install.sh --source --init systemd
#
# shellcheck shell=sh

set -eu

REPO="${RGO_REPO:-Quad4-Software/Reticulum-Go}"
GITHUB="${RGO_GITHUB:-https://github.com}"
GITHUB_API="${RGO_GITHUB_API:-https://api.github.com}"
GITHUB_RAW="${RGO_GITHUB_RAW:-https://raw.githubusercontent.com}"
MIN_GO="${RGO_MIN_GO:-1.27.1}"
BINARY_NAME="reticulum-go"
STATE_DIR="/var/lib/reticulum-go"
TOOL_LINKS="rgostatus rgoid rgoprobe rgopath rgocp rgox rnx rgosh rgopageserver rgoslow rgoselfcheck rgospeed rgodump rgosnap rgozen"

PREFIX="${PREFIX:-/usr/local}"
DESTDIR="${DESTDIR:-}"
BINDIR="${BINDIR:-}"
INIT="${INIT:-auto}"
TAG="${RGO_VERSION:-${VERSION:-latest}}"
METHOD="${RGO_METHOD:-}"
SERVICE="${RGO_SERVICE:-auto}"
GOAMD64_REQ="${RGO_GOAMD64:-auto}"
VERIFY="${RGO_VERIFY:-auto}"
DRY_RUN=0
ASSUME_YES=0
FORCE=0
PREFIX_SET=0
BINDIR_SET=0
SUDO=""
WORK_DIR=""
CHECKOUT_ROOT=""

OS=""
ARCH=""
GOAMD64="v1"
V3_OK=0
INIT_DETECTED="unknown"
ASSET=""
SRC_ROOT=""

log() {
	printf 'install.sh: %s\n' "$*"
}

err() {
	printf 'install.sh: %s\n' "$*" >&2
}

die() {
	err "$*"
	exit 1
}

usage() {
	cat <<'EOF'
Usage: install.sh [options]

Install reticulum-go (release binary or build from source) and optional
init service files (systemd, OpenRC, runit, or dinit).

Options:
  --binary            Fetch a pre-built GitHub release binary
  --source            Build from source (needs Go 1.27.1 or later)
  --dry-run, -n       Print detections and planned actions, write nothing
  --yes, -y           Non-interactive defaults (binary, install service)
  --no-service        Skip init service files
  --service           Install service files for the detected init
  --init TYPE         auto|systemd|openrc|runit|dinit|all|none
  --prefix DIR        Install prefix (default /usr/local, or PREFIX)
  --destdir DIR       Staging root (default empty, or DESTDIR)
  --bindir DIR        Binary directory (default PREFIX/bin, or BINDIR)
  --tag TAG           Release tag (default latest, or RGO_VERSION)
  --goamd64 LEVEL     auto|v1|v3 (linux/amd64 microarchitecture)
  --verify            Require cosign attestation (needs cosign)
  --no-verify         Skip cosign even if it is installed
  --force             Allow a v3 binary on a CPU that does not report v3
  -h, --help          Show this help

Environment:
  PREFIX DESTDIR BINDIR INIT VERSION RGO_VERSION RGO_REPO RGO_METHOD
  RGO_SERVICE RGO_GOAMD64 RGO_VERIFY RGO_GITHUB RGO_GITHUB_API

Examples:
  curl -fsSL https://raw.githubusercontent.com/Quad4-Software/Reticulum-Go/master/install.sh | sh
  curl -fsSL https://raw.githubusercontent.com/Quad4-Software/Reticulum-Go/master/install.sh | sh -s -- --dry-run
  ./install.sh --source --init runit
EOF
}

cleanup() {
	if [ -n "$WORK_DIR" ] && [ -d "$WORK_DIR" ]; then
		rm -rf "$WORK_DIR"
	fi
}

trap cleanup EXIT INT HUP TERM

is_checkout() {
	root="$1"
	[ -n "$root" ] || return 1
	[ -f "$root/go.mod" ] || return 1
	[ -d "$root/cmd/reticulum-go" ] || return 1
	[ -d "$root/packaging" ] || return 1
}

resolve_checkout() {
	script="$0"
	case "$script" in
	-|sh|dash|bash|ksh|*/sh|*/dash|*/bash)
		return 1
		;;
	esac
	if [ ! -f "$script" ]; then
		return 1
	fi
	dir=$(CDPATH='' cd -- "$(dirname "$script")" && pwd)
	if is_checkout "$dir"; then
		CHECKOUT_ROOT="$dir"
		return 0
	fi
	return 1
}

has_tty() {
	[ -r /dev/tty ] && [ -w /dev/tty ]
}

prompt() {
	msg="$1"
	def="$2"
	if [ "$ASSUME_YES" = 1 ] || [ "$DRY_RUN" = 1 ] || ! has_tty; then
		printf '%s\n' "$def"
		return 0
	fi
	printf '%s' "$msg" >/dev/tty
	ans=""
	read -r ans </dev/tty || ans="$def"
	if [ -z "$ans" ]; then
		ans="$def"
	fi
	printf '%s\n' "$ans"
}

need_cmd() {
	command -v "$1" >/dev/null 2>&1 || die "need '$1' on PATH"
}

have_cmd() {
	command -v "$1" >/dev/null 2>&1
}

make_work_dir() {
	if [ -n "$WORK_DIR" ]; then
		return 0
	fi
	if have_cmd mktemp; then
		WORK_DIR=$(mktemp -d "${TMPDIR:-/tmp}/rgo-install.XXXXXX")
	else
		WORK_DIR="${TMPDIR:-/tmp}/rgo-install.$$"
		mkdir -m 700 "$WORK_DIR"
	fi
}

as_root() {
	if [ "$DRY_RUN" = 1 ]; then
		if [ -n "$SUDO" ]; then
			printf 'install.sh: dry-run: %s' "$SUDO"
			for a in "$@"; do
				printf ' %s' "$a"
			done
			printf '\n'
		else
			printf 'install.sh: dry-run:'
			for a in "$@"; do
				printf ' %s' "$a"
			done
			printf '\n'
		fi
		return 0
	fi
	if [ -n "$SUDO" ]; then
		sudo "$@"
	else
		"$@"
	fi
}

http_get() {
	url="$1"
	dest="$2"
	if have_cmd curl; then
		curl -fL --proto '=https' --tlsv1.2 --retry 3 --retry-delay 1 -o "$dest" "$url" || return 1
	elif have_cmd wget; then
		wget -O "$dest" "$url" || return 1
	else
		die "need curl or wget to download $url"
	fi
}

http_effective_url() {
	url="$1"
	if have_cmd curl; then
		curl -fsSL --proto '=https' --tlsv1.2 -o /dev/null -w '%{url_effective}' "$url" || return 1
	else
		return 1
	fi
}

flag_in() {
	needle=$(LC_ALL=C printf '%s' "$1" | tr 'A-Z' 'a-z')
	hay=$(LC_ALL=C printf '%s' "$2" | tr 'A-Z' 'a-z' | tr ',' ' ')
	case " $hay " in
	*" $needle "*) return 0 ;;
	*) return 1 ;;
	esac
}

cpu_is_goamd64_v3() {
	flags="$1"
	for f in avx avx2 bmi1 bmi2 fma movbe; do
		flag_in "$f" "$flags" || return 1
	done
	if ! flag_in osxsave "$flags" && ! flag_in xsave "$flags"; then
		return 1
	fi
	if flag_in lzcnt "$flags" || flag_in abm "$flags"; then
		return 0
	fi
	return 1
}

linux_cpu_flags() {
	if [ -r /proc/cpuinfo ]; then
		awk '/^flags[[:space:]]*:/ { sub(/^[^:]*:[[:space:]]*/, ""); print; exit }' /proc/cpuinfo
	fi
}

bsd_cpu_flags() {
	if [ -r /var/run/dmesg.boot ]; then
		awk '
			BEGIN { out = "" }
			/Features/ {
				line = $0
				gsub(/[<>(),=]/, " ", line)
				out = out " " line
			}
			END { print out }
		' /var/run/dmesg.boot
		return 0
	fi
	if have_cmd dmesg; then
		dmesg 2>/dev/null | awk '
			BEGIN { out = "" }
			/Features/ || /AVX/ {
				line = $0
				gsub(/[<>(),=]/, " ", line)
				out = out " " line
			}
			END { print out }
		'
	fi
}

detect_v3() {
	V3_OK=0
	flags=""
	case "$OS" in
	linux)
		flags=$(linux_cpu_flags)
		;;
	darwin)
		feat=$(sysctl -n machdep.cpu.features 2>/dev/null || true)
		leaf7=$(sysctl -n machdep.cpu.leaf7_features 2>/dev/null || true)
		ext=$(sysctl -n machdep.cpu.extfeatures 2>/dev/null || true)
		flags="$feat $leaf7 $ext"
		;;
	*)
		flags=$(bsd_cpu_flags)
		;;
	esac
	if [ "$ARCH" = amd64 ] && cpu_is_goamd64_v3 "$flags"; then
		V3_OK=1
		return 0
	fi
	return 1
}

detect_os() {
	sys=$(uname -s)
	case "$sys" in
	Linux) OS=linux ;;
	Darwin) OS=darwin ;;
	FreeBSD) OS=freebsd ;;
	OpenBSD) OS=openbsd ;;
	NetBSD) OS=netbsd ;;
	DragonFly) OS=dragonfly ;;
	AIX) OS=aix ;;
	SunOS)
		if [ -f /etc/os-release ] && grep -q 'ID=illumos' /etc/os-release; then
			OS=illumos
		elif [ -f /etc/release ] && grep -qi illumos /etc/release; then
			OS=illumos
		else
			OS=solaris
		fi
		;;
	*)
		die "unsupported OS: $sys"
		;;
	esac
}

detect_arch() {
	machine=$(uname -m)
	case "$machine" in
	x86_64 | amd64) ARCH=amd64 ;;
	i386 | i486 | i586 | i686 | i86pc) ARCH=386 ;;
	aarch64 | arm64) ARCH=arm64 ;;
	armv6* | armv7* | arm) ARCH=arm ;;
	riscv64) ARCH=riscv64 ;;
	ppc64le | powerpc64le) ARCH=ppc64le ;;
	ppc64 | powerpc64) ARCH=ppc64 ;;
	mips64el | mips64le) ARCH=mips64le ;;
	mips64) ARCH=mips64 ;;
	mipsel | mipsle) ARCH=mipsle ;;
	mips) ARCH=mips ;;
	s390x) ARCH=s390x ;;
	*) die "unsupported architecture: $machine" ;;
	esac

	if [ "$OS" = darwin ] && [ "$(sysctl -n sysctl.proc_translated 2>/dev/null || echo 0)" = 1 ]; then
		ARCH=arm64
	fi

	bits=$(getconf LONG_BIT 2>/dev/null || echo 64)
	if [ "$bits" = 32 ]; then
		case "$ARCH" in
		amd64) ARCH=386 ;;
		arm64) ARCH=arm ;;
		esac
	fi
}

detect_init() {
	if [ -d /run/systemd/system ] && have_cmd systemctl; then
		INIT_DETECTED=systemd
		return
	fi
	if have_cmd rc-service || have_cmd openrc; then
		INIT_DETECTED=openrc
		return
	fi
	if [ -d /etc/sv ] || [ -d /var/service ] || have_cmd runsv; then
		INIT_DETECTED=runit
		return
	fi
	if have_cmd dinitctl || [ -d /etc/dinit.d ]; then
		INIT_DETECTED=dinit
		return
	fi
	INIT_DETECTED=unknown
}

asset_name() {
	if [ "$OS" = linux ] && [ "$ARCH" = amd64 ]; then
		if [ "$GOAMD64" = v3 ]; then
			printf '%s\n' "reticulum-go-linux-amd64-v3"
			return
		fi
		printf '%s\n' "reticulum-go-linux-amd64"
		return
	fi
	if [ "$OS" = linux ] && [ "$ARCH" = 386 ]; then
		printf '%s\n' "reticulum-go-linux-i686"
		return
	fi
	printf '%s\n' "reticulum-go-${OS}-${ARCH}"
}

release_url() {
	name="$1"
	if [ "$TAG" = latest ]; then
		printf '%s/%s/releases/latest/download/%s\n' "$GITHUB" "$REPO" "$name"
	else
		printf '%s/%s/releases/download/%s/%s\n' "$GITHUB" "$REPO" "$TAG" "$name"
	fi
}

raw_url() {
	ref="$1"
	path="$2"
	printf '%s/%s/%s/%s\n' "$GITHUB_RAW" "$REPO" "$ref" "$path"
}

archive_url() {
	if [ "$TAG" = latest ] || [ "$TAG" = master ]; then
		printf '%s/%s/archive/refs/heads/master.tar.gz\n' "$GITHUB" "$REPO"
	else
		printf '%s/%s/archive/refs/tags/%s.tar.gz\n' "$GITHUB" "$REPO" "$TAG"
	fi
}

resolve_git_ref() {
	if [ "$TAG" != latest ]; then
		printf '%s\n' "$TAG"
		return 0
	fi
	eff=$(http_effective_url "$GITHUB/$REPO/releases/latest" || true)
	if [ -n "$eff" ]; then
		printf '%s\n' "${eff##*/}"
		return 0
	fi
	if have_cmd curl; then
		json=$(curl -fsSL --proto '=https' --tlsv1.2 "$GITHUB_API/repos/$REPO/releases/latest" || true)
		tag=$(printf '%s\n' "$json" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -1)
		if [ -n "$tag" ]; then
			printf '%s\n' "$tag"
			return 0
		fi
	fi
	printf '%s\n' master
}

version_ge() {
	awk -v h="$1" -v n="$2" 'BEGIN {
		split(h, a, ".")
		split(n, b, ".")
		for (i = 1; i <= 3; i++) {
			ai = a[i] + 0
			bi = b[i] + 0
			if (ai > bi) exit 0
			if (ai < bi) exit 1
		}
		exit 0
	}'
}

check_go() {
	need_cmd go
	have=$(go env GOVERSION 2>/dev/null || true)
	have=${have#go}
	have=$(printf '%s' "$have" | sed 's/[^0-9.].*//')
	if [ -z "$have" ]; then
		die "could not read go version"
	fi
	if ! version_ge "$have" "$MIN_GO"; then
		die "Go $MIN_GO or later required (found $have)"
	fi
	log "go $have"
}

dir_writable() {
	dir="$1"
	if [ -d "$dir" ] && [ -w "$dir" ]; then
		return 0
	fi
	parent=$(dirname "$dir")
	if [ -d "$parent" ] && [ -w "$parent" ]; then
		return 0
	fi
	return 1
}

resolve_privilege() {
	target="${DESTDIR}${BINDIR}"
	if dir_writable "$target" || dir_writable "$(dirname "$target")"; then
		SUDO=""
		return 0
	fi
	if [ "$(id -u)" -eq 0 ]; then
		SUDO=""
		return 0
	fi
	if have_cmd sudo; then
		SUDO=sudo
		return 0
	fi
	if [ "$PREFIX_SET" = 1 ]; then
		die "cannot write to $target (need root, sudo, or a writable --prefix)"
	fi
	PREFIX="$HOME/.local"
	BINDIR="${PREFIX}/bin"
	log "no root and $target not writable, using prefix $PREFIX"
	if ! dir_writable "$BINDIR" && ! dir_writable "$PREFIX"; then
		die "cannot write to $BINDIR"
	fi
	SUDO=""
}

choose_method() {
	if [ -n "$METHOD" ]; then
		return 0
	fi
	if [ "$DRY_RUN" = 1 ] || [ "$ASSUME_YES" = 1 ] || ! has_tty; then
		METHOD=binary
		log "method=binary (default)"
		return 0
	fi
	log "Install method"
	log "  1) Fetch pre-built binary from GitHub releases"
	log "  2) Build from source (Go $MIN_GO or later)"
	choice=$(prompt "Select [1/2]: " "1")
	case "$choice" in
	2 | source | s)
		METHOD=source
		;;
	*)
		METHOD=binary
		;;
	esac
}

choose_service() {
	if [ "$SERVICE" = yes ] || [ "$SERVICE" = no ]; then
		return 0
	fi
	if [ "$INIT" = none ]; then
		SERVICE=no
		return 0
	fi
	if [ "$INIT" != auto ] && [ "$INIT" != none ]; then
		SERVICE=yes
		return 0
	fi
	if [ "$INIT_DETECTED" = unknown ]; then
		SERVICE=no
		log "no init system detected, skipping service"
		return 0
	fi
	if [ "$DRY_RUN" = 1 ] || [ "$ASSUME_YES" = 1 ] || ! has_tty; then
		SERVICE=yes
		return 0
	fi
	ans=$(prompt "Install $INIT_DETECTED service files? [Y/n] " "Y")
	case "$ans" in
	n | N | no | NO)
		SERVICE=no
		;;
	*)
		SERVICE=yes
		;;
	esac
}

resolve_goamd64() {
	GOAMD64=v1
	if [ "$OS" != linux ] || [ "$ARCH" != amd64 ]; then
		return 0
	fi
	case "$GOAMD64_REQ" in
	v3)
		if [ "$V3_OK" = 1 ] || [ "$FORCE" = 1 ]; then
			GOAMD64=v3
		else
			die "CPU does not report GOAMD64=v3 flags (AVX2 BMI1 BMI2 FMA LZCNT MOVBE OSXSAVE). use --goamd64 v1 or --force"
		fi
		;;
	v1)
		GOAMD64=v1
		;;
	auto | "")
		if [ "$V3_OK" = 1 ]; then
			GOAMD64=v3
		else
			GOAMD64=v1
		fi
		;;
	*)
		die "unknown --goamd64 '$GOAMD64_REQ' (auto|v1|v3)"
		;;
	esac
}

install_file() {
	mode="$1"
	src="$2"
	dest="$3"
	as_root mkdir -p "$(dirname "$dest")"
	if have_cmd install; then
		as_root install -m "$mode" "$src" "$dest"
	else
		as_root cp "$src" "$dest"
		as_root chmod "$mode" "$dest"
	fi
}

install_binary_and_links() {
	src="$1"
	dest="${DESTDIR}${BINDIR}/${BINARY_NAME}"
	log "install $dest"
	as_root mkdir -p "${DESTDIR}${BINDIR}"
	install_file 755 "$src" "$dest"
	for name in $TOOL_LINKS; do
		log "link ${DESTDIR}${BINDIR}/$name -> $BINARY_NAME"
		as_root ln -sf "$BINARY_NAME" "${DESTDIR}${BINDIR}/$name"
	done
}

install_man_from_tree() {
	root="$1"
	man_src="$root/man"
	if [ ! -d "$man_src" ]; then
		return 0
	fi
	mandir="${DESTDIR}${PREFIX}/share/man"
	log "install man pages under $mandir"
	as_root mkdir -p "$mandir/man1" "$mandir/man8"
	for f in "$man_src"/*.1; do
		[ -f "$f" ] || continue
		base=$(basename "$f")
		install_file 644 "$f" "$mandir/man1/$base"
	done
	if [ -f "$man_src/reticulum-go.8" ]; then
		install_file 644 "$man_src/reticulum-go.8" "$mandir/man8/reticulum-go.8"
	fi
	as_root ln -sf reticulum-go-status.1 "$mandir/man1/rgostatus.1"
	as_root ln -sf reticulum-go-id.1 "$mandir/man1/rgoid.1"
	as_root ln -sf reticulum-go-probe.1 "$mandir/man1/rgoprobe.1"
	as_root ln -sf reticulum-go-path.1 "$mandir/man1/rgopath.1"
	as_root ln -sf reticulum-go-cp.1 "$mandir/man1/rgocp.1"
	as_root ln -sf reticulum-go-x.1 "$mandir/man1/rgox.1"
	as_root ln -sf reticulum-go-x.1 "$mandir/man1/rnx.1"
	as_root ln -sf reticulum-go-sh.1 "$mandir/man1/rgosh.1"
	as_root ln -sf reticulum-go-pageserver.1 "$mandir/man1/rgopageserver.1"
	as_root ln -sf reticulum-go-slow.1 "$mandir/man1/rgoslow.1"
	as_root ln -sf reticulum-go-speedtest.1 "$mandir/man1/rgospeed.1"
	as_root ln -sf reticulum-go-dump.1 "$mandir/man1/rgodump.1"
	as_root ln -sf reticulum-go-snapshot.1 "$mandir/man1/rgosnap.1"
	as_root ln -sf reticulum-go-self-check.1 "$mandir/man1/rgoselfcheck.1"
}

fetch_packaging_tree() {
	dest="$1"
	ref="$2"
	log "fetch packaging files (ref $ref)"
	set -- \
		scripts/install-service.sh \
		packaging/reticulum-go.config \
		packaging/systemd/reticulum-go.service \
		packaging/systemd/user.conf.example \
		packaging/openrc/reticulum-go \
		packaging/runit/reticulum-go/run \
		packaging/runit/reticulum-go/log/run \
		packaging/dinit/reticulum-go
	for rel in "$@"; do
		out="$dest/$rel"
		mkdir -p "$(dirname "$out")"
		http_get "$(raw_url "$ref" "$rel")" "$out"
	done
	chmod 755 "$dest/scripts/install-service.sh"
}

run_install_service() {
	tree="$1"
	init_type="$2"
	if [ ! -f "$tree/scripts/install-service.sh" ]; then
		die "missing $tree/scripts/install-service.sh"
	fi
	if [ -n "$DESTDIR" ]; then
		as_root sh "$tree/scripts/install-service.sh" --prefix "$PREFIX" --destdir "$DESTDIR" --bindir "$BINDIR" --init "$init_type"
	else
		as_root sh "$tree/scripts/install-service.sh" --prefix "$PREFIX" --bindir "$BINDIR" --init "$init_type"
	fi
}

verify_blob() {
	blob="$1"
	bundle="$2"
	key="$3"
	if ! have_cmd cosign; then
		return 1
	fi
	cosign verify-blob-attestation \
		--private-infrastructure \
		--key "$key" \
		--bundle "$bundle" \
		--type slsaprovenance1 \
		"$blob"
}

maybe_verify() {
	blob="$1"
	name="$2"
	if [ "$VERIFY" = no ]; then
		log "cosign verification skipped (--no-verify)"
		return 0
	fi
	if [ "$VERIFY" = yes ] && ! have_cmd cosign; then
		die "cosign is required for --verify"
	fi
	if ! have_cmd cosign; then
		log "cosign not found, skipping attestation check (install cosign or pass --verify)"
		return 0
	fi
	make_work_dir
	bundle="$WORK_DIR/${name}.cosign.bundle"
	key="$WORK_DIR/cosign.pub"
	ref=$(resolve_git_ref)
	if ! http_get "$(release_url "${name}.cosign.bundle")" "$bundle"; then
		if [ "$VERIFY" = yes ]; then
			die "failed to download ${name}.cosign.bundle"
		fi
		log "no cosign bundle for $name, skipping verification"
		return 0
	fi
	if [ -n "$CHECKOUT_ROOT" ] && [ -f "$CHECKOUT_ROOT/cosign.pub" ]; then
		cp "$CHECKOUT_ROOT/cosign.pub" "$key"
	elif ! http_get "$(raw_url "$ref" cosign.pub)" "$key"; then
		if [ "$VERIFY" = yes ]; then
			die "failed to download cosign.pub"
		fi
		log "could not fetch cosign.pub, skipping verification"
		return 0
	fi
	log "verify cosign attestation for $name"
	if ! verify_blob "$blob" "$bundle" "$key"; then
		die "cosign verification failed for $name"
	fi
	log "cosign attestation ok"
}

build_from_tree() {
	root="$1"
	out="$2"
	(
		cd "$root"
		export CGO_ENABLED=0
		export GOFLAGS="${GOFLAGS:--mod=vendor}"
		if [ "$OS" = linux ] && [ "$ARCH" = amd64 ]; then
			export GOAMD64
		fi
		ldflags="-s -w"
		if [ "$TAG" != latest ] && [ "$TAG" != master ]; then
			ldflags="$ldflags -X main.defaultVersion=${TAG}"
		elif [ -n "$CHECKOUT_ROOT" ] && have_cmd git; then
			ver=$(git -C "$root" describe --tags --always --dirty 2>/dev/null || true)
			if [ -n "$ver" ]; then
				ldflags="$ldflags -X main.defaultVersion=${ver}"
			fi
		fi
		log "go build (GOAMD64=${GOAMD64:-default}) -> $out"
		go build -ldflags "$ldflags" -o "$out" ./cmd/reticulum-go
	)
}

fetch_source_tree() {
	make_work_dir
	tarball="$WORK_DIR/src.tar.gz"
	url=$(archive_url)
	log "download source $url"
	http_get "$url" "$tarball"
	mkdir -p "$WORK_DIR/src"
	if have_cmd gzip; then
		gzip -dc "$tarball" | tar -C "$WORK_DIR/src" -xf -
	else
		tar -C "$WORK_DIR/src" -xzf "$tarball"
	fi
	for d in "$WORK_DIR/src"/*; do
		if [ -d "$d" ]; then
			SRC_ROOT="$d"
			return 0
		fi
	done
	die "source archive had no top-level directory"
}

print_plan() {
	log "os=$OS arch=$ARCH goamd64=$GOAMD64 v3_ok=$V3_OK"
	log "init_detected=$INIT_DETECTED init=$INIT service=$SERVICE"
	log "method=$METHOD tag=$TAG"
	log "prefix=$PREFIX bindir=$BINDIR destdir=${DESTDIR:-none}"
	if [ -n "$CHECKOUT_ROOT" ]; then
		log "checkout=$CHECKOUT_ROOT"
	fi
	if [ "$METHOD" = binary ]; then
		log "asset=$ASSET"
		log "url=$(release_url "$ASSET")"
	else
		if [ -n "$CHECKOUT_ROOT" ]; then
			log "source=checkout $CHECKOUT_ROOT"
		else
			log "source=$(archive_url)"
		fi
	fi
	if [ "$DRY_RUN" = 1 ]; then
		log "dry-run: no files will be written"
		log "would install ${DESTDIR}${BINDIR}/${BINARY_NAME}"
		log "would install tool links: $TOOL_LINKS"
		if [ "$METHOD" = source ] || [ -n "$CHECKOUT_ROOT" ]; then
			log "would install man pages under ${DESTDIR}${PREFIX}/share/man"
		fi
		if [ "$SERVICE" = yes ]; then
			svc="$INIT"
			if [ "$svc" = auto ]; then
				svc="$INIT_DETECTED"
			fi
			log "would install $svc service files (state dir $STATE_DIR)"
		else
			log "would skip service files"
		fi
		if [ -n "$SUDO" ]; then
			log "would use sudo for privileged paths"
		fi
	fi
}

install_from_binary() {
	need_cmd uname
	if ! have_cmd curl && ! have_cmd wget; then
		die "need curl or wget to fetch a release binary"
	fi
	make_work_dir
	bin="$WORK_DIR/$ASSET"
	url=$(release_url "$ASSET")
	log "download $url"
	if ! http_get "$url" "$bin"; then
		if [ "$OS" = linux ] && [ "$ARCH" = amd64 ] && [ "$GOAMD64" = v3 ]; then
			log "v3 asset missing, falling back to GOAMD64=v1"
			GOAMD64=v1
			ASSET=$(asset_name)
			bin="$WORK_DIR/$ASSET"
			url=$(release_url "$ASSET")
			log "download $url"
			http_get "$url" "$bin"
		else
			die "failed to download $url"
		fi
	fi
	chmod 755 "$bin"
	maybe_verify "$bin" "$ASSET"
	install_binary_and_links "$bin"
	if [ -n "$CHECKOUT_ROOT" ]; then
		install_man_from_tree "$CHECKOUT_ROOT"
	fi
	if [ "$SERVICE" = yes ]; then
		svc="$INIT"
		if [ "$svc" = auto ]; then
			svc="$INIT_DETECTED"
		fi
		if [ -n "$CHECKOUT_ROOT" ]; then
			run_install_service "$CHECKOUT_ROOT" "$svc"
		else
			ref=$(resolve_git_ref)
			pkg="$WORK_DIR/tree"
			fetch_packaging_tree "$pkg" "$ref"
			run_install_service "$pkg" "$svc"
		fi
	fi
}

install_from_source() {
	check_go
	need_cmd tar
	if [ -n "$CHECKOUT_ROOT" ]; then
		SRC_ROOT="$CHECKOUT_ROOT"
	else
		if ! have_cmd curl && ! have_cmd wget; then
			die "need curl or wget to fetch source"
		fi
		fetch_source_tree
	fi
	make_work_dir
	out="$WORK_DIR/$BINARY_NAME"
	build_from_tree "$SRC_ROOT" "$out"
	install_binary_and_links "$out"
	install_man_from_tree "$SRC_ROOT"
	if [ "$SERVICE" = yes ]; then
		svc="$INIT"
		if [ "$svc" = auto ]; then
			svc="$INIT_DETECTED"
		fi
		run_install_service "$SRC_ROOT" "$svc"
	fi
}

parse_args() {
	while [ $# -gt 0 ]; do
		case "$1" in
		--binary)
			METHOD=binary
			shift
			;;
		--source)
			METHOD=source
			shift
			;;
		--dry-run | -n)
			DRY_RUN=1
			shift
			;;
		--yes | -y)
			ASSUME_YES=1
			shift
			;;
		--no-service)
			SERVICE=no
			INIT=none
			shift
			;;
		--service)
			SERVICE=yes
			shift
			;;
		--init)
			[ $# -ge 2 ] || die "option $1 needs a value"
			INIT="$2"
			shift 2
			;;
		--init=*)
			INIT="${1#--init=}"
			shift
			;;
		--prefix)
			[ $# -ge 2 ] || die "option $1 needs a value"
			PREFIX="$2"
			PREFIX_SET=1
			shift 2
			;;
		--destdir)
			[ $# -ge 2 ] || die "option $1 needs a value"
			DESTDIR="$2"
			shift 2
			;;
		--bindir)
			[ $# -ge 2 ] || die "option $1 needs a value"
			BINDIR="$2"
			BINDIR_SET=1
			shift 2
			;;
		--tag | --version)
			[ $# -ge 2 ] || die "option $1 needs a value"
			TAG="$2"
			shift 2
			;;
		--tag=* | --version=*)
			TAG="${1#*=}"
			shift
			;;
		--goamd64)
			[ $# -ge 2 ] || die "option $1 needs a value"
			GOAMD64_REQ="$2"
			shift 2
			;;
		--goamd64=*)
			GOAMD64_REQ="${1#--goamd64=}"
			shift
			;;
		--verify)
			VERIFY=yes
			shift
			;;
		--no-verify)
			VERIFY=no
			shift
			;;
		--force)
			FORCE=1
			shift
			;;
		-h | --help)
			usage
			exit 0
			;;
		--)
			shift
			break
			;;
		-*)
			err "unknown option: $1"
			usage >&2
			exit 1
			;;
		*)
			err "unexpected argument: $1"
			usage >&2
			exit 1
			;;
		esac
	done
}

main() {
	parse_args "$@"

	case "$INIT" in
	auto | systemd | openrc | runit | dinit | all | none) ;;
	*) die "unknown --init '$INIT' (auto|systemd|openrc|runit|dinit|all|none)" ;;
	esac

	case "$METHOD" in
	"" | binary | source) ;;
	*) die "unknown method '$METHOD' (binary|source)" ;;
	esac

	if [ "$BINDIR_SET" = 0 ] || [ -z "$BINDIR" ]; then
		BINDIR="${PREFIX}/bin"
	fi

	resolve_checkout || true
	detect_os
	detect_arch
	detect_v3 || true
	detect_init
	resolve_goamd64
	ASSET=$(asset_name)

	if [ "$DRY_RUN" = 0 ]; then
		resolve_privilege
	else
		target="${DESTDIR}${BINDIR}"
		if ! dir_writable "$target" && ! dir_writable "$(dirname "$target")" && [ "$(id -u)" -ne 0 ]; then
			if have_cmd sudo; then
				SUDO=sudo
			fi
		fi
	fi

	choose_method
	choose_service

	if [ "$SERVICE" = yes ] && [ "$INIT" = auto ] && [ "$INIT_DETECTED" = unknown ]; then
		if [ "$ASSUME_YES" = 1 ] || [ "$DRY_RUN" = 1 ]; then
			SERVICE=no
			log "no init system detected, skipping service"
		else
			die "could not detect init system (set --init systemd|openrc|runit|dinit|all or --no-service)"
		fi
	fi

	print_plan

	if [ "$DRY_RUN" = 1 ]; then
		return 0
	fi

	case "$METHOD" in
	binary) install_from_binary ;;
	source) install_from_source ;;
	*) die "internal error: method=$METHOD" ;;
	esac

	log "installed ${DESTDIR}${BINDIR}/${BINARY_NAME}"
	if have_cmd "${DESTDIR}${BINDIR}/${BINARY_NAME}" || [ -x "${DESTDIR}${BINDIR}/${BINARY_NAME}" ]; then
		if [ -z "$DESTDIR" ]; then
			"${BINDIR}/${BINARY_NAME}" --version 2>/dev/null || true
		fi
	fi
	log "done"
}

main "$@"
