// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import (
	"context"
	"fmt"
	"time"
)

const mirrorPollInterval = 15 * time.Minute

func (n *Node) mirrorScheduler(ctx context.Context, syncInterval time.Duration) {
	if syncInterval <= 0 {
		syncInterval = time.Duration(n.cfg.MirrorIntervalHrs) * time.Hour
	}
	if syncInterval <= 0 {
		syncInterval = 24 * time.Hour
	}
	ticker := time.NewTicker(mirrorPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n.syncMirrors(syncInterval)
		}
	}
}

func (n *Node) syncMirrors(syncInterval time.Duration) {
	if syncInterval <= 0 {
		syncInterval = time.Duration(n.cfg.MirrorIntervalHrs) * time.Hour
	}
	n.mu.RLock()
	groups := n.access.Groups
	n.mu.RUnlock()
	for _, ga := range groups {
		for name, ra := range ga.Repositories {
			repoType, err := n.git.ConfigValue(ra.Path, "repository.rngit.type")
			if err != nil || repoType != "mirror" {
				continue
			}
			synced, _ := n.git.ConfigValue(ra.Path, "repository.rngit.upstream.sync")
			if synced != "" {
				var ts int64
				if _, err := fmt.Sscanf(synced, "%d", &ts); err == nil {
					if time.Since(time.Unix(ts, 0)) < syncInterval {
						continue
					}
				}
			}
			source, err := n.git.ConfigValue(ra.Path, "repository.rngit.upstream.source")
			if err != nil || source == "" {
				continue
			}
			if err := n.git.FetchAll(ra.Path, source); err == nil {
				_ = n.git.SetConfig(ra.Path, "repository.rngit.upstream.sync", fmt.Sprintf("%d", time.Now().Unix()))
			}
			_ = name
		}
	}
}
