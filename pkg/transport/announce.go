// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package transport

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"sync"
	"time"

	"git.quad4.io/Networks/Reticulum-Go/pkg/rate"
)

type AnnounceEntry struct {
	Data        []byte
	HopCount    int
	RetryCount  int
	LastRetry   time.Time
	SourceIface string
	Priority    float64
	Hash        string
}

type AnnounceManager struct {
	announces     map[string]*AnnounceEntry
	announceQueue map[string][]*AnnounceEntry
	rateLimiter   *rate.Limiter
	mutex         sync.RWMutex
}

func NewAnnounceManager() *AnnounceManager {
	return &AnnounceManager{
		announces:     make(map[string]*AnnounceEntry),
		announceQueue: make(map[string][]*AnnounceEntry),
		rateLimiter:   rate.NewLimiter(rate.DefaultBurstFreq, 10.0),
		mutex:         sync.RWMutex{},
	}
}

func (am *AnnounceManager) ProcessAnnounce(data []byte, sourceIface string) error {
	hash := sha256.Sum256(data)
	hashStr := hex.EncodeToString(hash[:])

	am.mutex.Lock()
	defer am.mutex.Unlock()

	if entry, exists := am.announces[hashStr]; exists {
		if entry.HopCount <= int(data[0]) {
			return nil
		}
		entry.HopCount = int(data[0])
		entry.Data = data
		entry.RetryCount = 0
		entry.LastRetry = time.Now()
		entry.Priority = calculatePriority(int(data[0]), 0)
		return nil
	}

	entry := &AnnounceEntry{
		Data:        data,
		HopCount:    int(data[0]),
		RetryCount:  0,
		LastRetry:   time.Now(),
		SourceIface: sourceIface,
		Priority:    calculatePriority(int(data[0]), 0),
		Hash:        hashStr,
	}

	am.announces[hashStr] = entry

	for iface := range am.announceQueue {
		if iface != sourceIface {
			am.queueAnnounce(entry, iface)
		}
	}

	return nil
}

func (am *AnnounceManager) queueAnnounce(entry *AnnounceEntry, iface string) {
	queue := am.announceQueue[iface]

	if len(queue) >= MaxQueueSize {
		// Remove lowest priority announce if queue is full
		queue = queue[:len(queue)-1]
	}

	insertIdx := sort.Search(len(queue), func(i int) bool {
		return queue[i].Priority < entry.Priority
	})

	queue = append(queue[:insertIdx], append([]*AnnounceEntry{entry}, queue[insertIdx:]...)...)
	am.announceQueue[iface] = queue
}

func (am *AnnounceManager) GetNextAnnounce(iface string) *AnnounceEntry {
	am.mutex.Lock()
	defer am.mutex.Unlock()

	queue := am.announceQueue[iface]
	if len(queue) == 0 {
		return nil
	}

	entry := queue[0]
	now := time.Now()

	if entry.RetryCount >= MaxRetries {
		am.announceQueue[iface] = queue[1:]
		delete(am.announces, entry.Hash)
		return am.GetNextAnnounce(iface)
	}

	if now.Sub(entry.LastRetry) < RetryInterval {
		return nil
	}

	if !am.rateLimiter.Allow() {
		return nil
	}

	entry.RetryCount++
	entry.LastRetry = now
	entry.Priority = calculatePriority(entry.HopCount, entry.RetryCount)

	am.announceQueue[iface] = queue[1:]
	am.queueAnnounce(entry, iface)

	return entry
}

func calculatePriority(hopCount int, retryCount int) float64 {
	den := hopCount
	if den < 1 {
		den = 1
	}
	basePriority := 1.0 / float64(den)
	retryPenalty := float64(retryCount) * MinPriorityDelta
	return basePriority - retryPenalty
}

func (am *AnnounceManager) CleanupExpired() {
	am.mutex.Lock()
	defer am.mutex.Unlock()

	now := time.Now()
	expiredHashes := make([]string, 0)

	for hash, entry := range am.announces {
		if entry.RetryCount >= MaxRetries || now.Sub(entry.LastRetry) > RetryInterval*MaxRetries {
			expiredHashes = append(expiredHashes, hash)
		}
	}

	for _, hash := range expiredHashes {
		delete(am.announces, hash)
		for iface, queue := range am.announceQueue {
			newQueue := make([]*AnnounceEntry, 0, len(queue))
			for _, entry := range queue {
				if entry.Hash != hash {
					newQueue = append(newQueue, entry)
				}
			}
			am.announceQueue[iface] = newQueue
		}
	}
}
