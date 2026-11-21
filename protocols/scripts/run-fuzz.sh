#!/usr/bin/env bash
set -euo pipefail

GOCMD="${GOCMD:-go}"
FUZZTIME="${FUZZTIME:-15s}"
# Prefer vendored modules when vendor/ is present (airgapped / CI parity).
GOVENDORFLAGS=()
if [[ -d vendor ]]; then
	GOVENDORFLAGS=(-mod=vendor)
fi

for pkg in ./pkg/lxmf/... ./pkg/mf/... ./pkg/rrc/... ./internal/gorrcd/...; do
	while IFS= read -r name; do
		[[ -z "${name}" ]] && continue
		echo "fuzz ${name} ${pkg}"
		"${GOCMD}" test "${GOVENDORFLAGS[@]}" -run='^$' -fuzz="${name}" -fuzztime="${FUZZTIME}" "${pkg}"
	done < <("${GOCMD}" test "${GOVENDORFLAGS[@]}" -list . "${pkg}" 2>/dev/null | grep '^Fuzz' || true)
done
