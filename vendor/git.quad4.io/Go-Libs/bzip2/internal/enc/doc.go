// SPDX-License-Identifier: 0BSD
// Copyright (c)2026 Quad4.io

// Package enc implements the bzip2 encoder pipeline used by the parent module's public API:
// stream and block headers, RLE, Burrows–Wheeler transform, move-to-front, Huffman coding,
// and bit-level output. It is internal to this module and not a stable API.
package enc
