// SPDX-License-Identifier: 0BSD
// Copyright (c)2026 Quad4.io

package bzip2

import (
	"errors"
	"io"
)

func writeFull(dst io.Writer, p []byte) error {
	for len(p) > 0 {
		n, err := dst.Write(p)
		if n < 0 || len(p) < n {
			return errors.New("bzip2: invalid Write result")
		}
		p = p[n:]
		if err != nil {
			return err
		}
		if n == 0 && len(p) > 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}
