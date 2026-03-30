// SPDX-License-Identifier: 0BSD
// Copyright (c) 2024-2026 Sudo-Ivan / Quad4.io
package link

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"io"

	"git.quad4.io/Go-Libs/bzip2/pkg/bzip2"
	"git.quad4.io/Networks/Reticulum-Go/pkg/resource"
)

type incomingResourceAsm struct {
	adv *resource.ResourceAdvertisement
	buf []byte
}

func (l *Link) startIncomingResource(adv *resource.ResourceAdvertisement) {
	l.incomingMu.Lock()
	defer l.incomingMu.Unlock()
	l.incomingRx = &incomingResourceAsm{adv: adv}
}

func (l *Link) resetIncomingResource() {
	l.incomingMu.Lock()
	defer l.incomingMu.Unlock()
	l.incomingRx = nil
}

func (l *Link) appendIncomingResourcePart(data []byte) error {
	l.incomingMu.Lock()
	rx := l.incomingRx
	if rx == nil {
		l.incomingMu.Unlock()
		return nil
	}

	if len(data) == 0 {
		if int64(len(rx.buf)) >= rx.adv.TransferSize {
			inner := rx.buf[:rx.adv.TransferSize]
			l.incomingRx = nil
			l.incomingMu.Unlock()
			return l.deliverIncomingResource(inner, rx.adv)
		}
		l.incomingMu.Unlock()
		return nil
	}

	rx.buf = append(rx.buf, data...)
	if int64(len(rx.buf)) < rx.adv.TransferSize {
		l.incomingMu.Unlock()
		return nil
	}

	inner := rx.buf[:rx.adv.TransferSize]
	l.incomingRx = nil
	l.incomingMu.Unlock()
	return l.deliverIncomingResource(inner, rx.adv)
}

func (l *Link) deliverIncomingResource(inner []byte, adv *resource.ResourceAdvertisement) error {
	payload, err := l.assembleIncomingPayload(inner, adv)
	if err != nil {
		return err
	}
	if l.resourceConcludedCallback != nil {
		l.resourceConcludedCallback(payload)
	}
	return nil
}

func (l *Link) assembleIncomingPayload(inner []byte, adv *resource.ResourceAdvertisement) ([]byte, error) {
	var innerPlain []byte
	if adv.Encrypted {
		p, err := l.decrypt(inner)
		if err != nil {
			return nil, err
		}
		innerPlain = p
	} else {
		innerPlain = inner
	}

	if len(innerPlain) < resource.RANDOM_HASH_SIZE {
		return nil, errors.New("incoming resource too short for random hash")
	}
	if len(adv.RandomHash) == resource.RANDOM_HASH_SIZE && !bytes.Equal(innerPlain[:resource.RANDOM_HASH_SIZE], adv.RandomHash) {
		return nil, errors.New("incoming resource random hash mismatch")
	}
	data := innerPlain[resource.RANDOM_HASH_SIZE:]

	if adv.Compressed {
		r := bzip2.NewReader(bytes.NewReader(data))
		decompressed, err := io.ReadAll(r)
		if err != nil {
			return nil, err
		}
		data = decompressed
	}

	sum := sha256.Sum256(append(append([]byte(nil), data...), adv.RandomHash...))
	if len(adv.Hash) != len(sum) || !bytes.Equal(sum[:], adv.Hash) {
		return nil, errors.New("incoming resource hash mismatch")
	}

	return data, nil
}
