// SPDX-License-Identifier: LicenseRef-Reticulum
package call

import (
	"time"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/phonebook"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/proto"
)

const (
	DefaultRingTime    = 60 * time.Second
	DefaultWaitTime    = 70 * time.Second
	DefaultConnectTime = 5 * time.Second

	DefaultRateInterval = time.Minute
	DefaultRateBurst    = 3

	defaultJitterTargetMs  = 80
	defaultJitterMaxFrames = 32
	speechLowHz            = 250
	speechHighHz           = 8500
	defaultAGCdB           = -15
	ringGain               = 0.12
	availableResendCount   = 25
	availableResendGap     = 80 * time.Millisecond
	identifyDelay          = 250 * time.Millisecond
	identifyRetryCount     = 8
	identifyRetryGap       = 80 * time.Millisecond
	rejectFlushDelay       = 30 * time.Millisecond
	statePollInterval      = 20 * time.Millisecond
	maxCaptureTick         = 20 * time.Millisecond
	receiveTick            = 10 * time.Millisecond
	statsInterval          = 2 * time.Second
	ringFrameSamples       = 960
	maxLossPercent         = 100
	captureSkipMs          = 75
	captureEaseMs          = 225
	dialGain               = 0.04
	busyGain               = 0.04
	busyToneTime           = 4250 * time.Millisecond
)

func (cfg Config) withDefaults() Config {
	if cfg.AppName == "" {
		cfg.AppName = proto.AppName
	}
	if cfg.AspectName == "" {
		cfg.AspectName = proto.AspectName
	}
	if cfg.Profile == 0 {
		cfg.Profile = proto.DefaultProfile
	}
	if cfg.Mode == 0 {
		cfg.Mode = proto.DefaultMode
	}
	if cfg.RingTime == 0 {
		cfg.RingTime = DefaultRingTime
	}
	if cfg.WaitTime == 0 {
		cfg.WaitTime = DefaultWaitTime
	}
	if cfg.ConnectTime == 0 {
		cfg.ConnectTime = DefaultConnectTime
	}
	if cfg.AnnounceInterval == 0 {
		cfg.AnnounceInterval = DefaultAnnounceInterval
	}
	if cfg.AllowPolicy == 0 && len(cfg.Allowed) == 0 && cfg.AllowFunc == nil {
		cfg.AllowPolicy = phonebook.AllowAll
	}
	if cfg.Device != nil {
		cfg.UseAudio = true
	}
	return cfg
}
