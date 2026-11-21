#!/bin/sh
# Install native libraries required by LXST/LXMF CGO builds in CI.
set -eu

if [ "${CI_SKIP_APT_DEPS:-0}" = "1" ]; then
	exit 0
fi
if ! command -v apt-get >/dev/null 2>&1; then
	exit 0
fi

sudo apt-get update -qq
sudo apt-get install -y libopus-dev libcodec2-dev
