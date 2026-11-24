// SPDX-License-Identifier: LicenseRef-Reticulum
package librrc

import (
	"fmt"
	"strings"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/rrc"
)

type clientRecord struct {
	handle uint64
	client *rrc.Client
	queue  *eventQueue
}

func ClientDial(nodeHandle, identityHandle uint64, hubHash []byte, nick, name, version string, timeoutMs int) (uint64, int) {
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
	if len(hubHash) != rrc.IdentityLength {
		return 0, setLastError(fmt.Errorf("%w: hub hash", errInvalidArg))
	}

	q := newEventQueue(defaultQueueCapacity)
	timeout := time.Duration(timeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 45 * time.Second
	}

	cfg := rrc.ClientConfig{
		Nick:           nick,
		Name:           name,
		Version:        version,
		DialTimeout:    timeout,
		WelcomeTimeout: timeout,
		Handlers: rrc.ClientHandlers{
			OnWelcome: func(body *rrc.WelcomeBody, env *rrc.Envelope) {
				text := ""
				if body != nil {
					text = body.HubName
				}
				q.push(Event{Kind: EventWelcome, Body: text, MsgType: env.Type})
			},
			OnJoined: func(room string, members [][]byte, env *rrc.Envelope) {
				// Member list (when the hub includes it): hex hashes, one
				// per line, so embedders can show who is in the room.
				var b strings.Builder
				for _, m := range members {
					fmt.Fprintf(&b, "%x\n", m)
				}
				q.push(Event{Kind: EventJoined, Room: room, Body: b.String(), MsgType: env.Type})
			},
			OnParted: func(room string, env *rrc.Envelope) {
				q.push(Event{Kind: EventParted, Room: room, MsgType: env.Type})
			},
			OnMsg: func(env *rrc.Envelope) {
				text, _ := rrc.BodyAsString(env.Body)
				q.push(Event{
					Kind:    EventMsg,
					Sender:  append([]byte(nil), env.Sender...),
					Room:    env.Room,
					Nick:    env.Nick,
					Body:    text,
					MsgType: env.Type,
				})
			},
			OnNotice: func(env *rrc.Envelope) {
				text, _ := rrc.BodyAsString(env.Body)
				q.push(Event{
					Kind:    EventNotice,
					Sender:  append([]byte(nil), env.Sender...),
					Room:    env.Room,
					Nick:    env.Nick,
					Body:    text,
					MsgType: env.Type,
				})
			},
			OnAction: func(env *rrc.Envelope) {
				text, _ := rrc.BodyAsString(env.Body)
				q.push(Event{
					Kind:    EventAction,
					Sender:  append([]byte(nil), env.Sender...),
					Room:    env.Room,
					Nick:    env.Nick,
					Body:    text,
					MsgType: env.Type,
				})
			},
			OnError: func(env *rrc.Envelope) {
				text, _ := rrc.BodyAsString(env.Body)
				q.push(Event{
					Kind:    EventError,
					Room:    env.Room,
					Body:    text,
					MsgType: env.Type,
				})
			},
			OnPong: func(env *rrc.Envelope) {
				q.push(Event{Kind: EventPong, MsgType: env.Type})
			},
			OnClose: func() {
				q.push(Event{Kind: EventClose})
			},
		},
	}

	client, err := rrc.Dial(tr, id, hubHash, cfg)
	if err != nil {
		return 0, setLastError(err)
	}

	rec := &clientRecord{client: client, queue: q}
	runtimeMu.Lock()
	handle := handles.insert(kindClient, rec)
	rec.handle = handle
	runtimeMu.Unlock()
	return handle, OK
}

func ClientJoin(handle uint64, room string) int {
	rec, err := clientByHandle(handle)
	if err != nil {
		return setLastError(err)
	}
	if err := rec.client.Join(room); err != nil {
		return setLastError(err)
	}
	return OK
}

func ClientPart(handle uint64, room string) int {
	rec, err := clientByHandle(handle)
	if err != nil {
		return setLastError(err)
	}
	if err := rec.client.Part(room); err != nil {
		return setLastError(err)
	}
	return OK
}

func ClientSendMsg(handle uint64, room, text string) int {
	rec, err := clientByHandle(handle)
	if err != nil {
		return setLastError(err)
	}
	if err := rec.client.SendMsg(room, text); err != nil {
		return setLastError(err)
	}
	return OK
}

func ClientSendNotice(handle uint64, room, text string) int {
	rec, err := clientByHandle(handle)
	if err != nil {
		return setLastError(err)
	}
	if err := rec.client.SendNotice(room, text); err != nil {
		return setLastError(err)
	}
	return OK
}

func ClientSendAction(handle uint64, room, text string) int {
	rec, err := clientByHandle(handle)
	if err != nil {
		return setLastError(err)
	}
	if err := rec.client.SendAction(room, text); err != nil {
		return setLastError(err)
	}
	return OK
}

func ClientPing(handle uint64) int {
	rec, err := clientByHandle(handle)
	if err != nil {
		return setLastError(err)
	}
	if err := rec.client.Ping(nil); err != nil {
		return setLastError(err)
	}
	return OK
}

func ClientClose(handle uint64) int {
	rec, err := clientByHandle(handle)
	if err != nil {
		return setLastError(err)
	}
	rec.client.Close()
	rec.queue.clear()
	runtimeMu.Lock()
	handles.delete(handle)
	runtimeMu.Unlock()
	return OK
}

func ClientEventPoll(handle uint64, timeoutMs int) (Event, int) {
	rec, err := clientByHandle(handle)
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

func clientByHandle(id uint64) (*clientRecord, error) {
	ref, err := handles.get(id, kindClient)
	if err != nil {
		return nil, err
	}
	return ref.(*clientRecord), nil
}
