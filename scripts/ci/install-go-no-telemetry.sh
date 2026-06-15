#!/bin/sh
# Install a prebuilt go-no-telemetry toolchain from cache, artifact, or source fallback.
# Usage: install-go-no-telemetry.sh <version>
set -eu

. "$(dirname "$0")/priv.sh"

VERSION="${1:?Go version required}"
VERSION="${VERSION#go}"
VERSION="${VERSION#v}"
INSTALL_ROOT="${CI_GO_NO_TELEMETRY_ROOT:-/usr/local/go-no-telemetry}"
WORKFLOW_FILE="${CI_GO_NO_TELEMETRY_WORKFLOW:-go-no-telemetry-toolchain.yml}"

ARCH="$(uname -m)"
case "$ARCH" in
    x86_64)  ARCH="amd64" ;;
    aarch64) ARCH="arm64" ;;
    *)       echo "Unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

ARTIFACT_BASE="go-no-telemetry-${VERSION}-linux-${ARCH}"
ARCHIVE="${ARTIFACT_BASE}.tar.gz"
ARTIFACT_DIR="${CI_GO_NO_TELEMETRY_ARTIFACT_DIR:-}"

export_paths() {
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
}

verify_toolchain() {
    if [ ! -x "${INSTALL_ROOT}/bin/go" ]; then
        return 1
    fi
    if [ "$(GOROOT="$INSTALL_ROOT" "${INSTALL_ROOT}/bin/go" telemetry)" != "off" ]; then
        echo "go telemetry is not off" >&2
        return 1
    fi
    GO_VER="$(GOROOT="$INSTALL_ROOT" "${INSTALL_ROOT}/bin/go" version | awk '{print $3}')"
    case "$GO_VER" in
        go${VERSION}*) return 0 ;;
        *)
            echo "toolchain version mismatch: expected go${VERSION}, got ${GO_VER}" >&2
            return 1
            ;;
    esac
}

install_from_archive() {
    archive="$1"
    expected_sha="${archive}.sha256"
    if [ -f "$expected_sha" ]; then
        ACTUAL="$(sha256sum "$archive" | awk '{print $1}')"
        EXPECTED="$(tr -d '\n\r ' < "$expected_sha")"
        if [ "$ACTUAL" != "$EXPECTED" ]; then
            echo "SHA256 mismatch for ${archive}" >&2
            return 1
        fi
    fi

    PARENT="$(dirname "$INSTALL_ROOT")"
    rm -rf "$INSTALL_ROOT"
    mkdir -p "$PARENT"
    tar -C "$PARENT" -xzf "$archive"
    verify_toolchain
}

if verify_toolchain; then
    export_paths
    go version
    go telemetry
    exit 0
fi

if [ -n "$ARTIFACT_DIR" ]; then
    if [ -f "${ARTIFACT_DIR}/${ARCHIVE}" ]; then
        install_from_archive "${ARTIFACT_DIR}/${ARCHIVE}"
        export_paths
        go version
        go telemetry
        exit 0
    fi
    if [ -f "${ARTIFACT_DIR}/${ARCHIVE%.tar.gz}" ]; then
        install_from_archive "${ARTIFACT_DIR}/${ARCHIVE%.tar.gz}"
        export_paths
        go version
        go telemetry
        exit 0
    fi
fi

if [ -n "${GITHUB_ACTIONS:-}" ] && command -v gh >/dev/null 2>&1; then
    REPO="${GITHUB_REPOSITORY:?}"
    BRANCH="${CI_GO_NO_TELEMETRY_BRANCH:-}"
    if [ -z "$BRANCH" ]; then
        BRANCH="${GITHUB_REF_NAME:-master}"
        case "$BRANCH" in
            master|dev) ;;
            *) BRANCH=master ;;
        esac
    fi
    RUN_ID="$(gh run list \
        --repo "$REPO" \
        --workflow "$WORKFLOW_FILE" \
        --branch "$BRANCH" \
        --status success \
        --limit 1 \
        --json databaseId \
        --jq '.[0].databaseId' 2>/dev/null || true)"
    if [ -n "$RUN_ID" ] && [ "$RUN_ID" != "null" ]; then
        DL_DIR="$(mktemp -d)"
        if gh run download "$RUN_ID" \
            --repo "$REPO" \
            --name "$ARTIFACT_BASE" \
            --dir "$DL_DIR" 2>/dev/null; then
            if [ -f "${DL_DIR}/${ARCHIVE}" ]; then
                install_from_archive "${DL_DIR}/${ARCHIVE}"
                rm -rf "$DL_DIR"
                export_paths
                go version
                go telemetry
                exit 0
            fi
        fi
        rm -rf "$DL_DIR"
    fi
fi

echo "No prebuilt go-no-telemetry artifact found; building from source..." >&2
"$(dirname "$0")/setup-go-no-telemetry.sh" "$VERSION"
