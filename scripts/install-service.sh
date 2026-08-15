#!/bin/sh
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io
#
# Install reticulum-go service files for systemd, openrc, runit, and/or dinit.
#
# Usage:
#   install-service.sh [--prefix DIR] [--destdir DIR] [--bindir DIR] [--init TYPE]
#
# INIT TYPE: auto (default), systemd, openrc, runit, dinit, all

set -eu

PREFIX="${PREFIX:-/usr/local}"
DESTDIR="${DESTDIR:-}"
BINDIR="${BINDIR:-}"
INIT="${INIT:-auto}"
STATE_DIR="/var/lib/reticulum-go"

ROOT="$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)"
PKG="$ROOT/packaging"

usage() {
	cat <<'EOF'
Usage: install-service.sh [options]

Options:
  --prefix DIR    Install prefix (default /usr/local, or PREFIX env)
  --destdir DIR   Staging root (default empty, or DESTDIR env)
  --bindir DIR    Binary directory (default PREFIX/bin, or BINDIR env)
  --init TYPE     auto|systemd|openrc|runit|dinit|all (default auto)
  -h, --help      Show this help
EOF
}

while [ $# -gt 0 ]; do
	case "$1" in
	--prefix)
		PREFIX="$2"
		shift 2
		;;
	--destdir)
		DESTDIR="$2"
		shift 2
		;;
	--bindir)
		BINDIR="$2"
		shift 2
		;;
	--init)
		INIT="$2"
		shift 2
		;;
	-h|--help)
		usage
		exit 0
		;;
	*)
		echo "unknown option: $1" >&2
		usage >&2
		exit 1
		;;
	esac
done

if [ -z "$BINDIR" ]; then
	BINDIR="${PREFIX}/bin"
fi

subst_bindir() {
	sed "s|@BINDIR@|${BINDIR}|g"
}

detect_init() {
	if [ -d /run/systemd/system ] && command -v systemctl >/dev/null 2>&1; then
		echo systemd
		return
	fi
	if command -v rc-service >/dev/null 2>&1 || command -v openrc >/dev/null 2>&1; then
		echo openrc
		return
	fi
	if [ -d /etc/sv ] || [ -d /var/service ] || command -v runsv >/dev/null 2>&1; then
		echo runit
		return
	fi
	if command -v dinitctl >/dev/null 2>&1 || [ -d /etc/dinit.d ]; then
		echo dinit
		return
	fi
	echo unknown
}

install_state() {
	mkdir -p "${DESTDIR}${STATE_DIR}/logfile"
	chmod 700 "${DESTDIR}${STATE_DIR}" "${DESTDIR}${STATE_DIR}/logfile" 2>/dev/null || true
	if [ ! -f "${DESTDIR}${STATE_DIR}/config" ]; then
		install -m 600 "$PKG/reticulum-go.config" "${DESTDIR}${STATE_DIR}/config"
		echo "Installed sample config to ${DESTDIR}${STATE_DIR}/config"
	fi
}

install_systemd() {
	unit_dir="${DESTDIR}/usr/lib/systemd/system"
	mkdir -p "$unit_dir"
	subst_bindir <"$PKG/systemd/reticulum-go.service" >"$unit_dir/reticulum-go.service"
	chmod 644 "$unit_dir/reticulum-go.service"
	example_dir="${DESTDIR}/usr/share/doc/reticulum-go"
	mkdir -p "$example_dir"
	install -m 644 "$PKG/systemd/user.conf.example" "$example_dir/reticulum-go.user.conf.example"
	echo "Installed systemd unit to $unit_dir/reticulum-go.service"
	echo "Optional User= drop-in example: $example_dir/reticulum-go.user.conf.example"
	if [ -z "$DESTDIR" ] && command -v systemctl >/dev/null 2>&1; then
		systemctl daemon-reload || true
		echo "Enable with: systemctl enable --now reticulum-go"
	fi
}

install_openrc() {
	init_dir="${DESTDIR}/etc/init.d"
	mkdir -p "$init_dir"
	subst_bindir <"$PKG/openrc/reticulum-go" >"$init_dir/reticulum-go"
	chmod 755 "$init_dir/reticulum-go"
	echo "Installed OpenRC service to $init_dir/reticulum-go"
	if [ -z "$DESTDIR" ]; then
		echo "Enable with: rc-update add reticulum-go default && rc-service reticulum-go start"
	fi
}

install_runit() {
	sv_dir="${DESTDIR}/etc/sv/reticulum-go"
	mkdir -p "$sv_dir/log"
	subst_bindir <"$PKG/runit/reticulum-go/run" >"$sv_dir/run"
	subst_bindir <"$PKG/runit/reticulum-go/log/run" >"$sv_dir/log/run"
	chmod 755 "$sv_dir/run" "$sv_dir/log/run"
	echo "Installed runit service to $sv_dir"
	if [ -z "$DESTDIR" ]; then
		if [ -d /var/service ]; then
			echo "Enable with: ln -s /etc/sv/reticulum-go /var/service/"
		elif [ -d /etc/service ]; then
			echo "Enable with: ln -s /etc/sv/reticulum-go /etc/service/"
		else
			echo "Enable by linking $sv_dir into your runit service directory"
		fi
	fi
}

install_dinit() {
	dinit_dir="${DESTDIR}/etc/dinit.d"
	mkdir -p "$dinit_dir"
	subst_bindir <"$PKG/dinit/reticulum-go" >"$dinit_dir/reticulum-go"
	chmod 644 "$dinit_dir/reticulum-go"
	echo "Installed dinit service to $dinit_dir/reticulum-go"
	if [ -z "$DESTDIR" ]; then
		echo "Enable with: dinitctl enable reticulum-go && dinitctl start reticulum-go"
	fi
}

install_state

case "$INIT" in
auto)
	detected="$(detect_init)"
	if [ "$detected" = unknown ]; then
		echo "could not detect init system (set --init systemd|openrc|runit|dinit|all)" >&2
		exit 1
	fi
	echo "Detected init: $detected"
	INIT="$detected"
	;;
esac

case "$INIT" in
systemd) install_systemd ;;
openrc) install_openrc ;;
runit) install_runit ;;
dinit) install_dinit ;;
all)
	install_systemd
	install_openrc
	install_runit
	install_dinit
	;;
*)
	echo "unknown init type: $INIT" >&2
	exit 1
	;;
esac
