// SPDX-License-Identifier: LicenseRef-Reticulum
package opusfile

import (
	"fmt"
	"os"
	"strings"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/audio/opus"
)

const decodeFrame = 5760

type Clip struct {
	PCM []int16
}

func Load(path string) (*Clip, error) {
	if path == "" || strings.ContainsRune(path, 0) {
		return nil, fmt.Errorf("invalid ringtone path")
	}
	b, err := os.ReadFile(path) // #nosec G304 -- operator ringtone path after NUL check
	if err != nil {
		return nil, err
	}
	if len(b) > maxFileBytes {
		return nil, fmt.Errorf("ringtone larger than 4MB")
	}
	return Decode(b)
}

func Decode(data []byte) (*Clip, error) {
	pkts, err := packets(data)
	if err != nil {
		return nil, err
	}
	if len(pkts) < 3 {
		return nil, fmt.Errorf("opus file missing packets")
	}
	head, err := parseHead(pkts[0])
	if err != nil {
		return nil, err
	}
	if len(pkts[1]) < 8 || string(pkts[1][:8]) != opusTagsMagic {
		return nil, fmt.Errorf("opus tags")
	}
	dec, err := opus.NewDecoderConfig(opus.DecoderConfig{
		SampleRate:   opus.DefaultSampleRate,
		Channels:     head.channels,
		FrameSamples: decodeFrame,
	})
	if err != nil {
		return nil, err
	}
	defer func() { _ = dec.Close() }()

	pcm := make([]int16, 0, opus.DefaultSampleRate)
	for _, pkt := range pkts[2:] {
		if len(pkt) == 0 {
			continue
		}
		frame, err := dec.Decode(pkt)
		if err != nil || len(frame) == 0 {
			continue
		}
		pcm = append(pcm, frame...)
	}
	if skip := head.preSkip; skip > 0 {
		if skip >= len(pcm) {
			return nil, fmt.Errorf("empty opus pcm")
		}
		pcm = pcm[skip:]
	}
	if len(pcm) == 0 {
		return nil, fmt.Errorf("empty opus pcm")
	}
	return &Clip{PCM: pcm}, nil
}

func (c *Clip) Fill(dst []int16, pos *int) {
	if c == nil || len(c.PCM) == 0 || len(dst) == 0 {
		clear(dst)
		return
	}
	p := *pos
	if p < 0 || p >= len(c.PCM) {
		p = 0
	}
	for i := range dst {
		dst[i] = c.PCM[p]
		p++
		if p >= len(c.PCM) {
			p = 0
		}
	}
	*pos = p
}
