#!/bin/sh
# Validate conventional commit message format.
# Usage: commit-msg-check.sh <path-to-commit-msg-file>
#
# Skip: SKIP_COMMIT_MSG_HOOK=1
set -eu

if [ "${SKIP_COMMIT_MSG_HOOK:-0}" = "1" ]; then
	exit 0
fi

MSG_FILE="${1:-}"
if [ -z "$MSG_FILE" ] || [ ! -f "$MSG_FILE" ]; then
	echo "commit-msg: missing message file" >&2
	exit 1
fi

SUBJECT="$(sed -n '1p' "$MSG_FILE")"

# Allow merge and revert commits.
case "$SUBJECT" in
Merge\ * | Revert\ *)
	exit 0
	;;
esac

PATTERN='^(feat|fix|refactor|chore|docs|test|ci|perf|build)(\([a-z0-9./_-]+\))?(!)?: .+'

if echo "$SUBJECT" | grep -Eq "$PATTERN"; then
	exit 0
fi

echo "commit-msg: subject does not match Conventional Commits" >&2
echo "commit-msg: expected: type(scope): summary" >&2
echo "commit-msg: types: feat fix refactor chore docs test ci perf build" >&2
echo "commit-msg: got: $SUBJECT" >&2
echo "commit-msg: skip with SKIP_COMMIT_MSG_HOOK=1" >&2
exit 1
