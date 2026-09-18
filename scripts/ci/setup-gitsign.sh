#!/bin/sh
# Configure git commit signing via gitsign (keyless x509 signatures).
#
# Defaults point at the public Sigstore instance. To sign against a private
# Fulcio/Rekor backed by your own OIDC issuer (for example auth.quad4.io),
# override the SIGSTORE_* variables:
#
#   SIGSTORE_FULCIO_URL=https://fulcio.quad4.io \
#   SIGSTORE_REKOR_URL=https://rekor.quad4.io \
#   SIGSTORE_OIDC_ISSUER=https://auth.quad4.io \
#   SIGSTORE_OIDC_CLIENT_ID=reticulum-go \
#   SIGSTORE_ROOT_FILE=/path/to/sigstore-root.json \
#   sh scripts/ci/setup-gitsign.sh
#
# The public Fulcio CA only federates a fixed issuer allowlist (GitHub,
# Google, Microsoft, oauth2.sigstore.dev). A private issuer requires running
# your own Fulcio with the issuer in its OIDC config.
#
# Env: GITSIGN_SCOPE=local (default, this repo only) or global.
set -eu

if ! command -v gitsign >/dev/null 2>&1; then
	echo "setup-gitsign: gitsign not on PATH" >&2
	echo "setup-gitsign: install from https://github.com/sigstore/gitsign" >&2
	exit 1
fi

SCOPE="${GITSIGN_SCOPE:-local}"
case "$SCOPE" in
local | global) ;;
*)
	echo "setup-gitsign: GITSIGN_SCOPE must be local or global" >&2
	exit 1
	;;
esac

FULCIO="${SIGSTORE_FULCIO_URL:-https://fulcio.sigstore.dev}"
REKOR="${SIGSTORE_REKOR_URL:-https://rekor.sigstore.dev}"
ISSUER="${SIGSTORE_OIDC_ISSUER:-https://oauth2.sigstore.dev/auth}"
CLIENT_ID="${SIGSTORE_OIDC_CLIENT_ID:-sigstore}"

flag="--$SCOPE"

git config "$flag" commit.gpgsign true
git config "$flag" tag.gpgsign true
git config "$flag" gpg.format x509
git config "$flag" gpg.x509.program gitsign
git config "$flag" gitsign.fulcio "$FULCIO"
git config "$flag" gitsign.rekor "$REKOR"
git config "$flag" gitsign.issuer "$ISSUER"
git config "$flag" gitsign.clientID "$CLIENT_ID"

if [ -n "${SIGSTORE_ROOT_FILE:-}" ]; then
	git config "$flag" gitsign.rekorRoot "$SIGSTORE_ROOT_FILE"
	echo "setup-gitsign: custom trust root $SIGSTORE_ROOT_FILE"
fi

echo "setup-gitsign: configured ($SCOPE)"
echo "  fulcio: $FULCIO"
echo "  rekor:  $REKOR"
echo "  issuer: $ISSUER"
