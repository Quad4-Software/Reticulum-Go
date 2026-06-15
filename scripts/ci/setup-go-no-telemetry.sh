#!/bin/sh
# Build and install go-no-telemetry from source at a pinned Go version.
# Official Go is used only as a SHA-pinned bootstrap compiler (default 1.24.6).
# Usage: setup-go-no-telemetry.sh <version>
#   version: Go release to build, e.g. 1.26.4
set -eu

. "$(dirname "$0")/priv.sh"

VERSION="${1:?Go version required}"
VERSION="${VERSION#go}"
VERSION="${VERSION#v}"
BOOTSTRAP_VER="${CI_GO_BOOTSTRAP_VERSION:-1.24.6}"
INSTALL_ROOT="${CI_GO_NO_TELEMETRY_ROOT:-/usr/local/go-no-telemetry}"
REPO="${CI_GO_NO_TELEMETRY_REPO:-https://github.com/Quad4-Software/go-no-telemetry.git}"
FORK_META_REF="${CI_GO_NO_TELEMETRY_FORK_REF:-origin/master}"
UPSTREAM_TAG="go${VERSION}"
BOOTSTRAP_ROOT="/tmp/go-bootstrap-${BOOTSTRAP_VER}"
WORK_DIR="${CI_GO_NO_TELEMETRY_WORK:-/tmp/go-no-telemetry-src}"

ARCH="$(uname -m)"
case "$ARCH" in
    x86_64)  ARCH="amd64" ;;
    aarch64) ARCH="arm64" ;;
    *)       echo "Unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

install_bootstrap() {
    GO_VERSION="go${BOOTSTRAP_VER#go}"
    TARBALL="${GO_VERSION}.linux-${ARCH}.tar.gz"
    BASE="https://dl.google.com/go"

    curl -fsSL "${BASE}/${TARBALL}.sha256" | tr -d '\n\r ' > /tmp/go-bootstrap.sha256
    EXPECTED="$(cat /tmp/go-bootstrap.sha256)"

    curl -fsSL "${BASE}/${TARBALL}" -o /tmp/go-bootstrap.tar.gz
    ACTUAL="$(sha256sum /tmp/go-bootstrap.tar.gz | awk '{print $1}')"
    if [ "$ACTUAL" != "$EXPECTED" ]; then
        echo "SHA256 mismatch for bootstrap ${TARBALL}" >&2
        rm -f /tmp/go-bootstrap.tar.gz /tmp/go-bootstrap.sha256
        exit 1
    fi

    rm -rf "$BOOTSTRAP_ROOT"
    mkdir -p "$BOOTSTRAP_ROOT"
    tar -C "$BOOTSTRAP_ROOT" --strip-components=1 -xzf /tmp/go-bootstrap.tar.gz
    rm -f /tmp/go-bootstrap.tar.gz /tmp/go-bootstrap.sha256
}

build_at_version() {
    root="$1"
    cd "$root"

    if ! git remote | grep -qx upstream; then
        git remote add upstream https://go.googlesource.com/go
    fi
    if ! git rev-parse "$FORK_META_REF" >/dev/null 2>&1; then
        echo "fork metadata ref '$FORK_META_REF' not found" >&2
        exit 1
    fi
    fork_sha="$(git rev-parse "$FORK_META_REF")"

    git fetch upstream "refs/tags/${UPSTREAM_TAG}:refs/tags/${UPSTREAM_TAG}" 2>/dev/null \
        || git fetch upstream tag "${UPSTREAM_TAG}" --no-tags
    git checkout -f "$UPSTREAM_TAG"
    git checkout "$fork_sha" -- fork scripts bootstrap.sh LICENSE PATENTS go.env .gitignore codereview.cfg

    ./scripts/apply-fork.sh
    if git cat-file -e "${UPSTREAM_TAG}:src/cmd/go/internal/envcmd/env.go" 2>/dev/null; then
        git checkout "$UPSTREAM_TAG" -- src/cmd/go/internal/envcmd/env.go
    fi

    printf 'go%s\ntime %s\n' "$VERSION" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" > VERSION

    cd src
    export GOROOT_BOOTSTRAP="${BOOTSTRAP_ROOT}"
    export GOTOOLCHAIN=local
    export CGO_ENABLED=0
    ./make.bash
    cd "$root"
}

if [ ! -x "${BOOTSTRAP_ROOT}/bin/go" ]; then
    install_bootstrap
fi

export PATH="${BOOTSTRAP_ROOT}/bin:$PATH"
"${BOOTSTRAP_ROOT}/bin/go" version

rm -rf "$WORK_DIR"
if [ -n "${CI_GO_NO_TELEMETRY_SRC:-}" ] && [ -d "${CI_GO_NO_TELEMETRY_SRC}/.git" ]; then
    cp -a "${CI_GO_NO_TELEMETRY_SRC}" "$WORK_DIR"
else
    git clone "$REPO" "$WORK_DIR"
fi

cd "$WORK_DIR"
if git rev-parse "v${VERSION}" >/dev/null 2>&1; then
    git checkout -f "v${VERSION}"
    cd src
    export GOROOT_BOOTSTRAP="${BOOTSTRAP_ROOT}"
    export GOTOOLCHAIN=local
    export CGO_ENABLED=0
    ./make.bash
    cd ..
elif [ -x "./scripts/build-version.sh" ]; then
    export GOROOT_BOOTSTRAP="${BOOTSTRAP_ROOT}"
    export FORK_META_REF="$FORK_META_REF"
    ./scripts/build-version.sh "$VERSION"
else
    build_at_version "$WORK_DIR"
fi

cd "$WORK_DIR"
GO="${WORK_DIR}/bin/go"

if [ ! -x "$GO" ]; then
    echo "missing ${GO} after build" >&2
    exit 1
fi

if [ "$("$GO" telemetry)" != "off" ]; then
    echo "go telemetry is not off" >&2
    exit 1
fi

GO_VER="$("$GO" version | awk '{print $3}')"
case "$GO_VER" in
    go${VERSION}*) ;;
    *)
        echo "toolchain version mismatch: expected go${VERSION}, got ${GO_VER}" >&2
        exit 1
        ;;
esac

PARENT="$(dirname "$INSTALL_ROOT")"
if [ -w "$PARENT" ]; then
    rm -rf "$INSTALL_ROOT"
    cp -a "$WORK_DIR" "$INSTALL_ROOT"
else
    run_priv rm -rf "$INSTALL_ROOT"
    run_priv cp -a "$WORK_DIR" "$INSTALL_ROOT"
fi
rm -rf "$WORK_DIR"

export GOROOT="$INSTALL_ROOT"
export PATH="${INSTALL_ROOT}/bin:$PATH"
export GOTOOLCHAIN=local

if [ -n "${GITHUB_PATH:-}" ]; then
    echo "${INSTALL_ROOT}/bin" >> "$GITHUB_PATH"
fi
if [ -n "${GITEA_PATH:-}" ]; then
    echo "${INSTALL_ROOT}/bin" >> "$GITEA_PATH"
fi
if [ -n "${GITHUB_ENV:-}" ]; then
    echo "GOROOT=${INSTALL_ROOT}" >> "$GITHUB_ENV"
    echo "GOTOOLCHAIN=local" >> "$GITHUB_ENV"
fi

go version
go telemetry
