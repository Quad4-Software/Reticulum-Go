#!/bin/bash

# Test script for page-downloader

echo "=== Page Downloader Test ==="
echo ""
echo "Testing against Go pageserver..."
echo "Make sure the pageserver is running first!"
echo ""

# Go pageserver destination hash (from logs)
DEST_HASH="28a73d05aa6b25f9328995b5d5e4df03"
PAGE_PATH="/page/index.mu"

echo "Target: $DEST_HASH:$PAGE_PATH"
echo ""

./page-downloader "$DEST_HASH:$PAGE_PATH"

