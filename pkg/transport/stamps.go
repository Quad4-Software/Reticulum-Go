// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
)

// StampsSettings resolves the effective RNS 1.6.0 stamp configuration
// (Transport.py STAMPS_* / cost constants) for this transport.
func (t *Transport) StampsSettings() common.StampsSettings {
	if t == nil {
		return common.DefaultStampsSettings()
	}
	return t.GetConfig().StampsSettings()
}

func (t *Transport) StampsEnabled() bool       { return t.StampsSettings().Enabled }
func (t *Transport) StampsEnforced() bool      { return t.StampsSettings().Enforced }
func (t *Transport) LRStampsEnforced() bool    { return t.StampsSettings().LREnforced }
func (t *Transport) StampDestinations() bool   { return t.StampsSettings().StampDestinations }
func (t *Transport) StampPRs() bool            { return t.StampsSettings().StampPRs }
func (t *Transport) StampLRs() bool            { return t.StampsSettings().StampLRs }
func (t *Transport) AlwaysStampLRs() bool      { return t.StampsSettings().AlwaysStampLRs }
func (t *Transport) DestinationCost() float64  { return t.StampsSettings().DestinationCost }
func (t *Transport) DestinationValue() float64 { return t.StampsSettings().DestinationValue }
func (t *Transport) PRCost() float64           { return t.StampsSettings().PRCost }
func (t *Transport) PRValue() float64          { return t.StampsSettings().PRValue }
func (t *Transport) LRCost() float64           { return t.StampsSettings().LRCost }
func (t *Transport) LinkStampTimeout() float64 { return t.StampsSettings().LinkStampTimeout }
func (t *Transport) PRStampTimeout() float64   { return t.StampsSettings().PRStampTimeout }

// interfaceDestinationCost resolves the receiving interface's required
// announce stamp cost, falling back to the transport default when the
// interface does not carry a per-interface destination_cost.
func (t *Transport) interfaceDestinationCost(iface common.NetworkInterface, settings common.StampsSettings) float64 {
	if iface != nil {
		if sc, ok := iface.(interface {
			GetStampCosts() (float64, float64, bool, bool)
		}); ok {
			dest, _, destSet, _ := sc.GetStampCosts()
			if destSet {
				return dest
			}
		}
	}
	return settings.DestinationCost
}
