// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import (
	"fmt"
	"strings"

	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
)

func (n *Node) startPageNode() error {
	if n.dest == nil {
		return fmt.Errorf("destination not ready")
	}
	_ = n.dest.RegisterRequestHandlerAny("/page/index.mu", func(_ string, _ []byte, _ []byte, _ []byte, _ *identity.Identity, _ int64) any {
		return n.renderRepoIndex()
	}, destination.AllowAll, nil)
	return nil
}

func (n *Node) renderRepoIndex() []byte {
	var b strings.Builder
	b.WriteString(">Reticulum Git Node\n\n")
	b.WriteString(n.cfg.NodeName)
	b.WriteString("\n\nRepositories destination:\n`F")
	b.WriteString(n.ReposDestHash())
	b.WriteString("`\n\n")
	n.mu.RLock()
	defer n.mu.RUnlock()
	for group, ga := range n.access.Groups {
		b.WriteString(group)
		b.WriteString(" (")
		b.WriteString(fmt.Sprintf("%d", len(ga.Repositories)))
		b.WriteString(" repos)\n")
		for name := range ga.Repositories {
			b.WriteString("  - ")
			b.WriteString(name)
			b.WriteString("\n")
		}
	}
	return []byte(b.String())
}
