// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"quad4/reticulum-go/pkg/securemem"
)

func closeSecBuf(b **securemem.Buf) {
	if b == nil || *b == nil {
		return
	}
	_ = (*b).Close()
	*b = nil
}

func setSecBuf(dst **securemem.Buf, src []byte) error {
	closeSecBuf(dst)
	if len(src) == 0 {
		return nil
	}
	buf, err := securemem.New(len(src))
	if err != nil {
		return err
	}
	if err := buf.CopyFrom(src); err != nil {
		_ = buf.Close()
		return err
	}
	*dst = buf
	return nil
}

func (l *Link) closeAllSecretKeys() {
	closeSecBuf(&l.prv)
	closeSecBuf(&l.sigPriv)
	closeSecBuf(&l.sharedKey)
	closeSecBuf(&l.derivedKey)
	closeSecBuf(&l.hmacKey)
	closeSecBuf(&l.sessionKey)
}

func bufLen(b *securemem.Buf) int {
	if b == nil {
		return 0
	}
	return b.Len()
}

func bufBytes(b *securemem.Buf) []byte {
	if b == nil {
		return nil
	}
	return b.Bytes()
}
