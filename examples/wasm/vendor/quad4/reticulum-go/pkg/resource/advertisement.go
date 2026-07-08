// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package resource

import (
	"fmt"
	"math"

	"quad4/msgpack/v5/pkg/msgpack"
)

type ResourceAdvertisement struct {
	TransferSize  int64
	DataSize      int64
	Parts         int
	Hash          []byte
	RandomHash    []byte
	OriginalHash  []byte
	Hashmap       []byte
	Compressed    bool
	Encrypted     bool
	Split         bool
	HasMetadata   bool
	SegmentIndex  uint16
	TotalSegments uint16
	RequestID     []byte
	IsRequest     bool
	IsResponse    bool
	Flags         byte
}

func NewResourceAdvertisement(res *Resource) *ResourceAdvertisement {
	if res == nil {
		return nil
	}

	var flags byte
	if res.HasMetadata() {
		flags |= AdvFlagHasMetadata
	}
	if res.IsResponse() {
		flags |= AdvFlagIsResponse
	}
	if res.IsRequest() {
		flags |= AdvFlagIsRequest
	}

	res.mutex.RLock()
	split := res.split
	compressed := res.compressed
	encrypted := res.encrypted
	randomHash := res.randomHash
	originalHash := res.originalHash
	segmentIndex := res.segmentIndex
	totalSegments := res.totalSegments
	res.mutex.RUnlock()

	if split {
		flags |= AdvFlagSplit
	}
	if compressed {
		flags |= AdvFlagCompressed
	}
	if encrypted {
		flags |= AdvFlagEncrypted
	}

	hashmap := res.getHashmap()

	return &ResourceAdvertisement{
		TransferSize:  res.GetTransferSize(),
		DataSize:      res.GetDataSize(),
		Parts:         int(res.GetSegments()),
		Hash:          res.GetHash(),
		RandomHash:    randomHash,
		OriginalHash:  originalHash,
		Hashmap:       hashmap,
		Compressed:    compressed,
		Encrypted:     encrypted,
		Split:         split,
		HasMetadata:   res.HasMetadata(),
		SegmentIndex:  segmentIndex,
		TotalSegments: totalSegments,
		RequestID:     res.GetRequestID(),
		IsRequest:     res.IsRequest(),
		IsResponse:    res.IsResponse(),
		Flags:         flags,
	}
}

func (ra *ResourceAdvertisement) Pack(segment int, linkMDU int) ([]byte, error) {
	hashmapMaxLen := hashmapEntriesPerAdvSegment(linkMDU)
	hashmapStart := segment * hashmapMaxLen
	hashmapEnd := min(hashmapStart+hashmapMaxLen, len(ra.Hashmap)/MapHashLen)

	hashmap := ra.Hashmap[hashmapStart*MapHashLen : hashmapEnd*MapHashLen]

	dict := map[string]any{
		"t": packInt64Compact(ra.TransferSize),
		"d": packInt64Compact(ra.DataSize),
		"n": ra.Parts,
		"h": ra.Hash,
		"r": ra.RandomHash,
		"o": ra.OriginalHash,
		"i": int(ra.SegmentIndex),
		"l": int(ra.TotalSegments),
		"q": ra.RequestID,
		"f": ra.Flags,
		"m": hashmap,
	}

	return msgpack.Marshal(dict)
}

func packInt64Compact(v int64) any {
	if v >= int64(math.MinInt) && v <= int64(math.MaxInt) {
		return int(v)
	}
	return v
}

func UnpackResourceAdvertisement(data []byte) (*ResourceAdvertisement, error) {
	var dict map[string]any
	if err := msgpack.Unmarshal(data, &dict); err != nil {
		return nil, fmt.Errorf("failed to unpack advertisement: %w", err)
	}

	ra := &ResourceAdvertisement{}

	switch t := dict["t"].(type) {
	case int:
		ra.TransferSize = int64(t)
	case int8:
		ra.TransferSize = int64(t)
	case int16:
		ra.TransferSize = int64(t)
	case int32:
		ra.TransferSize = int64(t)
	case int64:
		ra.TransferSize = t
	case uint8:
		ra.TransferSize = int64(t)
	case uint16:
		ra.TransferSize = int64(t)
	case uint32:
		ra.TransferSize = int64(t)
	case uint64:
		if t > uint64(math.MaxInt64) {
			return nil, fmt.Errorf("transfer size overflow")
		}
		ra.TransferSize = int64(t) // #nosec G115 - checked for overflow
	}

	switch d := dict["d"].(type) {
	case int:
		ra.DataSize = int64(d)
	case int8:
		ra.DataSize = int64(d)
	case int16:
		ra.DataSize = int64(d)
	case int32:
		ra.DataSize = int64(d)
	case int64:
		ra.DataSize = d
	case uint8:
		ra.DataSize = int64(d)
	case uint16:
		ra.DataSize = int64(d)
	case uint32:
		ra.DataSize = int64(d)
	case uint64:
		if d > uint64(math.MaxInt64) {
			return nil, fmt.Errorf("data size overflow")
		}
		ra.DataSize = int64(d) // #nosec G115 - checked for overflow
	}

	switch n := dict["n"].(type) {
	case int:
		ra.Parts = n
	case int8:
		ra.Parts = int(n)
	case int16:
		ra.Parts = int(n)
	case int32:
		ra.Parts = int(n)
	case int64:
		ra.Parts = int(n)
	case uint8:
		ra.Parts = int(n)
	case uint16:
		ra.Parts = int(n)
	case uint32:
		ra.Parts = int(n)
	case uint64:
		if n > uint64(math.MaxInt32) {
			return nil, fmt.Errorf("parts count overflow")
		}
		ra.Parts = int(n) // #nosec G115 - checked for overflow
	}

	if h, ok := dict["h"].([]byte); ok {
		ra.Hash = h
	}

	if r, ok := dict["r"].([]byte); ok {
		ra.RandomHash = r
	}

	if o, ok := dict["o"].([]byte); ok {
		ra.OriginalHash = o
	}

	if m, ok := dict["m"].([]byte); ok {
		ra.Hashmap = m
	}

	if f, ok := dict["f"]; ok {
		flags, err := wireFlagsFromAny(f)
		if err != nil {
			return nil, err
		}
		ra.Flags = flags
	}

	ra.Encrypted = ra.Flags&AdvFlagEncrypted != 0
	ra.Compressed = ra.Flags&AdvFlagCompressed != 0
	ra.Split = ra.Flags&AdvFlagSplit != 0
	ra.IsRequest = ra.Flags&AdvFlagIsRequest != 0
	ra.IsResponse = ra.Flags&AdvFlagIsResponse != 0
	ra.HasMetadata = ra.Flags&AdvFlagHasMetadata != 0

	if i, ok := dict["i"].(uint16); ok {
		ra.SegmentIndex = i
	} else if i, ok := dict["i"].(uint64); ok {
		if i > uint64(math.MaxUint16) {
			return nil, fmt.Errorf("segment index overflow")
		}
		ra.SegmentIndex = uint16(i) // #nosec G115 - checked for overflow
	} else if i, ok := dict["i"].(int); ok {
		if i < 0 || i > math.MaxUint16 {
			return nil, fmt.Errorf("segment index out of range")
		}
		ra.SegmentIndex = uint16(i) // #nosec G115
	} else if i, ok := dict["i"].(int64); ok {
		if i < 0 || i > math.MaxUint16 {
			return nil, fmt.Errorf("segment index out of range")
		}
		ra.SegmentIndex = uint16(i) // #nosec G115
	}

	if l, ok := dict["l"].(uint16); ok {
		ra.TotalSegments = l
	} else if l, ok := dict["l"].(uint64); ok {
		if l > uint64(math.MaxUint16) {
			return nil, fmt.Errorf("total segments overflow")
		}
		ra.TotalSegments = uint16(l) // #nosec G115 - checked for overflow
	} else if l, ok := dict["l"].(int); ok {
		if l < 0 || l > math.MaxUint16 {
			return nil, fmt.Errorf("total segments out of range")
		}
		ra.TotalSegments = uint16(l) // #nosec G115
	} else if l, ok := dict["l"].(int64); ok {
		if l < 0 || l > math.MaxUint16 {
			return nil, fmt.Errorf("total segments out of range")
		}
		ra.TotalSegments = uint16(l) // #nosec G115
	}

	if q, ok := dict["q"].([]byte); ok {
		ra.RequestID = q
	}

	return ra, nil
}

func wireFlagsFromAny(f any) (byte, error) {
	switch v := f.(type) {
	case uint8:
		return v, nil
	case int:
		if v < 0 || v > 255 {
			return 0, fmt.Errorf("advertisement flags out of range")
		}
		return byte(v), nil
	case int8:
		if v < 0 {
			return 0, fmt.Errorf("advertisement flags out of range")
		}
		return byte(v), nil
	case int16:
		if v < 0 || v > 255 {
			return 0, fmt.Errorf("advertisement flags out of range")
		}
		return byte(v), nil
	case int32:
		if v < 0 || v > 255 {
			return 0, fmt.Errorf("advertisement flags out of range")
		}
		return byte(v), nil
	case int64:
		if v < 0 || v > 255 {
			return 0, fmt.Errorf("advertisement flags out of range")
		}
		return byte(v), nil
	case uint16:
		if v > 255 {
			return 0, fmt.Errorf("advertisement flags out of range")
		}
		return byte(v), nil
	case uint32:
		if v > 255 {
			return 0, fmt.Errorf("advertisement flags out of range")
		}
		return byte(v), nil
	case uint64:
		if v > 255 {
			return 0, fmt.Errorf("advertisement flags out of range")
		}
		return byte(v), nil
	default:
		return 0, nil
	}
}

func hashmapEntriesPerAdvSegment(linkMDU int) int {
	if linkMDU <= 0 {
		linkMDU = 384
	}
	return (linkMDU - Overhead) / MapHashLen
}

// HashmapEntriesPerSegment is the number of map-hash slots per advertisement or HMU segment for a link MDU.
func HashmapEntriesPerSegment(linkMDU int) int {
	return hashmapEntriesPerAdvSegment(linkMDU)
}

func IsRequestAdvertisement(data []byte) bool {
	adv, err := UnpackResourceAdvertisement(data)
	if err != nil {
		return false
	}
	return adv.IsRequest && adv.RequestID != nil
}

func IsResponseAdvertisement(data []byte) bool {
	adv, err := UnpackResourceAdvertisement(data)
	if err != nil {
		return false
	}
	return adv.IsResponse && adv.RequestID != nil
}

func ReadRequestID(data []byte) []byte {
	adv, err := UnpackResourceAdvertisement(data)
	if err != nil {
		return nil
	}
	return adv.RequestID
}

func ReadTransferSize(data []byte) int64 {
	adv, err := UnpackResourceAdvertisement(data)
	if err != nil {
		return 0
	}
	return adv.TransferSize
}

func ReadSize(data []byte) int64 {
	adv, err := UnpackResourceAdvertisement(data)
	if err != nil {
		return 0
	}
	return adv.DataSize
}
