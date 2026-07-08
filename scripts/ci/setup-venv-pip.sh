#!/bin/sh
# Create a Python venv in the workspace and pip install packages; expose bin on PATH for later steps.
# Usage: setup-venv-pip.sh <pip_package> [more packages...]
set -eu

WS="${GITEA_WORKSPACE:-${GITHUB_WORKSPACE:-.}}"
cd "$WS" || exit 1

python_bin() {
	if command -v python3 >/dev/null 2>&1; then
		echo python3
	elif command -v python >/dev/null 2>&1; then
		echo python
	else
		echo "setup-venv-pip: python not found" >&2
		return 1
	fi
}

venv_activate() {
	case "$(uname -s)" in
	MINGW* | MSYS* | CYGWIN*)
		echo ".venv/Scripts/activate"
		;;
	*)
		echo ".venv/bin/activate"
		;;
	esac
}

venv_bin_dir() {
	case "$(uname -s)" in
	MINGW* | MSYS* | CYGWIN*)
		echo "$WS/.venv/Scripts"
		;;
	*)
		echo "$WS/.venv/bin"
		;;
	esac
}

PYTHON="$(python_bin)"
ACTIVATE="$(venv_activate)"
BINDIR="$(venv_bin_dir)"

"$PYTHON" -m venv .venv
# shellcheck disable=SC1090
. "$ACTIVATE"
"$PYTHON" -m pip install --upgrade pip
"$PYTHON" -m pip install "$@"
deactivate

if [ -n "${GITHUB_PATH:-}" ]; then
	echo "$BINDIR" >> "$GITHUB_PATH"
fi
if [ -n "${GITEA_PATH:-}" ]; then
	echo "$BINDIR" >> "$GITEA_PATH"
fi
