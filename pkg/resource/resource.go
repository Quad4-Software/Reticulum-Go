// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package resource

import (
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Resource struct {
	mutex             sync.RWMutex
	data              []byte
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
	uniqueBytes := make(map[byte]struct{})
	for i := 0; i < sampleSize; i++ {
		uniqueBytes[data[i]] = struct{}{}
	}

	// Calculate entropy-based compression estimate
	uniqueRatio := float64(len(uniqueBytes)) / float64(sampleSize)
	return CompressionEntropyBase + (CompressionEntropyRange * uniqueRatio)
}

func estimateFileCompression(size int64, extension string) int64 {
	compressionRatios := map[string]float64{
		".txt":  CompressionRatioText,
		".log":  CompressionRatioText,
		".json": CompressionRatioText,
		".xml":  CompressionRatioText,
		".html": CompressionRatioText,
		".csv":  CompressionRatioCSV,
		".doc":  CompressionRatioOfficeLegacy,
		".docx": CompressionRatioOfficeModern,
		".pdf":  CompressionRatioOfficeModern,
		".jpg":  CompressionRatioAlreadyPacked,
		".jpeg": CompressionRatioAlreadyPacked,
		".png":  CompressionRatioAlreadyPacked,
		".gif":  CompressionRatioAlreadyPacked,
		".mp3":  CompressionRatioAlreadyPacked,
		".mp4":  CompressionRatioAlreadyPacked,
		".zip":  CompressionRatioAlreadyPacked,
		".gz":   CompressionRatioAlreadyPacked,
		".rar":  CompressionRatioAlreadyPacked,
	}

	ratio, exists := compressionRatios[extension]
	if !exists {
		ratio = CompressionRatioUnknown
	}

	return int64(float64(size) * ratio)
}

// PrepareOutboundForLink builds the inner ciphertext blob, hash, hashmap, and
// segment counts for sending a resource compatible with Reticulum peers.
// sdu is the maximum plaintext length per link data packet (link MDU).
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

	var body []byte
	switch {
	case r.data != nil:
		body = r.data
	case r.fileHandle != nil:
		if _, err := r.fileHandle.Seek(0, io.SeekStart); err != nil {
			return err
		}
		b, err := io.ReadAll(r.fileHandle)
		if err != nil {
			return err
		}
		body = b
	default:
		return errors.New("no data")
	}

	uncompressed := body
	randomHash := make([]byte, RandomHashSize)
	if _, err := io.ReadFull(rand.Reader, randomHash); err != nil {
		return err
	}

	payload := uncompressed
	if r.autoCompress {
		compressed, err := bzip2CompressBody(uncompressed)
		if err != nil {
			return err
		}
		if len(compressed) < len(uncompressed) {
			payload = compressed
			r.compressed = true
		} else {
			r.compressed = false
		}
	} else {
		r.compressed = false
	}

	h := sha256.Sum256(append(append([]byte(nil), uncompressed...), randomHash...))
	r.hash = h[:]
	r.randomHash = append([]byte(nil), randomHash...)
	r.originalHash = append([]byte(nil), r.hash...)

	plain := append(append([]byte(nil), randomHash...), payload...)
	innerBlob, err := encrypt(plain)
	if err != nil {
		return err
	}

	r.encrypted = true
	r.split = false
	r.totalSegments = 1
	r.segmentIndex = 1

	partCount := (len(innerBlob) + sdu - 1) / sdu
	if partCount > int(MaxSegments) {
		return errors.New("resource too large")
	}
	r.segments = uint16(partCount) // #nosec G115
	r.transferSize = int64(len(innerBlob))
	r.dataSize = int64(len(uncompressed))

	r.hashmap = make([]byte, partCount*MapHashLen)
	for i := 0; i < partCount; i++ {
		start := i * sdu
		end := start + sdu
		if end > len(innerBlob) {
			end = len(innerBlob)
		}
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
	return false
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
	for i := 0; i < n; i++ {
		off := i * MapHashLen
		if string(r.hashmap[off:off+MapHashLen]) == string(mh) {
			return i
		}
	}
	return -1
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
	end := start + sdu
	if end > len(r.outboundCipher) {
		end = len(r.outboundCipher)
	}
	out := make([]byte, end-start)
	copy(out, r.outboundCipher[start:end])
	return out
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
