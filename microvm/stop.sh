#!/usr/bin/env bash
# Stop a Firecracker microvm started by microvm/run.sh.

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUT="${ROOT}/out"
PID_FILE="${OUT}/firecracker.pid"
PASTA_PID_FILE="${OUT}/pasta.pid"
HOST_BRIDGE_PID_FILE="${OUT}/host-bridge.pid"
API_SOCK="${API_SOCK:-${OUT}/firecracker.sock}"

if [[ -S "${API_SOCK}" ]]; then
	curl --unix-socket "${API_SOCK}" -s -X PUT "http://localhost/actions" \
		-H "Content-Type: application/json" \
		-d '{"action_type": "SendCtrlAltDel"}' >/dev/null 2>&1 || true
	sleep 0.5 || true
fi

if [[ -f "${HOST_BRIDGE_PID_FILE}" ]]; then
	hpid="$(cat "${HOST_BRIDGE_PID_FILE}" 2>/dev/null || true)"
	if [[ -n "${hpid}" ]] && kill -0 "${hpid}" 2>/dev/null; then
		kill "${hpid}" 2>/dev/null || true
		sleep 0.2 || true
		kill -9 "${hpid}" 2>/dev/null || true
	fi
	rm -f "${HOST_BRIDGE_PID_FILE}"
fi

if [[ -f "${PID_FILE}" ]]; then
	pid="$(cat "${PID_FILE}" 2>/dev/null || true)"
	if [[ -n "${pid}" ]] && kill -0 "${pid}" 2>/dev/null; then
		kill "${pid}" 2>/dev/null || true
		for _ in 1 2 3 4 5; do
			kill -0 "${pid}" 2>/dev/null || break
			sleep 0.2
		done
		kill -9 "${pid}" 2>/dev/null || true
	fi
	rm -f "${PID_FILE}"
fi

if [[ -f "${PASTA_PID_FILE}" ]]; then
	ppid="$(cat "${PASTA_PID_FILE}" 2>/dev/null || true)"
	if [[ -n "${ppid}" ]] && kill -0 "${ppid}" 2>/dev/null; then
		kill "${ppid}" 2>/dev/null || true
		sleep 0.2 || true
		kill -9 "${ppid}" 2>/dev/null || true
	fi
	rm -f "${PASTA_PID_FILE}"
fi

rm -f "${API_SOCK}" "${OUT}/vsock.sock" "${OUT}/vsock.sock_"* 2>/dev/null || true
echo "stopped"
