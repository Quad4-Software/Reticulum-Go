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

# Refuse placeholder git identities. Repo-local config has been set to
# the literal strings user.name / user.email, which then land in commits.
ident_name="${GIT_AUTHOR_NAME:-$(git config --get user.name || true)}"
ident_email="${GIT_AUTHOR_EMAIL:-$(git config --get user.email || true)}"
case "$ident_name" in
user.name | user.email)
	echo "commit-msg: placeholder author name: $ident_name" >&2
	echo "commit-msg: set user.name to a person name, not a git config key" >&2
	exit 1
	;;
esac
case "$ident_email" in
user.name | user.email)
	echo "commit-msg: placeholder author email: $ident_email" >&2
	echo "commit-msg: set user.email to the rngcs identity hash" >&2
	exit 1
	;;
esac

SUBJECT="$(sed -n '1p' "$MSG_FILE")"

# Allow merge and revert commits.
case "$SUBJECT" in
Merge\ * | Revert\ *)
	exit 0
	;;
esac

PATTERN='^(feat|fix|refactor|chore|docs|test|ci|perf|build)(\([a-z0-9./_-]+\))?(!)?: .+'

if ! echo "$SUBJECT" | grep -Eq "$PATTERN"; then
	echo "commit-msg: subject does not match Conventional Commits" >&2
	echo "commit-msg: expected: type(scope): summary" >&2
	echo "commit-msg: types: feat fix refactor chore docs test ci perf build" >&2
	echo "commit-msg: got: $SUBJECT" >&2
	echo "commit-msg: skip with SKIP_COMMIT_MSG_HOOK=1" >&2
	exit 1
fi

# DCO sign-off: every commit certifies the Developer Certificate of Origin.
# Add with git commit -s. Skip only for exceptional cases.
if [ "${SKIP_DCO_HOOK:-0}" != "1" ] && ! grep -qE '^Signed-off-by: .+ <[^>]+>' "$MSG_FILE"; then
	echo "commit-msg: missing Signed-off-by trailer (DCO)" >&2
	echo "commit-msg: add it with git commit -s or a 'Signed-off-by: Name <addr>' line" >&2
	echo "commit-msg: LXMF addresses are accepted in place of email" >&2
	echo "commit-msg: skip with SKIP_DCO_HOOK=1" >&2
	exit 1
fi

# AI provenance: on machines with ai.* git config, the prepare-commit-msg
# hook injects Harness/Model/Method trailers. A message that reaches this
# point either lacks them (injection bypassed) or carries values that do
# not match the configured machine identity (written by hand), so fail
# both ways. For commits with no AI involvement, commit with
# SKIP_AI_HOOK=1 so no trailers are injected and this check is skipped.
if [ "${SKIP_AI_HOOK:-0}" != "1" ]; then
	for key in harness model method; do
		want="$(git config "ai.$key" || true)"
		[ -n "$want" ] || continue
		trailer="$(echo "$key" | sed 's/./\U&/')"
		got="$(grep -E "^${trailer}: " "$MSG_FILE" | sed "s/^${trailer}: //" || true)"
		if [ -z "$got" ]; then
			echo "commit-msg: missing $trailer trailer while ai.$key is configured" >&2
			echo "commit-msg: let prepare-commit-msg inject it, or use SKIP_AI_HOOK=1" >&2
			exit 1
		fi
		if [ "$got" != "$want" ]; then
			echo "commit-msg: $trailer trailer '$got' does not match ai.$key '$want'" >&2
			echo "commit-msg: do not write Harness/Model/Method by hand; the hook" >&2
			echo "commit-msg: injects them from git config ai.* on this machine" >&2
			exit 1
		fi
	done
fi

exit 0
