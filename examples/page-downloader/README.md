# Page Downloader

A CLI tool to download pages from Reticulum nodes using the request/response mechanism over established links.

## Usage

```bash
./page-downloader -config /path/to/config <dest_hash>:<page_path>
```

Provide a Reticulum config with at least one online TCP or Backbone hub from
[directory.rns.recipes](https://directory.rns.recipes/). There is no baked-in hub.

### Options

- `-config <path>` - Reticulum config file (required unless `-udp`)
- `-timeout <seconds>` - Timeout for link establishment and request (default: 30)
- `-once` - Exit after the first successful page response
- `-udp` - Local UDP mode for lab tests

### Examples

```bash
./page-downloader -config ~/reticulum-go/config -once \
  92798ea245a0afcfa559348e42d628c6:/page/index.mu
```

## Building

```bash
cd examples/page-downloader
go build
```

## Compatible With

- Reticulum-Go pageserver
- librns C / Odin pageserver examples
- NomadNet nodes (`nomadnetwork` / `node`)
- Any peer that implements link request/response for `/page/` paths
