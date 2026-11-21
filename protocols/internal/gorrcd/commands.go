// SPDX-License-Identifier: LicenseRef-Reticulum
package gorrcd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/rrc"
)

func (s *Service) handleCommand(peer []byte, room, text string) {
	parts := splitCmd(text)
	if len(parts) == 0 {
		return
	}
	cmd := strings.ToLower(parts[0])
	id, ok := idFrom(peer)
	if !ok {
		return
	}
	switch cmd {
	case "reload":
		s.cmdReload(peer, id)
	case "stats":
		s.cmdStats(peer, id)
	case "list":
		s.cmdList(peer)
	case "who", "names":
		s.cmdWho(peer, id, room, parts)
	case "kick":
		s.cmdKick(peer, id, room, parts)
	case "kline":
		s.cmdKline(peer, id, parts)
	case "register":
		s.cmdRegister(peer, id, room, parts)
	case "unregister":
		s.cmdUnregister(peer, id, room, parts)
	case "topic":
		s.cmdTopic(peer, id, room, parts)
	case "op", "deop", "voice", "devoice":
		s.cmdPriv(peer, id, room, cmd, parts)
	case "mode":
		s.cmdMode(peer, id, room, parts)
	case "ban":
		s.cmdBan(peer, id, room, parts)
	case "invite":
		s.cmdInvite(peer, id, room, parts)
	default:
		s.errorTo(peer, room, rrcdUnrecognized)
	}
}

func splitCmd(text string) []string {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "/") {
		text = text[1:]
	}
	fields := strings.Fields(text)
	out := fields[:0]
	for _, f := range fields {
		if f != "" {
			out = append(out, f)
		}
	}
	return out
}

func (s *Service) requireServerOp(peer []byte, id ID) bool {
	if s.trust.IsTrusted(id) {
		return true
	}
	s.errorTo(peer, "", rrcdNotAuthorized)
	return false
}

func (s *Service) requireRoomOp(peer []byte, id ID, room string) bool {
	if s.rooms.IsOp(room, id, s.trust.IsTrusted(id)) {
		return true
	}
	s.errorTo(peer, room, rrcdNotAuthorized)
	return false
}

func (s *Service) cmdReload(peer []byte, id ID) {
	if !s.requireServerOp(peer, id) {
		return
	}
	s.Reload(peer)
}

func (s *Service) cmdStats(peer []byte, id ID) {
	if !s.requireServerOp(peer, id) {
		return
	}
	if s.hub == nil {
		return
	}
	s.notice(peer, "", s.stats.Format(s.hub, s.cfg, s.trust, s.rooms))
}

func (s *Service) cmdList(peer []byte) {
	rooms := s.rooms.PublicRegistered()
	if len(rooms) == 0 {
		s.notice(peer, "", "No public rooms registered")
		return
	}
	var b strings.Builder
	b.WriteString("Registered public rooms:")
	for _, r := range rooms {
		if r.Topic != "" {
			fmt.Fprintf(&b, "\n  %s - %s", r.Name, r.Topic)
		} else {
			fmt.Fprintf(&b, "\n  %s", r.Name)
		}
	}
	s.notice(peer, "", b.String())
}

func (s *Service) cmdWho(peer []byte, id ID, room string, parts []string) {
	target := room
	if len(parts) >= 2 {
		target = parts[1]
	}
	if target == "" {
		s.notice(peer, "", "usage: /who [room]")
		return
	}
	r, err := s.normRoom(target)
	if err != nil {
		s.notice(peer, "", "bad room: "+err.Error())
		return
	}
	if s.rooms.IsPrivate(r) && !s.trust.IsTrusted(id) {
		s.notice(peer, "", "room "+r+" is private")
		return
	}
	if s.hub == nil {
		s.notice(peer, "", "members in "+r+": (none)")
		return
	}
	members := s.hub.RoomMemberInfo(r)
	items := make([]string, 0, len(members))
	for _, m := range members {
		hexid := fmt.Sprintf("%x", m.Hash)
		if m.Nick != "" {
			items = append(items, fmt.Sprintf("%s (%s)", m.Nick, trimHex(hexid, 12)))
		} else {
			items = append(items, hexid)
		}
	}
	body := "(none)"
	if len(items) > 0 {
		body = strings.Join(items, ", ")
	}
	s.notice(peer, "", "members in "+r+": "+body)
}

func (s *Service) cmdKick(peer []byte, id ID, room string, parts []string) {
	if len(parts) < 3 {
		s.notice(peer, "", "usage: /kick <room> <nick|hashprefix>")
		return
	}
	r, err := s.normRoom(parts[1])
	if err != nil {
		s.notice(peer, room, "bad room: "+err.Error())
		return
	}
	if !s.requireRoomOp(peer, id, r) {
		return
	}
	target, matches, err := s.resolve(parts[2], r)
	if err != nil {
		s.notice(peer, room, err.Error())
		return
	}
	if len(matches) != 1 && target == (ID{}) {
		s.notice(peer, room, formatAmbiguous(parts[2], matches))
		return
	}
	th := target.Bytes()
	if s.hub == nil || !s.hub.IsMember(th, r) {
		s.notice(peer, room, "target not in room")
		return
	}
	_ = s.hub.RemoveFromRoom(th, r)
	s.errorTo(th, r, "kicked from "+r)
	s.notice(peer, room, "kicked "+parts[2]+" from "+r)
}

func (s *Service) cmdKline(peer []byte, id ID, parts []string) {
	if !s.requireServerOp(peer, id) {
		return
	}
	if len(parts) < 2 {
		s.notice(peer, "", "usage: /kline add|del|list [nick|hashprefix|hash]")
		return
	}
	op := strings.ToLower(parts[1])
	if op == "list" {
		items := s.trust.BannedHex()
		sort.Strings(items)
		body := "(none)"
		if len(items) > 0 {
			body = strings.Join(items, ", ")
		}
		s.notice(peer, "", "klines: "+body)
		return
	}
	if op != "add" && op != "del" {
		s.notice(peer, "", "usage: /kline add|del|list [nick|hashprefix|hash]")
		return
	}
	if len(parts) < 3 {
		s.notice(peer, "", "usage: /kline "+op+" <nick|hashprefix|hash>")
		return
	}
	token := parts[2]
	if op == "add" {
		target, matches, err := s.resolve(token, "")
		if err == nil && target != (ID{}) {
			s.trust.Ban(target)
			_ = persistBannedIdentities(s.cfg.ConfigPath, s.trust.BannedHex())
			if s.hub != nil {
				s.hub.Disconnect(target.Bytes())
			}
			s.notice(peer, "", "kline added for "+token)
			return
		}
		if len(matches) > 1 {
			s.notice(peer, "", formatAmbiguous(token, matches))
			return
		}
		h, perr := parseFullID(token)
		if perr != nil {
			s.notice(peer, "", "bad identity hash: "+perr.Error())
			return
		}
		s.trust.Ban(h)
		_ = persistBannedIdentities(s.cfg.ConfigPath, s.trust.BannedHex())
		s.notice(peer, "", "kline added for "+h.Hex())
		return
	}
	h, err := parseFullID(token)
	if err != nil {
		s.notice(peer, "", "bad identity hash: "+err.Error())
		return
	}
	if s.trust.Unban(h) {
		_ = persistBannedIdentities(s.cfg.ConfigPath, s.trust.BannedHex())
		s.notice(peer, "", "kline removed for "+h.Hex())
		return
	}
	s.notice(peer, "", "not klined: "+h.Hex())
}

func (s *Service) cmdRegister(peer []byte, id ID, room string, parts []string) {
	if len(parts) < 2 {
		s.notice(peer, "", "usage: /register <room>")
		return
	}
	r, err := s.normRoom(parts[1])
	if err != nil {
		s.notice(peer, "", "bad room: "+err.Error())
		return
	}
	if s.hub == nil || room == "" || rrc.NormalizeRoom(room) != r || !s.hub.IsMember(peer, r) {
		s.notice(peer, room, "must be present in the room to register it")
		return
	}
	founder, ok := s.rooms.Founder(r)
	if !ok || founder != id {
		s.errorTo(peer, r, "only the room founder can register")
		return
	}
	if s.cfg.RoomRegistryPath == "" {
		s.notice(peer, room, "cannot register room: no room_registry_path")
		return
	}
	if err := s.rooms.Register(r, id); err != nil {
		s.notice(peer, room, err.Error())
		return
	}
	s.notice(peer, room, "registered room "+r)
}

func (s *Service) cmdUnregister(peer []byte, id ID, room string, parts []string) {
	if len(parts) < 2 {
		s.notice(peer, "", "usage: /unregister <room>")
		return
	}
	r, err := s.normRoom(parts[1])
	if err != nil {
		s.notice(peer, "", "bad room: "+err.Error())
		return
	}
	if s.hub == nil || room == "" || rrc.NormalizeRoom(room) != r || !s.hub.IsMember(peer, r) {
		s.notice(peer, room, "must be present in the room to unregister it")
		return
	}
	founder, ok := s.rooms.Founder(r)
	if !ok || founder != id {
		s.errorTo(peer, r, "only the room founder can unregister")
		return
	}
	if err := s.rooms.Unregister(r); err != nil {
		s.notice(peer, room, err.Error())
		return
	}
	s.notice(peer, room, "unregistered room "+r)
}

func (s *Service) cmdTopic(peer []byte, id ID, room string, parts []string) {
	if len(parts) < 2 {
		s.notice(peer, "", "usage: /topic <room> [topic]")
		return
	}
	r, err := s.normRoom(parts[1])
	if err != nil {
		s.notice(peer, "", "bad room: "+err.Error())
		return
	}
	if len(parts) == 2 {
		t := s.rooms.Topic(r)
		if t == "" {
			t = "(none)"
		}
		s.notice(peer, room, "topic for "+r+": "+t)
		return
	}
	if !s.rooms.IsOp(r, id, s.trust.IsTrusted(id)) && s.rooms.TopicOpsOnly(r) {
		s.errorTo(peer, r, rrcdNotAuthorizedTopic)
		return
	}
	topic := strings.TrimSpace(strings.Join(parts[2:], " "))
	s.rooms.SetTopic(r, topic)
	_ = s.rooms.Persist(r)
	shown := topic
	if shown == "" {
		shown = "(cleared)"
	}
	if s.hub != nil {
		s.hub.BroadcastNotice(r, "topic for "+r+" is now: "+shown)
	}
}

func (s *Service) cmdPriv(peer []byte, id ID, room, cmd string, parts []string) {
	if len(parts) < 3 {
		s.notice(peer, "", "usage: /"+cmd+" <room> <nick|hashprefix|hash>")
		return
	}
	r, err := s.normRoom(parts[1])
	if err != nil {
		s.notice(peer, "", "bad room: "+err.Error())
		return
	}
	if !s.requireRoomOp(peer, id, r) {
		return
	}
	target, matches, err := s.resolve(parts[2], r)
	if err != nil || target == (ID{}) {
		s.notice(peer, room, formatAmbiguous(parts[2], matches))
		return
	}
	founder, hasFounder := s.rooms.Founder(r)
	switch cmd {
	case "op":
		_ = s.rooms.AddOp(r, target)
		s.notice(peer, room, "op granted in "+r)
	case "deop":
		if hasFounder && target == founder {
			s.notice(peer, room, "cannot deop founder")
			return
		}
		_ = s.rooms.DelOp(r, target)
		s.notice(peer, room, "op removed in "+r)
	case "voice":
		_ = s.rooms.AddVoice(r, target)
		s.notice(peer, room, "voice granted in "+r)
	case "devoice":
		_ = s.rooms.DelVoice(r, target)
		s.notice(peer, room, "voice removed in "+r)
	}
}

func (s *Service) cmdMode(peer []byte, id ID, room string, parts []string) {
	if len(parts) < 3 {
		s.notice(peer, "", "usage: /mode <room> (+m|-m|+i|-i|+t|-t|+n|-n|+p|-p|+k|-k|+r|-r) [key] | /mode <room> (+o|-o|+v|-v) <nick|hashprefix|hash>")
		return
	}
	r, err := s.normRoom(parts[1])
	if err != nil {
		s.notice(peer, "", "bad room: "+err.Error())
		return
	}
	if !s.requireRoomOp(peer, id, r) {
		return
	}
	flag := strings.ToLower(parts[2])
	if flag == "+r" || flag == "-r" {
		s.notice(peer, room, "use /register or /unregister to change +r")
		return
	}
	if flag == "+o" || flag == "-o" || flag == "+v" || flag == "-v" {
		if len(parts) < 4 {
			s.notice(peer, room, "usage: /mode <room> (+o|-o|+v|-v) <nick|hashprefix|hash>")
			return
		}
		target, matches, err := s.resolve(parts[3], r)
		if err != nil || target == (ID{}) {
			s.notice(peer, room, formatAmbiguous(parts[3], matches))
			return
		}
		founder, hasFounder := s.rooms.Founder(r)
		switch flag {
		case "+o":
			_ = s.rooms.AddOp(r, target)
		case "-o":
			if hasFounder && target == founder {
				s.notice(peer, room, "cannot deop founder")
				return
			}
			_ = s.rooms.DelOp(r, target)
		case "+v":
			_ = s.rooms.AddVoice(r, target)
		case "-v":
			_ = s.rooms.DelVoice(r, target)
		}
		if s.hub != nil {
			s.hub.BroadcastNotice(r, "mode for "+r+" is now: "+flag+" "+target.Prefix(12))
		}
		return
	}
	on := strings.HasPrefix(flag, "+")
	ch := strings.TrimLeft(flag, "+-")
	if len(ch) != 1 {
		s.notice(peer, room, "supported modes: +m -m +i -i +k -k +t -t +n -n +p -p +r -r +o -o +v -v")
		return
	}
	key := ""
	if ch == "k" && on {
		if len(parts) < 4 {
			s.notice(peer, room, "usage: /mode <room> +k <key>")
			return
		}
		key = strings.TrimSpace(strings.Join(parts[3:], " "))
	}
	if err := s.rooms.SetFlag(r, ch, on, key); err != nil {
		if err.Error() == "unknown flag" {
			s.notice(peer, room, "supported modes: +m -m +i -i +k -k +t -t +n -n +p -p +r -r +o -o +v -v")
			return
		}
		s.notice(peer, room, err.Error())
		return
	}
	if s.hub != nil {
		s.hub.BroadcastNotice(r, "mode for "+r+" is now: "+s.rooms.ModeString(r))
	}
}

func (s *Service) cmdBan(peer []byte, id ID, room string, parts []string) {
	if len(parts) < 3 {
		s.notice(peer, "", "usage: /ban <room> add|del|list [nick|hashprefix|hash]")
		return
	}
	r, err := s.normRoom(parts[1])
	if err != nil {
		s.notice(peer, "", "bad room: "+err.Error())
		return
	}
	op := strings.ToLower(parts[2])
	if op == "list" {
		items := s.rooms.ListBans(r)
		if len(items) == 0 {
			s.notice(peer, room, "no bans in "+r)
			return
		}
		s.notice(peer, room, "bans in "+r+": "+strings.Join(items, ", "))
		return
	}
	if op != "add" && op != "del" {
		s.notice(peer, room, "usage: /ban <room> add|del|list [nick|hashprefix|hash]")
		return
	}
	if len(parts) < 4 {
		s.notice(peer, room, "usage: /ban "+r+" "+op+" <nick|hashprefix|hash>")
		return
	}
	if !s.requireRoomOp(peer, id, r) {
		return
	}
	target, matches, err := s.resolve(parts[3], r)
	if err != nil || target == (ID{}) {
		s.notice(peer, room, formatAmbiguous(parts[3], matches))
		return
	}
	if op == "add" {
		_ = s.rooms.AddBan(r, target)
		th := target.Bytes()
		if s.hub != nil && s.hub.IsMember(th, r) {
			_ = s.hub.RemoveFromRoom(th, r)
			s.errorTo(th, r, "banned from "+r)
		}
		s.notice(peer, room, "ban added in "+r)
		return
	}
	_ = s.rooms.DelBan(r, target)
	s.notice(peer, room, "ban removed in "+r)
}

func (s *Service) cmdInvite(peer []byte, id ID, room string, parts []string) {
	if len(parts) < 3 {
		s.notice(peer, "", "usage: /invite <room> add|del|list [nick|hashprefix|hash]")
		return
	}
	r, err := s.normRoom(parts[1])
	if err != nil {
		s.notice(peer, "", "bad room: "+err.Error())
		return
	}
	if !s.requireRoomOp(peer, id, r) {
		return
	}
	op := strings.ToLower(parts[2])
	if op == "list" {
		items := s.rooms.ListInvites(r)
		body := "(none)"
		if len(items) > 0 {
			body = strings.Join(items, ", ")
		}
		s.notice(peer, room, "invites in "+r+": "+body)
		return
	}
	if op != "add" && op != "del" {
		s.notice(peer, room, "usage: /invite <room> add|del|list [nick|hashprefix|hash]")
		return
	}
	if len(parts) < 4 {
		s.notice(peer, room, "usage: /invite "+r+" "+op+" <nick|hashprefix|hash>")
		return
	}
	if op == "add" {
		target, matches, err := s.resolve(parts[3], "")
		if err != nil || target == (ID{}) {
			s.errorTo(peer, r, "invite failed: "+formatAmbiguous(parts[3], matches))
			return
		}
		th := target.Bytes()
		if s.rooms.HasKey(r) {
			s.notice(th, r, "You have been invited to join "+r+". This invite allows joining without the key (+k).")
		} else {
			s.notice(th, r, "You have been invited to join "+r+".")
		}
		if s.rooms.NeedsStoredInvite(r) {
			_ = s.rooms.AddInvite(r, target, s.cfg.RoomInviteTimeoutS)
			s.notice(peer, room, fmt.Sprintf("invite added in %s (expires in %ds)", r, int(s.cfg.RoomInviteTimeoutS)))
			return
		}
		s.notice(peer, room, "invite sent to "+parts[3]+" for "+r)
		return
	}
	target, matches, err := s.resolve(parts[3], "")
	if err != nil || target == (ID{}) {
		s.notice(peer, room, formatAmbiguous(parts[3], matches))
		return
	}
	_ = s.rooms.DelInvite(r, target)
	s.notice(peer, room, "invite removed in "+r)
}

func (s *Service) normRoom(name string) (string, error) {
	r := rrc.NormalizeRoom(name)
	if r == "" {
		return "", fmt.Errorf("room name must not be empty")
	}
	if uint64(len(r)) > s.cfg.MaxRoomNameBytes {
		return "", fmt.Errorf("room name too long: %d bytes > %d bytes", len(r), s.cfg.MaxRoomNameBytes)
	}
	return r, nil
}

func (s *Service) resolve(token, room string) (ID, []rrc.PeerInfo, error) {
	token = strings.TrimSpace(token)
	var hashes [][]byte
	if s.hub != nil {
		if prefix, ok := hasHexPrefix(token); ok {
			hashes = s.hub.LookupHashPrefix(prefix)
		} else {
			hashes = s.hub.LookupNick(token)
		}
	}
	var matches []rrc.PeerInfo
	for _, h := range hashes {
		if room != "" && !s.hub.IsMember(h, room) {
			continue
		}
		matches = append(matches, rrc.PeerInfo{Hash: h, Nick: s.hub.Nick(h)})
	}
	if len(matches) == 1 {
		id, ok := idFrom(matches[0].Hash)
		if ok {
			return id, matches, nil
		}
	}
	if len(matches) > 1 {
		return ID{}, matches, nil
	}
	id, err := parseFullID(token)
	if err != nil {
		return ID{}, matches, nil
	}
	return id, matches, nil
}

func formatAmbiguous(token string, matches []rrc.PeerInfo) string {
	if len(matches) == 0 {
		return "target '" + token + "' not found"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "ambiguous: '%s' matches %d identities:", token, len(matches))
	for _, m := range matches {
		nick := "(no nick)"
		if m.Nick != "" {
			nick = "nick='" + m.Nick + "'"
		}
		fmt.Fprintf(&b, "\n  - %s %s", trimHex(fmt.Sprintf("%x", m.Hash), 16), nick)
	}
	b.WriteString("\nUse full or longer identity hash to disambiguate.")
	return b.String()
}

func trimHex(s string, n int) string {
	if n <= 0 || n >= len(s) {
		return s
	}
	return s[:n]
}
