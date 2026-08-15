// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsutil

import (
	"fmt"
	"sort"
	"strings"

	"quad4/reticulum-go/pkg/common"
)

// SelectInterfaces enables only the named config interfaces.
// selector empty, "all", or "*" leaves Enabled flags unchanged.
// Otherwise selector is a comma-separated list of interface section names
// (case-insensitive). Matching sections are enabled and all others disabled.
// Returns the names that remain enabled.
func SelectInterfaces(cfg *common.ReticulumConfig, selector string) ([]string, error) {
	if cfg == nil {
		return nil, fmt.Errorf("nil config")
	}
	sel := strings.TrimSpace(selector)
	if sel == "" || strings.EqualFold(sel, "all") || sel == "*" {
		out := make([]string, 0, len(cfg.Interfaces))
		for name, ic := range cfg.Interfaces {
			if ic != nil && ic.Enabled {
				out = append(out, name)
			}
		}
		sort.Strings(out)
		return out, nil
	}

	want := make(map[string]struct{})
	for part := range strings.SplitSeq(sel, ",") {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}
		want[strings.ToLower(name)] = struct{}{}
	}
	if len(want) == 0 {
		return nil, fmt.Errorf("empty -iface selector")
	}

	matched := make(map[string]struct{})
	for name, ic := range cfg.Interfaces {
		if ic == nil {
			continue
		}
		key := strings.ToLower(name)
		if _, ok := want[key]; ok {
			ic.Enabled = true
			matched[key] = struct{}{}
		} else {
			ic.Enabled = false
		}
	}

	missing := make([]string, 0)
	for key := range want {
		if _, ok := matched[key]; !ok {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("unknown interface(s): %s", strings.Join(missing, ", "))
	}

	out := make([]string, 0, len(matched))
	for name, ic := range cfg.Interfaces {
		if ic != nil && ic.Enabled {
			out = append(out, name)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no interfaces enabled after -iface filter")
	}
	sort.Strings(out)
	return out, nil
}
