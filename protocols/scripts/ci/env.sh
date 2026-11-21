# shellcheck shell=sh
# Shared CI environment for reticulum-go-protocols (source from run-suite.sh and workflows).

export GOFLAGS="${GOFLAGS:--mod=vendor}"
export GOPROXY="${GOPROXY:-off}"
export GOSUMDB="${GOSUMDB:-off}"
export GOTOOLCHAIN="${GOTOOLCHAIN:-local}"
