// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"time"

	"quad4/reticulum-go/pkg/debug"
)

// OutgoingAnnounceQueue is the optional per-interface delayed TX list used
// when announce_cap would be exceeded. Matches Python Interface.announce_queue.
type OutgoingAnnounceQueue interface {
	ShouldQueueAnnounce() bool
	QueueOutgoingAnnounce(raw []byte, dest []byte, hops byte, emitted uint32)
	NoteAnnounceSent(nbytes int)
	AnnounceQueueLen() int
	DropAnnounceQueue() int
	NextOutgoingAnnounce() ([]byte, bool)
}

type queuedAnnounce struct {
	dest    [16]byte
	queued  time.Time
	hops    byte
	emitted uint32
	raw     []byte
}

// SetAnnounceCap stores announce_cap from config as a fraction of bitrate.
// percent 0 keeps the 2 percent default. Values are config percents (1 to 100).
func (i *BaseInterface) SetAnnounceCap(percent float64) {
	i.Mutex.Lock()
	defer i.Mutex.Unlock()
	if percent <= 0 {
		i.announceCap = 0
		return
	}
	i.announceCap = percent / 100.0
}

func (i *BaseInterface) announceCapFraction() float64 {
	if i.announceCap > 0 {
		return i.announceCap
	}
	return DefaultAnnounceCapFraction
}

// ShouldQueueAnnounce reports whether a forwarded announce must wait.
// Local origin (hops 0) is decided by the caller, matching Python Transport.outbound.
func (i *BaseInterface) ShouldQueueAnnounce() bool {
	i.Mutex.RLock()
	defer i.Mutex.RUnlock()
	if len(i.announceQueue) > 0 {
		return true
	}
	if i.Bitrate <= 0 {
		return true
	}
	return time.Now().Before(i.announceAllowedAt)
}

// QueueOutgoingAnnounce stores a forwarded announce for later TX.
// Same-destination entries keep the first unless emitted is strictly newer.
// Drops when the queue is already at MaxQueuedAnnounces.
func (i *BaseInterface) QueueOutgoingAnnounce(raw []byte, dest []byte, hops byte, emitted uint32) {
	if i == nil || len(raw) == 0 {
		return
	}
	entry := queuedAnnounce{
		queued:  time.Now(),
		hops:    hops,
		emitted: emitted,
		raw:     append([]byte(nil), raw...),
	}
	copy(entry.dest[:], dest)

	i.Mutex.Lock()
	defer i.Mutex.Unlock()
	if len(i.announceQueue) >= MaxQueuedAnnounces {
		return
	}
	for idx := range i.announceQueue {
		if i.announceQueue[idx].dest == entry.dest {
			if emitted > i.announceQueue[idx].emitted {
				i.announceQueue[idx] = entry
			}
			return
		}
	}
	i.announceQueue = append(i.announceQueue, entry)
	if debug.Enabled(debug.DebugVerbose) {
		debug.Log(debug.DebugVerbose, "Queued announce for later transmission",
			"name", i.Name, "queue", len(i.announceQueue), "hops", hops)
	}
}

// NoteAnnounceSent records announce_cap airtime after an immediate send.
func (i *BaseInterface) NoteAnnounceSent(nbytes int) {
	i.Mutex.Lock()
	defer i.Mutex.Unlock()
	i.noteAnnounceSentLocked(nbytes)
}

func (i *BaseInterface) noteAnnounceSentLocked(nbytes int) {
	capFrac := i.announceCapFraction()
	if i.Bitrate <= 0 || nbytes <= 0 || capFrac <= 0 {
		return
	}
	txTime := (float64(nbytes) * 8) / float64(i.Bitrate)
	wait := txTime / capFrac
	i.announceAllowedAt = time.Now().Add(time.Duration(wait * float64(time.Second)))
}

// AnnounceQueueLen is the number of announces waiting for announce_cap.
func (i *BaseInterface) AnnounceQueueLen() int {
	i.Mutex.RLock()
	defer i.Mutex.RUnlock()
	return len(i.announceQueue)
}

// DropAnnounceQueue discards queued outgoing announces and returns how many were dropped.
func (i *BaseInterface) DropAnnounceQueue() int {
	i.Mutex.Lock()
	defer i.Mutex.Unlock()
	n := len(i.announceQueue)
	i.announceQueue = nil
	return n
}

// NextOutgoingAnnounce pops the next announce that may be sent now.
// Prefer fewer hops, then older queue time. Returns false when empty, stale-only, or still capped.
func (i *BaseInterface) NextOutgoingAnnounce() ([]byte, bool) {
	i.Mutex.Lock()
	defer i.Mutex.Unlock()

	now := time.Now()
	kept := i.announceQueue[:0]
	for _, e := range i.announceQueue {
		if now.Sub(e.queued) > QueuedAnnounceLife {
			continue
		}
		kept = append(kept, e)
	}
	i.announceQueue = kept
	if len(i.announceQueue) == 0 {
		return nil, false
	}
	if now.Before(i.announceAllowedAt) || i.Bitrate <= 0 {
		return nil, false
	}

	sel := 0
	for idx := 1; idx < len(i.announceQueue); idx++ {
		a := i.announceQueue[idx]
		b := i.announceQueue[sel]
		if a.hops < b.hops || (a.hops == b.hops && a.queued.Before(b.queued)) {
			sel = idx
		}
	}
	chosen := i.announceQueue[sel]
	i.announceQueue = append(i.announceQueue[:sel], i.announceQueue[sel+1:]...)
	i.noteAnnounceSentLocked(len(chosen.raw))
	return append([]byte(nil), chosen.raw...), true
}
