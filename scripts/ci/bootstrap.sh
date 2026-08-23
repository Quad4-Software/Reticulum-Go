#!/bin/sh
# Install pinned Go-based dev tools into GOBIN (default: go env GOBIN or GOPATH/bin).
set -eu

ROOT="$(CDPATH='' cd -- "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

. "$ROOT/scripts/ci/dev-tools.env"

if ! command -v go >/dev/null 2>&1; then
	echo "bootstrap: install Go $CI_GO_VERSION first (mise install, or https://go.dev/dl/)" >&2
	exit 1
fi

GOBIN="${GOBIN:-$(go env GOBIN)}"
if [ -z "$GOBIN" ]; then
	GOBIN="$(go env GOPATH)/bin"
fi
mkdir -p "$GOBIN"
export GOBIN
export PATH="$GOBIN:$PATH"

echo "bootstrap: installing dev tools into $GOBIN"

install_tool() {
	module="$1"
	version="$2"
	env GOFLAGS= GOSUMDB=sum.golang.org GOPROXY=https://proxy.golang.org,direct \
		go install "${module}@${version}"
}

install_tool "github.com/go-task/task/v3/cmd/task" "$CI_TASK_VERSION"
install_tool "github.com/mgechev/revive" "$CI_REVIVE_VERSION"
install_tool "honnef.co/go/tools/cmd/staticcheck" "$CI_STATICCHECK_VERSION"

echo ""
echo "bootstrap: Go tools installed. Ensure GOBIN is on PATH:"
echo "  export PATH=\"$GOBIN:\$PATH\""
echo ""
echo "bootstrap: optional system packages (distro package manager):"
echo "  shellcheck, yamllint"
echo ""
echo "bootstrap: next steps:"
echo "  task doctor"
echo "  task hooks:install"
