// SPDX-License-Identifier: LicenseRef-Reticulum
package call

import (
	"context"
	"math"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/debug"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/audio/filter"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/audio/io"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/audio/opus"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/audio/opusfile"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/audio/tone"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/media"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/proto"
)

func (c *Call) openPipelines() error {
	c.mutex.Lock()
	if c.opened {
		c.mutex.Unlock()
		return nil
	}
	if c.state.Load() == int32(StateEnded) {
		c.mutex.Unlock()
		return ErrNotActive
	}
	c.opened = true
	profile := int(c.profile.Load())
	params := proto.ProfileParams(profile)
	c.params = params
	c.mutex.Unlock()

	enc, err := newProfileEncoder(params)
	if err != nil {
		c.mutex.Lock()
		c.opened = false
		c.mutex.Unlock()
		return err
	}
	dec, err := newProfileDecoder(params)
	if err != nil {
		_ = enc.Close()
		c.mutex.Lock()
		c.opened = false
		c.mutex.Unlock()
		return err
	}

	owned := true
	var dev io.Device
	if c.cfg.Device != nil {
		dev = c.cfg.Device
		owned = false
	} else if c.cfg.UseAudio {
		role := io.RoleCapture
		if c.cfg.DuplexIO {
			role = io.RoleDuplex
		}
		dev, err = io.Open(io.Options{
			Role:       role,
			Speaker:    c.cfg.Speaker,
			Microphone: c.cfg.Microphone,
		})
		if err != nil {
			_ = enc.Close()
			_ = dec.Close()
			c.mutex.Lock()
			c.opened = false
			c.mutex.Unlock()
			return err
		}
		if dev == nil {
			_ = enc.Close()
			_ = dec.Close()
			c.mutex.Lock()
			c.opened = false
			c.mutex.Unlock()
			return io.ErrDeviceInit
		}
	} else {
		dev = io.NewNullDeviceSize(params.FrameSamples())
	}
	if err := dev.Start(); err != nil {
		_ = enc.Close()
		_ = dec.Close()
		c.mutex.Lock()
		c.opened = false
		c.mutex.Unlock()
		return err
	}

	c.mutex.Lock()
	if c.state.Load() == int32(StateEnded) {
		c.opened = false
		c.mutex.Unlock()
		_ = enc.Close()
		_ = dec.Close()
		if owned {
			_ = dev.Close()
		}
		return ErrNotActive
	}
	c.encoder = enc
	c.decoder = dec
	c.device = dev
	c.deviceOwned = owned
	c.recvKind = params.Codec
	if c.cfg.UseAudio && owned {
		c.skipLeft = io.DefaultSampleRate * captureSkipMs / 1000
		c.easeTotal = io.DefaultSampleRate * captureEaseMs / 1000
		c.easeLeft = c.easeTotal
	}
	c.mutex.Unlock()

	if !c.incoming.Load() {
		_ = c.sendSignals(proto.StatusEstablished)
	}
	return nil
}

func (c *Call) startPipelines() error {
	c.mutex.Lock()
	if c.started {
		c.mutex.Unlock()
		return nil
	}
	if c.state.Load() == int32(StateEnded) {
		c.mutex.Unlock()
		return ErrNotActive
	}
	if c.encoder == nil {
		c.mutex.Unlock()
		return ErrPipelinesClosed
	}
	ctx, cancel := context.WithCancel(context.Background()) // #nosec G118 -- stored on Call and invoked from end
	c.cancel = cancel
	c.started = true
	c.wg.Add(3)
	c.mutex.Unlock()

	c.applyModeSquelch()

	go c.mediaSender(ctx)
	go c.mediaReceiver(ctx)
	go c.statsLoop(ctx)
	return nil
}

func (c *Call) getCodecAndDevice() (opus.Encoder, opus.Decoder, io.Device, proto.CodecParams) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.encoder, c.decoder, c.device, c.params
}

func (c *Call) mediaSender(ctx context.Context) {
	defer c.wg.Done()
	_, _, _, params := c.getCodecAndDevice()
	tick := opus.DurationOf(params.FrameMs)
	if c.cfg.UseAudio && tick > maxCaptureTick {
		tick = maxCaptureTick
	}
	ticker := time.NewTicker(tick)
	defer ticker.Stop()
	var bufs pcmScratch
	if !c.txArmed.Load() {
		select {
		case <-ctx.Done():
			return
		case <-c.txArm:
		}
	}
	c.sendMediaTick(&bufs)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.sendMediaTick(&bufs)
		}
	}
}

type pcmScratch struct {
	pending []int16
	silence []int16
	scratch []int16
	down    []int16
	wire    []byte
}

func (c *Call) sendMediaTick(bufs *pcmScratch) {
	if c.state.Load() != int32(StateActive) || !c.txArmed.Load() {
		return
	}
	l := c.getLink()
	if l == nil {
		return
	}
	encoder, _, device, params := c.getCodecAndDevice()
	if encoder == nil {
		return
	}
	if c.squelched.Load() {
		return
	}
	want := encoder.FrameSamples()
	c.collectCapture(device, params, want, bufs)
	if len(bufs.pending) < want {
		return
	}
	var encoded []byte
	var err error
	c.mutex.Lock()
	if c.encoder != nil {
		encoded, err = c.encoder.Encode(bufs.pending[:want])
		params = c.params
	}
	c.mutex.Unlock()
	n := copy(bufs.pending, bufs.pending[want:])
	bufs.pending = bufs.pending[:n]
	if err != nil {
		// LXST 0.5.4 Sources.py: encode errors are logged and the frame dropped.
		debug.Log(debug.DebugTrace, "lxst encode error", "error", err)
		return
	}
	if len(encoded) == 0 {
		return
	}
	wire, err := proto.PackFrameInto(bufs.wire[:0], params.Codec, encoded)
	if err != nil {
		return
	}
	bufs.wire = wire
	if err := l.SendPacket(wire); err != nil {
		return
	}
	c.sentCount.Add(1)
}

func (c *Call) collectCapture(device io.Device, params proto.CodecParams, want int, bufs *pcmScratch) {
	if device != nil && !c.muted.Load() {
		got, err := device.ReadPCM()
		if err != nil || len(got) == 0 {
			return
		}
		if c.cfg.UseAudio {
			got, c.skipLeft = dropLeading(got, c.skipLeft)
			if len(got) == 0 {
				return
			}
		}
		if c.cfg.Filters {
			got = copyPCM(&bufs.scratch, got)
			c.bandpass.Process(got)
			c.agc.Process(got)
			c.echo.Process(got)
		}
		if c.cfg.UseAudio && c.easeLeft > 0 {
			c.easeLeft = rampIn(got, c.easeLeft, c.easeTotal)
		}
		filter.ApplyGain(got, math.Float64frombits(c.txGainBits.Load()))
		if params.SampleRate > 0 && params.SampleRate != io.DefaultSampleRate && len(got) != want {
			got = opus.DownsampleInto(got, io.DefaultSampleRate, params.SampleRate, bufs.down)
			bufs.down = got
		}
		bufs.pending = append(bufs.pending, got...)
		return
	}
	if len(bufs.pending) >= want {
		return
	}
	n := want - len(bufs.pending)
	if cap(bufs.silence) < n {
		bufs.silence = make([]int16, n)
	}
	bufs.pending = append(bufs.pending, bufs.silence[:n]...)
}

func dropLeading(pcm []int16, left int) ([]int16, int) {
	if left <= 0 || len(pcm) == 0 {
		return pcm, left
	}
	if left >= len(pcm) {
		return pcm[:0], left - len(pcm)
	}
	return pcm[left:], 0
}

func rampIn(pcm []int16, left, total int) int {
	if total <= 0 || left <= 0 {
		return 0
	}
	done := total - left
	for i := range pcm {
		if left <= 0 {
			return 0
		}
		g := float64(done) / float64(total)
		pcm[i] = int16(float64(pcm[i]) * g)
		done++
		left--
	}
	return left
}

func copyPCM(dst *[]int16, src []int16) []int16 {
	if cap(*dst) < len(src) {
		*dst = make([]int16, len(src))
	} else {
		*dst = (*dst)[:len(src)]
	}
	copy(*dst, src)
	return *dst
}

func (c *Call) mediaReceiver(ctx context.Context) {
	defer c.wg.Done()
	ticker := time.NewTicker(receiveTick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.recvWake:
			c.playReadyFrame(time.Now())
		case now := <-ticker.C:
			c.playReadyFrame(now)
		}
	}
}

func (c *Call) playReadyFrame(now time.Time) {
	frame, ok := c.jitter.PopReady(now)
	if !ok {
		return
	}
	codec, payload, err := proto.SplitFrame(frame.Payload)
	if err != nil {
		return
	}
	if err := c.ensureRecvDecoder(codec, payload); err != nil {
		return
	}
	_, _, device, _ := c.getCodecAndDevice()
	var pcm []int16
	c.mutex.Lock()
	if c.decoder == nil {
		c.mutex.Unlock()
		return
	}
	pcm, err = c.decoder.Decode(payload)
	if err != nil {
		pcm, err = c.decoder.DecodePLC()
	}
	c.mutex.Unlock()
	if err != nil {
		return
	}
	filter.ApplyGain(pcm, math.Float64frombits(c.rxGainBits.Load()))
	if c.echo != nil {
		c.echo.SetReference(pcm)
	}
	if c.events.OnFrame != nil {
		c.events.OnFrame(pcm)
	}
	if device != nil && !c.mutedRX.Load() {
		_ = device.WritePCM(pcm)
	}
}

func (c *Call) statsLoop(ctx context.Context) {
	defer c.wg.Done()
	ticker := time.NewTicker(statsInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.applyLinkStats()
		}
	}
}

func (c *Call) applyLinkStats() {
	l := c.getLink()
	if l == nil {
		return
	}
	metrics := media.LinkMetrics{
		RTT:      l.GetRTT(),
		RSSI:     l.GetRSSI(),
		SNR:      l.GetSNR(),
		Q:        l.GetQ(),
		LossRate: c.jitter.LossRate(),
		JitterMs: float64(c.jitter.TargetMs()),
	}
	c.adaptive.Update(metrics)
	encoder, _, _, params := c.getCodecAndDevice()
	j := c.adaptive.JitterMs()
	minJitter := params.FrameMs * params.BufferN
	if minJitter > 0 && j < minJitter {
		j = minJitter
	}
	c.jitter.SetTargetMs(j)
	if encoder != nil && params.Codec == proto.CodecOpus {
		c.applyOpusAdaptation(encoder, params, metrics)
	}
	metrics.JitterMs = float64(c.jitter.TargetMs())
	metrics.Bitrate = c.adaptive.Bitrate()
	metrics.UseFEC = c.adaptive.UseFEC()
	if c.events.OnStats != nil {
		c.events.OnStats(c, metrics)
	}
}

func (c *Call) applyOpusAdaptation(encoder opus.Encoder, params proto.CodecParams, metrics media.LinkMetrics) {
	br := c.adaptive.Bitrate()
	if params.Bitrate > 0 && br > params.Bitrate {
		br = params.Bitrate
	}
	if br < proto.MinBitrate {
		br = proto.MinBitrate
	}
	_ = encoder.SetBitrate(br)
	_ = encoder.SetFEC(c.adaptive.UseFEC())
	_ = encoder.SetPacketLossPerc(clampPercent(int(metrics.LossRate * 100)))
}

func clampPercent(v int) int {
	if v < 0 {
		return 0
	}
	if v > maxLossPercent {
		return maxLossPercent
	}
	return v
}

func (c *Call) stopRingtone() {
	c.mutex.Lock()
	c.ringOnce.Do(func() {
		if c.ringStop != nil {
			close(c.ringStop)
		}
	})
	c.mutex.Unlock()
	c.ringWG.Wait()
}

func (c *Call) ringtoneStoppedLocked() bool {
	select {
	case <-c.ringStop:
		return true
	default:
		return false
	}
}

func (c *Call) ringtoneClip() *opusfile.Clip {
	c.ringLoad.Do(func() {
		if c.cfg.RingtonePath == "" {
			return
		}
		clip, err := opusfile.Load(c.cfg.RingtonePath)
		if err != nil {
			return
		}
		c.ringClip = clip
	})
	return c.ringClip
}

func (c *Call) startRingtone() {
	if !c.cfg.UseAudio {
		return
	}
	clip := c.ringtoneClip()
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.ringtoneStoppedLocked() {
		return
	}
	if clip != nil {
		c.playLoop(clip, c.cfg.Ringer)
		return
	}
	c.playCadence(c.cfg.Ringer, func(buf, other []int16, pos uint64) {
		if tone.RingOn(pos, io.DefaultSampleRate) {
			c.ringA.Fill(buf)
			c.ringB.Fill(other)
			tone.Mix(buf, other)
		}
	})
}

func (c *Call) startDialTone() {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.ringtoneStoppedLocked() {
		return
	}
	c.playCadence(c.cfg.Speaker, func(buf, _ []int16, pos uint64) {
		if tone.DialOn(pos, io.DefaultSampleRate) {
			c.dial.Fill(buf)
		}
	})
}

func (c *Call) playLoop(clip *opusfile.Clip, deviceName string) {
	if !c.cfg.UseAudio {
		return
	}
	c.ringWG.Go(func() {
		dev := c.openPlayDevice(deviceName)
		if dev == nil {
			return
		}
		defer func() { _ = dev.Close() }()
		buf := make([]int16, ringFrameSamples)
		ticker := time.NewTicker(maxCaptureTick)
		defer ticker.Stop()
		pos := 0
		gain := c.cfg.RingtoneGain
		for {
			if c.state.Load() != int32(StateRinging) || c.answered.Load() {
				return
			}
			select {
			case <-c.ringStop:
				return
			case <-ticker.C:
			}
			clip.Fill(buf, &pos)
			filter.ApplyGain(buf, gain)
			_ = dev.WritePCM(buf)
		}
	})
}

func (c *Call) playCadence(deviceName string, fill func(buf, other []int16, pos uint64)) {
	if !c.cfg.UseAudio {
		return
	}
	c.ringWG.Go(func() {
		dev := c.openPlayDevice(deviceName)
		if dev == nil {
			return
		}
		defer func() { _ = dev.Close() }()
		buf := make([]int16, ringFrameSamples)
		other := make([]int16, ringFrameSamples)
		ticker := time.NewTicker(maxCaptureTick)
		defer ticker.Stop()
		for {
			if c.state.Load() != int32(StateRinging) || c.answered.Load() {
				return
			}
			select {
			case <-c.ringStop:
				return
			case <-ticker.C:
			}
			clear(buf)
			fill(buf, other, c.ringPos.Add(uint64(len(buf))))
			_ = dev.WritePCM(buf)
		}
	})
}

func (c *Call) startBusyTone() {
	if !c.cfg.UseAudio {
		return
	}
	src := tone.New(tone.DialHz, io.DefaultSampleRate, busyGain)
	go func() {
		dev := c.openPlayDevice(c.cfg.Speaker)
		if dev == nil {
			return
		}
		defer func() { _ = dev.Close() }()
		buf := make([]int16, ringFrameSamples)
		ticker := time.NewTicker(maxCaptureTick)
		defer ticker.Stop()
		deadline := time.Now().Add(busyToneTime)
		var pos uint64
		for time.Now().Before(deadline) {
			select {
			case <-c.ended:
				return
			case <-ticker.C:
			}
			clear(buf)
			pos += uint64(len(buf))
			if tone.BusyOn(pos, io.DefaultSampleRate) {
				src.Fill(buf)
			}
			_ = dev.WritePCM(buf)
		}
	}()
}

func (c *Call) openPlayDevice(name string) io.Device {
	if c.cfg.Device != nil {
		if err := c.cfg.Device.Start(); err != nil {
			debug.Log(debug.DebugInfo, "lxst host play device start failed", "error", err)
			return nil
		}
		return keepOpen{Device: c.cfg.Device}
	}
	dev, err := io.Open(io.Options{Role: io.RolePlayback, Speaker: name})
	if err != nil || dev == nil {
		debug.Log(debug.DebugInfo, "lxst play device failed", "error", err)
		return nil
	}
	if err := dev.Start(); err != nil {
		debug.Log(debug.DebugInfo, "lxst play device start failed", "error", err)
		_ = dev.Close()
		return nil
	}
	return dev
}

type keepOpen struct {
	io.Device
}

func (keepOpen) Close() error { return nil }
