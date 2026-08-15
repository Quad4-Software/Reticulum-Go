#!/usr/bin/env bash
# Boot reticulum-go inside Firecracker.
#
# Easiest path (fetch kernel, build rootfs, start guest + host bridge):
#   ./microvm/up.sh
#   make microvm-up
#
# Prerequisites:
#   firecracker on PATH, readable /dev/kvm
#   pasta on PATH when NET=1 (often unreliable inside an outer VM)
#
# Usage:
#   ./microvm/up.sh
#   NET=0 DETACH=1 ./microvm/run.sh
#   NET=1 ./microvm/run.sh
#   TAP_DEV=tap0 ./microvm/run.sh
#
# Nested VM: prefer NET=0 plus run-host-bridge.sh (what up.sh does).
# Guest listens on AF_VSOCK port 4242. Host CONNECT helper:
#   ./microvm/vsock-connect.sh

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUT="${ROOT}/out"
KERNEL="${KERNEL:-${OUT}/vmlinux}"
ROOTFS="${ROOTFS:-${OUT}/rootfs.ext4}"
API_SOCK="${API_SOCK:-${OUT}/firecracker.sock}"
VSOCK_UDS="${VSOCK_UDS:-${OUT}/vsock.sock}"
LOG_PATH="${LOG_PATH:-${OUT}/firecracker.log}"
CONFIG_PATH="${OUT}/vm-config.json"
PID_FILE="${OUT}/firecracker.pid"
PASTA_PID_FILE="${OUT}/pasta.pid"
GUEST_CID="${GUEST_CID:-3}"
VCPU_COUNT="${VCPU_COUNT:-1}"
MEM_MIB="${MEM_MIB:-256}"
FIRECRACKER_BIN="${FIRECRACKER_BIN:-firecracker}"
NET="${NET:-0}"
TAP_MAC="${TAP_MAC:-AA:FC:00:00:00:01}"
TAP_HOST_IP="${TAP_HOST_IP:-172.16.0.1}"
TAP_PREFIX="${TAP_PREFIX:-24}"

setup_pasta_tap() {
	local outif
	local tap="${1}"
	local br="${TAP_BRIDGE:-rnsbr0}"
	echo 1 >/proc/sys/net/ipv4/ip_forward 2>/dev/null || true
	for f in /proc/sys/net/ipv4/conf/*/rp_filter; do
		echo 0 >"${f}" 2>/dev/null || true
	done
	if ! ip link show "${br}" >/dev/null 2>&1; then
		ip link add name "${br}" type bridge
	fi
	if ! ip link show "${tap}" >/dev/null 2>&1; then
		ip tuntap add mode tap name "${tap}"
	fi
	ip link set "${tap}" nomaster 2>/dev/null || true
	ip addr flush dev "${tap}" 2>/dev/null || true
	ip addr flush dev "${br}" 2>/dev/null || true
	ip link set "${tap}" master "${br}"
	ip addr add "${TAP_HOST_IP}/${TAP_PREFIX}" dev "${br}"
	ip link set "${br}" up
	ip link set "${tap}" up
	outif="$(ip route show default | awk '{print $5; exit}')"
	if [[ -z "${outif}" ]]; then
		echo "no default route in pasta netns" >&2
		exit 1
	fi
	iptables -P FORWARD ACCEPT 2>/dev/null || true
	iptables -t nat -C POSTROUTING -s 172.16.0.0/24 -o "${outif}" -j MASQUERADE 2>/dev/null \
		|| iptables -t nat -A POSTROUTING -s 172.16.0.0/24 -o "${outif}" -j MASQUERADE
	iptables -C FORWARD -i "${br}" -o "${outif}" -j ACCEPT 2>/dev/null \
		|| iptables -A FORWARD -i "${br}" -o "${outif}" -j ACCEPT
	iptables -C FORWARD -i "${outif}" -o "${br}" -m state --state RELATED,ESTABLISHED -j ACCEPT 2>/dev/null \
		|| iptables -A FORWARD -i "${outif}" -o "${br}" -m state --state RELATED,ESTABLISHED -j ACCEPT
	echo "pasta bridge ${br} tap ${tap} ${TAP_HOST_IP}/${TAP_PREFIX} via ${outif}"
	ip -br link
	ip -br addr
}

if [[ ! -r /dev/kvm ]]; then
	echo "/dev/kvm is not readable. add your user to group kvm or run with sufficient privileges." >&2
	exit 1
fi
if [[ ! -f "${KERNEL}" ]]; then
	echo "missing kernel: ${KERNEL}" >&2
	echo "run: ${ROOT}/fetch-kernel.sh" >&2
	exit 1
fi
if [[ ! -f "${ROOTFS}" ]]; then
	echo "missing rootfs: ${ROOTFS}" >&2
	echo "run: ${ROOT}/build-rootfs.sh" >&2
	exit 1
fi
if ! command -v "${FIRECRACKER_BIN}" >/dev/null 2>&1; then
	echo "firecracker not found on PATH" >&2
	exit 1
fi

# Rootless clearnet: re-exec under pasta and create a TAP for Firecracker.
if [[ -z "${TAP_DEV:-}" && "${NET}" != "0" ]]; then
	if [[ "${RNS_MICROVM_IN_PASTA:-}" != "1" ]]; then
		if ! command -v pasta >/dev/null 2>&1; then
			echo "pasta not found. install passt/pasta or set NET=0 / TAP_DEV=..." >&2
			exit 1
		fi
		mkdir -p "${OUT}"
		export RNS_MICROVM_IN_PASTA=1
		export TAP_DEV="${TAP_NAME:-rns0}"
		if [[ "${DETACH:-0}" == "1" ]]; then
			# Keep pasta alive while firecracker runs.
			pasta -f --config-net -- env DETACH=0 RNS_MICROVM_IN_PASTA=1 TAP_DEV="${TAP_DEV}" \
				"${ROOT}/run.sh" \
				>"${OUT}/pasta.stdout" 2>"${OUT}/pasta.stderr" &
			echo $! >"${PASTA_PID_FILE}"
			for _ in $(seq 1 50); do
				if [[ -f "${PID_FILE}" ]]; then
					break
				fi
				sleep 0.1
			done
			if [[ ! -f "${PID_FILE}" ]]; then
				echo "firecracker failed to start under pasta (see ${OUT}/pasta.stderr)" >&2
				"${ROOT}/stop.sh" >/dev/null 2>&1 || true
				exit 1
			fi
			echo "detached pasta pid $(cat "${PASTA_PID_FILE}") firecracker pid $(cat "${PID_FILE}")"
			exit 0
		fi
		exec pasta -f --config-net -- env RNS_MICROVM_IN_PASTA=1 TAP_DEV="${TAP_DEV}" "${ROOT}/run.sh"
	fi
	TAP_DEV="${TAP_DEV:-rns0}"
	setup_pasta_tap "${TAP_DEV}"
fi

if [[ -f "${PID_FILE}" ]]; then
	old_pid="$(cat "${PID_FILE}" 2>/dev/null || true)"
	if [[ -n "${old_pid}" ]] && kill -0 "${old_pid}" 2>/dev/null; then
		echo "firecracker already running (pid ${old_pid}). stop with ${ROOT}/stop.sh" >&2
		exit 1
	fi
	rm -f "${PID_FILE}"
fi

rm -f "${API_SOCK}" "${VSOCK_UDS}" "${VSOCK_UDS}_"* "${CONFIG_PATH}"
mkdir -p "${OUT}"
: >"${LOG_PATH}"

BOOT_ARGS="console=ttyS0 reboot=k panic=1 pci=off nomodules root=/dev/vda rw init=/init"

# Build Firecracker JSON config. Kept under out/ (gitignored).
python3 - "${CONFIG_PATH}" "${KERNEL}" "${ROOTFS}" "${BOOT_ARGS}" "${VCPU_COUNT}" "${MEM_MIB}" "${GUEST_CID}" "${VSOCK_UDS}" "${LOG_PATH}" "${TAP_DEV:-}" "${TAP_MAC}" <<'PY'
import json
import sys

(
    config_path,
    kernel,
    rootfs,
    boot_args,
    vcpu_count,
    mem_mib,
    guest_cid,
    vsock_uds,
    log_path,
    tap_dev,
    tap_mac,
) = sys.argv[1:]

cfg = {
    "boot-source": {
        "kernel_image_path": kernel,
        "boot_args": boot_args,
    },
    "drives": [
        {
            "drive_id": "rootfs",
            "path_on_host": rootfs,
            "is_root_device": True,
            "is_read_only": False,
        }
    ],
    "machine-config": {
        "vcpu_count": int(vcpu_count),
        "mem_size_mib": int(mem_mib),
        "smt": False,
    },
    "vsock": {
        "guest_cid": int(guest_cid),
        "uds_path": vsock_uds,
    },
    "logger": {
        "log_path": log_path,
        "level": "Info",
        "show_level": True,
        "show_log_origin": False,
    },
}

if tap_dev:
    cfg["network-interfaces"] = [
        {
            "iface_id": "eth0",
            "guest_mac": tap_mac,
            "host_dev_name": tap_dev,
        }
    ]

with open(config_path, "w", encoding="utf-8") as f:
    json.dump(cfg, f, indent=2)
    f.write("\n")
PY

echo "starting firecracker"
echo "  kernel=${KERNEL}"
echo "  rootfs=${ROOTFS}"
echo "  guest_cid=${GUEST_CID}"
echo "  vsock_uds=${VSOCK_UDS}"
echo "  api_sock=${API_SOCK}"
echo "  log=${LOG_PATH}"
if [[ -n "${TAP_DEV:-}" ]]; then
	echo "  tap=${TAP_DEV}"
	echo "  net=guest 172.16.0.2/24 via ${TAP_HOST_IP}"
fi
echo "connect from host: ${ROOT}/vsock-connect.sh"
echo "stop with: ${ROOT}/stop.sh"
echo

# Foreground by default so serial console is usable. DETACH=1 backgrounds.
if [[ "${DETACH:-0}" == "1" ]]; then
	"${FIRECRACKER_BIN}" \
		--api-sock "${API_SOCK}" \
		--config-file "${CONFIG_PATH}" \
		>"${OUT}/firecracker.stdout" 2>"${OUT}/firecracker.stderr" &
	echo $! >"${PID_FILE}"
	echo "detached pid $(cat "${PID_FILE}")"
else
	"${FIRECRACKER_BIN}" \
		--api-sock "${API_SOCK}" \
		--config-file "${CONFIG_PATH}" \
		>"${OUT}/firecracker.stdout" 2>"${OUT}/firecracker.stderr" &
	fc_pid=$!
	echo "${fc_pid}" >"${PID_FILE}"
	trap '${ROOT}/stop.sh >/dev/null 2>&1 || true' INT TERM
	# Follow serial output while firecracker runs.
	tail -n +1 -F "${OUT}/firecracker.stdout" &
	tail_pid=$!
	wait "${fc_pid}" || true
	kill "${tail_pid}" 2>/dev/null || true
	rm -f "${PID_FILE}"
fi
