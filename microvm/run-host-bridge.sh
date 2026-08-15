#!/usr/bin/env bash
# Host bridge for nested Firecracker microvms.
#
# The outer VM has clearnet. The microvm guest talks VSOCK only.
# This process connects to a community hub and pipes HDLC frames into
# the guest through Firecracker vsock UDS (CONNECT protocol).
#
# Usage:
#   NET=0 DETACH=1 ./microvm/run.sh
#   ./microvm/run-host-bridge.sh

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="$(cd "${ROOT}/.." && pwd)"
OUT="${ROOT}/out"
CFG_SRC="${ROOT}/host-bridge.config"
CFG="${OUT}/host-bridge.config"
PID_FILE="${OUT}/host-bridge.pid"
LOG_FILE="${OUT}/host-bridge.log"
VSOCK_UDS="${VSOCK_UDS:-${OUT}/vsock.sock}"

if [[ ! -S "${VSOCK_UDS}" ]]; then
	echo "guest vsock uds missing: ${VSOCK_UDS}" >&2
	echo "start the microvm first: NET=0 DETACH=1 ${ROOT}/run.sh" >&2
	exit 1
fi

mkdir -p "${OUT}"
sed "s|__MICROVM_ROOT__|${ROOT}|g" "${CFG_SRC}" >"${CFG}"

if [[ -f "${PID_FILE}" ]]; then
	old="$(cat "${PID_FILE}" 2>/dev/null || true)"
	if [[ -n "${old}" ]] && kill -0 "${old}" 2>/dev/null; then
		echo "host bridge already running (pid ${old})" >&2
		exit 1
	fi
	rm -f "${PID_FILE}"
fi

BIN="${OUT}/reticulum-go-host"
CGO_ENABLED=0 GOFLAGS=-mod=mod go build \
	-C "${REPO}" \
	-ldflags='-w -s' \
	-o "${BIN}" \
	./cmd/reticulum-go

echo "starting host bridge (guest vsock pipe)"
echo "  config=${CFG}"
echo "  log=${LOG_FILE}"
echo "  add hubs in ${CFG_SRC} then restart the bridge"

if [[ "${DETACH:-0}" == "1" ]]; then
	"${BIN}" --config "${CFG}" >"${LOG_FILE}" 2>&1 &
	echo $! >"${PID_FILE}"
	echo "detached pid $(cat "${PID_FILE}")"
	exit 0
fi

"${BIN}" --config "${CFG}" 2>&1 | tee "${LOG_FILE}" &
echo $! >"${PID_FILE}"
trap 'kill "$(cat "${PID_FILE}" 2>/dev/null)" 2>/dev/null || true; rm -f "${PID_FILE}"' INT TERM
wait "$(cat "${PID_FILE}")" || true
rm -f "${PID_FILE}"
