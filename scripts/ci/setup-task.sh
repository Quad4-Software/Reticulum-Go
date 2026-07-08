#!/bin/sh
# Install go-task from GitHub releases with SHA256 verification.
# Usage: setup-task.sh [version]
set -eu

TASK_VERSION="${1:-3.49.1}"

ARCH="$(uname -m)"
case "$ARCH" in
    x86_64 | amd64) ARCH="amd64" ;;
    aarch64 | arm64) ARCH="arm64" ;;
    *) echo "Unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

BASE_URL="https://github.com/go-task/task/releases/download/v${TASK_VERSION}"

task_install_dir() {
    if [ -n "${RUNNER_TOOL_CACHE:-}" ]; then
        echo "${RUNNER_TOOL_CACHE}/task-bin"
        return
    fi
    if [ -w /usr/local/bin ] 2>/dev/null; then
        echo /usr/local/bin
        return
    fi
    echo "${HOME:-/tmp}/.local/bin"
}

INSTALL_DIR="$(task_install_dir)"
mkdir -p "$INSTALL_DIR"

OS="$(uname -s)"
case "$OS" in
    Linux)
        TARBALL="task_linux_${ARCH}.tar.gz"
        curl -fsSL "${BASE_URL}/task_checksums.txt" -o /tmp/task-checksums.txt
        curl -fsSL "${BASE_URL}/${TARBALL}" -o /tmp/task.tar.gz
        EXPECTED="$(grep "  ${TARBALL}\$" /tmp/task-checksums.txt | cut -d' ' -f1)"
        ACTUAL="$(sha256sum /tmp/task.tar.gz | cut -d' ' -f1)"
        if [ -z "$EXPECTED" ] || [ "$EXPECTED" != "$ACTUAL" ]; then
            echo "SHA256 verification failed for ${TARBALL}" >&2
            exit 1
        fi
        tar -xzf /tmp/task.tar.gz -C "$INSTALL_DIR" task
        rm -f /tmp/task.tar.gz /tmp/task-checksums.txt
        ;;
    Darwin)
        TARBALL="task_darwin_${ARCH}.tar.gz"
        curl -fsSL "${BASE_URL}/task_checksums.txt" -o /tmp/task-checksums.txt
        curl -fsSL "${BASE_URL}/${TARBALL}" -o /tmp/task.tar.gz
        EXPECTED="$(grep "  ${TARBALL}\$" /tmp/task-checksums.txt | cut -d' ' -f1)"
        ACTUAL="$(shasum -a 256 /tmp/task.tar.gz | cut -d' ' -f1)"
        if [ -z "$EXPECTED" ] || [ "$EXPECTED" != "$ACTUAL" ]; then
            echo "SHA256 verification failed for ${TARBALL}" >&2
            exit 1
        fi
        tar -xzf /tmp/task.tar.gz -C "$INSTALL_DIR" task
        rm -f /tmp/task.tar.gz /tmp/task-checksums.txt
        ;;
    MINGW* | MSYS* | CYGWIN*)
        ZIP="task_windows_${ARCH}.zip"
        curl -fsSL "${BASE_URL}/task_checksums.txt" -o /tmp/task-checksums.txt
        curl -fsSL "${BASE_URL}/${ZIP}" -o /tmp/task.zip
        EXPECTED="$(grep "  ${ZIP}\$" /tmp/task-checksums.txt | tr -d '\r' | cut -d' ' -f1)"
        ACTUAL="$(sha256sum /tmp/task.zip | cut -d' ' -f1)"
        if [ -z "$EXPECTED" ] || [ "$EXPECTED" != "$ACTUAL" ]; then
            echo "SHA256 verification failed for ${ZIP}" >&2
            exit 1
        fi
        unzip -qo /tmp/task.zip -d /tmp/task-extract
        cp /tmp/task-extract/task.exe "$INSTALL_DIR/task.exe"
        rm -rf /tmp/task-extract /tmp/task.zip /tmp/task-checksums.txt
        ;;
    *)
        echo "Unsupported OS: $OS" >&2
        exit 1
        ;;
esac

export PATH="${INSTALL_DIR}:$PATH"
if [ -n "${GITHUB_PATH:-}" ]; then
    echo "$INSTALL_DIR" >> "$GITHUB_PATH"
fi
if [ -n "${GITEA_PATH:-}" ]; then
    echo "$INSTALL_DIR" >> "$GITEA_PATH"
fi

TASK_BIN="${INSTALL_DIR}/task"
if [ -f "${INSTALL_DIR}/task.exe" ]; then
    TASK_BIN="${INSTALL_DIR}/task.exe"
fi
"$TASK_BIN" --version
