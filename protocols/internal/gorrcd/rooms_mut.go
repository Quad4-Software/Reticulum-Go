// SPDX-License-Identifier: LicenseRef-Reticulum
package gorrcd

import (
	"fmt"
	"sort"
	"time"
)

func (r *RoomRegistry) Founder(room string) (ID, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	st := r.rooms[room]
	if st == nil || !st.HasFounder {
		return ID{}, false
	}
	return st.Founder, true
}

func (r *RoomRegistry) Topic(room string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	st := r.rooms[room]
	if st == nil {
		return ""
	}
	return st.Topic
}

func (r *RoomRegistry) SetTopic(room, topic string) {
	r.mu.Lock()
	st := r.ensureLocked(room, ID{}, false)
	st.Topic = topic
	st.LastUsed = float64(time.Now().Unix())
	r.mu.Unlock()
}

func (r *RoomRegistry) TopicOpsOnly(room string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	st := r.rooms[room]
	return st != nil && st.TopicOpsOnly
}

func (r *RoomRegistry) Register(room string, founder ID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	st := r.ensureLocked(room, founder, true)
	st.Registered = true
	st.NoOutsideMsgs = true
	st.TopicOpsOnly = true
	st.Ops[founder] = struct{}{}
	st.LastUsed = float64(time.Now().Unix())
	return r.writeLocked()
}

func (r *RoomRegistry) Unregister(room string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	st := r.rooms[room]
	if st == nil || !st.Registered {
		return fmt.Errorf("room %s is not registered", room)
	}
	st.Registered = false
	return r.writeLocked()
}

func (r *RoomRegistry) SetFlag(room, flag string, on bool, key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	st := r.ensureLocked(room, ID{}, false)
	switch flag {
	case "m":
		st.Moderated = on
	case "i":
		st.InviteOnly = on
	case "t":
		st.TopicOpsOnly = on
	case "n":
		st.NoOutsideMsgs = on
	case "p":
		st.Private = on
	case "k":
		if on {
			if key == "" {
				return fmt.Errorf("key must not be empty")
			}
			st.Key = key
		} else {
			st.Key = ""
		}
	default:
		return fmt.Errorf("unknown flag")
	}
	st.LastUsed = float64(time.Now().Unix())
	if st.Registered {
		return r.writeLocked()
	}
	return nil
}

func (r *RoomRegistry) AddOp(room string, target ID) error {
	return r.mutateSet(room, func(st *RoomState) { st.Ops[target] = struct{}{} })
}

func (r *RoomRegistry) DelOp(room string, target ID) error {
	return r.mutateSet(room, func(st *RoomState) {
		delete(st.Ops, target)
	})
}

func (r *RoomRegistry) AddVoice(room string, target ID) error {
	return r.mutateSet(room, func(st *RoomState) { st.Voiced[target] = struct{}{} })
}

func (r *RoomRegistry) DelVoice(room string, target ID) error {
	return r.mutateSet(room, func(st *RoomState) { delete(st.Voiced, target) })
}

func (r *RoomRegistry) AddBan(room string, target ID) error {
	return r.mutateSet(room, func(st *RoomState) { st.Bans[target] = struct{}{} })
}

func (r *RoomRegistry) DelBan(room string, target ID) error {
	return r.mutateSet(room, func(st *RoomState) { delete(st.Bans, target) })
}

func (r *RoomRegistry) ListBans(room string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	st := r.rooms[room]
	if st == nil {
		return nil
	}
	out := make([]string, 0, len(st.Bans))
	for id := range st.Bans {
		out = append(out, id.Hex())
	}
	sort.Strings(out)
	return out
}

func (r *RoomRegistry) AddInvite(room string, target ID, ttlS float64) error {
	if ttlS <= 0 {
		ttlS = 900
	}
	exp := float64(time.Now().Unix()) + ttlS
	return r.mutateSet(room, func(st *RoomState) { st.Invited[target] = exp })
}

func (r *RoomRegistry) DelInvite(room string, target ID) error {
	return r.mutateSet(room, func(st *RoomState) { delete(st.Invited, target) })
}

func (r *RoomRegistry) HasKey(room string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	st := r.rooms[room]
	return st != nil && st.Key != ""
}

func (r *RoomRegistry) NeedsStoredInvite(room string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	st := r.rooms[room]
	if st == nil {
		return false
	}
	return st.InviteOnly || st.Key != ""
}

func (r *RoomRegistry) ListInvites(room string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	st := r.rooms[room]
	if st == nil {
		return nil
	}
	now := float64(time.Now().Unix())
	out := make([]string, 0)
	for id, exp := range st.Invited {
		if exp > now {
			out = append(out, fmt.Sprintf("%s expires_in=%ds", id.Hex(), int(exp-now)))
		}
	}
	sort.Strings(out)
	return out
}

func (r *RoomRegistry) IsPrivate(room string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	st := r.rooms[room]
	return st != nil && st.Private
}

func (r *RoomRegistry) mutateSet(room string, fn func(*RoomState)) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	st := r.ensureLocked(room, ID{}, false)
	fn(st)
	st.LastUsed = float64(time.Now().Unix())
	if st.Registered {
		return r.writeLocked()
	}
	return nil
}
