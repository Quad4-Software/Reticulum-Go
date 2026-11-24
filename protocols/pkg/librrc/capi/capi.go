//go:build cgo

package capi

/*
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <limits.h>

typedef struct rrc_event {
	int kind;
	uint8_t sender[16];
	size_t sender_len;
	uint8_t peer[16];
	size_t peer_len;
	char room[128];
	int room_truncated;
	char nick[64];
	int nick_truncated;
	char body[1024];
	int body_truncated;
	uint64_t msg_type;
} rrc_event;

static inline int rrc_size_as_cint(size_t n) {
	if (n > (size_t)INT_MAX) {
		return -1;
	}
	return (int)n;
}
*/
import "C"

import (
	"math"
	"strings"
	"sync"
	"unsafe"

	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/librrc"
)

const maxCGoBytes = math.MaxInt32

var (
	versionCString *C.char
)

func init() {
	versionCString = C.CString(librrc.Version())
}

//export rrc_version
func rrc_version() *C.char {
	return versionCString
}

//export rrc_last_error
func rrc_last_error(buf *C.char, bufLen C.size_t, written *C.size_t) C.int {
	if written != nil {
		*written = 0
	}
	msg := librrc.LastError()
	if buf == nil || bufLen == 0 {
		if written != nil {
			*written = sizeFromInt(len(msg))
		}
		if len(msg) > 0 {
			return cCode(librrc.ErrTruncated)
		}
		return cCode(librrc.OK)
	}
	n := copyCString(buf, bufLen, msg)
	if written != nil {
		*written = sizeFromInt(n)
	}
	if n < len(msg) {
		return cCode(librrc.ErrTruncated)
	}
	return cCode(librrc.OK)
}

//export rrc_envelope_create
func rrc_envelope_create(msgType C.uint64_t, sender *C.uint8_t, senderLen C.size_t) C.uint64_t {
	data, code := goBytesFromC(sender, senderLen)
	if code != librrc.OK {
		return 0
	}
	id, _ := librrc.EnvelopeCreate(uint64(msgType), data)
	return C.uint64_t(id)
}

//export rrc_envelope_set_room
func rrc_envelope_set_room(handle C.uint64_t, room *C.char) C.int {
	return cCode(librrc.EnvelopeSetRoom(uint64(handle), cStringOrEmpty(room)))
}

//export rrc_envelope_set_nick
func rrc_envelope_set_nick(handle C.uint64_t, nick *C.char) C.int {
	return cCode(librrc.EnvelopeSetNick(uint64(handle), cStringOrEmpty(nick)))
}

//export rrc_envelope_set_body_text
func rrc_envelope_set_body_text(handle C.uint64_t, text *C.char) C.int {
	return cCode(librrc.EnvelopeSetBodyText(uint64(handle), cStringOrEmpty(text)))
}

//export rrc_envelope_set_destination
func rrc_envelope_set_destination(handle C.uint64_t, dest *C.uint8_t, destLen C.size_t) C.int {
	data, code := goBytesFromC(dest, destLen)
	if code != librrc.OK {
		return cCode(code)
	}
	return cCode(librrc.EnvelopeSetDestination(uint64(handle), data))
}

//export rrc_envelope_get_type
func rrc_envelope_get_type(handle C.uint64_t, out *C.uint64_t) C.int {
	if out == nil {
		return cCode(librrc.ErrInvalidArg)
	}
	var t uint64
	code := librrc.EnvelopeGetType(uint64(handle), &t)
	if code != librrc.OK {
		return cCode(code)
	}
	*out = C.uint64_t(t)
	return cCode(librrc.OK)
}

//export rrc_envelope_get_sender
func rrc_envelope_get_sender(handle C.uint64_t, out *C.uint8_t, outLen C.size_t, written *C.size_t) C.int {
	data, code := librrc.EnvelopeGetSender(uint64(handle))
	if code != librrc.OK {
		return cCode(code)
	}
	return writeBytes(out, outLen, written, data)
}

//export rrc_envelope_get_room
func rrc_envelope_get_room(handle C.uint64_t, buf *C.char, bufLen C.size_t, written *C.size_t) C.int {
	s, code := librrc.EnvelopeGetRoom(uint64(handle))
	if code != librrc.OK {
		return cCode(code)
	}
	return writeString(buf, bufLen, written, s)
}

//export rrc_envelope_get_nick
func rrc_envelope_get_nick(handle C.uint64_t, buf *C.char, bufLen C.size_t, written *C.size_t) C.int {
	s, code := librrc.EnvelopeGetNick(uint64(handle))
	if code != librrc.OK {
		return cCode(code)
	}
	return writeString(buf, bufLen, written, s)
}

//export rrc_envelope_get_body_text
func rrc_envelope_get_body_text(handle C.uint64_t, buf *C.char, bufLen C.size_t, written *C.size_t) C.int {
	s, code := librrc.EnvelopeGetBodyText(uint64(handle))
	if code != librrc.OK {
		return cCode(code)
	}
	return writeString(buf, bufLen, written, s)
}

//export rrc_envelope_marshal
func rrc_envelope_marshal(handle C.uint64_t, out *C.uint8_t, outLen C.size_t, written *C.size_t) C.int {
	data, code := librrc.EnvelopeMarshal(uint64(handle))
	if code != librrc.OK {
		return cCode(code)
	}
	return writeBytes(out, outLen, written, data)
}

//export rrc_envelope_unmarshal
func rrc_envelope_unmarshal(data *C.uint8_t, dataLen C.size_t) C.uint64_t {
	raw, code := goBytesFromC(data, dataLen)
	if code != librrc.OK {
		return 0
	}
	id, _ := librrc.EnvelopeUnmarshal(raw)
	return C.uint64_t(id)
}

//export rrc_envelope_destroy
func rrc_envelope_destroy(handle C.uint64_t) C.int {
	return cCode(librrc.EnvelopeDestroy(uint64(handle)))
}

//export rrc_normalize_room
func rrc_normalize_room(in *C.char, out *C.char, outLen C.size_t, written *C.size_t) C.int {
	s, code := librrc.NormalizeRoom(cStringOrEmpty(in))
	if code != librrc.OK {
		return cCode(code)
	}
	return writeString(out, outLen, written, s)
}

//export rrc_sanitize_nick
func rrc_sanitize_nick(in *C.char, out *C.char, outLen C.size_t, written *C.size_t) C.int {
	s, code := librrc.SanitizeNick(cStringOrEmpty(in))
	if code != librrc.OK {
		return cCode(code)
	}
	return writeString(out, outLen, written, s)
}

//export rrc_node_create
func rrc_node_create(configPath *C.char) C.uint64_t {
	id, _ := librrc.NodeCreate(cStringOrEmpty(configPath))
	return C.uint64_t(id)
}

//export rrc_node_start
func rrc_node_start(node C.uint64_t) C.int {
	return cCode(librrc.NodeStart(uint64(node)))
}

//export rrc_node_stop
func rrc_node_stop(node C.uint64_t) C.int {
	return cCode(librrc.NodeStop(uint64(node)))
}

//export rrc_node_destroy
func rrc_node_destroy(node C.uint64_t) C.int {
	return cCode(librrc.NodeDestroy(uint64(node)))
}

//export rrc_node_set_identity
func rrc_node_set_identity(node, identity C.uint64_t) C.int {
	return cCode(librrc.NodeSetIdentity(uint64(node), uint64(identity)))
}

//export rrc_node_add_udp_interface
func rrc_node_add_udp_interface(node C.uint64_t, name, localAddr, peerAddr *C.char) C.int {
	return cCode(librrc.NodeAddUDPInterface(uint64(node), cStringOrEmpty(name), cStringOrEmpty(localAddr), cStringOrEmpty(peerAddr)))
}

//export rrc_node_add_tcp_client_interface
func rrc_node_add_tcp_client_interface(node C.uint64_t, name, target *C.char) C.int {
	return cCode(librrc.NodeAddTCPClientInterface(uint64(node), cStringOrEmpty(name), cStringOrEmpty(target)))
}

//export rrc_node_add_tcp_server_interface
func rrc_node_add_tcp_server_interface(node C.uint64_t, name, listen *C.char) C.int {
	return cCode(librrc.NodeAddTCPServerInterface(uint64(node), cStringOrEmpty(name), cStringOrEmpty(listen)))
}

//export rrc_node_add_auto_interface
func rrc_node_add_auto_interface(node C.uint64_t, name *C.char) C.int {
	return cCode(librrc.NodeAddAutoInterface(uint64(node), cStringOrEmpty(name)))
}

//export rrc_node_add_backbone_client_interface
func rrc_node_add_backbone_client_interface(node C.uint64_t, name, target *C.char) C.int {
	return cCode(librrc.NodeAddBackboneClientInterface(uint64(node), cStringOrEmpty(name), cStringOrEmpty(target)))
}

//export rrc_node_interface_count
func rrc_node_interface_count(node C.uint64_t) C.int {
	n, code := librrc.NodeInterfaceCount(uint64(node))
	if code != librrc.OK {
		return cCode(code)
	}
	return C.int(n)
}

//export rrc_node_interface_names
func rrc_node_interface_names(node C.uint64_t, out *C.char, outLen C.size_t, written *C.size_t) C.int {
	names, code := librrc.NodeInterfaceNames(uint64(node))
	if code != librrc.OK {
		return cCode(code)
	}
	return writeString(out, outLen, written, strings.Join(names, "\n"))
}

//export rrc_node_has_path
func rrc_node_has_path(node C.uint64_t, destHash *C.uint8_t, destLen C.size_t, hasPath *C.int) C.int {
	if hasPath == nil {
		return cCode(librrc.ErrInvalidArg)
	}
	hash, code := goBytesFromC(destHash, destLen)
	if code != librrc.OK {
		return cCode(code)
	}
	ok, code := librrc.NodeHasPath(uint64(node), hash)
	if code != librrc.OK {
		return cCode(code)
	}
	if ok {
		*hasPath = 1
	} else {
		*hasPath = 0
	}
	return cCode(librrc.OK)
}

//export rrc_identity_generate
func rrc_identity_generate() C.uint64_t {
	id, _ := librrc.IdentityGenerate()
	return C.uint64_t(id)
}

//export rrc_identity_load
func rrc_identity_load(path *C.char) C.uint64_t {
	if path == nil {
		return 0
	}
	id, _ := librrc.IdentityLoad(cStringOrEmpty(path))
	return C.uint64_t(id)
}

//export rrc_identity_save
func rrc_identity_save(identity C.uint64_t, path *C.char) C.int {
	if path == nil {
		return cCode(librrc.ErrInvalidArg)
	}
	return cCode(librrc.IdentitySave(uint64(identity), cStringOrEmpty(path)))
}

//export rrc_identity_destroy
func rrc_identity_destroy(identity C.uint64_t) C.int {
	return cCode(librrc.IdentityDestroy(uint64(identity)))
}

//export rrc_identity_hash
func rrc_identity_hash(identity C.uint64_t, out *C.uint8_t, outLen C.size_t, written *C.size_t) C.int {
	data, code := librrc.IdentityHash(uint64(identity))
	if code != librrc.OK {
		return cCode(code)
	}
	return writeBytes(out, outLen, written, data)
}

//export rrc_identity_seed_destination
func rrc_identity_seed_destination(identity C.uint64_t, destHash *C.uint8_t, destLen C.size_t) C.int {
	hash, code := goBytesFromC(destHash, destLen)
	if code != librrc.OK {
		return cCode(code)
	}
	return cCode(librrc.IdentitySeedDestination(uint64(identity), hash))
}

//export rrc_hub_create
func rrc_hub_create(node, identity C.uint64_t, name, version *C.char) C.uint64_t {
	id, _ := librrc.HubCreate(uint64(node), uint64(identity), cStringOrEmpty(name), cStringOrEmpty(version))
	return C.uint64_t(id)
}

//export rrc_hub_start
func rrc_hub_start(hub C.uint64_t) C.int {
	return cCode(librrc.HubStart(uint64(hub)))
}

//export rrc_hub_announce
func rrc_hub_announce(hub C.uint64_t) C.int {
	return cCode(librrc.HubAnnounce(uint64(hub)))
}

//export rrc_hub_hash
func rrc_hub_hash(hub C.uint64_t, out *C.uint8_t, outLen C.size_t, written *C.size_t) C.int {
	data, code := librrc.HubHash(uint64(hub))
	if code != librrc.OK {
		return cCode(code)
	}
	return writeBytes(out, outLen, written, data)
}

//export rrc_hub_peer_count
func rrc_hub_peer_count(hub C.uint64_t, count *C.size_t) C.int {
	if count == nil {
		return cCode(librrc.ErrInvalidArg)
	}
	n, code := librrc.HubPeerCount(uint64(hub))
	if code != librrc.OK {
		return cCode(code)
	}
	*count = sizeFromInt(n)
	return cCode(librrc.OK)
}

//export rrc_hub_destroy
func rrc_hub_destroy(hub C.uint64_t) C.int {
	return cCode(librrc.HubDestroy(uint64(hub)))
}

//export rrc_hub_event_poll
func rrc_hub_event_poll(hub C.uint64_t, timeoutMs C.int, event *C.rrc_event) C.int {
	if event == nil {
		return cCode(librrc.ErrInvalidArg)
	}
	ev, code := librrc.HubEventPoll(uint64(hub), int(timeoutMs))
	fillEvent(event, ev)
	return cCode(code)
}

//export rrc_client_dial
func rrc_client_dial(node, identity C.uint64_t, hubHash *C.uint8_t, hubHashLen C.size_t,
	nick, name, version *C.char, timeoutMs C.int) C.uint64_t {
	hash, code := goBytesFromC(hubHash, hubHashLen)
	if code != librrc.OK {
		return 0
	}
	id, _ := librrc.ClientDial(uint64(node), uint64(identity), hash,
		cStringOrEmpty(nick), cStringOrEmpty(name), cStringOrEmpty(version), int(timeoutMs))
	return C.uint64_t(id)
}

//export rrc_client_join
func rrc_client_join(client C.uint64_t, room *C.char) C.int {
	return cCode(librrc.ClientJoin(uint64(client), cStringOrEmpty(room)))
}

//export rrc_client_part
func rrc_client_part(client C.uint64_t, room *C.char) C.int {
	return cCode(librrc.ClientPart(uint64(client), cStringOrEmpty(room)))
}

//export rrc_client_send_msg
func rrc_client_send_msg(client C.uint64_t, room, text *C.char) C.int {
	return cCode(librrc.ClientSendMsg(uint64(client), cStringOrEmpty(room), cStringOrEmpty(text)))
}

//export rrc_client_send_notice
func rrc_client_send_notice(client C.uint64_t, room, text *C.char) C.int {
	return cCode(librrc.ClientSendNotice(uint64(client), cStringOrEmpty(room), cStringOrEmpty(text)))
}

//export rrc_client_send_action
func rrc_client_send_action(client C.uint64_t, room, text *C.char) C.int {
	return cCode(librrc.ClientSendAction(uint64(client), cStringOrEmpty(room), cStringOrEmpty(text)))
}

//export rrc_client_ping
func rrc_client_ping(client C.uint64_t) C.int {
	return cCode(librrc.ClientPing(uint64(client)))
}

//export rrc_client_close
func rrc_client_close(client C.uint64_t) C.int {
	return cCode(librrc.ClientClose(uint64(client)))
}

//export rrc_client_event_poll
func rrc_client_event_poll(client C.uint64_t, timeoutMs C.int, event *C.rrc_event) C.int {
	if event == nil {
		return cCode(librrc.ErrInvalidArg)
	}
	ev, code := librrc.ClientEventPoll(uint64(client), int(timeoutMs))
	fillEvent(event, ev)
	return cCode(code)
}

func fillEvent(dst *C.rrc_event, ev librrc.Event) {
	dst.kind = cInt(ev.Kind)
	dst.msg_type = C.uint64_t(ev.MsgType)
	dst.sender_len = 0
	dst.peer_len = 0
	dst.room_truncated = 0
	dst.nick_truncated = 0
	dst.body_truncated = 0
	if len(ev.Sender) > 0 {
		n := copyCBytes(&dst.sender[0], C.size_t(len(dst.sender)), ev.Sender)
		dst.sender_len = sizeFromInt(n)
	}
	if len(ev.Peer) > 0 {
		n := copyCBytes(&dst.peer[0], C.size_t(len(dst.peer)), ev.Peer)
		dst.peer_len = sizeFromInt(n)
	}
	if _, trunc := copyCStringField(&dst.room[0], C.size_t(len(dst.room)), ev.Room); trunc {
		dst.room_truncated = 1
	}
	if _, trunc := copyCStringField(&dst.nick[0], C.size_t(len(dst.nick)), ev.Nick); trunc {
		dst.nick_truncated = 1
	}
	if _, trunc := copyCStringField(&dst.body[0], C.size_t(len(dst.body)), ev.Body); trunc {
		dst.body_truncated = 1
	}
}

func copyCString(dst *C.char, capacity C.size_t, s string) int {
	limit, ok := sizeToInt(capacity)
	if !ok || limit == 0 {
		return 0
	}
	room := limit - 1
	n := len(s)
	if n > room {
		n = room
	}
	if n > 0 {
		C.memcpy(unsafe.Pointer(dst), unsafe.Pointer(unsafe.StringData(s)), sizeFromInt(n))
	}
	p := unsafe.Slice((*byte)(unsafe.Pointer(dst)), limit)
	p[n] = 0
	return n
}

func copyCStringField(dst *C.char, capacity C.size_t, s string) (int, bool) {
	limit, ok := sizeToInt(capacity)
	if !ok || limit == 0 {
		return 0, len(s) > 0
	}
	room := limit - 1
	n := len(s)
	trunc := n > room
	if n > room {
		n = room
	}
	if n > 0 {
		C.memcpy(unsafe.Pointer(dst), unsafe.Pointer(unsafe.StringData(s)), sizeFromInt(n))
	}
	p := unsafe.Slice((*byte)(unsafe.Pointer(dst)), limit)
	p[n] = 0
	return n, trunc
}

func writeString(buf *C.char, bufLen C.size_t, written *C.size_t, s string) C.int {
	if written != nil {
		*written = 0
	}
	if buf == nil || bufLen == 0 {
		if written != nil {
			*written = sizeFromInt(len(s))
		}
		if len(s) > 0 {
			return cCode(librrc.ErrTruncated)
		}
		return cCode(librrc.OK)
	}
	n := copyCString(buf, bufLen, s)
	if written != nil {
		*written = sizeFromInt(n)
	}
	if n < len(s) {
		return cCode(librrc.ErrTruncated)
	}
	return cCode(librrc.OK)
}

func writeBytes(out *C.uint8_t, outLen C.size_t, written *C.size_t, src []byte) C.int {
	if written != nil {
		*written = 0
	}
	if out == nil || outLen == 0 {
		if written != nil {
			*written = sizeFromInt(len(src))
		}
		if len(src) > 0 {
			return cCode(librrc.ErrTruncated)
		}
		return cCode(librrc.OK)
	}
	n := copyCBytes(out, outLen, src)
	if written != nil {
		*written = sizeFromInt(n)
	}
	if n < len(src) {
		return cCode(librrc.ErrTruncated)
	}
	return cCode(librrc.OK)
}

func cStringOrEmpty(p *C.char) string {
	if p == nil {
		return ""
	}
	return C.GoString(p)
}

func copyCBytes(dst *C.uint8_t, capacity C.size_t, src []byte) int {
	limit, ok := sizeToInt(capacity)
	if !ok || limit == 0 {
		return 0
	}
	n := len(src)
	if n > limit {
		n = limit
	}
	if n > 0 {
		C.memcpy(unsafe.Pointer(dst), unsafe.Pointer(&src[0]), sizeFromInt(n))
	}
	return n
}

func sizeToInt(n C.size_t) (int, bool) {
	if n > C.size_t(maxCGoBytes) {
		return 0, false
	}
	return int(n), true
}

func sizeFromInt(n int) C.size_t {
	if n < 0 {
		return 0
	}
	return C.size_t(n)
}

func goBytesFromC(ptr *C.uint8_t, n C.size_t) ([]byte, int) {
	if n == 0 {
		return nil, librrc.OK
	}
	if ptr == nil {
		return nil, librrc.ErrInvalidArg
	}
	cint := C.rrc_size_as_cint(n)
	if cint < 0 {
		return nil, librrc.ErrInvalidArg
	}
	return C.GoBytes(unsafe.Pointer(ptr), cint), librrc.OK
}

func cCode(code int) C.int {
	switch code {
	case librrc.OK:
		return 0
	case librrc.ErrInvalidArg:
		return 1
	case librrc.ErrInvalidHandle:
		return 2
	case librrc.ErrNotFound:
		return 3
	case librrc.ErrState:
		return 4
	case librrc.ErrIO:
		return 5
	case librrc.ErrInternal:
		return 6
	case librrc.ErrTimeout:
		return 7
	case librrc.ErrTruncated:
		return 8
	default:
		return 6
	}
}

// #nosec G115 -- RRC event fields fit the C ABI int type
func cInt(v int) C.int {
	return C.int(v)
}

var _ = sync.Mutex{}
