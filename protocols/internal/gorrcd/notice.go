// SPDX-License-Identifier: LicenseRef-Reticulum
package gorrcd

import (
	"strings"
	"unicode/utf8"
)

const maxNoticeChunk = 512

func (s *Service) notice(peer []byte, room, text string) {
	if text == "" || s.hub == nil {
		return
	}
	lines := strings.SplitSeq(text, "\n")
	for line := range lines {
		if line == "" {
			continue
		}
		for len(line) > 0 {
			n := min(maxNoticeChunk, len(line))
			for n > 0 && !utf8.ValidString(line[:n]) {
				n--
			}
			if n == 0 {
				break
			}
			_ = s.hub.SendNotice(peer, room, line[:n])
			line = line[n:]
		}
	}
}

func (s *Service) errorTo(peer []byte, room, text string) {
	if s.hub == nil {
		return
	}
	s.stats.Inc("errors_sent", 1)
	_ = s.hub.SendError(peer, room, text)
}

func (s *Service) noticeChunksOrResource(peer []byte, room, text, kind string) {
	if s.cfg.EnableResourceTransfer && len(text) > maxNoticeChunk && s.res != nil {
		if s.res.Send(peer, kind, []byte(text), room, "utf-8") {
			return
		}
	}
	s.notice(peer, room, text)
}
