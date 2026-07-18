# Firecracker microvm

Run `reticulum-go` inside an Amazon Firecracker microVM. The default path is aimed at nested VMs (Firecracker inside another VM) where guest TAP networking is unreliable.

## Requirements

- Linux with `/dev/kvm` readable
- firecracker on `PATH`
- Go toolchain (to build the guest rootfs and host bridge binary)
- Optional: pasta only when using `NET=1` rootless TAP (often broken when nested)

## Quick start

```bash
make microvm-up
# or: task microvm:up
# or: ./microvm/up.sh
```

That will:

1. Download a Firecracker CI guest kernel into `microvm/out/` (first run)
2. Build a static guest rootfs with busybox + `reticulum-go`
3. Start the microVM detached (`NET=0`, VSOCK only)
4. Start a host bridge that pipes HDLC into the guest over Firecracker vsock

Stop everything:

```bash
make microvm-stop
# or: ./microvm/stop.sh
```

## Add a community hub

Edit `microvm/host-bridge.config` and uncomment or add an interface under `[interfaces]`, for example:

```ini
[[My Hub]]
  type = BackboneInterface
  enabled = Yes
  remote = hub.example.com
  target_port = 4242
  max_reconnect_tries = -1
```

TCP hubs use TCPClientInterface with target_host / target_port. Then restart:

```bash
make microvm-stop
make microvm-up
```

The host process owns clearnet. The guest joins the mesh through the `Microvm Guest` pipe (Firecracker vsock CONNECT).

## Layout

| Path | Role |
|------|------|
| `microvm/up.sh` | One-shot prepare + start |
| `microvm/fetch-kernel.sh` | Download `out/vmlinux` |
| `microvm/build-rootfs.sh` | Build `out/rootfs.ext4` |
| `microvm/run.sh` | Start Firecracker |
| `microvm/run-host-bridge.sh` | Host reticulum + vsock pipe |
| `microvm/stop.sh` | Stop guest, pasta, and host bridge |
| `microvm/guest/reticulum.config` | Guest config (VSOCK hub, persistent storage) |
| `microvm/host-bridge.config` | Host config (vsock pipe + your hubs) |
| `microvm/out/` | Kernel, rootfs, sockets, logs (gitignored) |

## Networking modes

### Default: host bridge (recommended in nested VMs)

```bash
NET=0 ./microvm/up.sh
```

Guest listens on AF_VSOCK. Host uses PipeInterface + `vsock-connect.sh` to speak Firecracker UDS `CONNECT`. No guest TAP.

### Guest-only

```bash
./microvm/up.sh --guest-only
# later:
./microvm/run-host-bridge.sh
```

### Rootless TAP (`NET=1`)

Uses pasta to create a TAP and NAT. Useful on bare metal. Nested Firecracker often fails with tap write errors. Prefer the host bridge there.

```bash
NET=1 ./microvm/up.sh --guest-only
```

### Host TAP

If you already have a TAP (requires `CAP_NET_ADMIN` on the host):

```bash
TAP_DEV=tap0 DETACH=1 ./microvm/run.sh
```

Configure guest IP via `microvm/guest/microvm-net` before `build-rootfs.sh`.

## Persistence

Guest storage lives on the rootfs under `/etc/reticulum/storage` (next to the guest config). Rebuilds of `rootfs.ext4` replace that disk image. Keep identities you care about outside the image or avoid `--rebuild` when you need them.

## Makefile and Task

| Make | Task | Action |
|------|------|--------|
| `make microvm-up` | `task microvm:up` | Fetch kernel if needed, build rootfs if needed, start guest + host bridge |
| `make microvm-stop` | `task microvm:stop` | Stop guest and host bridge |
| `make microvm-kernel` | `task microvm:kernel` | Fetch kernel only |
| `make microvm-rootfs` | `task microvm:rootfs` | Build rootfs only |
| `make microvm-rebuild` | `task microvm:rebuild` | Force kernel+rootfs rebuild then up |

## Logs

- Guest serial: `microvm/out/firecracker.stdout`
- Firecracker log: `microvm/out/firecracker.log`
- Host bridge: `microvm/out/host-bridge.log`

## Related documents

- [Interfaces](interfaces.md) for VSOCK, Pipe, TCP, and Backbone keys
- [Configuration](configuration.md) for config file format
- [Architecture](architecture.md) for deployment models
