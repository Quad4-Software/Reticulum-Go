#!/bin/sh
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
IMAGE="${RGESP_MUSL_IMAGE:-rgesp-musl}"

docker build -f "$ROOT/docker/Dockerfile.alpine" -t "$IMAGE" "$ROOT"
docker run --rm "$IMAGE" -version
docker run --rm --entrypoint rgesp-dial "$IMAGE" -h >/dev/null

echo "musl alpine image $IMAGE ok"
