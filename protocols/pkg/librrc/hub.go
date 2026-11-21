// SPDX-License-Identifier: LicenseRef-Reticulum
package librrc

import (
	"time"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/rrc"
)

type hubRecord struct {
	handle uint64
	hub    *rrc.Hub
	dest   []byte
	queue  *eventQueue
}

func HubCreate(nodeHandle, identityHandle uint64, name, version string) (uint64, int) {
	tr, code := NodeTransport(nodeHandle)
	if code != OK {
		return 0, code
	}
	id, code := nodeIdentity(nodeHandle)
	if code != OK {
		return 0, code
	}
	idRec, err := identityByHandle(identityHandle)
	if err != nil {
		return 0, setLastError(err)
	}
	if idRec.identity != id {
		id = idRec.identity
	}

	dest, err := rrc.NewHubDestination(id, tr)
	if err != nil {
		return 0, setLastError(err)
	}

	q := newEventQueue(defaultQueueCapacity)
	rec := &hubRecord{
		dest:  append([]byte(nil), dest.GetHash()...),
		queue: q,
	}

	cfg := rrc.HubConfig{
		Name:    name,
		Version: version,
		Handlers: rrc.HubHandlers{
			OnHello: func(peer []byte, body *rrc.HelloBody, env *rrc.Envelope) {
				ev := Event{Kind: EventHello, Peer: append([]byte(nil), peer...), MsgType: env.Type}
				if body != nil {
					ev.Body = body.ClientName
					ev.Nick = body.ClientVersion
				}
				q.push(ev)
			},
			OnJoin: func(peer []byte, room string, env *rrc.Envelope) {
				q.push(Event{Kind: EventJoin, Peer: append([]byte(nil), peer...), Room: room, MsgType: env.Type})
			},
			OnPart: func(peer []byte, room string, env *rrc.Envelope) {
				q.push(Event{Kind: EventPart, Peer: append([]byte(nil), peer...), Room: room, MsgType: env.Type})
			},
			OnMsg: func(peer []byte, env *rrc.Envelope) {
				text, _ := rrc.BodyAsString(env.Body)
				q.push(Event{
					Kind:    EventMsg,
					Peer:    append([]byte(nil), peer...),
					Sender:  append([]byte(nil), env.Sender...),
					Room:    env.Room,
					Nick:    env.Nick,
					Body:    text,
					MsgType: env.Type,
				})
			},
			OnClose: func(peer []byte) {
				q.push(Event{Kind: EventClose, Peer: append([]byte(nil), peer...)})
			},
		},
	}

	hub, err := rrc.NewHub(tr, dest, cfg)
	if err != nil {
		return 0, setLastError(err)
	}
	rec.hub = hub

	runtimeMu.Lock()
	hubHandle := handles.insert(kindHub, rec)
	rec.handle = hubHandle
	runtimeMu.Unlock()
	return hubHandle, OK
}

func HubStart(handle uint64) int {
	rec, err := hubByHandle(handle)
	if err != nil {
		return setLastError(err)
	}
	rec.hub.Start()
	return OK
}

func HubAnnounce(handle uint64) int {
	rec, err := hubByHandle(handle)
	if err != nil {
		return setLastError(err)
	}
	dest := rec.hub.Destination()
	if dest == nil {
		return setLastError(errState)
	}
	if err := dest.Announce(false, nil, nil); err != nil {
		return setLastError(err)
	}
	return OK
}

func HubHash(handle uint64) ([]byte, int) {
	rec, err := hubByHandle(handle)
	if err != nil {
		return nil, setLastError(err)
	}
	return append([]byte(nil), rec.dest...), OK
}

func HubPeerCount(handle uint64) (int, int) {
	rec, err := hubByHandle(handle)
	if err != nil {
		return 0, setLastError(err)
	}
	return rec.hub.PeerCount(), OK
}

func HubDestroy(handle uint64) int {
	rec, err := hubByHandle(handle)
	if err != nil {
		return setLastError(err)
	}
	rec.hub.Close()
	rec.queue.clear()
	runtimeMu.Lock()
	handles.delete(handle)
	runtimeMu.Unlock()
	return OK
}

func HubEventPoll(handle uint64, timeoutMs int) (Event, int) {
	rec, err := hubByHandle(handle)
	if err != nil {
		return Event{}, setLastError(err)
	}
	timeout := time.Duration(timeoutMs) * time.Millisecond
	ev, ok := rec.queue.poll(timeout)
	if !ok && ev.Kind == EventTimeout {
		return ev, ErrTimeout
	}
	return ev, OK
}

func hubByHandle(id uint64) (*hubRecord, error) {
	ref, err := handles.get(id, kindHub)
	if err != nil {
		return nil, err
	}
	return ref.(*hubRecord), nil
}
