#!/usr/bin/env bash
# One-shot Firecracker microvm bring-up.
#
# Requires: firecracker on PATH, readable /dev/kvm
# Optional: pasta when NET=1
#
# Usage:
#   ./microvm/up.sh
#   ./microvm/up.sh --guest-only
#   ./microvm/up.sh --rebuild
#   ./microvm/stop.sh

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUT="${ROOT}/out"
GUEST_ONLY=0
REBUILD=0

for arg in "$@"; do
	case "${arg}" in
	--guest-only) GUEST_ONLY=1 ;;
	--rebuild) REBUILD=1 ;;
	-h | --help)
		sed -n '2,12p' "$0" | sed 's/^# \{0,1\}//'
		exit 0
		;;
	*)
		echo "unknown argument: ${arg}" >&2
		exit 1
		;;
	esac
done

if ! command -v firecracker >/dev/null 2>&1; then
	echo "firecracker not found on PATH" >&2
	exit 1
fi
if [[ ! -r /dev/kvm ]]; then
	echo "/dev/kvm is not readable" >&2
	exit 1
fi

mkdir -p "${OUT}"

if [[ "${REBUILD}" == "1" || ! -f "${OUT}/vmlinux" ]]; then
	echo "==> fetching guest kernel"
	"${ROOT}/fetch-kernel.sh"
fi
if [[ "${REBUILD}" == "1" || ! -f "${OUT}/rootfs.ext4" ]]; then
	echo "==> building guest rootfs"
	"${ROOT}/build-rootfs.sh"
fi

"${ROOT}/stop.sh" >/dev/null 2>&1 || true

echo "==> starting microvm guest"
DETACH=1 NET="${NET:-0}" "${ROOT}/run.sh"

if [[ "${GUEST_ONLY}" == "1" ]]; then
	echo "guest running (guest-only). bridge later with: ${ROOT}/run-host-bridge.sh"
	echo "stop with: ${ROOT}/stop.sh"
	exit 0
fi

echo "==> starting host bridge"
# Wait briefly for guest vsock listen.
for _ in $(seq 1 50); do
	if [[ -S "${OUT}/vsock.sock" ]]; then
		break
	fi
	sleep 0.1
done
DETACH=1 "${ROOT}/run-host-bridge.sh"

echo
echo "microvm is up"
echo "  guest serial: ${OUT}/firecracker.stdout"
echo "  host bridge:  ${OUT}/host-bridge.log"
echo "  stop:         ${ROOT}/stop.sh"
echo
echo "Edit ${ROOT}/host-bridge.config to add TCP or Backbone hubs, then:"
echo "  ${ROOT}/stop.sh && ${ROOT}/up.sh"
