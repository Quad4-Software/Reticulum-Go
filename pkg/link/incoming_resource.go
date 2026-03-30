// SPDX-License-Identifier: 0BSD
// Copyright (c) 2024-2026 Sudo-Ivan / Quad4.io
package link

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"io"
	"math"

	"git.quad4.io/Go-Libs/bzip2/pkg/bzip2"
	"git.quad4.io/Networks/Reticulum-Go/pkg/packet"
	"git.quad4.io/Networks/Reticulum-Go/pkg/resource"
)

const (
	hashmapNotExhausted = 0x00
	hashmapExhausted    = 0xff
)

type incomingResourceAsm struct {
	adv *resource.ResourceAdvertisement
	sdu int

	partSlots            [][]byte
	mapHashes            [][]byte
	hashmapHeight        int
	totalParts           int
	consecutiveCompleted int
	waitingForHmu        bool
}

func (rx *incomingResourceAsm) applyHashmapSegment(segment int, hashmapBytes []byte) {
	segLen := resource.HashmapEntriesPerSegment(rx.sdu)
	if segLen <= 0 {
		segLen = 1
	}
	hashes := len(hashmapBytes) / resource.MAPHASH_LEN
	for i := 0; i < hashes; i++ {
		idx := i + segment*segLen
		if idx >= rx.totalParts {
			break
		}
		if rx.mapHashes[idx] == nil {
			rx.hashmapHeight++
		}
		off := i * resource.MAPHASH_LEN
		rx.mapHashes[idx] = append([]byte(nil), hashmapBytes[off:off+resource.MAPHASH_LEN]...)
	}
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
	if len(adv.Hashmap) == 0 || len(adv.Hashmap)%resource.MAPHASH_LEN != 0 {
		return errors.New("invalid advertisement hashmap")
	}

	rx := &incomingResourceAsm{
		adv:                  adv,
		sdu:                  sdu,
		partSlots:            make([][]byte, adv.Parts),
		mapHashes:            make([][]byte, adv.Parts),
		totalParts:           adv.Parts,
		consecutiveCompleted: -1,
	}
	rx.applyHashmapSegment(0, adv.Hashmap)

	l.incomingMu.Lock()
	l.incomingRx = rx
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
	if rx.waitingForHmu {
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
	exhausted := false
	batch := 0
	for pn := searchStart; pn < end; pn++ {
		if rx.partSlots[pn] != nil {
			continue
		}
		mh := rx.mapHashes[pn]
		if mh != nil {
			requestedHashes = append(requestedHashes, mh...)
			batch++
			if batch >= resource.WINDOW {
				break
			}
			continue
		}
		exhausted = true
		break
	}

	if len(requestedHashes) == 0 && !exhausted {
		l.incomingMu.Unlock()
		return nil
	}

	var prefix []byte
	if exhausted {
		if rx.hashmapHeight == 0 || rx.hashmapHeight-1 >= len(rx.mapHashes) {
			l.incomingMu.Unlock()
			return errors.New("incoming resource cannot request hashmap extension")
		}
		last := rx.mapHashes[rx.hashmapHeight-1]
		if len(last) != resource.MAPHASH_LEN {
			l.incomingMu.Unlock()
			return errors.New("invalid last map hash for HMU request")
		}
		prefix = append([]byte{hashmapExhausted}, last...)
		rx.waitingForHmu = true
	} else {
		prefix = []byte{hashmapNotExhausted}
	}

	reqBody := append(prefix, rx.adv.Hash...)
	reqBody = append(reqBody, requestedHashes...)
	l.incomingMu.Unlock()

	return l.SendPacketWithContext(reqBody, packet.ContextResourceReq)
}

func (l *Link) resetIncomingResource() {
	l.incomingMu.Lock()
	l.incomingRx = nil
	l.incomingMu.Unlock()
}

func (l *Link) applyIncomingHashmapUpdate(resHash []byte, segment int, hashmapBytes []byte) error {
	l.incomingMu.Lock()
	rx := l.incomingRx
	if rx == nil || rx.adv == nil || !bytes.Equal(resHash, rx.adv.Hash) {
		l.incomingMu.Unlock()
		return nil
	}
	rx.applyHashmapSegment(segment, hashmapBytes)
	rx.waitingForHmu = false
	l.incomingMu.Unlock()
	return l.sendIncomingResourceReqNext()
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
		if rx.partSlots[i] != nil {
			continue
		}
		if len(rx.mapHashes[i]) != resource.MAPHASH_LEN {
			continue
		}
		if bytes.Equal(rx.mapHashes[i], mh) {
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

func wireInt(v any) (int, bool) {
	switch x := v.(type) {
	case int:
		return x, true
	case int8:
		return int(x), true
	case int16:
		return int(x), true
	case int32:
		return int(x), true
	case int64:
		if x > int64(math.MaxInt) || x < int64(math.MinInt) {
			return 0, false
		}
		return int(x), true
	case uint8:
		return int(x), true
	case uint16:
		return int(x), true
	case uint32:
		if int64(x) > int64(math.MaxInt) {
			return 0, false
		}
		return int(x), true
	case uint64:
		if x > uint64(math.MaxInt) {
			return 0, false
		}
		return int(x), true
	default:
		return 0, false
	}
}
