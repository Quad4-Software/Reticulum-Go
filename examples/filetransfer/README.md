# File Transfer Example

This example demonstrates a simple file transfer server and client using Reticulum-Go, mirroring the Python `Filetransfer.py` example.

## Features

- **Server Mode**: Serves files from a specified directory
- **Client Mode**: Connects to a server, lists files, and downloads them
- **Link-based Communication**: Uses RNS Links for reliable connections
- **Resource Transfer**: Uses RNS Resources for efficient file transfers with progress tracking
- **Automatic Compression**: Files are automatically compressed if beneficial
- **Interactive Menu**: Easy-to-use interface for browsing and downloading files

## Building

```bash
cd examples/filetransfer
go build
```

## Usage

### Server

Start a file server to share files from a directory:

```bash
# Run server
./filetransfer --server --serve /path/to/files

# With custom ports (for local testing)
./filetransfer --server --serve /path/to/files --listen-port 4243
```

The server will:
1. Display its destination hash
2. Wait for you to press enter to send announces
3. Accept connections from clients
4. Send the file list to connected clients
5. Transfer requested files

### Client

Connect to a server and download files:

```bash
# Run client (use the destination hash from server output)
./filetransfer --destination <server_hash>

# With custom ports (for local testing)
./filetransfer --destination <server_hash> --listen-port 4245 --target-port 4243
```

The client will:
1. Request a path to the destination if unknown
2. Establish a link with the server
3. Receive and display the file list
4. Show an interactive menu for file selection
5. Download selected files with progress indication

### Interactive Menu

Once connected, the client shows:

```
==================================================
Files on server:
  (0)	document.pdf
  (1)	image.jpg
  (2)	archive.zip
==================================================
Enter filename or number to download, or 'q' to quit
> 
```

You can:
- Type the filename: `document.pdf`
- Type the number: `0`  
- Type `q` to quit

During download, you'll see:
- Real-time progress percentage
- Transfer statistics (time, size, transfer rate)
- Final save location

## Local Testing

To test server and client on the same machine, use different ports:

**Terminal 1 (Server):**
```bash
mkdir test_files
echo "Hello Reticulum!" > test_files/test.txt
./filetransfer --server --serve ./test_files --listen-port 4243
# Press enter to announce
```

**Terminal 2 (Client):**
```bash
./filetransfer --destination <hash_from_server> --listen-port 4245 --target-port 4243
# Select files from the menu
```

## File Handling

- **Downloaded Files**: Saved in the current directory
- **Duplicate Names**: Automatically numbered (e.g., `file.txt.1`, `file.txt.2`)
- **Large Files**: Automatically chunked into segments for efficient transfer
- **Compression**: Automatically applied when beneficial (text files compress well, media files don't)

## Security

- **Encrypted Transfer**: All data is encrypted over RNS Links
- **Authentication**: Clients must know the server's destination hash
- **Access Control**: Only files in the served directory are accessible
- **Unique Identity**: Each server has a unique cryptographic identity

## Statistics

After each download completes, you'll see:
- **Time Taken**: Total download duration
- **File Size**: Actual file size
- **Data Transferred**: Amount of data sent (may be less due to compression)
- **Effective Rate**: File size / time (actual throughput)
- **Transfer Rate**: Data transferred / time (network throughput)

## Notes

- The example uses msgpack for encoding the file list
- Resources handle segmentation and reassembly automatically
- Progress callbacks provide real-time transfer status
- The server creates a new random identity on each startup

## Comparison with Python Version

This Go implementation mirrors the Python `Filetransfer.py` example:
- ✅ Server/client architecture
- ✅ Link establishment and management
- ✅ File list transmission
- ✅ Resource-based file transfers
- ✅ Interactive menu for file selection
- ✅ Progress tracking and statistics
- ✅ Automatic file compression
- ✅ Port configuration for local testing

## Known Limitations

- Link resource attachment may need additional implementation for full functionality
- Some advanced resource features (windowing, retries) may need further development
- This serves as both a working example and a test case for the Link/Resource implementations

