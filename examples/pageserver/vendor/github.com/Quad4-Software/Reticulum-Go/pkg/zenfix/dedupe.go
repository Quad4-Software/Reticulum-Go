// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package zenfix

import "fmt"

func dedupeFindings(in []Finding) []Finding {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]Finding, 0, len(in))
	for _, f := range in {
		key := fmt.Sprintf("%s:%d:%d:%s", f.File, f.Line, f.Col, f.Rule)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, f)
	}
	return out
}
