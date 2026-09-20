#!/bin/sh
# Reapply local vendor patches that `go mod vendor` deletes on regeneration.
#
# vendor/ is generated output, but a few upstream packages need local fixes
# that are not (yet) upstream. `go mod vendor` wipes them every time, so any
# vendoring run must be followed by this script. vendor-sync.sh calls it
# automatically; run it by hand after a manual `go mod vendor`.
#
# Usage: vendor-patches.sh [dir ...]
#   Each dir is a module root containing vendor/. Defaults to the repo root.
#
# With "check" as the first argument, verifies the patches are present and
# exits nonzero if `go mod vendor` wiped them.
set -eu

ROOT="$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)"

MODE="apply"
if [ "${1:-}" = "check" ]; then
	MODE="check"
	shift
fi
if [ $# -eq 0 ]; then
	set -- "$ROOT"
fi

# go.bug.st/serial lacks a linux/ppc64 (big-endian) specialbaudrate stub: the
# generic file needs unix.TCGETS2, which x/sys does not define for ppc64.
# Give ppc64 the same InvalidSpeed stub upstream uses for ppc64le.
serial_ppc64_apply() {
	vdir="$1/vendor/go.bug.st/serial"
	src="$vdir/serial_specialbaudrate_linux.go"
	[ -f "$src" ] || return 0
	sed -i 's|^//go:build linux && !ppc64le$|//go:build linux \&\& !ppc64le \&\& !ppc64|' "$src"
	cat > "$vdir/serial_specialbaudrate_linux_ppc64.go" <<'EOF'
//
// Copyright 2014-2026 Cristian Maglie. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//

package serial

func (port *unixPort) setSpecialBaudrate(speed uint32) error {
	// TODO: unimplemented
	return &PortError{code: InvalidSpeed}
}
EOF
}

serial_ppc64_check() {
	vdir="$1/vendor/go.bug.st/serial"
	src="$vdir/serial_specialbaudrate_linux.go"
	[ -f "$src" ] || return 0
	if ! grep -q '!ppc64$' "$src" || [ ! -f "$vdir/serial_specialbaudrate_linux_ppc64.go" ]; then
		echo "vendor-patches: $src missing ppc64 patch; run scripts/vendor-patches.sh" >&2
		return 1
	fi
}

rc=0
for dir in "$@"; do
	case "$MODE" in
	apply) serial_ppc64_apply "$dir" ;;
	check) serial_ppc64_check "$dir" || rc=1 ;;
	esac
done
exit "$rc"
