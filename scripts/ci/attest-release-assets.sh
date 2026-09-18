#!/bin/sh
# Create SLSA v1 cosign bundle attestations next to each release binary under DIR.
# Requires: cosign on PATH; COSIGN_KEY_PATH to cosign private key PEM; COSIGN_PASSWORD
# if the key is encrypted. Run from repository root so scripts/ci/slsa-predicate.py resolves.
#
# Every bundle carries an RFC 3161 timestamp so signatures remain verifiable
# after the key rotates or expires. Default TSA is the public Sigstore service.
# Set COSIGN_TSA_URL=none to disable, or point it at a private TSA.
#
# Set COSIGN_REKOR_URL (for example https://rekor.sigstore.dev or a self-hosted
# instance) to also upload each attestation to a transparency log.
#
# Usage: attest-release-assets.sh <directory>
set -eu

DIR="${1:?directory}"
KEY="${COSIGN_KEY_PATH:?set COSIGN_KEY_PATH}"
TSA_URL="${COSIGN_TSA_URL:-https://tsa.sigstore.dev/api/v1/timestamp}"
REKOR_URL="${COSIGN_REKOR_URL:-}"

if [ ! -f "$KEY" ]; then
    echo "attest-release-assets.sh: missing key file $KEY" >&2
    exit 1
fi

PRED="$(mktemp "${TMPDIR:-/tmp}/slsa-pred.XXXXXX")"
trap 'rm -f "$PRED"' EXIT INT

python3 scripts/ci/slsa-predicate.py > "$PRED"

find "$DIR" -type f ! -name '*.sha256' ! -name '*.cosign.bundle' | while IFS= read -r f; do
    case "$f" in
        */.git/*) continue ;;
    esac
    echo "attest: $f"
    set -- attest-blob --yes \
        --key "$KEY" \
        --predicate "$PRED" \
        --type slsaprovenance1 \
        --bundle "${f}.cosign.bundle" \
        --use-signing-config=false
    if [ -n "$TSA_URL" ] && [ "$TSA_URL" != "none" ]; then
        set -- "$@" --timestamp-server-url "$TSA_URL"
    fi
    if [ -n "$REKOR_URL" ]; then
        set -- "$@" --rekor-url "$REKOR_URL"
    fi
    cosign "$@" "$f" >/dev/null
done

echo "attest-release-assets.sh: done"
