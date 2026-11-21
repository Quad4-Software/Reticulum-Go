// SPDX-License-Identifier: LicenseRef-Reticulum

package session

import (
	"fmt"
	"strconv"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/audio/io"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/call"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/proto"
)

type Info struct {
	State        call.State
	StateName    string
	Incoming     bool
	MutedTX      bool
	MutedRX      bool
	Squelched    bool
	Transmitting bool
	Profile      int
	ProfileName  string
	Mode         int
	ModeName     string
	LocalHash    string
	DestHash     string
	RemoteHash   string
	Sent         uint64
	Recv         uint64
	Reason       string
	Err          string
	Audio        string
	AudioStats   io.HostStats
	AllowPolicy  string
	Announced    bool
	Aspect       string
}

func (i Info) String() string {
	return fmt.Sprintf(
		"state=%s incoming=%t tx=%t profile=%s mode=%s dest=%s remote=%s sent=%d recv=%d audio=%s allow=%s announced=%t reason=%s err=%s",
		i.StateName,
		i.Incoming,
		i.Transmitting,
		i.ProfileName,
		i.ModeName,
		i.DestHash,
		i.RemoteHash,
		i.Sent,
		i.Recv,
		i.Audio,
		i.AllowPolicy,
		i.Announced,
		i.Reason,
		i.Err,
	)
}

func (s *Session) Info() Info {
	s.mutex.Lock()
	reason := s.reason
	errText := ""
	if s.lastErr != nil {
		errText = s.lastErr.Error()
	}
	profile := s.cfg.Profile
	mode := s.cfg.Mode
	allow := policyName(s.cfg.AllowPolicy, s.cfg.Allowed, s.cfg.AllowFunc)
	announced := s.announced
	s.mutex.Unlock()
	if profile == 0 {
		profile = proto.DefaultProfile
	}
	if mode == 0 {
		mode = proto.DefaultMode
	}
	info := Info{
		State:       call.StateIdle,
		StateName:   call.StateIdle.String(),
		Profile:     profile,
		ProfileName: proto.ProfileName(profile),
		Mode:        mode,
		ModeName:    proto.ModeName(mode),
		LocalHash:   call.FormatHash(s.cfg.Identity.Hash()),
		DestHash:    call.FormatHash(s.phone.DestHash()),
		Reason:      reason,
		Err:         errText,
		Audio:       s.audioKind(),
		AllowPolicy: allow,
		Announced:   announced,
		Aspect:      proto.AppName + "." + proto.AspectName,
	}
	if s.host != nil {
		info.AudioStats = s.host.Stats()
	}
	c := s.Active()
	if c == nil {
		return info
	}
	info.State = c.State()
	info.StateName = info.State.String()
	info.Incoming = c.Incoming()
	info.MutedTX = c.MutedTX()
	info.MutedRX = c.MutedRX()
	info.Squelched = c.Squelched()
	info.Transmitting = c.Transmitting()
	info.Profile = c.Profile()
	info.ProfileName = proto.ProfileName(info.Profile)
	info.Mode = c.Mode()
	info.ModeName = proto.ModeName(info.Mode)
	info.RemoteHash = call.Fingerprint(c.RemoteIdentity())
	info.Sent = c.SentFrames()
	info.Recv = c.RecvFrames()
	return info
}

func (s *Session) audioKind() string {
	if s.host != nil {
		return "host"
	}
	if s.cfg.Device != nil {
		return "device"
	}
	return "none"
}

func (s *Session) LastError() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.lastErr
}

func (s *Session) LastReason() string {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.reason
}

func boolField(v bool) string {
	return strconv.FormatBool(v)
}
