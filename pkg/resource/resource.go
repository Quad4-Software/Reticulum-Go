// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package resource

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"maps"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"quad4/msgpack/v5/pkg/msgpack"
)

type Resource struct {
	mutex             sync.RWMutex
	data              []byte
	sourceData        []byte
	fileHandle        io.ReadWriteSeeker
	fileName          string
	hash              []byte
	randomHash        []byte
	originalHash      []byte
	status            byte
	compressed        bool
	autoCompress      bool
	encrypted         bool
	split             bool
	segments          uint16
	segmentIndex      uint16
	totalSegments     uint16
	completedParts    map[uint16]bool
	transferSize      int64
	dataSize          int64
	progress          float64
	createdAt         time.Time
	completedAt       time.Time
	callback          func(*Resource)
	progressCallback  func(*Resource)
	readOffset        int64
	requestID         []byte
	isResponse        bool
	hashmap           []byte
	outboundCipher    []byte
	outboundPartSent  []bool
	outboundSentCount int
	metadata          map[string]any
	metadataPacked    []byte
}

func New(data any, autoCompress bool) (*Resource, error) {
	r := &Resource{
		status:         StatusPending,
		compressed:     false,
		autoCompress:   autoCompress,
		completedParts: make(map[uint16]bool),
		createdAt:      time.Now(),
		progress:       0.0,
	}

	switch v := data.(type) {
	case []byte:
		r.data = v
		r.dataSize = int64(len(v))
	case io.ReadWriteSeeker:
		r.fileHandle = v
		size, err := v.Seek(0, io.SeekEnd)
		if err != nil {
			return nil, err
		}
		r.dataSize = size
		_, err = v.Seek(0, io.SeekStart)
		if err != nil {
			return nil, err
		}

		if namer, ok := v.(interface{ Name() string }); ok {
			r.fileName = namer.Name()
		}
	default:
		return nil, errors.New("unsupported data type")
	}

	// Calculate segments needed
	r.segments = uint16((r.dataSize + DefaultSegmentSize - 1) / DefaultSegmentSize) // #nosec G115
	if r.segments > MaxSegments {
		return nil, errors.New("resource too large")
	}

	// Calculate transfer size
	r.transferSize = r.dataSize
	if r.autoCompress {
		// Estimate compressed size based on data type and content
		if r.data != nil {
			// For in-memory data, we can analyze content
			compressibility := estimateCompressibility(r.data)
			r.transferSize = int64(float64(r.dataSize) * compressibility)
		} else if r.fileHandle != nil {
			// For file handles, use extension-based estimation
			ext := strings.ToLower(filepath.Ext(r.fileName))
			r.transferSize = estimateFileCompression(r.dataSize, ext)
		}

		// Ensure minimum size and add compression overhead
		if r.transferSize < r.dataSize/10 {
			r.transferSize = r.dataSize / 10
		}
		r.transferSize += 64 // Header overhead for compression
	}

	// Calculate resource hash
	if err := r.calculateHash(); err != nil {
		return nil, err
	}

	return r, nil
}

func (r *Resource) calculateHash() error {
	h := sha256.New()

	if r.data != nil {
		h.Write(r.data)
	} else if r.fileHandle != nil {
		if _, err := r.fileHandle.Seek(0, io.SeekStart); err != nil {
			return err
		}
		if _, err := io.Copy(h, r.fileHandle); err != nil {
			return err
		}
		if _, err := r.fileHandle.Seek(0, io.SeekStart); err != nil {
			return err
		}
	}

	r.hash = h.Sum(nil)
	return nil
}

func (r *Resource) GetHash() []byte {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return append([]byte{}, r.hash...)
}

func (r *Resource) GetStatus() byte {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return r.status
}

func (r *Resource) GetProgress() float64 {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return r.progress
}

func (r *Resource) GetTransferSize() int64 {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return r.transferSize
}

func (r *Resource) GetDataSize() int64 {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return r.dataSize
}

func (r *Resource) GetSegments() uint16 {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return r.segments
}

func (r *Resource) Cancel() {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.status == StatusPending || r.status == StatusActive {
		r.status = StatusCancelled
		r.completedAt = time.Now()
		if r.callback != nil {
			r.callback(r)
		}
	}
}

func (r *Resource) SetCallback(callback func(*Resource)) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.callback = callback
}

func (r *Resource) SetProgressCallback(callback func(*Resource)) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.progressCallback = callback
}

// GetSegmentData returns the data for a specific segment
func (r *Resource) GetSegmentData(segment uint16) ([]byte, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	if segment >= r.segments {
		return nil, errors.New("invalid segment number")
	}

	start := int64(segment) * DefaultSegmentSize
	size := DefaultSegmentSize
	if segment == r.segments-1 {
		size = int(r.dataSize - start)
	}

	data := make([]byte, size)
	if r.data != nil {
		copy(data, r.data[start:start+int64(size)])
		return data, nil
	}

	if r.fileHandle != nil {
		if _, err := r.fileHandle.Seek(start, io.SeekStart); err != nil {
			return nil, err
		}
		if _, err := io.ReadFull(r.fileHandle, data); err != nil {
			return nil, err
		}
		return data, nil
	}

	return nil, errors.New("no data source available")
}

// MarkSegmentComplete marks a segment as completed and updates progress
func (r *Resource) MarkSegmentComplete(segment uint16) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if segment >= r.segments {
		return
	}

	r.completedParts[segment] = true
	completed := len(r.completedParts)
	r.progress = float64(completed) / float64(r.segments)

	if r.progressCallback != nil {
		r.progressCallback(r)
	}

	// Check if all segments are complete
	if completed == int(r.segments) {
		r.status = StatusComplete
		r.completedAt = time.Now()
		if r.callback != nil {
			r.callback(r)
		}
	}
}

// IsSegmentComplete checks if a specific segment is complete
func (r *Resource) IsSegmentComplete(segment uint16) bool {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return r.completedParts[segment]
}

// Activate marks the resource as active
func (r *Resource) Activate() {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if r.status == StatusPending {
		r.status = StatusActive
	}
}

// SetFailed marks the resource as failed
func (r *Resource) SetFailed() {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if r.status != StatusComplete {
		r.status = StatusFailed
		r.completedAt = time.Now()
		if r.callback != nil {
			r.callback(r)
		}
	}
}

// Helper functions for compression estimation
func estimateCompressibility(data []byte) float64 {
	// Sample the data to estimate compressibility
	sampleSize := min(len(data), 4096)

	// Count unique bytes in sample
	var seen [256]bool
	unique := 0
	for i := range sampleSize {
		b := data[i]
		if !seen[b] {
			seen[b] = true
			unique++
		}
	}

	// Calculate entropy-based compression estimate
	uniqueRatio := float64(unique) / float64(sampleSize)
	return CompressionEntropyBase + (CompressionEntropyRange * uniqueRatio)
}

func estimateFileCompression(size int64, extension string) int64 {
	var ratio float64
	switch extension {
	case ".txt", ".log", ".json", ".xml", ".html":
		ratio = CompressionRatioText
	case ".csv":
		ratio = CompressionRatioCSV
	case ".doc":
		ratio = CompressionRatioOfficeLegacy
	case ".docx", ".pdf":
		ratio = CompressionRatioOfficeModern
	case ".jpg", ".jpeg", ".png", ".gif", ".mp3", ".mp4", ".zip", ".gz", ".rar":
		ratio = CompressionRatioAlreadyPacked
	default:
		ratio = CompressionRatioUnknown
	}

	return int64(float64(size) * ratio)
}

// PrepareOutboundForLink builds the inner ciphertext blob, hash, hashmap, and
// segment counts for sending a resource compatible with Reticulum peers.
// sdu is the maximum plaintext length per link data packet (link MDU).
// Resources larger than MaxEfficientSize are split across multiple advertisements
// matching Python Resource.MAX_EFFICIENT_SIZE behavior.
func (r *Resource) PrepareOutboundForLink(encrypt func([]byte) ([]byte, error), sdu int) error {
	if sdu <= 0 {
		return errors.New("invalid sdu")
	}
	if encrypt == nil {
		return errors.New("nil encrypt")
	}

	r.mutex.Lock()
	defer r.mutex.Unlock()

	r.outboundPartSent = nil
	r.outboundSentCount = 0

	if err := r.ensureMetadataPackedLocked(); err != nil {
		return err
	}
	metaLen := len(r.metadataPacked)

	fileSize, err := r.rawBodySizeLocked()
	if err != nil {
		return err
	}
	totalSize := fileSize + int64(metaLen)

	if r.segmentIndex == 0 {
		r.segmentIndex = 1
	}

	if totalSize <= int64(MaxEfficientSize) {
		r.split = false
		r.totalSegments = 1
		r.segmentIndex = 1
		body, err := r.readRawBodyLocked(0, fileSize)
		if err != nil {
			return err
		}
		return r.finishPrepareSegmentLocked(encrypt, sdu, body, true)
	}

	r.split = true
	r.totalSegments = uint16((totalSize-1)/int64(MaxEfficientSize) + 1) // #nosec G115
	if r.segmentIndex < 1 || r.segmentIndex > r.totalSegments {
		return fmt.Errorf("invalid segment index %d of %d", r.segmentIndex, r.totalSegments)
	}

	firstRead := int64(MaxEfficientSize) - int64(metaLen)
	if firstRead < 0 {
		return errors.New("metadata exceeds efficient segment size")
	}

	var offset, length int64
	includeMeta := false
	if r.segmentIndex == 1 {
		offset = 0
		length = min(firstRead, fileSize)
		includeMeta = true
	} else {
		offset = firstRead + int64(r.segmentIndex-2)*int64(MaxEfficientSize)
		length = int64(MaxEfficientSize)
		if offset+length > fileSize {
			length = fileSize - offset
		}
		if length < 0 {
			length = 0
		}
	}

	body, err := r.readRawBodyLocked(offset, length)
	if err != nil {
		return err
	}
	return r.finishPrepareSegmentLocked(encrypt, sdu, body, includeMeta)
}

// PrepareNextOutboundSegment advances to the next split segment and prepares it.
func (r *Resource) PrepareNextOutboundSegment(encrypt func([]byte) ([]byte, error), sdu int) error {
	r.mutex.Lock()
	if !r.split || r.segmentIndex >= r.totalSegments {
		r.mutex.Unlock()
		return errors.New("no next segment")
	}
	r.segmentIndex++
	r.mutex.Unlock()
	return r.PrepareOutboundForLink(encrypt, sdu)
}

func (r *Resource) rawBodySizeLocked() (int64, error) {
	src := r.data
	if r.sourceData != nil {
		src = r.sourceData
	}
	switch {
	case src != nil:
		return int64(len(src)), nil
	case r.fileHandle != nil:
		cur, err := r.fileHandle.Seek(0, io.SeekCurrent)
		if err != nil {
			return 0, err
		}
		end, err := r.fileHandle.Seek(0, io.SeekEnd)
		if err != nil {
			return 0, err
		}
		if _, err := r.fileHandle.Seek(cur, io.SeekStart); err != nil {
			return 0, err
		}
		return end, nil
	default:
		return 0, errors.New("no data")
	}
}

func (r *Resource) readRawBodyLocked(offset, length int64) ([]byte, error) {
	if length < 0 {
		return nil, errors.New("negative read length")
	}
	if length == 0 {
		return []byte{}, nil
	}
	src := r.data
	if r.sourceData != nil {
		src = r.sourceData
	}
	switch {
	case src != nil:
		if offset > int64(len(src)) {
			return nil, errors.New("segment offset past end of data")
		}
		end := min(offset+length, int64(len(src)))
		return append([]byte(nil), src[offset:end]...), nil
	case r.fileHandle != nil:
		if _, err := r.fileHandle.Seek(offset, io.SeekStart); err != nil {
			return nil, err
		}
		buf := make([]byte, length)
		n, err := io.ReadFull(r.fileHandle, buf)
		if err == io.ErrUnexpectedEOF || err == io.EOF {
			return buf[:n], nil
		}
		if err != nil {
			return nil, err
		}
		return buf, nil
	default:
		return nil, errors.New("no data")
	}
}

func (r *Resource) finishPrepareSegmentLocked(encrypt func([]byte) ([]byte, error), sdu int, segmentBody []byte, includeMeta bool) error {
	keepOriginal := append([]byte(nil), r.originalHash...)

	uncompressed := segmentBody
	wireBody := uncompressed
	if includeMeta && len(r.metadataPacked) > 0 {
		wireBody = append(append([]byte(nil), r.metadataPacked...), uncompressed...)
	}

	randomHash := make([]byte, RandomHashSize)
	if _, err := io.ReadFull(rand.Reader, randomHash); err != nil {
		return err
	}

	payload := wireBody
	if r.autoCompress {
		compressed, err := bzip2CompressBody(wireBody)
		if err != nil {
			return err
		}
		if len(compressed) < len(wireBody) {
			payload = compressed
			r.compressed = true
		} else {
			r.compressed = false
		}
	} else {
		r.compressed = false
	}

	hb := sha256.New()
	hb.Write(wireBody)
	hb.Write(randomHash)
	r.hash = hb.Sum(nil)
	r.randomHash = append([]byte(nil), randomHash...)
	if len(keepOriginal) == sha256.Size {
		r.originalHash = keepOriginal
	} else {
		r.originalHash = append([]byte(nil), r.hash...)
	}

	// ExpectedProof prepends metadataPacked when present on segment 1.
	if r.split && r.sourceData == nil && r.data != nil {
		r.sourceData = r.data
	}
	r.data = append([]byte(nil), segmentBody...)

	plain := make([]byte, len(randomHash)+len(payload))
	copy(plain, randomHash)
	copy(plain[len(randomHash):], payload)
	innerBlob, err := encrypt(plain)
	if err != nil {
		return err
	}

	r.encrypted = true

	partCount := (len(innerBlob) + sdu - 1) / sdu
	if partCount > int(MaxSegments) {
		return errors.New("resource too large")
	}
	r.segments = uint16(partCount) // #nosec G115
	r.transferSize = int64(len(innerBlob))
	r.dataSize = int64(len(wireBody))

	r.hashmap = make([]byte, partCount*MapHashLen)
	for i := range partCount {
		start := i * sdu
		end := min(start+sdu, len(innerBlob))
		h := sha256.New()
		h.Write(innerBlob[start:end])
		h.Write(randomHash)
		partHash := h.Sum(nil)
		copy(r.hashmap[i*MapHashLen:], partHash[:MapHashLen])
	}

	r.outboundCipher = innerBlob
	r.readOffset = 0
	return nil
}

func (r *Resource) Read(p []byte) (n int, err error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.outboundCipher != nil {
		if r.readOffset >= int64(len(r.outboundCipher)) {
			return 0, io.EOF
		}
		n = copy(p, r.outboundCipher[r.readOffset:])
		r.readOffset += int64(n)
		return n, nil
	}

	if r.data != nil {
		if r.readOffset >= int64(len(r.data)) {
			return 0, io.EOF
		}
		n = copy(p, r.data[r.readOffset:])
		r.readOffset += int64(n)
		return n, nil
	}

	if r.fileHandle != nil {
		return r.fileHandle.Read(p)
	}

	return 0, errors.New("no data source available")
}

func (r *Resource) GetName() string {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return r.fileName
}

func (r *Resource) GetSize() int64 {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return r.dataSize
}

func (r *Resource) HasMetadata() bool {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return len(r.metadata) > 0 || len(r.metadataPacked) > 0
}

// SetMetadata attaches a metadata map transferred ahead of the file bytes.
// Keys and values must be msgpack-safe.
func (r *Resource) SetMetadata(meta map[string]any) error {
	if r == nil {
		return errors.New("nil resource")
	}
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if meta == nil {
		r.metadata = nil
		r.metadataPacked = nil
		return nil
	}
	r.metadata = meta
	r.metadataPacked = nil
	return r.ensureMetadataPackedLocked()
}

// Metadata returns a shallow copy of the attached metadata map.
func (r *Resource) Metadata() map[string]any {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	if len(r.metadata) == 0 {
		return nil
	}
	out := make(map[string]any, len(r.metadata))
	maps.Copy(out, r.metadata)
	return out
}

func (r *Resource) ensureMetadataPackedLocked() error {
	if len(r.metadata) == 0 {
		r.metadataPacked = nil
		return nil
	}
	if len(r.metadataPacked) > 0 {
		return nil
	}
	packed, err := msgpack.Marshal(r.metadata)
	if err != nil {
		return err
	}
	if len(packed) > MetadataMaxSize {
		return errors.New("resource metadata size exceeded")
	}
	// 3-byte big-endian length prefix (high 24 bits of a 32-bit length).
	blob := make([]byte, 3+len(packed))
	n := len(packed)
	blob[0] = byte(n >> 16) // #nosec G115 -- n bounded by MetadataMaxSize
	blob[1] = byte(n >> 8)  // #nosec G115 -- n bounded by MetadataMaxSize
	blob[2] = byte(n)       // #nosec G115 -- n bounded by MetadataMaxSize
	copy(blob[3:], packed)
	r.metadataPacked = blob
	return nil
}

func (r *Resource) IsRequest() bool {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return r.requestID != nil && !r.isResponse
}

func (r *Resource) IsResponse() bool {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return r.isResponse
}

func (r *Resource) GetRequestID() []byte {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	if r.requestID == nil {
		return nil
	}
	return append([]byte{}, r.requestID...)
}

func (r *Resource) SetRequestID(id []byte) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if id == nil {
		r.requestID = nil
		return
	}
	r.requestID = append([]byte{}, id...)
}

func (r *Resource) SetIsResponse(isResponse bool) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.isResponse = isResponse
}

func (r *Resource) getHashmap() []byte {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	if r.hashmap == nil {
		return nil
	}
	return append([]byte{}, r.hashmap...)
}

// PartIndexForMapHash returns the part index whose map hash equals mh, or -1.
func (r *Resource) PartIndexForMapHash(mh []byte) int {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	if r.hashmap == nil || len(mh) != MapHashLen {
		return -1
	}
	n := len(r.hashmap) / MapHashLen
	for i := range n {
		off := i * MapHashLen
		if bytes.Equal(r.hashmap[off:off+MapHashLen], mh) {
			return i
		}
	}
	return -1
}

// PartIndicesForMapHash returns all part indexes whose map hash equals mh.
func (r *Resource) PartIndicesForMapHash(mh []byte) []int {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	if r.hashmap == nil || len(mh) != MapHashLen {
		return nil
	}
	n := len(r.hashmap) / MapHashLen
	indexes := make([]int, 0, 1)
	for i := range n {
		off := i * MapHashLen
		if bytes.Equal(r.hashmap[off:off+MapHashLen], mh) {
			indexes = append(indexes, i)
		}
	}
	return indexes
}

// OutboundCiphertextSlice returns the ciphertext bytes for part i using the given SDU.
func (r *Resource) OutboundCiphertextSlice(partIndex int, sdu int) []byte {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	if r.outboundCipher == nil || sdu <= 0 {
		return nil
	}
	n := int(r.segments)
	if partIndex < 0 || partIndex >= n {
		return nil
	}
	start := partIndex * sdu
	if start >= len(r.outboundCipher) {
		return nil
	}
	end := min(start+sdu, len(r.outboundCipher))
	out := make([]byte, end-start)
	copy(out, r.outboundCipher[start:end])
	return out
}

// OutboundCiphertextView returns a copy of outbound ciphertext for part i.
func (r *Resource) OutboundCiphertextView(partIndex int, sdu int) []byte {
	return r.OutboundCiphertextSlice(partIndex, sdu)
}

// OutboundCiphertextSliceInto copies ciphertext bytes for part i into dst and returns dst.
// If dst capacity is insufficient, a new buffer is allocated.
func (r *Resource) OutboundCiphertextSliceInto(dst []byte, partIndex int, sdu int) []byte {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	if r.outboundCipher == nil || sdu <= 0 {
		return nil
	}
	n := int(r.segments)
	if partIndex < 0 || partIndex >= n {
		return nil
	}
	start := partIndex * sdu
	if start >= len(r.outboundCipher) {
		return nil
	}
	end := min(start+sdu, len(r.outboundCipher))
	need := end - start
	if cap(dst) < need {
		dst = make([]byte, need)
	} else {
		dst = dst[:need]
	}
	copy(dst, r.outboundCipher[start:end])
	return dst
}

// MarkOutboundPartSent records that part i has been transmitted at least once.
// It returns true when every part index has been sent at least once.
func (r *Resource) MarkOutboundPartSent(i int) bool {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	n := int(r.segments)
	if n == 0 {
		return true
	}
	if i < 0 || i >= n {
		return false
	}
	if r.outboundPartSent == nil {
		r.outboundPartSent = make([]bool, n)
	}
	if !r.outboundPartSent[i] {
		r.outboundPartSent[i] = true
		r.outboundSentCount++
	}
	return r.outboundSentCount >= n
}

// IsOutboundPartSent reports whether part i has already been sent at least once.
func (r *Resource) IsOutboundPartSent(i int) bool {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	n := int(r.segments)
	if i < 0 || i >= n || r.outboundPartSent == nil {
		return false
	}
	return r.outboundPartSent[i]
}

// HashmapSegment returns a copy of the hashmap bytes for segment index.
func (r *Resource) HashmapSegment(linkMDU int, segment int) []byte {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	if segment < 0 || len(r.hashmap) == 0 {
		return nil
	}
	entries := HashmapEntriesPerSegment(linkMDU)
	if entries <= 0 {
		entries = 1
	}
	totalEntries := len(r.hashmap) / MapHashLen
	startEntry := segment * entries
	if startEntry >= totalEntries {
		return nil
	}
	endEntry := min(startEntry+entries, totalEntries)
	start := startEntry * MapHashLen
	end := endEntry * MapHashLen
	out := make([]byte, end-start)
	copy(out, r.hashmap[start:end])
	return out
}

// MapHashAt returns the map hash at part index i.
func (r *Resource) MapHashAt(i int) []byte {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	if i < 0 {
		return nil
	}
	off := i * MapHashLen
	if off+MapHashLen > len(r.hashmap) {
		return nil
	}
	out := make([]byte, MapHashLen)
	copy(out, r.hashmap[off:off+MapHashLen])
	return out
}

// OutboundTransferComplete reports whether every part has been sent at least once.
func (r *Resource) OutboundTransferComplete() bool {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return int(r.segments) > 0 && r.outboundSentCount >= int(r.segments)
}

func (r *Resource) GetRandomHash() []byte {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	if r.randomHash == nil {
		return nil
	}
	return append([]byte{}, r.randomHash...)
}

// ExpectedProof returns SHA256(uncompressedPayload || resourceHash).
// When metadata is present on segment 1 the uncompressed payload is the
// metadata blob prepended to the file bytes.
func (r *Resource) ExpectedProof() ([]byte, bool) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	if len(r.hash) != sha256.Size {
		return nil, false
	}
	if r.data == nil {
		return nil, false
	}
	body := r.data
	if len(r.metadataPacked) > 0 && (r.segmentIndex == 0 || r.segmentIndex == 1) {
		body = append(append([]byte(nil), r.metadataPacked...), r.data...)
	}
	sum := sha256.Sum256(append(append([]byte(nil), body...), r.hash...))
	return sum[:], true
}

func (r *Resource) GetOriginalHash() []byte {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	if r.originalHash == nil {
		return nil
	}
	return append([]byte{}, r.originalHash...)
}

func (r *Resource) GetSegmentIndex() uint16 {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return r.segmentIndex
}

func (r *Resource) GetTotalSegments() uint16 {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return r.totalSegments
}

func (r *Resource) IsEncrypted() bool {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return r.encrypted
}

func (r *Resource) IsSplit() bool {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return r.split
}
