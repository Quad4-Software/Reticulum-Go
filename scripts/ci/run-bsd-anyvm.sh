#!/bin/sh
# Run reticulum-go BSD self-check via ghcr.io/anyvm-org/anyvm (QEMU in Docker).
# Requires Docker, KVM (/dev/kvm), and network for the first image and Go tarball fetch.
#
# Usage:
#   sh scripts/ci/run-bsd-anyvm.sh freebsd|openbsd|netbsd|hardenedbsd
set -eu

OS="${1:-}"
case "$OS" in
freebsd | openbsd | netbsd | hardenedbsd) ;;
*)
	echo "usage: $0 freebsd|openbsd|netbsd|hardenedbsd" >&2
	exit 2
	;;
esac

ROOT=$(CDPATH='' cd -- "$(dirname "$0")/../.." && pwd)
. "$ROOT/scripts/ci/versions.env"

CACHE="${BSD_ANYVM_CACHE:-$ROOT/.cache/anyvm-$OS}"
XFER="$CACHE/xfer"
IMAGE="${ANYVM_IMAGE:-ghcr.io/anyvm-org/anyvm:latest}"
GO_VERSION="${BSD_ANYVM_GO_VERSION:-$GO_VERSION}"
MEM="${BSD_ANYVM_MEM:-4096}"
CPU="${BSD_ANYVM_CPU:-4}"

case "$OS" in
freebsd)
	RELEASE="${ANYVM_FREEBSD_RELEASE:-14.3}"
	GO_TARBALL_OS=freebsd
	;;
openbsd)
	RELEASE="${ANYVM_OPENBSD_RELEASE:-7.7}"
	GO_TARBALL_OS=openbsd
	;;
netbsd)
	RELEASE="${ANYVM_NETBSD_RELEASE:-10.2}"
	GO_TARBALL_OS=netbsd
	;;
hardenedbsd)
	RELEASE="${ANYVM_HARDENEDBSD_RELEASE:-15}"
	GO_TARBALL_OS=freebsd
	;;
esac

GO_URL="${BSD_ANYVM_GO_URL:-https://go.dev/dl/go${GO_VERSION}.${GO_TARBALL_OS}-amd64.tar.gz}"

mkdir -p "$XFER"
if [ ! -f "$XFER/go.tgz" ]; then
	echo "Downloading $GO_TARBALL_OS Go $GO_VERSION..."
	curl -fsSL -o "$XFER/go.tgz" "$GO_URL"
fi

STAGE=$(mktemp -d)
trap 'rm -rf "$STAGE"' EXIT
rsync -a \
	--exclude '.git/' \
	--exclude '.cache/' \
	--exclude 'bin/' \
	--exclude 'node_modules/' \
	--exclude '.tools/' \
	--exclude 'examples/' \
	--exclude 'docs/' \
	--exclude 'bindings/' \
	--exclude 'packaging/' \
	--exclude 'microvm/' \
	"$ROOT/cmd" "$ROOT/pkg" "$ROOT/internal" "$ROOT/vendor" \
	"$ROOT/scripts" "$ROOT/tests" "$ROOT/go.mod" "$ROOT/go.sum" \
	"$STAGE/"
rm -f "$XFER/rns.tgz"
(cd "$STAGE" && tar -czf "$XFER/rns.tgz" .)
rm -rf "$STAGE"
trap - EXIT

cat >"$XFER/run.sh" <<'EOF'
#!/bin/sh
set -eu
uname -a
mkdir -p /usr/local
rm -rf /usr/local/go
tar -C /usr/local -xzf /root/xfer/go.tgz
export PATH=/usr/local/go/bin:$PATH
export GOTOOLCHAIN=local
export GOFLAGS=-mod=vendor
export GOPROXY=off
export CGO_ENABLED=0
rm -rf /root/rns
mkdir -p /root/rns
cd /root/rns
tar -xzf /root/xfer/rns.tgz
sh scripts/ci/bsd-guest-self-check.sh
echo OK
EOF
chmod +x "$XFER/run.sh"

exec docker run --rm \
	--device /dev/kvm \
	-v "$XFER:/xfer:ro" \
	-v "$CACHE:/cache" \
	"$IMAGE" \
	--os "$OS" --release "$RELEASE" --mem "$MEM" --cpu "$CPU" \
	--cache-dir /cache --data-dir /cache/data \
	--sync scp -v /xfer:/root/xfer \
	--vnc off \
	-- sh /root/xfer/run.sh
