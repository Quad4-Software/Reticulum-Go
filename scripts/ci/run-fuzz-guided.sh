#!/usr/bin/env sh
# Coverage-guided fuzz: baseline unit coverage, targeted fuzz by coverage gaps, recheck.
set -eu

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

FUZZTIME="${FUZZTIME:-20s}"
LOW_COV_FUZZTIME="${LOW_COV_FUZZTIME:-35s}"
LOW_COV_THRESHOLD="${LOW_COV_THRESHOLD:-70}"
COVER_DIR="${COVER_DIR:-.cache/fuzz-cover}"
mkdir -p "$COVER_DIR"

UNIT_COVER="$COVER_DIR/unit-before.out"
UNIT_AFTER="$COVER_DIR/unit-after.out"

PACKAGES="./pkg/packet ./pkg/transport ./pkg/identity ./pkg/link ./pkg/ifac ./pkg/backbone ./pkg/discovery ./pkg/blackhole ./pkg/librns"

package_low_coverage() {
	pkg="$1"
	pct="$(go tool cover -func="$UNIT_COVER" | awk -v p="$pkg" -F'\t' '
		$1 ~ p && $1 ~ /\.go:/ {gsub(/.*\//, "", $1); sum+=$3; n++}
		END { if (n>0) printf "%.1f", sum/n; else print "100" }
	')"
	awk -v v="$pct" -v t="$LOW_COV_THRESHOLD" 'BEGIN { exit !(v < t) }'
}

merge_cover_profile() {
	dst="$1"
	src="$2"
	if [ ! -f "$dst" ]; then
		cp "$src" "$dst"
		return
	fi
	tail -n +2 "$src" >> "$dst"
}

collect_unit_coverage() {
	out="$1"
	log="$2"
	rm -f "$out"
	for pkg in $PACKAGES; do
		partial="$COVER_DIR/$(echo "$pkg" | tr './' '_').out"
		echo "fuzz-guided: coverage $pkg" >>"$log"
		if ! go test -coverprofile="$partial" -covermode=atomic "$pkg" >>"$log" 2>&1; then
			return 1
		fi
		merge_cover_profile "$out" "$partial"
	done
	return 0
}

echo "fuzz-guided: collecting unit-test coverage"
if ! collect_unit_coverage "$UNIT_COVER" "$COVER_DIR/unit-before.log"; then
	tail -80 "$COVER_DIR/unit-before.log" >&2
	exit 1
fi
echo "fuzz-guided: before"
go tool cover -func="$UNIT_COVER" | tail -1

run_fuzz() {
	pkg="$1"
	target="$2"
	ftime="$3"
	echo "fuzz-guided: $target ($ftime) in $pkg"
	go test -fuzz="$target" -fuzztime="$ftime" "$pkg"
}

fuzz_time_for() {
	pkg="$1"
	default="$2"
	if package_low_coverage "$pkg"; then
		echo "$LOW_COV_FUZZTIME"
	else
		echo "$default"
	fi
}

run_fuzz ./pkg/packet FuzzPacketUnpack "$(fuzz_time_for packet "$FUZZTIME")"
run_fuzz ./pkg/packet FuzzPacketRoundTrip "$(fuzz_time_for packet 15s)"
run_fuzz ./pkg/transport FuzzDecodePathTableEntries "$(fuzz_time_for transport 25s)"
run_fuzz ./pkg/transport FuzzShouldUpdateAnnouncePath "$(fuzz_time_for transport 15s)"
run_fuzz ./pkg/identity FuzzDecodeKnownDestinations "$(fuzz_time_for identity 25s)"
run_fuzz ./pkg/identity FuzzIdentitySignVerify "$(fuzz_time_for identity 15s)"
run_fuzz ./pkg/link FuzzLinkHandleData "$(fuzz_time_for link 20s)"
run_fuzz ./pkg/ifac FuzzUnmask "$(fuzz_time_for ifac 15s)"
run_fuzz ./pkg/ifac FuzzMaskRoundTrip "$(fuzz_time_for ifac 15s)"
run_fuzz ./pkg/backbone FuzzHDLCDecoderFeed "$(fuzz_time_for backbone 15s)"
run_fuzz ./pkg/discovery FuzzDecodeAppData "$(fuzz_time_for discovery 10s)"
run_fuzz ./pkg/blackhole FuzzDecodeBlackholeMap "$(fuzz_time_for blackhole 10s)"
run_fuzz ./pkg/librns FuzzHandleTable "$(fuzz_time_for librns 10s)"
run_fuzz ./pkg/librns FuzzEventQueue "$(fuzz_time_for librns 10s)"
run_fuzz ./pkg/librns FuzzConfigPathCreate "$(fuzz_time_for librns 10s)"
run_fuzz ./pkg/librns FuzzValidatePath "$(fuzz_time_for librns 5s)"

echo "fuzz-guided: rechecking unit coverage"
if ! collect_unit_coverage "$UNIT_AFTER" "$COVER_DIR/unit-after.log"; then
	tail -80 "$COVER_DIR/unit-after.log" >&2
	exit 1
fi
echo "fuzz-guided: after"
go tool cover -func="$UNIT_AFTER" | tail -1
echo "fuzz-guided: done"
