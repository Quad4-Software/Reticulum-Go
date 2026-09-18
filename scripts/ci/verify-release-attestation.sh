#!/bin/sh
# Verify a cosign SLSA bundle for a release binary using the repository public key.
# Usage: verify-release-attestation.sh <blob-file> <bundle-file>
# Env: COSIGN_PUBLIC_KEY (default cosign.pub)
set -eu

BLOB="${1:?blob path}"
BUNDLE="${2:?bundle path}"
PUB="${COSIGN_PUBLIC_KEY:-cosign.pub}"

if [ ! -f "$PUB" ]; then
    echo "Missing $PUB (generate a key pair with cosign and commit the .pub file)" >&2
    exit 1
fi

# Bundles produced by attest-release-assets.sh include an RFC 3161 timestamp
# from COSIGN_TSA_URL. cosign verifies it against the TSA certificate chain
# embedded in the bundle. --insecure-ignore-tlog only skips Rekor log checks
# because attestations are signed with a static key.
exec cosign verify-blob-attestation \
    --insecure-ignore-tlog \
    --key "$PUB" \
    --bundle "$BUNDLE" \
    --type slsaprovenance1 \
    "$BLOB"
