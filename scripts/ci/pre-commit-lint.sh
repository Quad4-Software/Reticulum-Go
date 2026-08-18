#!/bin/sh
# Staged-file checks for pre-commit (workflows, taskfiles, scripts).
#
# YAML: yamllint when available, else python3 + PyYAML syntax load.
# Shell: shellcheck on staged *.sh under scripts/, root install.sh, and .githooks/.
#
# Skip:
#   SKIP_LINT_HOOK=1
#   SKIP_YAML_HOOK=1
#   SKIP_SHELLCHECK_HOOK=1
#
# If shell scripts are staged and shellcheck is missing, the hook fails
# unless SKIP_SHELLCHECK_HOOK=1.
set -eu

if [ "${SKIP_LINT_HOOK:-0}" = "1" ]; then
	exit 0
fi

ROOT="$(CDPATH='' cd -- "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

STAGED="$(git diff --cached --name-only --diff-filter=ACMR)"
if [ -z "$STAGED" ]; then
	exit 0
fi

fail=0
yaml_targets=
shell_targets=

is_yaml_name() {
	case "$1" in
	*.yml | *.yaml | *.YML | *.YAML) return 0 ;;
	*) return 1 ;;
	esac
}

while IFS= read -r path; do
	[ -n "$path" ] || continue
	[ -f "$path" ] || continue
	case "$path" in
	.github/workflows/*)
		if is_yaml_name "$path"; then
			yaml_targets="$yaml_targets $path"
		fi
		;;
	taskfiles/*)
		if is_yaml_name "$path"; then
			yaml_targets="$yaml_targets $path"
		fi
		;;
	Taskfile.yml | Taskfile.yaml)
		yaml_targets="$yaml_targets $path"
		;;
	scripts/*)
		case "$path" in
		*.sh)
			shell_targets="$shell_targets $path"
			;;
		esac
		;;
	.githooks/*)
		shell_targets="$shell_targets $path"
		;;
	install.sh)
		shell_targets="$shell_targets $path"
		;;
	esac
done <<EOF
$STAGED
EOF

check_yaml() {
	if [ "${SKIP_YAML_HOOK:-0}" = "1" ]; then
		return 0
	fi
	# shellcheck disable=SC2086
	set -- $yaml_targets
	if [ "$#" -eq 0 ]; then
		return 0
	fi

	echo "pre-commit: checking YAML ($# file(s))"
	if command -v yamllint >/dev/null 2>&1; then
		if [ -f "$ROOT/.yamllint.yml" ]; then
			yamllint -c "$ROOT/.yamllint.yml" "$@" || return 1
		else
			yamllint "$@" || return 1
		fi
		return 0
	fi

	python3 - "$@" <<'PY'
import sys
try:
	import yaml
except ImportError:
	print("pre-commit: install yamllint or PyYAML to check workflow/taskfile YAML", file=sys.stderr)
	sys.exit(1)

failed = 0
for path in sys.argv[1:]:
	try:
		with open(path, encoding="utf-8") as f:
			yaml.safe_load(f)
	except Exception as exc:
		print(f"pre-commit: YAML error in {path}: {exc}", file=sys.stderr)
		failed = 1
if failed:
	sys.exit(1)
PY
}

check_shell() {
	if [ "${SKIP_SHELLCHECK_HOOK:-0}" = "1" ]; then
		return 0
	fi
	# shellcheck disable=SC2086
	set -- $shell_targets
	if [ "$#" -eq 0 ]; then
		return 0
	fi

	echo "pre-commit: shellcheck ($# file(s))"
	if ! command -v shellcheck >/dev/null 2>&1; then
		echo "pre-commit: shellcheck not found (install it, or SKIP_SHELLCHECK_HOOK=1)" >&2
		return 1
	fi
	shellcheck -x -S warning "$@" || return 1
}

if ! check_yaml; then
	fail=1
fi
if ! check_shell; then
	fail=1
fi

exit "$fail"
