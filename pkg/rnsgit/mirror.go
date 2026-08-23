// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import (
	"context"
	"fmt"
	"time"
)

func (n *Node) mirrorScheduler(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n.syncMirrors()
		}
	}
}

func (n *Node) syncMirrors() {
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
					if time.Since(time.Unix(ts, 0)) < time.Duration(n.cfg.MirrorIntervalHrs)*time.Hour {
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
