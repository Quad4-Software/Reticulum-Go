// SPDX-License-Identifier: LicenseRef-Reticulum

// Package testconfig builds node configurations for tests in this module.
package testconfig

import "git.quad4.io/reticulum/Reticulum-Go/pkg/common"

// New returns the default configuration with network stamp generation off and
// cheap stamp costs, so tests do not wait for stamp mining on every announce.
func New() *common.ReticulumConfig {
	c := common.DefaultConfig()
	c.Stamps.StampDestinations = false
	c.Stamps.StampDestinationsSet = true
	c.Stamps.StampPRs = false
	c.Stamps.StampPRsSet = true
	c.Stamps.DestinationCost = 8
	c.Stamps.DestinationCostSet = true
	c.Stamps.DestinationValue = 8
	c.Stamps.DestinationValueSet = true
	c.Stamps.PRCost = 8
	c.Stamps.PRCostSet = true
	c.Stamps.PRValue = 8
	c.Stamps.PRValueSet = true
	c.Stamps.LRCost = 8
	c.Stamps.LRCostSet = true
	return c
}
