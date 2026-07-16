# Page Downloader

A CLI tool to download pages from Reticulum nodes using the request/response mechanism over established links.

## Usage

```bash
./page-downloader [options] <dest_hash>:<page_path>
```

### Options

- `-timeout <seconds>` - Timeout for link establishment and request (default: 30)

### Examples

Download a page from the Go pageserver:
```bash
./page-downloader 92798ea245a0afcfa559348e42d628c6:/page/index.mu
```

Download with custom timeout:
```bash
./page-downloader -timeout 60 92798ea245a0afcfa559348e42d628c6:/page/test.mu
```

## Building

```bash
cd examples/page-downloader
go build
```

## How It Works

1. Starts transport with TCP and AutoInterface
2. Waits for network initialization (2 seconds)
3. Creates an outgoing link to the specified destination hash
4. Waits for link establishment
5. Sends a request for the specified page path
6. Waits for and displays the response
7. Keeps link open until Ctrl+C

## Compatible With

- Reticulum-Go pageserver example
- Nomadnet nodes (nomadnetwork/node aspect)
- Any Reticulum node that implements the request/response pattern

## Troubleshooting

If you see "Request failed" or "Request timeout":
- Check that the destination is announcing and reachable
- Verify the destination hash is correct (32 hex characters)
- Ensure the page path exists on the target node
- Try increasing the timeout with `-timeout`
- Check debug logs for more details

