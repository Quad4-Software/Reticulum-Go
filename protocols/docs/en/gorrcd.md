# cmd/gorrcd

Go RRC hub daemon with operator feature parity against [rrcd](https://github.com/kc1awv/rrcd). State lives in `$GORRCD_HOME` or `~/.gorrcd/` (gorrcd.toml, hub_identity, rooms.toml) so it does not collide with Python `~/.rrcd`. First run writes defaults and exits 0.

```bash
make gorrcd
task gorrcd
sh scripts/ci/build-gorrcd.sh "$(go env GOOS)" "$(go env GOARCH)" bin
./bin/gorrcd-$(go env GOOS)-$(go env GOARCH)
```

Install:

```bash
make install-gorrcd
make install-gorrcd PREFIX=$HOME/.local
```

Attach to a running Reticulum shared instance when available, otherwise load interfaces from `~/.reticulum-go/config` (created with reticulum-go defaults including Auto Discovery on first run). For loopback tests, `--udp-listen` and `--udp-forward` bind a UDP pair and skip AutoInterface. `--ready-file` writes the hub destination hash when the daemon is up.

Trusted identities may use `/stats`, `/reload`, and `/kline`. Room founders and ops may use `/kick`, `/topic`, `/mode`, `/op`, `/deop`, `/voice`, `/devoice`, `/ban`, and `/invite`. Anyone may `/list` and `/who`.

Tagged `v*` pushes and Sunday preview snapshots publish cross-compiled gorrcd binaries (Linux including 32-bit, armv6, riscv64, ppc64le, Darwin Intel and Apple silicon, Windows, FreeBSD, OpenBSD including ppc64 and riscv64, and NetBSD). CI compiles the same matrix on every push and runs `--version` under qemu-user for linux/386, arm, riscv64, and ppc64le.

```bash
task test:gorrcd
task test:live
go test -short -v -count=1 ./internal/gorrcd/... ./cmd/gorrcd/...
```
