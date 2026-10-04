#!/usr/bin/env bash
# Verify or install pinned Go and Task without GitHub Actions.
#
# Usage:
#   bash scripts/ci/bootstrap-toolchain.sh verify
#   bash scripts/ci/bootstrap-toolchain.sh install
#
# After install, run scripts/ci/bootstrap.sh for revive and staticcheck.
set -euo pipefail

. "$(dirname "$0")/env.sh"
. "$ROOT/scripts/ci/versions.env"

mode="${1:-${CI_BOOTSTRAP:-verify}}"

toolchain="${CI_TOOLCHAIN_ROOT:-$ROOT/.cache/ci-toolchain}"
bindir="$toolchain/bin"
gopath="$toolchain/gopath"

export PATH="$bindir:$PATH"
export GOPATH="$gopath"
export GOCACHE="${GOCACHE:-$toolchain/gocache}"
export GOTOOLCHAIN=local

go_want="${GO_VERSION}"
task_want="${TASK_VERSION}"

semver_ge() {
	awk -v a="$1" -v b="$2" 'BEGIN {
		split(a, A, "."); split(b, B, ".")
		for (i = 1; i <= 3; i++) {
			ai = A[i] + 0; bi = B[i] + 0
			if (ai > bi) exit 0
			if (ai < bi) exit 1
		}
		exit 0
	}'
}

have_go() {
	command -v go >/dev/null 2>&1 || return 1
	local v
	v="$(go env GOVERSION 2>/dev/null | sed 's/^go//')"
	semver_ge "$v" "$go_want"
}

have_task() {
	command -v task >/dev/null 2>&1 || return 1
	local v
	v="$(task --version 2>/dev/null | awk '{print $NF}' | tr -d 'v')"
	semver_ge "$v" "$task_want"
}

verify_go_task() {
	local ok=1
	have_go || { echo "bootstrap-toolchain: need Go >= $go_want on PATH" >&2; ok=0; }
	have_task || { echo "bootstrap-toolchain: need Task >= $task_want on PATH" >&2; ok=0; }
	[ "$ok" -eq 1 ]
}

verify_all() {
	local ok=1
	have_go || { echo "bootstrap-toolchain: need Go >= $go_want on PATH" >&2; ok=0; }
	have_task || { echo "bootstrap-toolchain: need Task >= $task_want on PATH" >&2; ok=0; }
	command -v revive >/dev/null 2>&1 || { echo "bootstrap-toolchain: need revive on PATH" >&2; ok=0; }
	command -v staticcheck >/dev/null 2>&1 || { echo "bootstrap-toolchain: need staticcheck on PATH" >&2; ok=0; }
	command -v gosec >/dev/null 2>&1 || { echo "bootstrap-toolchain: need gosec on PATH" >&2; ok=0; }
	[ "$ok" -eq 1 ]
}

verify_go_task_only() {
	verify_go_task
}

linux_arch() {
	local m
	m="$(uname -m)"
	case "$m" in
	x86_64) echo amd64 ;;
	aarch64 | arm64) echo arm64 ;;
	*) echo "bootstrap-toolchain: unsupported cpu: $m" >&2; exit 1 ;;
	esac
}

fetch() {
	local url="$1" dest="$2"
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL "$url" -o "$dest"
	elif command -v wget >/dev/null 2>&1; then
		wget -qO "$dest" "$url"
	else
		echo "bootstrap-toolchain: need curl or wget" >&2
		exit 1
	fi
}

install_go() {
	if have_go; then
		return 0
	fi
	local arch go_tgz
	arch="$(linux_arch)"
	go_tgz="go${go_want}.linux-${arch}.tar.gz"
	mkdir -p "$toolchain"
	tmp="$(mktemp "${TMPDIR:-/tmp}/go-bootstrap.XXXXXX")"
	fetch "https://go.dev/dl/${go_tgz}" "$tmp"
	rm -rf "$toolchain/go"
	tar -C "$toolchain" -xzf "$tmp"
	rm -f "$tmp"
	ln -sf "$toolchain/go/bin/go" "$bindir/go"
	ln -sf "$toolchain/go/bin/gofmt" "$bindir/gofmt"
	export PATH="$bindir:$PATH"
}

install_task() {
	if have_task; then
		return 0
	fi
	install_go
	mkdir -p "$gopath/bin"
	env GOFLAGS= GOPROXY=https://proxy.golang.org,direct GOSUMDB=sum.golang.org \
		GOBIN="$bindir" go install "github.com/go-task/task/v3/cmd/task@v${task_want}"
	export PATH="$bindir:$PATH"
}

install_all() {
	mkdir -p "$bindir" "$gopath" "$GOCACHE"
	install_go
	install_task
	export GOBIN="$bindir"
	export PATH="$bindir:$gopath/bin:$PATH"
	sh "$ROOT/scripts/ci/bootstrap.sh"
	verify_all
	echo "bootstrap-toolchain: ready under $toolchain"
}

case "$mode" in
verify)
	verify_all
	echo "bootstrap-toolchain: verify OK"
	;;
install)
	install_all
	;;
*)
	echo "bootstrap-toolchain: usage: bootstrap-toolchain.sh verify|install" >&2
	exit 2
	;;
esac
