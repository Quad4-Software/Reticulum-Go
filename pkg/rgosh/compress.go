// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rgosh

import (
	"bytes"
	"io"

	"quad4/bzip2/pkg/bzip2"
)

func compressMaybe(data []byte) (out []byte, compressed bool) {
	if len(data) < CompressThresh {
		return data, false
	}
	var buf bytes.Buffer
	w, err := bzip2.NewWriter(&buf, 9)
	if err != nil {
		return data, false
	}
	if _, err := w.Write(data); err != nil {
		_ = w.Close()
		return data, false
	}
	if err := w.Close(); err != nil {
		return data, false
	}
	c := buf.Bytes()
	if len(c) >= len(data) {
		return data, false
	}
	return c, true
}

func decompressBounded(data []byte, max int) ([]byte, error) {
	if max <= 0 {
		max = MaxDecompressed
	}
	reader := bzip2.NewReader(bytes.NewReader(data))
	limited := io.LimitReader(reader, int64(max)+1)
	out, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if len(out) > max {
		return nil, ErrDecompressBomb
	}
	return out, nil
}
