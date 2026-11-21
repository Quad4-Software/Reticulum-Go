// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/link"
)

// Job intervals are counted in processing ticks matching upstream
// LXMRouter JOB_* constants, where one tick is PROCESSING_INTERVAL.
const (
	jobProcessingInterval = 4 * time.Second
	jobLinksTicks         = 1
	jobTransientTicks     = 60
	jobStoreTicks         = 120
	jobPeerSyncTicks      = 6
	jobPeerRotateTicks    = 56 * jobPeerSyncTicks
)

// Inbound link inactivity limits match upstream LXMRouter
// LINK_MAX_INACTIVITY and P_LINK_MAX_INACTIVITY.
const (
	linkMaxInactivitySeconds  = 10 * 60
	pLinkMaxInactivitySeconds = 3 * 60
)

// cleanLinks tears down inbound links that have been inactive beyond
// the upstream limits, matching LXMRouter.clean_links.
func (r *Router) cleanLinks() {
	var stale []*link.Link
	r.linksMu.Lock()
	for key, lnk := range r.directLinks {
		if lnk.NoDataFor() > linkMaxInactivitySeconds {
			stale = append(stale, lnk)
			delete(r.directLinks, key)
		}
	}
	for key, lnk := range r.propLinks {
		if lnk.NoDataFor() > pLinkMaxInactivitySeconds {
			stale = append(stale, lnk)
			delete(r.propLinks, key)
		}
	}
	r.linksMu.Unlock()
	for _, lnk := range stale {
		lnk.Teardown()
	}
}

// cleanThrottledPeers drops expired throttle entries, matching upstream
// LXMRouter.clean_throttled_peers.
func (r *Router) cleanThrottledPeers() {
	now := float64(time.Now().Unix())
	r.propMu.Lock()
	for key, until := range r.throttledPeers {
		if now >= until {
			delete(r.throttledPeers, key)
		}
	}
	r.propMu.Unlock()
}

func (r *Router) jobLoop() {
	defer r.wg.Done()
	ticker := time.NewTicker(jobProcessingInterval)
	defer ticker.Stop()

	var tickCount int64
	for {
		select {
		case <-r.stop:
			return
		case <-ticker.C:
			tickCount++
			r.runJobs(tickCount)
		}
	}
}

func (r *Router) runJobs(tick int64) {
	defer func() {
		if rec := recover(); rec != nil {
			Error("router job panic", "error", rec)
		}
	}()

	if tick%jobLinksTicks == 0 {
		r.cleanLinks()
	}
	if tick%jobTransientTicks == 0 {
		r.cleanTransientCaches()
	}

	if r.propagationEnabled {
		if tick%jobStoreTicks == 0 {
			if r.store != nil {
				r.store.CleanExpired()
				r.store.CleanToLimit()
			}
		}
		if tick%jobPeerSyncTicks == 0 {
			r.flushPeerDistribution()
			r.syncPeers()
			r.cleanThrottledPeers()
			r.savePeers()
			r.saveNodeStats()
		}
		if tick%jobPeerRotateTicks == 0 {
			r.rotatePeers()
		}
	}

	if tick%15 == 0 {
		r.saveLocallyDelivered()
		r.saveLocallyProcessed()
	}

	if r.cfg.LXMF.AnnounceIntervalMinutes > 0 && tick%int64(time.Duration(r.cfg.LXMF.AnnounceIntervalMinutes)*time.Minute/jobProcessingInterval) == 0 {
		r.announceDelivery()
	}
	if r.propagationEnabled && r.cfg.Propagation.AnnounceIntervalMinutes > 0 &&
		tick%int64(time.Duration(r.cfg.Propagation.AnnounceIntervalMinutes)*time.Minute/jobProcessingInterval) == 0 {
		go r.announcePropagationNode()
	}
}
