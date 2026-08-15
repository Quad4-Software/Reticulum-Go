#!/usr/bin/env sh
# Coverage-guided fuzz: baseline unit coverage, targeted fuzz by coverage gaps, recheck.
set -eu

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

FUZZTIME="${FUZZTIME:-10s}"
LOW_COV_FUZZTIME="${LOW_COV_FUZZTIME:-20s}"
LOW_COV_THRESHOLD="${LOW_COV_THRESHOLD:-70}"
COVER_DIR="${COVER_DIR:-.cache/fuzz-cover}"
mkdir -p "$COVER_DIR"

UNIT_COVER="$COVER_DIR/unit-before.out"
UNIT_AFTER="$COVER_DIR/unit-after.out"

PACKAGES="./pkg/packet ./pkg/transport ./pkg/identity ./pkg/link ./pkg/ifac ./pkg/backbone ./pkg/discovery ./pkg/blackhole ./pkg/librns ./pkg/announce ./pkg/destination ./pkg/resource ./pkg/buffer ./pkg/channel ./pkg/interfaces ./pkg/cryptography ./pkg/pageserver ./pkg/protect"

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
		if ! go test -short -count=1 -timeout 8m -coverprofile="$partial" -covermode=atomic "$pkg" >>"$log" 2>&1; then
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
	# -run=^$ skips the package unit suite so each target is fuzz-only.
	# Route through testsummary so CI only shows passing "ok" lines and,
	# on failure, the full crash/failure details (TESTSUMMARY_QUIET=1 in CI).
	# Anchor the name so FuzzFoo does not also match FuzzFooBar.
	go run ./scripts/ci/testsummary -run='^$' -fuzz="^${target}$" -fuzztime="$ftime" -timeout 5m "$pkg"
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
run_fuzz ./pkg/packet FuzzReadPCAPUDPPayloads "$(fuzz_time_for packet 15s)"
run_fuzz ./pkg/transport FuzzDecodePathTableEntries "$(fuzz_time_for transport 25s)"
run_fuzz ./pkg/transport FuzzShouldUpdateAnnouncePath "$(fuzz_time_for transport 15s)"
run_fuzz ./pkg/transport FuzzHandlePathRequest "$(fuzz_time_for transport 15s)"
run_fuzz ./pkg/transport FuzzProcessPathRequest "$(fuzz_time_for transport 15s)"
run_fuzz ./pkg/transport FuzzParsePathRequestWireExploratory "$(fuzz_time_for transport 10s)"
run_fuzz ./pkg/identity FuzzDecodeKnownDestinations "$(fuzz_time_for identity 25s)"
run_fuzz ./pkg/identity FuzzIdentitySignVerify "$(fuzz_time_for identity 15s)"
run_fuzz ./pkg/link FuzzLinkHandleData "$(fuzz_time_for link 20s)"
run_fuzz ./pkg/link FuzzLinkHandleInbound "$(fuzz_time_for link 15s)"
run_fuzz ./pkg/link FuzzSelectRequestedPartIndexesExploratory "$(fuzz_time_for link 10s)"
run_fuzz ./pkg/link FuzzSplitResourceMetadataExploratory "$(fuzz_time_for link 10s)"
run_fuzz ./pkg/announce FuzzHandleAnnounce "$(fuzz_time_for announce 15s)"
run_fuzz ./pkg/destination FuzzParseName "$(fuzz_time_for destination 10s)"
run_fuzz ./pkg/resource FuzzUnpackResourceAdvertisement "$(fuzz_time_for resource 15s)"
run_fuzz ./pkg/resource FuzzPrepareOutboundForLinkLayoutExploratory "$(fuzz_time_for resource 15s)"
run_fuzz ./pkg/cryptography FuzzRemovePKCS7PaddingExploratory "$(fuzz_time_for cryptography 10s)"
run_fuzz ./pkg/pageserver FuzzParseNodeStatusAppDataExploratory "$(fuzz_time_for pageserver 10s)"
run_fuzz ./pkg/ifac FuzzUnmask "$(fuzz_time_for ifac 15s)"
run_fuzz ./pkg/ifac FuzzMaskRoundTrip "$(fuzz_time_for ifac 15s)"
run_fuzz ./pkg/backbone FuzzHDLCDecoderFeed "$(fuzz_time_for backbone 15s)"
run_fuzz ./pkg/discovery FuzzDecodeAppData "$(fuzz_time_for discovery 10s)"
run_fuzz ./pkg/discovery FuzzEncodeDecodeInfoRoundTrip "$(fuzz_time_for discovery 10s)"
run_fuzz ./pkg/discovery FuzzStampValid "$(fuzz_time_for discovery 5s)"
run_fuzz ./pkg/blackhole FuzzDecodeBlackholeMap "$(fuzz_time_for blackhole 10s)"
run_fuzz ./pkg/buffer FuzzHandleMessageEOFExploratory "$(fuzz_time_for buffer 10s)"
run_fuzz ./pkg/channel FuzzHandleInboundEnvelopeExploratory "$(fuzz_time_for channel 10s)"
run_fuzz ./pkg/channel FuzzPackHandleInboundRoundTrip "$(fuzz_time_for channel 10s)"
run_fuzz ./pkg/interfaces FuzzKISSStreamDecoderRoundTrip "$(fuzz_time_for interfaces 10s)"
run_fuzz ./pkg/protect FuzzParseMode "$(fuzz_time_for protect 10s)"
run_fuzz ./pkg/protect FuzzAdmitPacket "$(fuzz_time_for protect 10s)"
run_fuzz ./pkg/protect FuzzPeekPacketClass "$(fuzz_time_for protect 5s)"
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
