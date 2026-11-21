// SPDX-License-Identifier: LicenseRef-Reticulum
package gorrcd

import (
	"bufio"
	"crypto/subtle"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/rrc"
)

type RoomState struct {
	Founder       ID
	HasFounder    bool
	Registered    bool
	Topic         string
	Moderated     bool
	InviteOnly    bool
	TopicOpsOnly  bool
	NoOutsideMsgs bool
	Private       bool
	Key           string
	Ops           map[ID]struct{}
	Voiced        map[ID]struct{}
	Bans          map[ID]struct{}
	Invited       map[ID]float64
	LastUsed      float64
}

func newRoomState(founder ID, hasFounder bool) *RoomState {
	st := &RoomState{
		Founder:    founder,
		HasFounder: hasFounder,
		Ops:        make(map[ID]struct{}),
		Voiced:     make(map[ID]struct{}),
		Bans:       make(map[ID]struct{}),
		Invited:    make(map[ID]float64),
	}
	if hasFounder {
		st.Ops[founder] = struct{}{}
	}
	return st
}

type RoomRegistry struct {
	mu       sync.Mutex
	path     string
	inviteTO float64
	rooms    map[string]*RoomState
}

func NewRoomRegistry(path string, inviteTimeoutS float64) *RoomRegistry {
	return &RoomRegistry{
		path:     path,
		inviteTO: inviteTimeoutS,
		rooms:    make(map[string]*RoomState),
	}
}

func (r *RoomRegistry) SetInviteTimeout(s float64) {
	r.mu.Lock()
	r.inviteTO = s
	r.mu.Unlock()
}

func (r *RoomRegistry) Ensure(room string, founder ID, hasFounder bool) *RoomState {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ensureLocked(room, founder, hasFounder)
}

func (r *RoomRegistry) ensureLocked(room string, founder ID, hasFounder bool) *RoomState {
	st := r.rooms[room]
	if st != nil {
		if !st.HasFounder && hasFounder {
			st.Founder = founder
			st.HasFounder = true
			st.Ops[founder] = struct{}{}
		}
		return st
	}
	st = newRoomState(founder, hasFounder)
	r.rooms[room] = st
	return st
}

func (r *RoomRegistry) Get(room string) *RoomState {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.rooms[room]
}

func (r *RoomRegistry) Touch(room string) {
	r.mu.Lock()
	st := r.ensureLocked(room, ID{}, false)
	st.LastUsed = float64(time.Now().Unix())
	r.mu.Unlock()
}

func (r *RoomRegistry) IsOp(room string, peer ID, serverOp bool) bool {
	if serverOp {
		return true
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	st := r.rooms[room]
	if st == nil {
		return false
	}
	if st.HasFounder && st.Founder == peer {
		return true
	}
	_, ok := st.Ops[peer]
	return ok
}

func (r *RoomRegistry) IsVoiced(room string, peer ID, serverOp bool) bool {
	if r.IsOp(room, peer, serverOp) {
		return true
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	st := r.rooms[room]
	if st == nil {
		return false
	}
	_, ok := st.Voiced[peer]
	return ok
}

func (r *RoomRegistry) IsBanned(room string, peer ID) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	st := r.rooms[room]
	if st == nil {
		return false
	}
	_, ok := st.Bans[peer]
	return ok
}

func (r *RoomRegistry) IsInvited(room string, peer ID) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	st := r.rooms[room]
	if st == nil {
		return false
	}
	exp, ok := st.Invited[peer]
	if !ok {
		return false
	}
	now := float64(time.Now().Unix())
	if exp <= now {
		delete(st.Invited, peer)
		return false
	}
	return true
}

func (r *RoomRegistry) ConsumeInvite(room string, peer ID) {
	r.mu.Lock()
	st := r.rooms[room]
	if st != nil {
		delete(st.Invited, peer)
	}
	r.mu.Unlock()
}

func (r *RoomRegistry) AllowJoin(room string, peer ID, body any, serverOp bool) error {
	r.mu.Lock()
	st := r.ensureLocked(room, peer, true)
	inviteOnly := st.InviteOnly
	key := st.Key
	banned := false
	if _, ok := st.Bans[peer]; ok {
		banned = true
	}
	invited := false
	if exp, ok := st.Invited[peer]; ok && exp > float64(time.Now().Unix()) {
		invited = true
	}
	isOp := serverOp || (st.HasFounder && st.Founder == peer)
	if !isOp {
		_, isOp = st.Ops[peer]
	}
	r.mu.Unlock()

	if banned {
		return fmt.Errorf("%s", rrcdBannedFromRoom)
	}
	if inviteOnly && !isOp && !invited {
		return fmt.Errorf("%s", rrcdInviteOnly)
	}
	if key != "" && !isOp && !invited {
		provided, _ := rrc.BodyAsString(body)
		if !keyEqual(provided, key) {
			return fmt.Errorf("%s", rrcdBadKey)
		}
	}
	return nil
}

func keyEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func (r *RoomRegistry) AllowContent(room string, peer ID, isMember bool, serverOp bool) error {
	r.mu.Lock()
	st := r.rooms[room]
	exists := st != nil
	noOut := false
	mod := false
	banned := false
	if st != nil {
		noOut = st.NoOutsideMsgs
		mod = st.Moderated
		_, banned = st.Bans[peer]
	}
	r.mu.Unlock()
	if !exists && !isMember {
		return fmt.Errorf("no such room")
	}
	if banned {
		return fmt.Errorf("%s", rrcdBannedFromRoom)
	}
	if !isMember && noOut {
		return fmt.Errorf("%s", rrcdNoOutside)
	}
	if mod && !r.IsVoiced(room, peer, serverOp) {
		return fmt.Errorf("%s", rrcdModerated)
	}
	return nil
}

func (r *RoomRegistry) ModeString(room string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	st := r.rooms[room]
	if st == nil {
		return "(none)"
	}
	var flags []byte
	if st.InviteOnly {
		flags = append(flags, 'i')
	}
	if st.Key != "" {
		flags = append(flags, 'k')
	}
	if st.Moderated {
		flags = append(flags, 'm')
	}
	if st.NoOutsideMsgs {
		flags = append(flags, 'n')
	}
	if st.Private {
		flags = append(flags, 'p')
	}
	if st.Registered {
		flags = append(flags, 'r')
	}
	if st.TopicOpsOnly {
		flags = append(flags, 't')
	}
	if len(flags) == 0 {
		return "(none)"
	}
	return "+" + string(flags)
}

func (r *RoomRegistry) InfoLine(room string) string {
	r.mu.Lock()
	st := r.rooms[room]
	reg := false
	topic := "(none)"
	r.mu.Unlock()
	if st != nil {
		reg = st.Registered
		if st.Topic != "" {
			topic = st.Topic
		}
	}
	kind := "unregistered"
	if reg {
		kind = "registered"
	}
	return fmt.Sprintf("room %s: %s; mode=%s; topic=%s", room, kind, r.ModeString(room), topic)
}

func (r *RoomRegistry) PublicRegistered() []struct{ Name, Topic string } {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]struct{ Name, Topic string }, 0)
	for name, st := range r.rooms {
		if st.Registered && !st.Private {
			out = append(out, struct{ Name, Topic string }{Name: name, Topic: st.Topic})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (r *RoomRegistry) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, st := range r.rooms {
		if st.Registered {
			n++
		}
	}
	return n
}

func (r *RoomRegistry) Snapshot() map[string]*RoomState {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]*RoomState, len(r.rooms))
	maps.Copy(out, r.rooms)
	return out
}

func (r *RoomRegistry) ReplaceAll(rooms map[string]*RoomState) {
	r.mu.Lock()
	r.rooms = rooms
	if r.rooms == nil {
		r.rooms = make(map[string]*RoomState)
	}
	r.mu.Unlock()
}

func (r *RoomRegistry) Persist(room string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.writeLocked()
}

func (r *RoomRegistry) Delete(room string) error {
	r.mu.Lock()
	delete(r.rooms, room)
	err := r.writeLocked()
	r.mu.Unlock()
	return err
}

func (r *RoomRegistry) PruneUnused(afterS float64, occupied map[string]struct{}, started time.Time) []string {
	now := float64(time.Now().Unix())
	r.mu.Lock()
	defer r.mu.Unlock()
	var gone []string
	for name, st := range r.rooms {
		if !st.Registered {
			continue
		}
		if _, live := occupied[name]; live {
			continue
		}
		last := st.LastUsed
		if last == 0 {
			last = float64(started.Unix())
		}
		if now-last >= afterS {
			gone = append(gone, name)
		}
	}
	for _, name := range gone {
		delete(r.rooms, name)
	}
	if len(gone) > 0 {
		_ = r.writeLocked()
	}
	return gone
}

func (r *RoomRegistry) writeLocked() error {
	if r.path == "" {
		return nil
	}
	var b strings.Builder
	b.WriteString("# gorrcd room registry\n\n[rooms]\n")
	names := make([]string, 0, len(r.rooms))
	for name, st := range r.rooms {
		if st.Registered {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		st := r.rooms[name]
		fmt.Fprintf(&b, "\n[rooms.%s]\n", quoteTableKey(name))
		if st.HasFounder {
			fmt.Fprintf(&b, "founder = %q\n", st.Founder.Hex())
		}
		if st.Topic != "" {
			fmt.Fprintf(&b, "topic = %q\n", st.Topic)
		}
		fmt.Fprintf(&b, "moderated = %v\n", st.Moderated)
		fmt.Fprintf(&b, "invite_only = %v\n", st.InviteOnly)
		fmt.Fprintf(&b, "topic_ops_only = %v\n", st.TopicOpsOnly)
		fmt.Fprintf(&b, "no_outside_msgs = %v\n", st.NoOutsideMsgs)
		fmt.Fprintf(&b, "private = %v\n", st.Private)
		if st.Key != "" {
			fmt.Fprintf(&b, "key = %q\n", st.Key)
		}
		fmt.Fprintf(&b, "operators = %s\n", formatIDList(st.Ops))
		fmt.Fprintf(&b, "voiced = %s\n", formatIDList(st.Voiced))
		fmt.Fprintf(&b, "bans = %s\n", formatIDList(st.Bans))
		fmt.Fprintf(&b, "invited = %s\n", formatInvited(st.Invited))
		if st.LastUsed > 0 {
			fmt.Fprintf(&b, "last_used_ts = %v\n", st.LastUsed)
		}
	}
	return atomicWrite(r.path, []byte(b.String()), 0o600)
}

func quoteTableKey(name string) string {
	if isBareTOMLKey(name) {
		return name
	}
	return strconv.Quote(name)
}

func isBareTOMLKey(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !((r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

func formatIDList(m map[ID]struct{}) string {
	if len(m) == 0 {
		return "[]"
	}
	ids := make([]string, 0, len(m))
	for id := range m {
		ids = append(ids, strconv.Quote(id.Hex()))
	}
	sort.Strings(ids)
	return "[" + strings.Join(ids, ", ") + "]"
}

func formatInvited(m map[ID]float64) string {
	if len(m) == 0 {
		return "{}"
	}
	now := float64(time.Now().Unix())
	parts := make([]string, 0, len(m))
	for id, exp := range m {
		if exp > now {
			parts = append(parts, fmt.Sprintf("%q = %v", id.Hex(), exp))
		}
	}
	sort.Strings(parts)
	return "{ " + strings.Join(parts, ", ") + " }"
}

func LoadRoomRegistry(path string) (map[string]*RoomState, error) {
	raw, err := os.ReadFile(filepath.Clean(path)) // #nosec G304 -- operator registry path
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]*RoomState{}, nil
		}
		return nil, err
	}
	return parseRoomsTOML(string(raw))
}

func parseRoomsTOML(text string) (map[string]*RoomState, error) {
	out := make(map[string]*RoomState)
	var cur string
	sc := bufio.NewScanner(strings.NewReader(text))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			name := strings.TrimSpace(line[1 : len(line)-1])
			if strings.EqualFold(name, "rooms") {
				cur = ""
				continue
			}
			if strings.HasPrefix(strings.ToLower(name), "rooms.") {
				cur = parseRoomTableName(name[len("rooms."):])
				if _, ok := out[cur]; !ok {
					out[cur] = newRoomState(ID{}, false)
					out[cur].Registered = true
				}
			} else {
				cur = ""
			}
			continue
		}
		if cur == "" {
			continue
		}
		k, v, ok := splitKV(line)
		if !ok {
			continue
		}
		applyRoomKey(out[cur], k, v)
	}
	return out, sc.Err()
}

func parseRoomTableName(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return unquote(s)
	}
	return s
}

func applyRoomKey(st *RoomState, k, v string) {
	switch k {
	case "founder":
		if id, err := parseFullID(unquote(v)); err == nil {
			st.Founder = id
			st.HasFounder = true
			st.Ops[id] = struct{}{}
		}
	case "topic":
		st.Topic = unquote(v)
	case "moderated":
		st.Moderated = parseBool(v)
	case "invite_only":
		st.InviteOnly = parseBool(v)
	case "topic_ops_only":
		st.TopicOpsOnly = parseBool(v)
	case "no_outside_msgs":
		st.NoOutsideMsgs = parseBool(v)
	case "private":
		st.Private = parseBool(v)
	case "key":
		st.Key = unquote(v)
	case "operators":
		for _, s := range parseStringList(v) {
			if id, err := parseFullID(s); err == nil {
				st.Ops[id] = struct{}{}
			}
		}
	case "voiced":
		for _, s := range parseStringList(v) {
			if id, err := parseFullID(s); err == nil {
				st.Voiced[id] = struct{}{}
			}
		}
	case "bans":
		for _, s := range parseStringList(v) {
			if id, err := parseFullID(s); err == nil {
				st.Bans[id] = struct{}{}
			}
		}
	case "invited":
		st.Invited = parseInvited(v)
	case "last_used_ts":
		st.LastUsed = parseFloat(v)
	}
}

func parseInvited(v string) map[ID]float64 {
	out := make(map[ID]float64)
	v = strings.TrimSpace(v)
	if strings.HasPrefix(v, "{") && strings.HasSuffix(v, "}") {
		v = strings.TrimSpace(v[1 : len(v)-1])
	}
	if v == "" {
		return out
	}
	parts := strings.Split(v, ",")
	now := float64(time.Now().Unix())
	for _, p := range parts {
		k, val, ok := splitKV(p)
		if !ok {
			continue
		}
		id, err := parseFullID(unquote(k))
		if err != nil {
			continue
		}
		exp := parseFloat(val)
		if exp > now {
			out[id] = exp
		}
	}
	return out
}
