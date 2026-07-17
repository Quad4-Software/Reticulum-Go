#!/bin/sh
# SPDX-License-Identifier: Apache-2.0
# Speedtest daemon entrypoint. Results go to stdout for docker logs.

set -eu

CONFIG_DIR="${RNS_CONFIG:-/data}"
DEFAULT_CFG="/etc/reticulum-go/config.default"
PORT="${SPEEDTEST_PORT:-4242}"
IFACE="${SPEEDTEST_IFACE:-all}"
BYTES="${SPEEDTEST_BYTES:-2097152}"
ANNOUNCE="${SPEEDTEST_ANNOUNCE:-120}"
JSON="${SPEEDTEST_JSON:-1}"
IDENTITY="${SPEEDTEST_IDENTITY:-$CONFIG_DIR/storage/identities/speedtest}"

mkdir -p "$CONFIG_DIR/storage/identities"

if [ ! -f "$CONFIG_DIR/config" ]; then
  echo "speedtest: seeding default config into $CONFIG_DIR/config" >&2
  sed "s/__SPEEDTEST_PORT__/${PORT}/g" "$DEFAULT_CFG" > "$CONFIG_DIR/config"
fi

JSON_FLAG=
case "$JSON" in
  1|true|yes|YES|True) JSON_FLAG=-json ;;
esac

echo "speedtest: starting daemon iface=${IFACE} bytes=${BYTES} announce=${ANNOUNCE}s" >&2
echo "speedtest: config=${CONFIG_DIR} identity=${IDENTITY}" >&2

# "$@" is docker CMD (optional extra flags). Results stay on stdout for docker logs.
exec /usr/local/bin/reticulum-go speedtest \
  -daemon \
  -config "$CONFIG_DIR" \
  -identity "$IDENTITY" \
  -iface "$IFACE" \
  -bytes "$BYTES" \
  -announce "$ANNOUNCE" \
  $JSON_FLAG \
  "$@"
