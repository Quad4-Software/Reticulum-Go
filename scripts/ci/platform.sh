# shellcheck shell=sh
# Shared CI platform helpers. Source from scripts/ci/*.sh.

ci_platform_os() {
	case "$(uname -s)" in
	Linux) echo linux ;;
	Darwin) echo darwin ;;
	MINGW* | MSYS* | CYGWIN*) echo windows ;;
	*) uname -s | tr '[:upper:]' '[:lower:]' ;;
	esac
}

ci_platform_arch() {
	case "$(uname -m)" in
	x86_64 | amd64) echo amd64 ;;
	aarch64 | arm64) echo arm64 ;;
	*) uname -m ;;
	esac
}

ci_platform_id() {
	echo "$(ci_platform_os)-$(ci_platform_arch)"
}

ci_go_no_telemetry_root() {
	if [ -n "${CI_GO_NO_TELEMETRY_ROOT:-}" ]; then
		echo "$CI_GO_NO_TELEMETRY_ROOT"
		return
	fi
	case "$(uname -s)" in
	Darwin)
		echo "${RUNNER_TOOL_CACHE:-${TMPDIR:-/tmp}}/go-no-telemetry"
		;;
	MINGW* | MSYS* | CYGWIN*)
		echo "${RUNNER_TOOL_CACHE:-${TEMP:-/tmp}}/go-no-telemetry"
		;;
	*)
		echo "/usr/local/go-no-telemetry"
		;;
	esac
}

ci_bootstrap_valid() {
	root="$1"
	go_bin="$(ci_go_bin "$root")"
	[ -x "$go_bin" ] && GOROOT="$root" "$go_bin" version >/dev/null 2>&1
}

ci_go_bin() {
	root="$1"
	if [ -x "${root}/bin/go" ]; then
		echo "${root}/bin/go"
	elif [ -x "${root}/bin/go.exe" ]; then
		echo "${root}/bin/go.exe"
	else
		echo "${root}/bin/go"
	fi
}
