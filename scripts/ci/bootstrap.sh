#!/bin/sh
# Install pinned Go-based dev tools into GOBIN (default: go env GOBIN or GOPATH/bin).
set -eu

ROOT="$(CDPATH='' cd -- "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

. "$ROOT/scripts/ci/dev-tools.env"

GOSEC_VERSION="${CI_GOSEC_VERSION:-v2.29.0}"

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

task_ver="$CI_TASK_VERSION"
case "$task_ver" in
v*) ;;
*) task_ver="v${task_ver}" ;;
esac

install_tool "github.com/go-task/task/v3/cmd/task" "$task_ver"
install_tool "github.com/mgechev/revive" "$CI_REVIVE_VERSION"
install_tool "honnef.co/go/tools/cmd/staticcheck" "$CI_STATICCHECK_VERSION"
install_tool "github.com/securego/gosec/v2/cmd/gosec" "$GOSEC_VERSION"

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
