#!/bin/sh
# SPDX-License-Identifier: LicenseRef-Reticulum
set -eu
ROOT="$(CDPATH='' cd -- "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
task build-libmf
task build-liblxmf
task build-liblxst
go test -count=1 ./pkg/libmf/... ./pkg/liblxmf/... ./pkg/liblxst/...
make -C bindings/python test
make -C bindings/python examples/lxmf-roundtrip
make -C bindings/python examples/lxst-roundtrip
make -C bindings/java examples/lxmf-roundtrip
if command -v kotlinc >/dev/null 2>&1; then
	make -C bindings/kotlin examples/lxmf-roundtrip
fi
make -C bindings/c examples/lxmf-smoke
make -C bindings/c examples/lxmf-interop
make -C bindings/c examples/lxst-smoke
make -C bindings/cpp test
