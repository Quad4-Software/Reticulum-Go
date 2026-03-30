// SPDX-License-Identifier: 0BSD
// Copyright (c) 2024-2026 Sudo-Ivan / Quad4.io
package link

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"io"

	"git.quad4.io/Go-Libs/bzip2/pkg/bzip2"
	"git.quad4.io/Networks/Reticulum-Go/pkg/packet"
	"git.quad4.io/Networks/Reticulum-Go/pkg/resource"
)

const (
	hashmapNotExhausted = 0x00
)

type incomingResourceAsm struct {
	adv *resource.ResourceAdvertisement
	sdu int

	partSlots            [][]byte
	hashmapFlat          []byte
	totalParts           int
	consecutiveCompleted int
}

func (l *Link) beginIncomingResource(adv *resource.ResourceAdvertisement) error {
	l.mutex.RLock()
	sdu := l.mdu
	l.mutex.RUnlock()
	if sdu <= 0 {
		return errors.New("invalid mdu for incoming resource")
	}
	if adv.Parts <= 0 {
		return errors.New("invalid parts in advertisement")
	}
	need := adv.Parts * resource.MAPHASH_LEN
	if len(adv.Hashmap) < need {
		return errors.New("advertisement hashmap shorter than parts")
	}

	l.incomingMu.Lock()
	l.incomingRx = &incomingResourceAsm{
		adv:                  adv,
		sdu:                  sdu,
		partSlots:            make([][]byte, adv.Parts),
		hashmapFlat:          append([]byte(nil), adv.Hashmap[:need]...),
		totalParts:           adv.Parts,
		consecutiveCompleted: -1,
	}
	l.incomingMu.Unlock()
	return l.sendIncomingResourceReqNext()
}

func (l *Link) sendIncomingResourceReqNext() error {
	l.incomingMu.Lock()
	rx := l.incomingRx
	if rx == nil {
		l.incomingMu.Unlock()
		return nil
	}

	searchStart := rx.consecutiveCompleted + 1
	if searchStart < 0 {
		searchStart = 0
	}
	if searchStart >= rx.totalParts {
		l.incomingMu.Unlock()
		return nil
	}

	end := searchStart + resource.WINDOW
	if end > rx.totalParts {
		end = rx.totalParts
	}

	var requestedHashes []byte
	batch := 0
	for j := searchStart; j < end; j++ {
		if rx.partSlots[j] == nil {
			off := j * resource.MAPHASH_LEN
			if off+resource.MAPHASH_LEN > len(rx.hashmapFlat) {
				l.incomingMu.Unlock()
				return errors.New("incoming resource hashmap underrun")
			}
			mh := rx.hashmapFlat[off : off+resource.MAPHASH_LEN]
			requestedHashes = append(requestedHashes, mh...)
			batch++
			if batch >= resource.WINDOW {
				break
			}
		}
	}

	if len(requestedHashes) == 0 {
		l.incomingMu.Unlock()
		return nil
	}

	reqBody := append(append([]byte{hashmapNotExhausted}, rx.adv.Hash...), requestedHashes...)
	l.incomingMu.Unlock()

	return l.SendPacketWithContext(reqBody, packet.ContextResourceReq)
}

func (l *Link) resetIncomingResource() {
	l.incomingMu.Lock()
	l.incomingRx = nil
	l.incomingMu.Unlock()
}

func (l *Link) appendIncomingResourcePart(data []byte) error {
	l.incomingMu.Lock()
	rx := l.incomingRx
	if rx == nil {
		l.incomingMu.Unlock()
		return nil
	}

	if len(data) == 0 {
		if l.incomingTransferComplete(rx) {
			inner := l.concatIncomingParts(rx)
			adv := rx.adv
			l.incomingRx = nil
			l.incomingMu.Unlock()
			return l.deliverIncomingResource(inner, adv)
		}
		l.incomingMu.Unlock()
		return nil
	}

	rh := rx.adv.RandomHash
	if len(rh) != resource.RANDOM_HASH_SIZE {
		l.incomingMu.Unlock()
		return errors.New("bad random hash in advertisement")
	}
	h := sha256.Sum256(append(append([]byte(nil), data...), rh...))
	mh := h[:resource.MAPHASH_LEN]

	idx := -1
	for i := 0; i < rx.totalParts; i++ {
		off := i * resource.MAPHASH_LEN
		if off+resource.MAPHASH_LEN > len(rx.hashmapFlat) {
			break
		}
		if bytes.Equal(rx.hashmapFlat[off:off+resource.MAPHASH_LEN], mh) {
			idx = i
			break
		}
	}
	if idx < 0 {
		l.incomingMu.Unlock()
		return errors.New("incoming resource part map hash mismatch")
	}
	rx.partSlots[idx] = append([]byte(nil), data...)

	rx.consecutiveCompleted = consecutivePrefix(rx.partSlots)

	if l.incomingTransferComplete(rx) {
		inner := l.concatIncomingParts(rx)
		adv := rx.adv
		l.incomingRx = nil
		l.incomingMu.Unlock()
		return l.deliverIncomingResource(inner, adv)
	}

	l.incomingMu.Unlock()
	return l.sendIncomingResourceReqNext()
}

func consecutivePrefix(slots [][]byte) int {
	h := -1
	for i := 0; i < len(slots); i++ {
		if len(slots[i]) == 0 {
			break
		}
		h = i
	}
	return h
}

func (l *Link) incomingTransferComplete(rx *incomingResourceAsm) bool {
	var sum int64
	for i := 0; i < rx.totalParts; i++ {
		if rx.partSlots[i] == nil {
			return false
		}
		sum += int64(len(rx.partSlots[i]))
	}
	return sum == rx.adv.TransferSize
}

func (l *Link) concatIncomingParts(rx *incomingResourceAsm) []byte {
	var b []byte
	for i := 0; i < rx.totalParts; i++ {
		b = append(b, rx.partSlots[i]...)
	}
	return b
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
