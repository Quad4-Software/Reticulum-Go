// SPDX-License-Identifier: LicenseRef-Reticulum
package opusfile

import (
	"encoding/binary"
	"fmt"
)

const (
	maxFileBytes   = 4 << 20
	maxPages       = 8192
	maxPacketBytes = 65536
	oggCapture     = "OggS"
	opusHeadMagic  = "OpusHead"
	opusTagsMagic  = "OpusTags"
	headMinLen     = 19
)

type page struct {
	segs [][]byte
}

func readPage(data []byte) (page, int, error) {
	if len(data) < 27 {
		return page{}, 0, fmt.Errorf("ogg page truncated")
	}
	if string(data[:4]) != oggCapture {
		return page{}, 0, fmt.Errorf("ogg capture pattern")
	}
	nseg := int(data[26])
	if len(data) < 27+nseg {
		return page{}, 0, fmt.Errorf("ogg segment table truncated")
	}
	body := 0
	for i := range nseg {
		body += int(data[27+i])
	}
	off := 27 + nseg
	if len(data) < off+body {
		return page{}, 0, fmt.Errorf("ogg page body truncated")
	}
	p := page{segs: make([][]byte, 0, nseg)}
	cursor := off
	for i := range nseg {
		n := int(data[27+i])
		p.segs = append(p.segs, data[cursor:cursor+n])
		cursor += n
	}
	return p, cursor, nil
}

func packets(data []byte) ([][]byte, error) {
	var out [][]byte
	var cur []byte
	off := 0
	pages := 0
	for off < len(data) {
		if pages >= maxPages {
			return nil, fmt.Errorf("ogg page limit")
		}
		p, n, err := readPage(data[off:])
		if err != nil {
			if off > 0 && len(data)-off < 4 {
				break
			}
			return nil, err
		}
		pages++
		off += n
		for _, seg := range p.segs {
			if len(cur)+len(seg) > maxPacketBytes {
				return nil, fmt.Errorf("ogg packet too large")
			}
			cur = append(cur, seg...)
			if len(seg) < 255 {
				out = append(out, cur)
				cur = nil
			}
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no ogg packets")
	}
	return out, nil
}

type header struct {
	channels int
	preSkip  int
	rate     int
}

func parseHead(pkt []byte) (header, error) {
	if len(pkt) < headMinLen || string(pkt[:8]) != opusHeadMagic {
		return header{}, fmt.Errorf("opus head")
	}
	ch := int(pkt[9])
	if ch < 1 || ch > 8 {
		return header{}, fmt.Errorf("opus channels %d", ch)
	}
	return header{
		channels: ch,
		preSkip:  int(binary.LittleEndian.Uint16(pkt[10:12])),
		rate:     int(binary.LittleEndian.Uint32(pkt[12:16])),
	}, nil
}
