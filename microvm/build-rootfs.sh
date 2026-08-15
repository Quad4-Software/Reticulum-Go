#!/usr/bin/env bash
# Build a minimal ext4 rootfs with busybox + static reticulum-go.
#
# Usage:
#   ./microvm/build-rootfs.sh
#   ROOTFS_SIZE_MIB=256 ./microvm/build-rootfs.sh

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="$(cd "${ROOT}/.." && pwd)"
OUT="${ROOT}/out"
STAGING="${OUT}/rootfs-staging"
ROOTFS_IMG="${OUT}/rootfs.ext4"
ROOTFS_SIZE_MIB="${ROOTFS_SIZE_MIB:-256}"

ARCH="$(uname -m)"
case "${ARCH}" in
x86_64)
	GOARCH=amd64
	BUSYBOX_ARCH=x86_64
	;;
aarch64 | arm64)
	ARCH=aarch64
	GOARCH=arm64
	BUSYBOX_ARCH=aarch64
	;;
*)
	echo "unsupported arch: ${ARCH}" >&2
	exit 1
	;;
esac

mkdir -p "${OUT}"
rm -rf "${STAGING}"
mkdir -p \
	"${STAGING}/bin" \
	"${STAGING}/sbin" \
	"${STAGING}/usr/bin" \
	"${STAGING}/usr/sbin" \
	"${STAGING}/etc/reticulum" \
	"${STAGING}/etc/reticulum/storage" \
	"${STAGING}/etc/reticulum/storage/identities" \
	"${STAGING}/proc" \
	"${STAGING}/sys" \
	"${STAGING}/dev" \
	"${STAGING}/tmp" \
	"${STAGING}/run" \
	"${STAGING}/var/log" \
	"${STAGING}/root"

BUSYBOX_URL_X86_64="${BUSYBOX_URL:-https://busybox.net/downloads/binaries/1.35.0-x86_64-linux-musl/busybox}"
BUSYBOX_CACHE="${OUT}/busybox-${BUSYBOX_ARCH}"

resolve_busybox() {
	if [[ -n "${BUSYBOX_BIN:-}" ]]; then
		if [[ ! -x "${BUSYBOX_BIN}" ]]; then
			echo "BUSYBOX_BIN is not executable: ${BUSYBOX_BIN}" >&2
			exit 1
		fi
		echo "${BUSYBOX_BIN}"
		return
	fi
	for candidate in /usr/lib/nix/busybox /usr/bin/busybox /bin/busybox; do
		if [[ -x "${candidate}" ]] && file "${candidate}" | grep -q 'statically linked'; then
			# Prefer a static binary that matches the guest arch when possible.
			if file "${candidate}" | grep -qE "x86-64|x86_64" && [[ "${GOARCH}" == "amd64" ]]; then
				echo "${candidate}"
				return
			fi
			if file "${candidate}" | grep -qE "aarch64|ARM aarch64" && [[ "${GOARCH}" == "arm64" ]]; then
				echo "${candidate}"
				return
			fi
		fi
	done
	if [[ "${GOARCH}" == "amd64" ]]; then
		if [[ ! -x "${BUSYBOX_CACHE}" ]]; then
			echo "downloading busybox (x86_64 musl static)"
			curl -fL --progress-bar -o "${BUSYBOX_CACHE}.partial" "${BUSYBOX_URL_X86_64}"
			mv -f "${BUSYBOX_CACHE}.partial" "${BUSYBOX_CACHE}"
			chmod +x "${BUSYBOX_CACHE}"
		fi
		echo "${BUSYBOX_CACHE}"
		return
	fi
	echo "no static busybox found for ${ARCH}. set BUSYBOX_BIN to a static busybox path." >&2
	exit 1
}

BUSYBOX_SRC="$(resolve_busybox)"
echo "using busybox: ${BUSYBOX_SRC}"
cp -f "${BUSYBOX_SRC}" "${STAGING}/bin/busybox"
chmod 755 "${STAGING}/bin/busybox"

# Relative applet links only. busybox --install emits absolute host paths.
while IFS= read -r applet; do
	[[ -n "${applet}" ]] || continue
	ln -sfn busybox "${STAGING}/bin/${applet}"
done < <("${STAGING}/bin/busybox" --list)

ln -sfn ../bin/busybox "${STAGING}/sbin/reboot"
ln -sfn ../bin/busybox "${STAGING}/sbin/poweroff"
if [[ -e "${STAGING}/bin/ip" ]]; then
	ln -sfn ../bin/ip "${STAGING}/sbin/ip"
fi

echo "building static reticulum-go (${GOARCH})"
# Prefer module mode so a stale vendor/modules.txt cannot block guest builds.
CGO_ENABLED=0 GOOS=linux GOARCH="${GOARCH}" GOFLAGS=-mod=mod go build \
	-C "${REPO}" \
	-ldflags='-w -s -extldflags "-static"' \
	-o "${STAGING}/usr/bin/reticulum-go" \
	./cmd/reticulum-go
chmod 755 "${STAGING}/usr/bin/reticulum-go"

cp -f "${ROOT}/guest/reticulum.config" "${STAGING}/etc/reticulum/config"
cp -f "${ROOT}/guest/microvm-net" "${STAGING}/etc/microvm-net"
cp -f "${ROOT}/guest/init" "${STAGING}/init"
chmod 755 "${STAGING}/init"

# Minimal passwd so shells have a root identity.
cat >"${STAGING}/etc/passwd" <<'EOF'
root:x:0:0:root:/root:/bin/sh
EOF
cat >"${STAGING}/etc/group" <<'EOF'
root:x:0:
EOF

rm -f "${ROOTFS_IMG}"
truncate -s "${ROOTFS_SIZE_MIB}M" "${ROOTFS_IMG}"
mkfs.ext4 -q -F -d "${STAGING}" -L rns-guest "${ROOTFS_IMG}"

echo "wrote ${ROOTFS_IMG}"
ls -lh "${ROOTFS_IMG}"
echo "staging left at ${STAGING} (safe to delete)"
