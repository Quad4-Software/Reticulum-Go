#!/usr/bin/env bash
# Download a Firecracker CI guest kernel into microvm/out/vmlinux.
#
# Usage:
#   ./microvm/fetch-kernel.sh
#   KERNEL_SERIES=6.1 ./microvm/fetch-kernel.sh

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUT="${ROOT}/out"
mkdir -p "${OUT}"

ARCH="$(uname -m)"
case "${ARCH}" in
x86_64 | aarch64) ;;
arm64) ARCH=aarch64 ;;
*)
	echo "unsupported arch: ${ARCH}" >&2
	exit 1
	;;
esac

S3="https://s3.amazonaws.com/spec.ccfc.min"
KERNEL_SERIES="${KERNEL_SERIES:-6.1}"

echo "listing Firecracker CI artifact prefixes"
PREFIXES_XML="$(curl -fsSL "${S3}?list-type=2&prefix=firecracker-ci/&delimiter=/")"
# Prefer dated CI builds (YYYYMMDD-...), newest last.
CI_PREFIX="$(printf '%s\n' "${PREFIXES_XML}" \
	| grep -oE 'firecracker-ci/[0-9]{8}-[^/<]+/' \
	| sort \
	| tail -n1 || true)"

if [[ -z "${CI_PREFIX}" ]]; then
	echo "no dated firecracker-ci prefixes found" >&2
	exit 1
fi

echo "using prefix ${CI_PREFIX}${ARCH}/"
KEYS_XML="$(curl -fsSL "${S3}?list-type=2&prefix=${CI_PREFIX}${ARCH}/vmlinux-")"
# Match bare object keys only (no trailing punctuation from XML tags).
mapfile -t ALL_KEYS < <(printf '%s\n' "${KEYS_XML}" \
	| grep -oE "${CI_PREFIX}${ARCH}/vmlinux-[0-9]+\.[0-9]+\.[0-9]+" \
	| sort -V \
	| uniq)

if [[ "${#ALL_KEYS[@]}" -eq 0 ]]; then
	echo "no vmlinux keys under ${CI_PREFIX}${ARCH}/" >&2
	exit 1
fi

LATEST_KEY=""
for key in "${ALL_KEYS[@]}"; do
	base="$(basename "${key}")"
	case "${base}" in
	vmlinux-${KERNEL_SERIES}.*)
		LATEST_KEY="${key}"
		;;
	esac
done
if [[ -z "${LATEST_KEY}" ]]; then
	LATEST_KEY="${ALL_KEYS[-1]}"
	echo "no vmlinux-${KERNEL_SERIES}.* found, falling back to ${LATEST_KEY}"
fi

DEST="${OUT}/vmlinux"
TMP="${DEST}.partial"

echo "downloading ${S3}/${LATEST_KEY}"
curl -fL --progress-bar -o "${TMP}" "${S3}/${LATEST_KEY}"
mv -f "${TMP}" "${DEST}"
chmod a-w "${DEST}" || true

printf '%s\n' "${LATEST_KEY}" >"${OUT}/vmlinux.source"
echo "wrote ${DEST}"
ls -lh "${DEST}"
