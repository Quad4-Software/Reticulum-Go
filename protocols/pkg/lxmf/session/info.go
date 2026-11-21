// SPDX-License-Identifier: LicenseRef-Reticulum

package session

import (
	"fmt"
	"strconv"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxmf"
)

type Info struct {
	LocalHash      string
	DestHash       string
	LastFrom       string
	LastTitle      string
	Sent           uint64
	Recv           uint64
	Dropped        uint64
	Err            string
	AllowPolicy    string
	Announced      bool
	Aspect         string
	DisplayName    string
	StampCost      int
	RequireStamp   bool
	DropUnverified bool
	Propagation    string
}

func (i Info) String() string {
	return fmt.Sprintf(
		"dest=%s from=%s sent=%d recv=%d dropped=%d allow=%s announced=%t stamp=%d prop=%s err=%s",
		i.DestHash,
		i.LastFrom,
		i.Sent,
		i.Recv,
		i.Dropped,
		i.AllowPolicy,
		i.Announced,
		i.StampCost,
		i.Propagation,
		i.Err,
	)
}

func (s *Session) Info() Info {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	errText := ""
	if s.lastErr != nil {
		errText = s.lastErr.Error()
	}
	prop := "none"
	if len(s.propHash) == lxmf.DestinationLength {
		prop = FormatHash(s.propHash)
	}
	return Info{
		LocalHash:      FormatHash(s.cfg.Identity.Hash()),
		DestHash:       FormatHash(s.messenger.DestinationHash()),
		LastFrom:       FormatHash(s.lastFrom),
		LastTitle:      s.lastTitle,
		Sent:           s.sent,
		Recv:           s.recv,
		Dropped:        s.dropped,
		Err:            errText,
		AllowPolicy:    s.policyNameLocked(),
		Announced:      s.announced,
		Aspect:         lxmf.AppName + "." + lxmf.AspectDelivery,
		DisplayName:    s.cfg.DisplayName,
		StampCost:      s.cfg.StampCost,
		RequireStamp:   s.cfg.RequireStamp,
		DropUnverified: s.cfg.DropUnverified,
		Propagation:    prop,
	}
}

func (s *Session) LastError() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.lastErr
}

func boolField(v bool) string {
	return strconv.FormatBool(v)
}
