// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package resource

import (
	"bytes"

	"quad4/bzip2/pkg/bzip2"
)

func bzip2CompressBody(in []byte) ([]byte, error) {
	var buf bytes.Buffer
	w, err := bzip2.NewWriter(&buf, 6)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(in); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
