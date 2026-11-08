// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package destination

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sync"

	"github.com/Quad4-Software/Reticulum-Go/internal/storage"
	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/debug"
	"github.com/Quad4-Software/Reticulum-Go/pkg/stamps"
)

const stamperQueueCap = 32

// SetValue sets the target stamp value for this destination, clamped to
// [DESTINATION_VALUE, STAMP_COST_CAP] like Python Destination.set_value.
func (d *Destination) SetValue(value float64) {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	settings := d.stampsSettingsLocked()
	d.targetValue = min(common.StampCostCap, max(value, settings.DestinationValue))
}

// TargetValue returns the effective target stamp value (Python
// Destination.target_value).
func (d *Destination) TargetValue() float64 {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	settings := d.stampsSettingsLocked()
	return min(common.StampCostCap, max(d.targetValue, settings.DestinationValue))
}

// SetLinkCost sets the required link-request stamp cost announced by this
// destination, clamped to [0, LINK_COST_CAP]. Set 0 to disable.
func (d *Destination) SetLinkCost(cost float64) {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	d.linkCost = max(0, min(cost, common.LinkCostCap))
	d.linkCostSet = true
}

// GetLinkCost returns the announced link-request stamp cost (Python
// Destination.get_link_cost): the destination-specific cost when set, else
// the transport LR_COST, clamped to [0, STAMP_COST_CAP].
func (d *Destination) GetLinkCost() float64 {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	settings := d.stampsSettingsLocked()
	cost := settings.LRCost
	if d.linkCostSet {
		cost = d.linkCost
	}
	return max(0, min(cost, common.StampCostCap))
}

// Stamp returns the destination stamp and its value, or nil when unstamped.
func (d *Destination) Stamp() (stamp []byte, value float64) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return append([]byte(nil), d.stamp...), d.stampValue
}

func (d *Destination) stampsSettingsLocked() common.StampsSettings {
	if d.transport != nil {
		return d.transport.GetConfig().StampsSettings()
	}
	return common.DefaultStampsSettings()
}

// stampMaterial returns dest_hash || public_key || name_hash, the detached
// stamp workblock (expand_rounds=0) per Python Destination.stamp_material.
func (d *Destination) stampMaterial() []byte {
	nameHash := sha256.Sum256([]byte(d.ExpandName()))
	material := make([]byte, 0, len(d.hashValue)+64+10)
	material = append(material, d.hashValue...)
	material = append(material, d.identity.GetPublicKey()...)
	material = append(material, nameHash[:10]...)
	return material
}

func (d *Destination) stampStoragePath() string {
	var cfgPath string
	if d.transport != nil {
		if cfg := d.transport.GetConfig(); cfg != nil {
			cfgPath = cfg.ConfigPath
		}
	}
	dir, err := storage.DataDir(cfgPath)
	if err != nil {
		return ""
	}
	return dir
}

// loadDestinationStamp tries the persisted stamp once, matching the stamp
// load at Python Destination creation.
func (d *Destination) loadDestinationStamp(settings common.StampsSettings) {
	if d.direction != In || d.identity == nil || !d.identity.HasPrivateKey() {
		return
	}
	if !settings.StampDestinations {
		return
	}
	storagePath := d.stampStoragePath()
	if storagePath == "" {
		return
	}
	stamp, value, ok := stamps.LoadDestinationStamp(storagePath, d.hashValue, d.stampMaterial(), settings.DestinationCost)
	if ok {
		d.stamp = stamp
		d.stampValue = value
	}
}

// awaitStamp implements Python Destination._await_stamp: reloads or generates
// a stamp meeting target_value, persisting new stamps. Callers hold no lock;
// stampGenMu serializes generation per destination.
func (d *Destination) awaitStamp(ctx context.Context, settings common.StampsSettings) bool {
	d.stampGenMu.Lock()
	defer d.stampGenMu.Unlock()

	target := min(common.StampCostCap, max(d.targetValue, settings.DestinationValue))

	d.mutex.RLock()
	current := d.stamp
	currentValue := d.stampValue
	d.mutex.RUnlock()

	if len(current) == 0 || currentValue < stamps.QuantizeCost(target) {
		storagePath := d.stampStoragePath()
		material := d.stampMaterial()
		loaded := false
		if storagePath != "" {
			if stamp, value, ok := stamps.LoadDestinationStamp(storagePath, d.hashValue, material, target); ok {
				d.setStamp(stamp, value)
				loaded = true
			}
		}
		if !loaded {
			stamp, value, err := stamps.GenerateStamp(ctx, material, target, stamps.StampLength)
			if err != nil || len(stamp) == 0 {
				debug.Log(debug.DebugWarning, "Destination stamp generation failed or was cancelled",
					"dest_hash", d.hexhash())
				return false
			}
			d.setStamp(stamp, value)
			if storagePath != "" {
				if err := stamps.StoreDestinationStamp(storagePath, d.hashValue, stamp, value); err != nil {
					debug.Log(debug.DebugWarning, "Could not persist destination stamp", "error", err)
				}
			}
		}
	}

	d.mutex.RLock()
	ok := len(d.stamp) > 0
	d.mutex.RUnlock()
	return ok
}

func (d *Destination) setStamp(stamp []byte, value float64) {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	d.stamp = append(d.stamp[:0], stamp...)
	d.stampValue = value
}

func (d *Destination) currentStamp() []byte {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return d.stamp
}

func (d *Destination) hexhash() string {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return hex.EncodeToString(d.hashValue)
}

type stampJob struct {
	dest          *Destination
	pathResponse  bool
	tag           []byte
	attachedIface common.NetworkInterface
}

var stamper = struct {
	sync.Mutex
	pending map[string]struct{}
	jobs    chan stampJob
	cancel  context.CancelFunc
	started bool
}{pending: make(map[string]struct{})}

// enqueueStampedAnnounce defers announce emission until a destination stamp
// exists, like Python Transport.enqueue_stamped_announce. Returns false when
// the bounded queue is full so callers can fall back to an unstamped emit.
func enqueueStampedAnnounce(job stampJob) bool {
	stamper.Lock()
	defer stamper.Unlock()

	key := string(job.dest.hashValue)
	if _, dup := stamper.pending[key]; dup {
		return true
	}
	if !stamper.started {
		ctx, cancel := context.WithCancel(context.Background())
		stamper.cancel = cancel
		stamper.jobs = make(chan stampJob, stamperQueueCap)
		stamper.started = true
		go stamperWorker(ctx, stamper.jobs)
	}
	select {
	case stamper.jobs <- job:
		stamper.pending[key] = struct{}{}
		return true
	default:
		return false
	}
}

// StopStamper cancels pending stamped-announce work. Called on shutdown.
func StopStamper() {
	stamper.Lock()
	defer stamper.Unlock()
	if stamper.cancel != nil {
		stamper.cancel()
	}
	stamper.pending = make(map[string]struct{})
	stamper.jobs = nil
	stamper.started = false
}

func stamperWorker(ctx context.Context, jobs chan stampJob) {
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-jobs:
			key := string(job.dest.hashValue)
			stamper.Lock()
			delete(stamper.pending, key)
			stamper.Unlock()

			settings := job.dest.stampsSettingsLocked()
			if !job.dest.awaitStamp(ctx, settings) {
				debug.Log(debug.DebugWarning, "Stamped announce skipped: stamp generation failed",
					"dest_hash", job.dest.hexhash())
				continue
			}
			if err := job.dest.Announce(job.pathResponse, job.tag, job.attachedIface); err != nil {
				debug.Log(debug.DebugWarning, "Stamped announce send failed",
					"dest_hash", job.dest.hexhash(), "error", err)
			}
		}
	}
}

// getLinkCostLocked is GetLinkCost's body for callers holding d.mutex.
func (d *Destination) getLinkCostLocked(settings common.StampsSettings) float64 {
	cost := settings.LRCost
	if d.linkCostSet {
		cost = d.linkCost
	}
	return max(0, min(cost, common.StampCostCap))
}
