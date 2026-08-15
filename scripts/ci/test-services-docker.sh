#!/bin/sh
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io
#
# Build reticulum-go and verify systemd, openrc, runit, and dinit service
# packaging inside Docker containers.

set -eu

ROOT="$(CDPATH='' cd -- "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

IMAGE_BASE="${IMAGE_BASE:-reticulum-go-svc-test}"
KEEP_IMAGES="${KEEP_IMAGES:-0}"
FAILED=0

log() {
	printf '%s\n' "$*"
}

die() {
	printf 'ERROR: %s\n' "$*" >&2
	exit 1
}

need_cmd() {
	command -v "$1" >/dev/null 2>&1 || die "missing required command: $1"
}

need_cmd docker
need_cmd make

log "==> Building reticulum-go binary"
make build

test -x "$ROOT/bin/reticulum-go" || die "bin/reticulum-go missing after build"

cleanup_image() {
	name="$1"
	if [ "$KEEP_IMAGES" = "1" ]; then
		return 0
	fi
	docker rmi -f "$name" >/dev/null 2>&1 || true
}

# Shared smoke: start daemon via the same argv the service uses, confirm logfile.
# Args: container image tag
smoke_daemon() {
	img="$1"
	docker run --rm \
		-v "$ROOT/bin/reticulum-go:/usr/local/bin/reticulum-go:ro" \
		"$img" \
		/bin/sh -c '
set -eu
mkdir -p /var/lib/reticulum-go/logfile
cat > /var/lib/reticulum-go/config <<EOF
[reticulum]
  enable_transport = Yes
  share_instance = Yes
  enable_sandbox = No
  enable_control_api = No

[logging]
  loglevel = 3
  destination = both
  logfile = /var/lib/reticulum-go/logfile/reticulum.log

[interfaces]
EOF
export HOME=/var/lib/reticulum-go
/usr/local/bin/reticulum-go daemon --config /var/lib/reticulum-go/config &
pid=$!
i=0
while [ "$i" -lt 30 ]; do
  if [ -f /var/lib/reticulum-go/logfile/reticulum.log ] && grep -q "Initializing Reticulum" /var/lib/reticulum-go/logfile/reticulum.log 2>/dev/null; then
    kill -TERM "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
    echo "logfile smoke ok"
    exit 0
  fi
  i=$((i + 1))
  sleep 0.2
done
kill -TERM "$pid" 2>/dev/null || true
wait "$pid" 2>/dev/null || true
echo "logfile missing or empty:" >&2
ls -la /var/lib/reticulum-go/logfile/ >&2 || true
cat /var/lib/reticulum-go/logfile/reticulum.log >&2 || true
exit 1
'
}

run_systemd() {
	tag="${IMAGE_BASE}-systemd"
	log "==> Testing systemd install + unit start"
	docker build -t "$tag" -f - "$ROOT" <<'EOF'
FROM debian:bookworm-slim
RUN apt-get update \
 && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      systemd systemd-sysv ca-certificates \
 && rm -rf /var/lib/apt/lists/*
STOPSIGNAL SIGRTMIN+3
CMD ["/lib/systemd/systemd"]
EOF

	cid="$(docker run -d --privileged --cgroupns=host \
		-v /sys/fs/cgroup:/sys/fs/cgroup:rw \
		-v "$ROOT:/src:ro" \
		-v "$ROOT/bin/reticulum-go:/usr/local/bin/reticulum-go:ro" \
		"$tag")"

	ok=0
	i=0
	while [ "$i" -lt 40 ]; do
		if docker exec "$cid" systemctl is-system-running --wait >/dev/null 2>&1 \
			|| docker exec "$cid" systemctl is-system-running >/dev/null 2>&1; then
			ok=1
			break
		fi
		# Accept "degraded" / "running" as usable for unit tests.
		state="$(docker exec "$cid" systemctl is-system-running 2>/dev/null || true)"
		case "$state" in
		running|degraded)
			ok=1
			break
			;;
		esac
		i=$((i + 1))
		sleep 0.5
	done

	if [ "$ok" != 1 ]; then
		log "systemd did not become ready (continuing with direct unit install check)"
	fi

	if docker exec "$cid" /bin/sh -c '
set -eu
mkdir -p /var/lib/reticulum-go/logfile
cp /src/packaging/reticulum-go.config /var/lib/reticulum-go/config
# Disable sandbox and interfaces for container smoke.
sed -i "s/enable_sandbox = Yes/enable_sandbox = No/" /var/lib/reticulum-go/config
printf "\n[interfaces]\n" >> /var/lib/reticulum-go/config
sh /src/scripts/install-service.sh --prefix /usr/local --init systemd
systemctl daemon-reload
systemctl start reticulum-go
systemctl is-active reticulum-go
i=0
while [ "$i" -lt 30 ]; do
  if [ -f /var/lib/reticulum-go/logfile/reticulum.log ] && grep -q "Initializing Reticulum" /var/lib/reticulum-go/logfile/reticulum.log; then
    systemctl stop reticulum-go
    echo systemd service ok
    exit 0
  fi
  i=$((i + 1))
  sleep 0.2
done
systemctl status reticulum-go --no-pager >&2 || true
journalctl -u reticulum-go -n 50 --no-pager >&2 || true
exit 1
'; then
		log "PASS systemd"
	else
		log "FAIL systemd"
		FAILED=$((FAILED + 1))
	fi

	docker rm -f "$cid" >/dev/null 2>&1 || true
	cleanup_image "$tag"
}

run_openrc() {
	tag="${IMAGE_BASE}-openrc"
	log "==> Testing openrc install + service start"
	docker build -t "$tag" -f - "$ROOT" <<'EOF'
FROM alpine:3.20
RUN apk add --no-cache openrc
EOF

	if docker run --rm \
		-v "$ROOT:/src:ro" \
		-v "$ROOT/bin/reticulum-go:/usr/local/bin/reticulum-go:ro" \
		"$tag" \
		/bin/sh -c '
set -eu
mkdir -p /run/openrc /var/lib/reticulum-go/logfile
touch /run/openrc/softlevel
# Initialize OpenRC runtime state directories (starting/started/exclusive/...).
openrc >/dev/null 2>&1 || true
cp /src/packaging/reticulum-go.config /var/lib/reticulum-go/config
sed -i "s/enable_sandbox = Yes/enable_sandbox = No/" /var/lib/reticulum-go/config
# Drop Auto Discovery interface for container smoke.
awk "
  BEGIN {skip=0}
  /^  \\[\\[Auto Discovery\\]\\]/ {skip=1; next}
  /^  \\[\\[/ {skip=0}
  /^\\[/ {skip=0}
  skip==0 {print}
" /var/lib/reticulum-go/config > /tmp/cfg && mv /tmp/cfg /var/lib/reticulum-go/config
sh /src/scripts/install-service.sh --prefix /usr/local --init openrc
export HOME=/var/lib/reticulum-go
RC_NO_DEPS=yes /etc/init.d/reticulum-go start
i=0
while [ "$i" -lt 30 ]; do
  if [ -f /var/lib/reticulum-go/logfile/reticulum.log ] && grep -q "Initializing Reticulum" /var/lib/reticulum-go/logfile/reticulum.log; then
    RC_NO_DEPS=yes /etc/init.d/reticulum-go stop || true
    echo openrc service ok
    exit 0
  fi
  i=$((i + 1))
  sleep 0.2
done
/etc/init.d/reticulum-go status >&2 || true
cat /var/lib/reticulum-go/logfile/reticulum.log >&2 || true
exit 1
'; then
		log "PASS openrc"
	else
		log "FAIL openrc"
		FAILED=$((FAILED + 1))
	fi
	cleanup_image "$tag"
}

run_runit() {
	tag="${IMAGE_BASE}-runit"
	log "==> Testing runit install + runsv"
	docker build -t "$tag" -f - "$ROOT" <<'EOF'
FROM alpine:3.20
RUN apk add --no-cache runit
EOF

	if docker run --rm \
		-v "$ROOT:/src:ro" \
		-v "$ROOT/bin/reticulum-go:/usr/local/bin/reticulum-go:ro" \
		"$tag" \
		/bin/sh -c '
set -eu
mkdir -p /var/lib/reticulum-go/logfile /var/log/reticulum-go /var/service
cp /src/packaging/reticulum-go.config /var/lib/reticulum-go/config
sed -i "s/enable_sandbox = Yes/enable_sandbox = No/" /var/lib/reticulum-go/config
awk "
  BEGIN {skip=0}
  /^  \\[\\[Auto Discovery\\]\\]/ {skip=1; next}
  /^  \\[\\[/ {skip=0}
  /^\\[/ {skip=0}
  skip==0 {print}
" /var/lib/reticulum-go/config > /tmp/cfg && mv /tmp/cfg /var/lib/reticulum-go/config
sh /src/scripts/install-service.sh --prefix /usr/local --init runit
ln -sfn /etc/sv/reticulum-go /var/service/reticulum-go
runsv /etc/sv/reticulum-go &
rsv_pid=$!
i=0
while [ "$i" -lt 40 ]; do
  if [ -f /var/lib/reticulum-go/logfile/reticulum.log ] && grep -q "Initializing Reticulum" /var/lib/reticulum-go/logfile/reticulum.log; then
    sv down /etc/sv/reticulum-go || true
    kill "$rsv_pid" 2>/dev/null || true
    wait "$rsv_pid" 2>/dev/null || true
    echo runit service ok
    exit 0
  fi
  i=$((i + 1))
  sleep 0.2
done
sv status /etc/sv/reticulum-go >&2 || true
cat /var/lib/reticulum-go/logfile/reticulum.log >&2 || true
kill "$rsv_pid" 2>/dev/null || true
exit 1
'; then
		log "PASS runit"
	else
		log "FAIL runit"
		FAILED=$((FAILED + 1))
	fi
	cleanup_image "$tag"
}

run_dinit() {
	tag="${IMAGE_BASE}-dinit"
	log "==> Testing dinit install + start"
	docker build -t "$tag" -f - "$ROOT" <<'EOF'
FROM alpine:3.20 AS build
RUN apk add --no-cache build-base git m4
WORKDIR /src
RUN git clone --depth 1 --branch v0.19.3 https://github.com/davmac314/dinit.git \
 && cd dinit \
 && make -j"$(nproc)" BUILD_SHUTDOWN=no \
 && make install DESTDIR=/out BUILD_SHUTDOWN=no SBINDIR=/usr/local/sbin

FROM alpine:3.20
RUN apk add --no-cache libstdc++
COPY --from=build /out/usr/local/sbin/ /usr/local/sbin/
ENV PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
EOF

	if docker run --rm \
		-v "$ROOT:/src:ro" \
		-v "$ROOT/bin/reticulum-go:/usr/local/bin/reticulum-go:ro" \
		"$tag" \
		/bin/sh -c '
set -eu
mkdir -p /var/lib/reticulum-go/logfile /run/dinit
cp /src/packaging/reticulum-go.config /var/lib/reticulum-go/config
sed -i "s/enable_sandbox = Yes/enable_sandbox = No/" /var/lib/reticulum-go/config
awk "
  BEGIN {skip=0}
  /^  \\[\\[Auto Discovery\\]\\]/ {skip=1; next}
  /^  \\[\\[/ {skip=0}
  /^\\[/ {skip=0}
  skip==0 {print}
" /var/lib/reticulum-go/config > /tmp/cfg && mv /tmp/cfg /var/lib/reticulum-go/config
sh /src/scripts/install-service.sh --prefix /usr/local --init dinit
# Container-mode dinit with an explicit control socket.
dinit -o -d /etc/dinit.d -p /run/dinit/control -q reticulum-go &
dinit_pid=$!
i=0
while [ "$i" -lt 40 ]; do
  if [ -f /var/lib/reticulum-go/logfile/reticulum.log ] && grep -q "Initializing Reticulum" /var/lib/reticulum-go/logfile/reticulum.log; then
    dinitctl -p /run/dinit/control stop reticulum-go 2>/dev/null || true
    kill "$dinit_pid" 2>/dev/null || true
    wait "$dinit_pid" 2>/dev/null || true
    echo dinit service ok
    exit 0
  fi
  i=$((i + 1))
  sleep 0.2
done
dinitctl -p /run/dinit/control list >&2 || true
cat /var/lib/reticulum-go/logfile/reticulum.log >&2 || true
kill "$dinit_pid" 2>/dev/null || true
exit 1
'; then
		log "PASS dinit"
	else
		log "FAIL dinit"
		FAILED=$((FAILED + 1))
	fi
	cleanup_image "$tag"
}

run_install_all() {
	tag="${IMAGE_BASE}-install-all"
	log "==> Testing install-service --init all (file placement)"
	docker build -t "$tag" -f - "$ROOT" <<'EOF'
FROM alpine:3.20
EOF
	if docker run --rm \
		-v "$ROOT:/src:ro" \
		"$tag" \
		/bin/sh -c '
set -eu
sh /src/scripts/install-service.sh --prefix /usr/local --destdir /stage --init all
test -f /stage/usr/lib/systemd/system/reticulum-go.service
test -x /stage/etc/init.d/reticulum-go
test -x /stage/etc/sv/reticulum-go/run
test -x /stage/etc/sv/reticulum-go/log/run
test -f /stage/etc/dinit.d/reticulum-go
test -f /stage/var/lib/reticulum-go/config
grep -q "/usr/local/bin/reticulum-go" /stage/usr/lib/systemd/system/reticulum-go.service
grep -q "/usr/local/bin/reticulum-go" /stage/etc/init.d/reticulum-go
grep -q "/usr/local/bin/reticulum-go" /stage/etc/sv/reticulum-go/run
grep -q "/usr/local/bin/reticulum-go" /stage/etc/dinit.d/reticulum-go
grep -q "destination = both" /stage/var/lib/reticulum-go/config
echo install-all ok
'; then
		log "PASS install-all"
	else
		log "FAIL install-all"
		FAILED=$((FAILED + 1))
	fi
	cleanup_image "$tag"
}

run_logfile_smoke() {
	tag="${IMAGE_BASE}-logfile"
	log "==> Testing logfile destination smoke"
	docker build -t "$tag" -f - "$ROOT" <<'EOF'
FROM alpine:3.20
EOF
	if smoke_daemon "$tag"; then
		log "PASS logfile"
	else
		log "FAIL logfile"
		FAILED=$((FAILED + 1))
	fi
	cleanup_image "$tag"
}

run_install_all
run_logfile_smoke
run_openrc
run_runit
run_dinit
run_systemd

if [ "$FAILED" -ne 0 ]; then
	log "==> $FAILED service test(s) failed"
	exit 1
fi

log "==> All service Docker tests passed"
