#!/bin/sh
# Shallow-clone upstream reference trees used by Python interop harnesses.
# Usage: clone-refs.sh
# Env:
#   LXMF_REF_REV          git ref for markqvist/LXMF (tag or 40-char SHA, default 1.2.0)
#   LXMF_REF_PYPI_SHA256  sha256 of the PyPI lxmf sdist, used when the git tag
#                         does not exist (e.g. 1.1.1 was a PyPI-only release)
#   RRC_REF_REV           git ref for kc1awv/rrcd (tag or 40-char SHA)
set -eu

ROOT="$(CDPATH='' cd -- "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

LXMF_REF_REV="${LXMF_REF_REV:-1.2.0}"
LXMF_REF_PYPI_SHA256="${LXMF_REF_PYPI_SHA256:-}"
RRC_REF_REV="${RRC_REF_REV:-f6d7e9d72bf83c70d7a9373ae8c5edaa052a7bf2}"

is_sha() {
	# 40 hex chars: a pinned commit.
	printf '%s' "$1" | grep -Eq '^[0-9a-fA-F]{40}$'
}

fetch_pypi_sdist() {
	pkg="$1"
	ver="$2"
	sha="$3"
	dest="$4"
	if [ -z "$sha" ]; then
		echo "clone-refs: no git tag $ver and no PyPI sha256 configured" >&2
		return 1
	fi
	tmp="$(mktemp -d)"
	trap 'rm -rf "$tmp"' EXIT
	json="$(curl -fsSL "https://pypi.org/pypi/${pkg}/${ver}/json")"
	url="$(printf '%s' "$json" | python3 -c '
import json, sys
d = json.load(sys.stdin)
print(next(u["url"] for u in d["urls"] if u["packagetype"] == "sdist"))
')"
	curl -fsSL "$url" -o "$tmp/pkg.tar.gz"
	actual="$(sha256sum "$tmp/pkg.tar.gz" | awk '{print $1}')"
	if [ "$actual" != "$sha" ]; then
		echo "clone-refs: sha256 mismatch for ${pkg}-${ver} sdist" >&2
		return 1
	fi
	mkdir -p "$tmp/extract"
	tar -xzf "$tmp/pkg.tar.gz" -C "$tmp/extract"
	mv "$tmp/extract/${pkg}-${ver}" "$dest"
	rm -rf "$tmp"
	trap - EXIT
	echo "clone-refs: $dest from PyPI sdist ${pkg}==${ver}"
}

clone_ref() {
	name="$1"
	url="$2"
	rev="$3"
	dest="$ROOT/$name"
	pypi_pkg="$4"
	pypi_sha="$5"
	if [ -d "$dest/.git" ] || [ -f "$dest/PKG-INFO" ]; then
		echo "clone-refs: $name already present"
		return 0
	fi
	rm -rf "$dest"
	if is_sha "$rev"; then
		git init "$dest"
		git -C "$dest" remote add origin "$url"
		git -C "$dest" fetch --depth 1 origin "$rev"
		git -C "$dest" checkout --detach FETCH_HEAD
	elif git clone --depth 1 --branch "$rev" "$url" "$dest" 2>/dev/null; then
		:
	elif [ -n "$pypi_pkg" ]; then
		fetch_pypi_sdist "$pypi_pkg" "$rev" "$pypi_sha" "$dest"
	else
		echo "clone-refs: failed to fetch $name at $rev" >&2
		return 1
	fi
	echo "clone-refs: $name at $rev"
}

clone_ref LXMF-ref https://github.com/markqvist/LXMF.git "$LXMF_REF_REV" lxmf "$LXMF_REF_PYPI_SHA256"
clone_ref RRC-ref https://github.com/kc1awv/rrcd.git "$RRC_REF_REV" "" ""

echo "clone-refs: OK"
