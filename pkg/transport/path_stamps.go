// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/debug"
	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/Reticulum-Go/pkg/stamps"
)

const (
	// prExtPayloadLen is the stamped path-request trailer size:
	// [0xC1, flag] + 14-byte stamp (Transport.PR_EXT_PAYLOADLENGTH).
	prExtPayloadLen    = 2 + stamps.StampLength
	prStampCacheMax    = 256
	prStampQueueCap    = 32
	prStampedPRDataLen = 64
)

// prStampCache is a FIFO-bounded cache of generated path-request stamps
// keyed on stamp material (dst+tag), matching Transport.pr_stamp_cache.
type prStampCache struct {
	sync.Mutex
	m     map[string][]byte
	order []string
}

func (c *prStampCache) get(material []byte) ([]byte, bool) {
	c.Lock()
	defer c.Unlock()
	s, ok := c.m[string(material)]
	return s, ok
}

func (c *prStampCache) put(material, stamp []byte) {
	c.Lock()
	defer c.Unlock()
	if c.m == nil {
		c.m = make(map[string][]byte)
	}
	key := string(material)
	if _, exists := c.m[key]; !exists {
		c.order = append(c.order, key)
	}
	c.m[key] = append([]byte(nil), stamp...)
	for len(c.order) > prStampCacheMax {
		delete(c.m, c.order[0])
		c.order = c.order[1:]
	}
}

type prStampJob struct {
	destHash  []byte
	tag       []byte
	ifaceName string
	recursive bool
}

// enqueueStampedPR defers a path request until its stamp is generated,
// matching Transport.enqueue_stamped_pr. Dedupes per stamp material.
func (t *Transport) enqueueStampedPR(job prStampJob) bool {
	t.prStampMu.Lock()
	defer t.prStampMu.Unlock()

	key := string(append(append([]byte{}, job.destHash...), job.tag...))
	if _, dup := t.prStampPending[key]; dup {
		return true
	}
	if t.prStampJobs == nil {
		t.prStampJobs = make(chan prStampJob, prStampQueueCap)
		go t.prStampWorker()
	}
	select {
	case t.prStampJobs <- job:
		if t.prStampPending == nil {
			t.prStampPending = make(map[string]struct{})
		}
		t.prStampPending[key] = struct{}{}
		return true
	default:
		return false
	}
}

func (t *Transport) prStampWorker() {
	for job := range t.prStampJobs {
		key := string(append(append([]byte{}, job.destHash...), job.tag...))
		t.prStampMu.Lock()
		delete(t.prStampPending, key)
		t.prStampMu.Unlock()

		settings := t.StampsSettings()
		material := append(append([]byte{}, job.destHash...), job.tag...)
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(settings.PRStampTimeout*float64(time.Second)))
		stamp, _, err := stamps.GenerateStamp(ctx, material, settings.PRValue, stamps.StampLength)
		cancel()
		if err != nil || len(stamp) == 0 {
			debug.Log(debug.DebugWarning, "Could not generate path request stamp, emission cancelled",
				"dest_hash", fmt.Sprintf("%x", job.destHash), "error", err)
			continue
		}
		t.prStamps.put(material, stamp)
		ext := append([]byte{stamps.ExtPayloadMarker, 0x00}, stamp...)
		if err := t.requestPath(job.destHash, job.ifaceName, job.tag, ext, job.recursive); err != nil {
			debug.Log(debug.DebugVerbose, "Stamped path request send failed",
				"dest_hash", fmt.Sprintf("%x", job.destHash), "error", err)
		}
	}
}

// validatePRStamp mirrors Transport.validate_pr_stamp: the stamp must be
// StampLength bytes and satisfy requiredCost against workblock dst+tag.
func validatePRStamp(stamp, destHash, tag []byte, requiredCost float64) bool {
	if len(stamp) != stamps.StampLength {
		return false
	}
	material := append(append([]byte{}, destHash...), tag...)
	return stamps.StampValid(stamp, requiredCost, material)
}

// interfacePRCost resolves the receiving interface's required path-request
// stamp cost, falling back to the transport default.
func (t *Transport) interfacePRCost(iface common.NetworkInterface, settings common.StampsSettings) float64 {
	if iface != nil {
		if sc, ok := iface.(interface {
			GetStampCosts() (float64, float64, bool, bool)
		}); ok {
			_, pr, _, prSet := sc.GetStampCosts()
			if prSet {
				return pr
			}
		}
	}
	return settings.PRCost
}

// prTransportID returns the 16-byte transport identity carried in emitted
// path requests (Python 1.6.0 always includes Transport.identity.hash).
func (t *Transport) prTransportID() []byte {
	if t.transportIdentity != nil {
		if h := t.transportIdentity.Hash(); len(h) == identity.TruncatedHashLength/8 {
			return h
		}
	}
	return make([]byte, identity.TruncatedHashLength/8)
}

// ParsePathRequest exposes parsePathRequestWire for cross-implementation
// cross-reference tests.
func ParsePathRequest(data []byte) (destHash, requestorTransportID, tag, ext []byte, ok bool) {
	return parsePathRequestWire(data)
}

// ValidatePRStamp exposes validatePRStamp for cross-reference tests.
func ValidatePRStamp(stamp, destHash, tag []byte, requiredCost float64) bool {
	return validatePRStamp(stamp, destHash, tag, requiredCost)
}
