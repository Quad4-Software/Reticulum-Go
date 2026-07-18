// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package sdr

import (
	"context"
	"math"
	"math/rand"
	"sync"
	"time"
)

const (
	// Speed of light in m/s.
	speedOfLight = 299792458.0
	// Boltzmann constant J/K (for thermal noise density).
	boltzmann = 1.380649e-23
)

// ChannelModel is a math-backed IQ channel between transmitters and receivers.
type ChannelModel struct {
	mu sync.Mutex

	FrequencyHz float64
	DistanceM   float64
	TXPowerW    float64
	NoiseFigDB  float64
	SampleRate  float64
	TempK       float64
	Seed        int64

	rng *rand.Rand

	// Extra SNR offset in dB after FSPL (antenna gains, etc).
	GainDB float64
}

// NewChannelModel builds a free-space AWGN channel model.
func NewChannelModel(freqHz, distanceM, sampleRate float64, seed int64) *ChannelModel {
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	if sampleRate <= 0 {
		sampleRate = 2e6
	}
	if freqHz <= 0 {
		freqHz = 433e6
	}
	if distanceM <= 0 {
		distanceM = 100
	}
	return &ChannelModel{
		FrequencyHz: freqHz,
		DistanceM:   distanceM,
		TXPowerW:    0.1,
		NoiseFigDB:  6,
		SampleRate:  sampleRate,
		TempK:       290,
		Seed:        seed,
		GainDB:      0,
		rng:         rand.New(rand.NewSource(seed)), // #nosec G404 -- AWGN channel sim seed, not crypto
	}
}

// FreeSpacePathLossDB returns FSPL in dB.
// FSPL = 20*log10(d) + 20*log10(f) + 20*log10(4π/c)
func FreeSpacePathLossDB(distanceM, freqHz float64) float64 {
	if distanceM <= 0 || freqHz <= 0 {
		return 0
	}
	return 20*math.Log10(distanceM) + 20*math.Log10(freqHz) + 20*math.Log10(4*math.Pi/speedOfLight)
}

// ThermalNoisePowerW returns k*T*B * NF_linear.
func ThermalNoisePowerW(tempK, bandwidthHz, noiseFigDB float64) float64 {
	if tempK <= 0 {
		tempK = 290
	}
	if bandwidthHz <= 0 {
		bandwidthHz = 1
	}
	nfLin := math.Pow(10, noiseFigDB/10)
	return boltzmann * tempK * bandwidthHz * nfLin
}

// LinkSNRdB computes receive SNR from TX power, FSPL, gains, and thermal noise.
func (c *ChannelModel) LinkSNRdB() float64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	fspl := FreeSpacePathLossDB(c.DistanceM, c.FrequencyHz)
	rxPowerW := c.TXPowerW * math.Pow(10, (c.GainDB-fspl)/10)
	noiseW := ThermalNoisePowerW(c.TempK, c.SampleRate, c.NoiseFigDB)
	if noiseW <= 0 || rxPowerW <= 0 {
		return -100
	}
	return 10 * math.Log10(rxPowerW/noiseW)
}

// Apply transforms clean TX IQ into RX IQ with AWGN at the modeled SNR.
func (c *ChannelModel) Apply(tx []Complex64) []Complex64 {
	if len(tx) == 0 {
		return nil
	}
	snrDB := c.LinkSNRdB()
	return AddAWGN(tx, snrDB, c.rng)
}

// AddAWGN adds complex AWGN so the resulting SNR approximates snrDB.
// Signal power is measured from tx. Noise variance per I/Q is N0/2.
func AddAWGN(tx []Complex64, snrDB float64, rng *rand.Rand) []Complex64 {
	if rng == nil {
		rng = rand.New(rand.NewSource(1)) // #nosec G404 -- AWGN fallback seed, not crypto
	}
	var sigPow float64
	for _, s := range tx {
		sigPow += float64(s.I)*float64(s.I) + float64(s.Q)*float64(s.Q)
	}
	sigPow /= float64(len(tx))
	if sigPow <= 0 {
		sigPow = 1
	}
	snrLin := math.Pow(10, snrDB/10)
	noisePow := sigPow / snrLin
	sigma := math.Sqrt(noisePow / 2) // per I and Q

	out := make([]Complex64, len(tx))
	for i, s := range tx {
		ni, nq := boxMuller(rng)
		out[i] = Complex64{
			I: s.I + float32(sigma*ni),
			Q: s.Q + float32(sigma*nq),
		}
	}
	return out
}

func boxMuller(rng *rand.Rand) (float64, float64) {
	u1 := rng.Float64()
	u2 := rng.Float64()
	if u1 < 1e-12 {
		u1 = 1e-12
	}
	mag := math.Sqrt(-2 * math.Log(u1))
	z0 := mag * math.Cos(2*math.Pi*u2)
	z1 := mag * math.Sin(2*math.Pi*u2)
	return z0, z1
}

// MeasuredSNRdB estimates SNR from clean reference and noisy observation.
func MeasuredSNRdB(clean, noisy []Complex64) float64 {
	n := len(clean)
	if n == 0 || len(noisy) != n {
		return -100
	}
	var sig, noise float64
	for i := range n {
		si := float64(clean[i].I)
		sq := float64(clean[i].Q)
		sig += si*si + sq*sq
		di := float64(noisy[i].I) - si
		dq := float64(noisy[i].Q) - sq
		noise += di*di + dq*dq
	}
	if noise <= 0 {
		return 100
	}
	return 10 * math.Log10(sig/noise)
}

// SimDevice is a Device that pipes TX IQ through a ChannelModel into a peer RX ring.
type SimDevice struct {
	*Mock
	ch   *ChannelModel
	peer *SimDevice
}

// NewSimDevice builds a channel-backed mock device.
func NewSimDevice(cfg Config, ch *ChannelModel) *SimDevice {
	return &SimDevice{Mock: NewMock(cfg), ch: ch}
}

// LinkSimDevices cross-wires two sim devices. TX samples pass through the channel model.
func LinkSimDevices(a, b *SimDevice) {
	a.peer = b
	b.peer = a
}

// StartTX applies the channel model then injects into the peer RX ring.
func (d *SimDevice) StartTX(ctx context.Context, in <-chan []Complex64) error {
	d.mu.Lock()
	if !d.open {
		d.mu.Unlock()
		return errNotOpen
	}
	peer := d.peer
	ch := d.ch
	d.mu.Unlock()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case block, ok := <-in:
				if !ok {
					return
				}
				out := block
				if ch != nil {
					out = ch.Apply(block)
				}
				if peer == nil {
					d.underruns.Add(1)
					continue
				}
				peer.InjectRX(out)
			}
		}
	}()
	return nil
}

// StartRX mirrors Mock.StartRX.
func (d *SimDevice) StartRX(ctx context.Context, out chan<- []Complex64) error {
	return d.Mock.StartRX(ctx, out)
}

// Caps reports RX and TX.
func (d *SimDevice) Caps() Caps {
	c := d.Mock.Caps()
	c.DeviceType = "sim"
	return c
}

// TransmitBurst encodes payload, applies the channel, and injects into the peer.
func (d *SimDevice) TransmitBurst(modem *BurstModem, payload []byte) error {
	iq, err := modem.Encode(payload)
	if err != nil {
		return err
	}
	out := iq
	if d.ch != nil {
		out = d.ch.Apply(iq)
	}
	if d.peer != nil {
		d.peer.InjectRX(out)
	}
	return nil
}

func init() {
	RegisterOpener("sim", func(cfg Config) (Device, error) {
		ch := NewChannelModel(float64(cfg.Frequency), 100, float64(cfg.SampleRate), 1)
		return NewSimDevice(cfg, ch), nil
	})
}
