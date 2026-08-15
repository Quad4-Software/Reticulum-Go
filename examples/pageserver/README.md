# pageserver

Serves static pages under `/page/` and files under `/file/` over Reticulum
request handlers, with interfaces driven by a Reticulum configuration file.

The production entrypoint is the main binary:

```text
reticulum-go pageserver [flags]
```

This directory keeps sample `pages/` / `files/` trees and a thin wrapper that
calls the same `pkg/cli` / `pkg/pageserver` code.

## Build (example wrapper)

From this directory:

```text
go build -o example-pageserver .
```

Or from the repo root:

```text
make build
./bin/reticulum-go pageserver -h
```

## Run

Default demo pageserver (from this directory so `pages/` and `files/` resolve):

```text
make run
task example:pageserver
```

Or:

```text
reticulum-go pageserver [flags]
./example-pageserver [flags]
go run -mod=mod . [flags]
```

Language-specific librns pageservers:

```text
task example:pageserver:c
task example:pageserver:odin
task example:pageserver:zig
```

## Configuration file

Interface configuration (TCP hubs, AutoInterface, UDP) lives in a Reticulum
configuration file. Default path:

```text
~/.reticulum-go/config
```

Override with `-config /path/to/config`. If the file does not exist it is
created with a single `Auto Discovery` (`AutoInterface`) entry using
`pkg/reticulumconfig`. No external TCP hubs are baked in. Add your own.

The format is the standard Reticulum INI-style layout:

```ini
[reticulum]
  enable_transport = yes
  share_instance = yes

[logging]
  loglevel = 4

[interfaces]

  [[Auto Discovery]]
    type = AutoInterface
    enabled = yes

  [[My TCP Hub]]
    type = TCPClientInterface
    enabled = yes
    target_host = hub.example.com
    target_port = 4242

  [[Local UDP]]
    type = UDPInterface
    enabled = no
    address = 0.0.0.0
    port = 37696
```

Edit the file to enable or disable individual interfaces, change target
hosts, or add new ones. Supported interface types: `AutoInterface`,
`TCPClientInterface`, `UDPInterface`. For local interop tests, the Go side
loads a config that adds a `UDPInterface` with the chosen listen and target
ports.

## Logging and verbosity

If you do **not** pass `-log-level` or `-debug`, verbosity comes from the
config file `[logging]` `loglevel` (1-7), same scale as `-debug`. If that
value is missing or out of range, the binary falls back to **critical-only**
(level 1).

Pass **`-log-level`** or **`-debug`** on the command line to override the
file for that run.

### Debug levels (`-debug` / `-log-level`)

| Level | Name     | What you see |
|:-----:|----------|--------------|
| 1 | critical | Fatal-style messages from this tool, default when no `-debug` / `-log-level` |
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
| `-config` / `-c` | `""` | Reticulum config path (default `~/.reticulum-go/config`, created if missing). |
| `-pages-dir` / `-p` | `pages` | Pages directory for `/page/`. |
| `-files-dir` / `-f` | `files` | Files directory for `/file/`. |
| `-node-name` / `-n` | built-in name | Node display name in announces. |
| `-announce-interval` / `-a` | `360` | Periodic announces every N **whole minutes** (`0` = off after initial announce). Default 360 = 6 hours. |
| `-page-refresh` / `-pages-refresh-interval` | `0` | Rescan pages dir every N **seconds** (`0` = startup only). |
| `-file-refresh` / `-files-refresh-interval` | `0` | Rescan files dir every N **seconds** (`0` = startup only). |
| `-identity` / `-identity-path` | `""` | Identity file path (default `~/.reticulum-go/storage/identity`). |
| `-debug` | (see `pkg/debug`) | Same scale as `-log-level` when passed on the CLI. |
| `-log-level` | `-1` | Sets level `1`-`7`. `-1` uses config. Overrides config and `-debug` when set. |

See also `man reticulum-go-pageserver`.
