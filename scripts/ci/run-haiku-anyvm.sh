#!/bin/sh
# Run reticulum-go Haiku self-check via ghcr.io/anyvm-org/anyvm (QEMU in Docker).
# Requires Docker, KVM (/dev/kvm), and network for the first image/bootstrap fetch.
set -eu

ROOT=$(CDPATH='' cd -- "$(dirname "$0")/../.." && pwd)
CACHE="${HAIKU_ANYVM_CACHE:-$ROOT/.cache/anyvm-haiku}"
XFER="$CACHE/xfer"
IMAGE="${HAIKU_ANYVM_IMAGE:-ghcr.io/anyvm-org/anyvm:latest}"
GO_BOOTSTRAP_URL="${HAIKU_GO_BOOTSTRAP_URL:-https://github.com/korli/go/releases/download/go1.26.1-haiku1/go-1.26.1-haiku-amd64-bootstrap.tbz}"
GO_HAIKU_VERSION="${HAIKU_GO_VERSION:-1.26.1}"

mkdir -p "$XFER"
if [ ! -f "$XFER/go-bootstrap.tbz" ]; then
	echo "Downloading Haiku Go bootstrap..."
	curl -fsSL -o "$XFER/go-bootstrap.tbz" "$GO_BOOTSTRAP_URL"
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
rm -f "$XFER/rns.zip"
(cd "$STAGE" && zip -qr "$XFER/rns.zip" .)

cat > "$XFER/run.sh" << EOF
#!/bin/sh
set -eu
uname -a
cd /boot/home/user
mkdir -p go-bootstrap
if [ ! -x /boot/home/user/go-bootstrap/bin/go ]; then
  tar -xjf /boot/home/user/xfer/go-bootstrap.tbz -C /boot/home/user/go-bootstrap --strip-components=1
fi
export GOROOT=/boot/home/user/go-bootstrap
export PATH="\$GOROOT/bin:\$PATH"
export GOTOOLCHAIN=local
export GOFLAGS=-mod=vendor
export GOPROXY=off
export GOCACHE=/boot/home/user/.cache/go-build
mkdir -p "\$GOCACHE"
go version
rm -rf /boot/home/user/rns
mkdir -p /boot/home/user/rns
cd /boot/home/user/rns
unzip -q /boot/home/user/xfer/rns.zip
sed -i 's/^go 1\\.26\\.[0-9][0-9]*\$/go ${GO_HAIKU_VERSION}/' go.mod
sed -i 's/go 1\\.26\\.[2-9][0-9]*/go ${GO_HAIKU_VERSION}/g; s/go 1\\.26\\.[0-9][0-9][0-9]*/go ${GO_HAIKU_VERSION}/g' vendor/modules.txt
mkdir -p bin
go build -buildvcs=false -ldflags='-s -w' -o bin/reticulum-go ./cmd/reticulum-go
go test -buildvcs=false -short -count=1 -timeout 15m ./pkg/selfcheck/ ./pkg/sandbox/ ./pkg/protect/
go test -buildvcs=false -short -count=1 -timeout 10m ./pkg/transport/ -run 'Protect|HandlePacket'
./bin/reticulum-go self-check --binary "\$(pwd)/bin/reticulum-go" --json --full
echo OK
EOF
chmod +x "$XFER/run.sh"

# Fresh working disk each run (BFS gets unhappy after failed partial extracts)
docker run --rm -v "$CACHE:/cache" alpine:3.20 \
	sh -c 'rm -f /cache/data/haiku/v2.0.2/haiku-r1beta5.qcow2 /cache/data/haiku/v2.0.2/haiku-r1beta5.serial.log /cache/data/haiku/v2.0.2/haiku-r1beta5.knownhosts' \
	>/dev/null

exec docker run --rm \
	--device /dev/kvm \
	-v "$XFER:/xfer:ro" \
	-v "$CACHE:/cache" \
	"$IMAGE" \
	--os haiku --release r1beta5 --mem "${HAIKU_ANYVM_MEM:-6144}" --cpu "${HAIKU_ANYVM_CPU:-4}" \
	--cache-dir /cache --data-dir /cache/data \
	--sync scp -v /xfer:/boot/home/user/xfer \
	--vnc off \
	-- sh /boot/home/user/xfer/run.sh
