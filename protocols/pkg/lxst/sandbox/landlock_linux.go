//go:build linux

// SPDX-License-Identifier: LicenseRef-Reticulum
package sandbox

import (
	"strings"

	"github.com/landlock-lsm/go-landlock/landlock"
)

func restrictLandlock(pol policy) error {
	var rules []landlock.Rule
	if len(pol.ro) > 0 {
		rules = append(rules, landlock.RODirs(pol.ro...).IgnoreIfMissing())
	}
	var dev []string
	var rw []string
	for _, p := range pol.rw {
		if p == "/dev" || strings.HasPrefix(p, "/dev/") {
			dev = append(dev, p)
		} else {
			rw = append(rw, p)
		}
	}
	if len(dev) > 0 {
		rules = append(rules, landlock.RWDirs(dev...).IgnoreIfMissing().WithIoctlDev())
	}
	if len(rw) > 0 {
		rules = append(rules, landlock.RWDirs(rw...).IgnoreIfMissing())
	}
	if len(rules) == 0 {
		return nil
	}
	return landlock.V5.BestEffort().RestrictPaths(rules...)
}

func landlockAvailable() bool {
	return true
}
