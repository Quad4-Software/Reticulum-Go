// SPDX-License-Identifier: 0BSD
// Copyright (c)2026 Quad4.io

package bzip2

import (
	stdbz2 "compress/bzip2"
	"io"
)

// NewReader returns a reader that decompresses bzip2 data from r. It uses the standard
// library compress/bzip2 reader so the format matches streams produced by [NewWriter].
func NewReader(r io.Reader) io.Reader {
	return stdbz2.NewReader(r)
}
