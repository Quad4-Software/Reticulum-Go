#!/usr/bin/env bash
# Host-side Firecracker vsock CONNECT helper.
#
# Guest reticulum-go listens on AF_VSOCK port 4242 by default.
# Firecracker exposes host access via a Unix socket plus a CONNECT line.
#
# Usage:
#   ./microvm/vsock-connect.sh
#   ./microvm/vsock-connect.sh 4242
#   VSOCK_UDS=/path/to/vsock.sock ./microvm/vsock-connect.sh

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUT="${ROOT}/out"
VSOCK_UDS="${VSOCK_UDS:-${OUT}/vsock.sock}"
PORT="${1:-4242}"

if [[ ! -S "${VSOCK_UDS}" ]]; then
	echo "vsock uds not found: ${VSOCK_UDS}" >&2
	echo "start the vm with ${ROOT}/run.sh first" >&2
	exit 1
fi

exec python3 -u "${ROOT}/vsock_connect.py" "${VSOCK_UDS}" "${PORT}"
