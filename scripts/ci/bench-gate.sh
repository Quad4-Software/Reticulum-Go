#!/usr/bin/env sh
# Run load/concurrency benchmarks and fail when results exceed thresholds.
# Usage: bench-gate.sh
set -eu

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

THRESHOLDS="${BENCH_THRESHOLDS:-$ROOT/scripts/ci/bench-thresholds.tsv}"
COUNT="${BENCH_COUNT:-3}"
BENCHTIME="${BENCH_TIME:-1s}"
TIMEOUT="${BENCH_TIMEOUT:-12m}"
PACKAGES="./pkg/packet ./pkg/backbone ./pkg/transport ./pkg/identity ./pkg/cryptography ./pkg/ifac ./pkg/interfaces"
BENCH_RE='BenchmarkPacketThroughput|BenchmarkHubEcho|BenchmarkSimConcurrentLineRelay|BenchmarkSimLineRelayThroughput|BenchmarkHasPath_ParallelMixed|BenchmarkHandleAnnouncePacket_DebugCritical|BenchmarkSimIFACMaskUnmask|BenchmarkIdentityHashCached|BenchmarkDeriveKey|BenchmarkRememberUnchanged|BenchmarkTCPHDLCDecoderBurst|BenchmarkHandlePacketCopy'

if [ ! -f "$THRESHOLDS" ]; then
	echo "bench-gate: thresholds file not found: $THRESHOLDS" >&2
	exit 1
fi

OUT="$(mktemp)"
trap 'rm -f "$OUT"' EXIT

echo "bench-gate: timeout=$TIMEOUT benchtime=$BENCHTIME count=$COUNT"
go test -p 1 -timeout "$TIMEOUT" -run=^$ -bench="$BENCH_RE" -benchmem -benchtime="$BENCHTIME" -count="$COUNT" $PACKAGES >"$OUT" 2>&1 || {
	cat "$OUT"
	exit 1
}
cat "$OUT"

awk -v thresholds="$THRESHOLDS" '
function median(vals, n,    i, j, tmp) {
	for (i = 1; i <= n; i++) {
		for (j = i + 1; j <= n; j++) {
			if (vals[i] > vals[j]) {
				tmp = vals[i]; vals[i] = vals[j]; vals[j] = tmp
			}
		}
	}
	if (n % 2 == 1) return vals[(n + 1) / 2]
	return (vals[n / 2] + vals[n / 2 + 1]) / 2
}
BEGIN {
	while ((getline line < thresholds) > 0) {
		if (line ~ /^#/ || line ~ /^[[:space:]]*$/) continue
		split(line, f, "\t")
		pat[f[1]] = f[2]
		allocPat[f[1]] = f[3]
	}
	close(thresholds)
}
/^Benchmark/ {
	name = $1
	gsub(/-[0-9]+$/, "", name)
	nsVal = 0
	allocVal = 0
	for (i = 1; i <= NF; i++) {
		if ($i == "ns/op") nsVal = $(i - 1) + 0
		if ($i == "allocs/op") allocVal = $(i - 1) + 0
	}
	ns[name, ++nsCount[name]] = nsVal
	alloc[name, ++allocCount[name]] = allocVal
}
END {
	failed = 0
	for (name in nsCount) {
		n = nsCount[name]
		if (n == 0) continue
		for (i = 1; i <= n; i++) {
			nsVals[i] = ns[name, i]
			allocVals[i] = alloc[name, i]
		}
		gotNS = median(nsVals, n)
		gotAlloc = median(allocVals, n)
		limitNS = 0
		limitAlloc = 0
		matched = ""
		for (patName in pat) {
			if (name ~ patName) {
				limitNS = pat[patName]
				limitAlloc = allocPat[patName]
				matched = patName
			}
		}
		if (matched == "") continue
		if (gotNS > limitNS) {
			printf "FAIL %s: %.0f ns/op > %d ns/op\n", name, gotNS, limitNS
			failed = 1
		} else {
			printf "OK   %s: %.0f ns/op (limit %d)\n", name, gotNS, limitNS
		}
		if (gotAlloc > limitAlloc) {
			printf "FAIL %s: %.0f allocs/op > %d allocs/op\n", name, gotAlloc, limitAlloc
			failed = 1
		} else {
			printf "OK   %s: %.0f allocs/op (limit %d)\n", name, gotAlloc, limitAlloc
		}
	}
	if (failed) exit 1
}
' "$OUT"

echo "bench-gate: all thresholds satisfied"
