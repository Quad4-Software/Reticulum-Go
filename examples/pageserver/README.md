# pageserver

Serves static pages under `/page/` and files under `/file/` over Reticulum request handlers, with optional UDP or TCP/Auto interfaces. Run from this directory:

```text
go run . [flags]
```

## Logging and verbosity

If you do **not** pass `-log-level` or `-debug`, the process runs at **critical-only** (level 1): library and transport **INFO** lines are suppressed, and stderr shows a short **startup summary** (node destination hash, registered page paths, and file paths).

Pass **`-log-level`** or **`-debug`** to raise verbosity.

### Debug levels (`-debug` / `-log-level`)

| Level | Name     | What you see |
|:-----:|----------|--------------|
| 1 | critical | Fatal-style messages from this tool; default when no `-debug` / `-log-level` |
| 2 | error    | Errors and above |
| 3 | info     | General informational logs (includes much Reticulum stack output) |
| 4 | verbose  | More detail |
| 5 | trace    | Very chatty |
| 6 | packets  | Packet-level detail |
| 7 | all      | Everything |

`-log-level` overrides `-debug` when both are set.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-debug` | `3` in `pkg/debug` | Same scale as `-log-level`. Omitted means this binary forces level **1** (critical-only). If you pass `-debug`, that value is used. |
| `-log-level` | `-1` (unset) | Sets level `1`–`7`. If set, overrides the default-quiet behavior. |
| `-udp` | `false` | Use a local UDP interface instead of TCP hubs / Auto. |
| `-listen-port` | `4242` | UDP listen address port when `-udp` is set. |
| `-target-port` | `0` | UDP peer port when `-udp` is set (`0` = no target). |
| `-tcp-target-host` | `""` | TCP client target host in non-UDP mode; empty selects the default hub. |
| `-tcp-target-port` | `4242` | TCP client target port. |
| `-tcp-name` | `Beleth RNS Hub` | Interface name when using `-tcp-target-host`. |
| `-fresh-identity` | `false` | Delete the on-disk identity before start (new destination hash). |
| `-identity-path` | `""` | Identity file path (default: `~/.reticulum-go/storage/identity`). |
| `-no-auto-discovery` | `false` | Disable `AutoInterface` (TCP-only path). |
| `-announce-interval` | `6h` | Period for repeated announces; `0` disables repeats (initial announce still sent). |
| `-pages-dir` | `pages` | Directory of pages served under `/page/`. |
| `-files-dir` | `files` | Directory of files served under `/file/`. |
| `-pages-refresh-interval` | `0` | Rescan `pages-dir` on this interval; `0` = scan only at startup. |
| `-files-refresh-interval` | `0` | Rescan `files-dir` on this interval; `0` = scan only at startup. |
| `-intercept-packets` | `false` | Log raw packets to a file (debugging). |
| `-intercept-output` | `packets.log` | Output path when `-intercept-packets` is set. |

Constants such as announce rate targets are compiled into the binary (see `main.go`); they are not CLI flags.
