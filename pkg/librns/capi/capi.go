//go:build cgo

package capi

/*
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <limits.h>

typedef struct rns_event {
	int kind;
	uint8_t link_id[16];
	size_t link_id_len;
	uint8_t destination_hash[16];
	size_t destination_hash_len;
	uint8_t identity_hash[16];
	size_t identity_hash_len;
	uint8_t request_id[16];
	size_t request_id_len;
	uint8_t hops;
	char path[256];
	int path_truncated;
	char error_message[256];
	int error_message_truncated;
	uint8_t *app_data;
	size_t app_data_len;
	size_t app_data_cap;
	int app_data_truncated;
} rns_event;

typedef struct rns_path_entry {
	uint8_t hash[16];
	size_t hash_len;
	uint8_t via[16];
	size_t via_len;
	uint8_t hops;
	char iface[64];
	double timestamp;
	double expires;
} rns_path_entry;

typedef struct rns_interface_entry {
	char name[96];
	char type_name[32];
	int online;
	int enabled;
	uint64_t rx_bytes;
	uint64_t tx_bytes;
	uint64_t rx_packets;
	uint64_t tx_packets;
} rns_interface_entry;

typedef void (*rns_event_callback)(const rns_event *event, void *user_data);

static inline int rns_size_as_cint(size_t n) {
	if (n > (size_t)INT_MAX) {
		return -1;
	}
	return (int)n;
}

static inline void call_rns_event_callback(rns_event_callback cb, const rns_event *event, void *user_data) {
	if (cb != NULL) {
		cb(event, user_data);
	}
}
*/
import "C"

import (
	"math"
	"sync"
	"time"
	"unsafe"

	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/librns"
)

const maxCGoBytes = math.MaxInt32

var (
	versionCString *C.char

	cbMu      sync.Mutex
	cbFn      map[uint64]C.rns_event_callback
	cbUser    map[uint64]unsafe.Pointer
	cbScratch map[uint64][]byte
)

func init() {
	versionCString = C.CString(librns.Version())
	cbFn = make(map[uint64]C.rns_event_callback)
	cbUser = make(map[uint64]unsafe.Pointer)
	cbScratch = make(map[uint64][]byte)
}

//export rns_version
func rns_version() *C.char {
	return versionCString
}

//export rns_last_error
func rns_last_error(buf *C.char, bufLen C.size_t, written *C.size_t) C.int {
	if written != nil {
		*written = 0
	}
	msg := librns.LastError()
	if buf == nil || bufLen == 0 {
		if written != nil {
			*written = sizeFromInt(len(msg))
		}
		if len(msg) > 0 {
			return cCode(librns.ErrTruncated)
		}
		return cCode(librns.OK)
	}
	n := copyCString(buf, bufLen, msg)
	if written != nil {
		*written = sizeFromInt(n)
	}
	if n < len(msg) {
		return cCode(librns.ErrTruncated)
	}
	return cCode(librns.OK)
}

//export rns_node_create
func rns_node_create(configPath *C.char) C.uint64_t {
	path := cStringOrEmpty(configPath)
	id, code := librns.NodeCreate(path)
	if code != librns.OK {
		return 0
	}
	return C.uint64_t(id)
}

//export rns_node_start
func rns_node_start(node C.uint64_t) C.int {
	return cCode(librns.NodeStart(uint64(node)))
}

//export rns_node_stop
func rns_node_stop(node C.uint64_t) C.int {
	return cCode(librns.NodeStop(uint64(node)))
}

//export rns_node_destroy
func rns_node_destroy(node C.uint64_t) C.int {
	return cCode(librns.NodeDestroy(uint64(node)))
}

//export rns_node_set_identity
func rns_node_set_identity(node, identity C.uint64_t) C.int {
	return cCode(librns.NodeSetIdentity(uint64(node), uint64(identity)))
}

//export rns_node_resume
func rns_node_resume(node C.uint64_t) C.int {
	return cCode(librns.NodeResume(uint64(node)))
}

//export rns_node_pause
func rns_node_pause(node C.uint64_t) C.int {
	return cCode(librns.NodePause(uint64(node)))
}

//export rns_node_refresh_paths
func rns_node_refresh_paths(node C.uint64_t, destHashes *C.uint8_t, count C.size_t) C.int {
	n, ok := sizeToInt(count)
	if !ok {
		return cCode(librns.ErrInvalidArg)
	}
	var hashes [][]byte
	if n > 0 {
		if destHashes == nil {
			return cCode(librns.ErrInvalidArg)
		}
		raw := unsafe.Slice((*byte)(unsafe.Pointer(destHashes)), n*16)
		hashes = make([][]byte, n)
		for i := 0; i < n; i++ {
			hashes[i] = append([]byte(nil), raw[i*16:(i+1)*16]...)
		}
	}
	return cCode(librns.NodeRefreshPaths(uint64(node), hashes...))
}

//export rns_identity_generate
func rns_identity_generate() C.uint64_t {
	id, code := librns.IdentityGenerate()
	if code != librns.OK {
		return 0
	}
	return C.uint64_t(id)
}

//export rns_identity_load
func rns_identity_load(path *C.char) C.uint64_t {
	if path == nil {
		return 0
	}
	id, code := librns.IdentityLoad(C.GoString(path))
	if code != librns.OK {
		return 0
	}
	return C.uint64_t(id)
}

//export rns_identity_save
func rns_identity_save(identity C.uint64_t, path *C.char) C.int {
	if path == nil {
		return cCode(librns.ErrInvalidArg)
	}
	return cCode(librns.IdentitySave(uint64(identity), C.GoString(path)))
}

//export rns_identity_destroy
func rns_identity_destroy(identity C.uint64_t) C.int {
	return cCode(librns.IdentityDestroy(uint64(identity)))
}

//export rns_identity_hash
func rns_identity_hash(identity C.uint64_t, hexBuf *C.char, hexBufLen C.size_t, written *C.size_t) C.int {
	if written != nil {
		*written = 0
	}
	hexHash, code := librns.IdentityHashHex(uint64(identity))
	if code != librns.OK {
		return cCode(code)
	}
	if hexBuf == nil || hexBufLen == 0 {
		if written != nil {
			*written = sizeFromInt(len(hexHash))
		}
		return cCode(librns.ErrTruncated)
	}
	n := copyCString(hexBuf, hexBufLen, hexHash)
	if written != nil {
		*written = sizeFromInt(n)
	}
	if n < len(hexHash) {
		return cCode(librns.ErrTruncated)
	}
	return cCode(librns.OK)
}

//export rns_identity_hash_bytes
func rns_identity_hash_bytes(identity C.uint64_t, out *C.uint8_t, outLen C.size_t, written *C.size_t) C.int {
	if written != nil {
		*written = 0
	}
	hash, code := librns.IdentityHashBytes(uint64(identity))
	if code != librns.OK {
		return cCode(code)
	}
	return copyBytesResult(out, outLen, written, hash)
}

//export rns_identity_public_key
func rns_identity_public_key(identity C.uint64_t, out *C.uint8_t, outLen C.size_t, written *C.size_t) C.int {
	if written != nil {
		*written = 0
	}
	pub, code := librns.IdentityPublicKey(uint64(identity))
	if code != librns.OK {
		return cCode(code)
	}
	return copyBytesResult(out, outLen, written, pub)
}

//export rns_identity_from_public_key
func rns_identity_from_public_key(pub *C.uint8_t, pubLen C.size_t) C.uint64_t {
	raw, code := goBytesFromC(pub, pubLen)
	if code != librns.OK {
		return 0
	}
	id, code := librns.IdentityFromPublicKey(raw)
	if code != librns.OK {
		return 0
	}
	return C.uint64_t(id)
}

//export rns_identity_sign
func rns_identity_sign(identity C.uint64_t, data *C.uint8_t, dataLen C.size_t, sigOut *C.uint8_t, sigOutLen C.size_t, written *C.size_t) C.int {
	if written != nil {
		*written = 0
	}
	raw, code := goBytesFromC(data, dataLen)
	if code != librns.OK {
		return cCode(code)
	}
	sig, code := librns.IdentitySign(uint64(identity), raw)
	if code != librns.OK {
		return cCode(code)
	}
	return copyBytesResult(sigOut, sigOutLen, written, sig)
}

//export rns_identity_verify
func rns_identity_verify(identity C.uint64_t, data *C.uint8_t, dataLen C.size_t, sig *C.uint8_t, sigLen C.size_t) C.int {
	raw, code := goBytesFromC(data, dataLen)
	if code != librns.OK {
		return cCode(code)
	}
	signature, code := goBytesFromC(sig, sigLen)
	if code != librns.OK {
		return cCode(code)
	}
	return cCode(librns.IdentityVerify(uint64(identity), raw, signature))
}

//export rns_rsg_create
func rns_rsg_create(identity C.uint64_t, message *C.uint8_t, messageLen C.size_t, embed C.int, out *C.uint8_t, outLen C.size_t, written *C.size_t) C.int {
	if written != nil {
		*written = 0
	}
	msg, code := goBytesFromC(message, messageLen)
	if code != librns.OK {
		return cCode(code)
	}
	blob, code := librns.RSGCreate(uint64(identity), msg, embed != 0)
	if code != librns.OK {
		return cCode(code)
	}
	return copyBytesResult(out, outLen, written, blob)
}

//export rns_rsg_validate
func rns_rsg_validate(rsg *C.uint8_t, rsgLen C.size_t, message *C.uint8_t, messageLen C.size_t, requiredSignerHash *C.uint8_t, requiredSignerHashLen C.size_t) C.int {
	rsgBytes, code := goBytesFromC(rsg, rsgLen)
	if code != librns.OK {
		return cCode(code)
	}
	msg, code := goBytesFromC(message, messageLen)
	if code != librns.OK {
		return cCode(code)
	}
	required, code := goBytesFromC(requiredSignerHash, requiredSignerHashLen)
	if code != librns.OK {
		return cCode(code)
	}
	return cCode(librns.RSGValidate(rsgBytes, msg, required))
}

//export rns_rsg_sign_file
func rns_rsg_sign_file(identity C.uint64_t, path *C.char, out *C.uint8_t, outLen C.size_t, written *C.size_t) C.int {
	if written != nil {
		*written = 0
	}
	if path == nil {
		return cCode(librns.ErrInvalidArg)
	}
	blob, code := librns.RSGSignFile(uint64(identity), C.GoString(path))
	if code != librns.OK {
		return cCode(code)
	}
	return copyBytesResult(out, outLen, written, blob)
}

//export rns_rsg_verify_file
func rns_rsg_verify_file(rsg *C.uint8_t, rsgLen C.size_t, path *C.char, requiredSignerHash *C.uint8_t, requiredSignerHashLen C.size_t) C.int {
	if path == nil {
		return cCode(librns.ErrInvalidArg)
	}
	rsgBytes, code := goBytesFromC(rsg, rsgLen)
	if code != librns.OK {
		return cCode(code)
	}
	required, code := goBytesFromC(requiredSignerHash, requiredSignerHashLen)
	if code != librns.OK {
		return cCode(code)
	}
	return cCode(librns.RSGVerifyFile(rsgBytes, C.GoString(path), required))
}

//export rns_rsm_verify
func rns_rsm_verify(rsm *C.uint8_t, rsmLen C.size_t, requiredSignerHash *C.uint8_t, requiredSignerHashLen C.size_t, messageOut *C.uint8_t, messageOutLen C.size_t, written *C.size_t) C.int {
	if written != nil {
		*written = 0
	}
	rsmBytes, code := goBytesFromC(rsm, rsmLen)
	if code != librns.OK {
		return cCode(code)
	}
	required, code := goBytesFromC(requiredSignerHash, requiredSignerHashLen)
	if code != librns.OK {
		return cCode(code)
	}
	msg, code := librns.RSMVerify(rsmBytes, required)
	if code != librns.OK {
		return cCode(code)
	}
	return copyBytesResult(messageOut, messageOutLen, written, msg)
}

func copyBytesResult(out *C.uint8_t, outLen C.size_t, written *C.size_t, src []byte) C.int {
	if written != nil {
		*written = sizeFromInt(len(src))
	}
	if out == nil || outLen == 0 {
		if len(src) == 0 {
			return cCode(librns.OK)
		}
		return cCode(librns.ErrTruncated)
	}
	n := copyCBytes(out, outLen, src)
	if written != nil {
		*written = sizeFromInt(n)
	}
	if n < len(src) {
		return cCode(librns.ErrTruncated)
	}
	return cCode(librns.OK)
}

//export rns_destination_create
func rns_destination_create(node, identity C.uint64_t, appName *C.char, aspects **C.char, aspectCount C.size_t, acceptsLinks C.int) C.uint64_t {
	if appName == nil {
		return 0
	}
	count, ok := sizeToInt(aspectCount)
	if !ok {
		return 0
	}
	aspectSlice := cStringArray(aspects, count)
	id, code := librns.DestinationCreate(uint64(node), uint64(identity), C.GoString(appName), aspectSlice, acceptsLinks != 0)
	if code != librns.OK {
		return 0
	}
	return C.uint64_t(id)
}

//export rns_destination_set_proof_strategy
func rns_destination_set_proof_strategy(destination C.uint64_t, strategy C.int) C.int {
	if strategy < 0 || strategy > 255 {
		return cCode(librns.ErrInvalidArg)
	}
	return cCode(librns.DestinationSetProofStrategy(uint64(destination), byte(strategy))) // #nosec G115 -- range checked above, values validated in DestinationSetProofStrategy
}

//export rns_destination_enable_ratchets
func rns_destination_enable_ratchets(destination C.uint64_t, path *C.char) C.int {
	p := ""
	if path != nil {
		p = C.GoString(path)
	}
	return cCode(librns.DestinationEnableRatchets(uint64(destination), p))
}

//export rns_destination_enforce_ratchets
func rns_destination_enforce_ratchets(destination C.uint64_t) C.int {
	return cCode(librns.DestinationEnforceRatchets(uint64(destination)))
}

//export rns_destination_announce
func rns_destination_announce(destination C.uint64_t, appData *C.uint8_t, appDataLen C.size_t) C.int {
	data, code := goBytesFromC(appData, appDataLen)
	if code != librns.OK {
		return cCode(code)
	}
	return cCode(librns.DestinationAnnounce(uint64(destination), data))
}

//export rns_destination_hash
func rns_destination_hash(destination C.uint64_t, hashOut *C.uint8_t, hashOutLen C.size_t, written *C.size_t) C.int {
	if written != nil {
		*written = 0
	}
	hash, code := librns.DestinationHash(uint64(destination))
	if code != librns.OK {
		return cCode(code)
	}
	return copyFixedBytes(hashOut, hashOutLen, written, hash)
}

//export rns_destination_destroy
func rns_destination_destroy(destination C.uint64_t) C.int {
	return cCode(librns.DestinationDestroy(uint64(destination)))
}

//export rns_destination_encrypt
func rns_destination_encrypt(destHash *C.uint8_t, plaintext *C.uint8_t, plaintextLen C.size_t, out *C.uint8_t, outLen C.size_t, written *C.size_t) C.int {
	if written != nil {
		*written = 0
	}
	if destHash == nil {
		return cCode(librns.ErrInvalidArg)
	}
	hash := C.GoBytes(unsafe.Pointer(destHash), identity.TruncatedHashLength/8)
	raw, code := goBytesFromC(plaintext, plaintextLen)
	if code != librns.OK {
		return cCode(code)
	}
	ct, code := librns.DestinationEncrypt(hash, raw)
	if code != librns.OK {
		return cCode(code)
	}
	return copyBytesResult(out, outLen, written, ct)
}

//export rns_packet_send
func rns_packet_send(node C.uint64_t, destHash *C.uint8_t, plaintext *C.uint8_t, plaintextLen C.size_t) C.int {
	if destHash == nil {
		return cCode(librns.ErrInvalidArg)
	}
	hash := C.GoBytes(unsafe.Pointer(destHash), identity.TruncatedHashLength/8)
	raw, code := goBytesFromC(plaintext, plaintextLen)
	if code != librns.OK {
		return cCode(code)
	}
	return cCode(librns.PacketSend(uint64(node), hash, raw))
}

//export rns_destination_register_request_handler
func rns_destination_register_request_handler(destination C.uint64_t, path *C.char) C.int {
	if path == nil {
		return cCode(librns.ErrInvalidArg)
	}
	return cCode(librns.DestinationRegisterRequestHandler(uint64(destination), C.GoString(path)))
}

//export rns_path_request
func rns_path_request(node C.uint64_t, destHash *C.uint8_t) C.int {
	if destHash == nil {
		return cCode(librns.ErrInvalidArg)
	}
	hash := C.GoBytes(unsafe.Pointer(destHash), identity.TruncatedHashLength/8)
	return cCode(librns.PathRequest(uint64(node), hash))
}

//export rns_path_table
func rns_path_table(node C.uint64_t, out *C.rns_path_entry, outCap C.size_t, written *C.size_t, maxHops C.int) C.int {
	if written != nil {
		*written = 0
	}
	rows, code := librns.PathTable(uint64(node), int(maxHops))
	if code != librns.OK {
		return cCode(code)
	}
	if out == nil || outCap == 0 {
		if written != nil {
			*written = sizeFromInt(len(rows))
		}
		if len(rows) > 0 {
			return cCode(librns.ErrTruncated)
		}
		return cCode(librns.OK)
	}
	capacity, ok := sizeToInt(outCap)
	if !ok {
		return cCode(librns.ErrInvalidArg)
	}
	n := len(rows)
	if n > capacity {
		n = capacity
	}
	if written != nil {
		*written = sizeFromInt(n)
	}
	slice := unsafe.Slice(out, capacity)
	for i := 0; i < n; i++ {
		fillPathEntry(&slice[i], rows[i])
	}
	if len(rows) > capacity {
		return cCode(librns.ErrTruncated)
	}
	return cCode(librns.OK)
}

//export rns_interfaces
func rns_interfaces(node C.uint64_t, out *C.rns_interface_entry, outCap C.size_t, written *C.size_t) C.int {
	if written != nil {
		*written = 0
	}
	rows, code := librns.NodeInterfaces(uint64(node))
	if code != librns.OK {
		return cCode(code)
	}
	if written != nil {
		*written = sizeFromInt(len(rows))
	}
	if out == nil || outCap == 0 {
		if len(rows) > 0 {
			return cCode(librns.ErrTruncated)
		}
		return cCode(librns.OK)
	}
	capacity, ok := sizeToInt(outCap)
	if !ok {
		return cCode(librns.ErrInvalidArg)
	}
	n := len(rows)
	if n > capacity {
		n = capacity
	}
	slice := unsafe.Slice(out, capacity)
	for i := 0; i < n; i++ {
		fillInterfaceEntry(&slice[i], rows[i])
	}
	if len(rows) > capacity {
		return cCode(librns.ErrTruncated)
	}
	return cCode(librns.OK)
}

//export rns_link_open
func rns_link_open(node C.uint64_t, destHash *C.uint8_t) C.uint64_t {
	if destHash == nil {
		return 0
	}
	hash := C.GoBytes(unsafe.Pointer(destHash), identity.TruncatedHashLength/8)
	id, code := librns.LinkOpen(uint64(node), hash)
	if code != librns.OK {
		return 0
	}
	return C.uint64_t(id)
}

//export rns_link_send
func rns_link_send(link C.uint64_t, data *C.uint8_t, dataLen C.size_t) C.int {
	if data == nil && dataLen > 0 {
		return cCode(librns.ErrInvalidArg)
	}
	payload, code := goBytesFromC(data, dataLen)
	if code != librns.OK {
		return cCode(code)
	}
	return cCode(librns.LinkSend(uint64(link), payload))
}

//export rns_link_send_resource
func rns_link_send_resource(link C.uint64_t, data *C.uint8_t, dataLen C.size_t, name *C.char) C.int {
	if data == nil && dataLen > 0 {
		return cCode(librns.ErrInvalidArg)
	}
	payload, code := goBytesFromC(data, dataLen)
	if code != librns.OK {
		return cCode(code)
	}
	var fileName string
	if name != nil {
		fileName = C.GoString(name)
	}
	return cCode(librns.LinkSendResource(uint64(link), payload, fileName))
}

//export rns_link_close
func rns_link_close(link C.uint64_t) C.int {
	return cCode(librns.LinkClose(uint64(link)))
}

//export rns_link_id
func rns_link_id(link C.uint64_t, idOut *C.uint8_t, idOutLen C.size_t, written *C.size_t) C.int {
	if written != nil {
		*written = 0
	}
	id, code := librns.LinkID(uint64(link))
	if code != librns.OK {
		return cCode(code)
	}
	return copyFixedBytes(idOut, idOutLen, written, id)
}

//export rns_link_from_id
func rns_link_from_id(node C.uint64_t, linkID *C.uint8_t, linkIDLen C.size_t) C.uint64_t {
	id, code := goBytesFromC(linkID, linkIDLen)
	if code != librns.OK || len(id) == 0 {
		return 0
	}
	h, code := librns.LinkFromID(uint64(node), id)
	if code != librns.OK {
		return 0
	}
	return C.uint64_t(h)
}

//export rns_link_request
func rns_link_request(node, link C.uint64_t, path *C.char, data *C.uint8_t, dataLen C.size_t, timeoutMs C.int, requestIDOut *C.uint8_t, requestIDOutLen C.size_t, written *C.size_t) C.int {
	if path == nil {
		return cCode(librns.ErrInvalidArg)
	}
	payload, code := goBytesFromC(data, dataLen)
	if code != librns.OK {
		return cCode(code)
	}
	id, code := librns.LinkRequest(uint64(node), uint64(link), C.GoString(path), payload, int(timeoutMs))
	if code != librns.OK {
		return cCode(code)
	}
	return copyFixedBytes(requestIDOut, requestIDOutLen, written, id)
}

//export rns_request_respond
func rns_request_respond(node C.uint64_t, requestID *C.uint8_t, requestIDLen C.size_t, data *C.uint8_t, dataLen C.size_t) C.int {
	rid, code := goBytesFromC(requestID, requestIDLen)
	if code != librns.OK {
		return cCode(code)
	}
	payload, code := goBytesFromC(data, dataLen)
	if code != librns.OK {
		return cCode(code)
	}
	return cCode(librns.RequestRespond(uint64(node), rid, payload))
}

//export rns_request_respond_file
func rns_request_respond_file(node C.uint64_t, requestID *C.uint8_t, requestIDLen C.size_t, filename *C.char, data *C.uint8_t, dataLen C.size_t) C.int {
	if filename == nil {
		return cCode(librns.ErrInvalidArg)
	}
	rid, code := goBytesFromC(requestID, requestIDLen)
	if code != librns.OK {
		return cCode(code)
	}
	payload, code := goBytesFromC(data, dataLen)
	if code != librns.OK {
		return cCode(code)
	}
	return cCode(librns.RequestRespondFile(uint64(node), rid, C.GoString(filename), payload))
}

//export rns_event_poll
func rns_event_poll(node C.uint64_t, event *C.rns_event, timeoutMs C.int) C.int {
	if event == nil {
		return cCode(librns.ErrInvalidArg)
	}
	timeout := time.Duration(timeoutMs) * time.Millisecond
	if timeoutMs < 0 {
		timeout = 0
	}
	ev, code := librns.EventPoll(uint64(node), timeout)
	if code != librns.OK {
		return cCode(code)
	}
	fillEvent(event, ev)
	return cCode(librns.OK)
}

//export rns_set_event_callback
func rns_set_event_callback(node C.uint64_t, callback C.rns_event_callback, userData unsafe.Pointer) C.int {
	id := uint64(node)
	if callback == nil {
		cbMu.Lock()
		delete(cbFn, id)
		delete(cbUser, id)
		delete(cbScratch, id)
		cbMu.Unlock()
		return cCode(librns.SetEventCallback(id, nil))
	}
	cbMu.Lock()
	cbFn[id] = callback
	cbUser[id] = userData
	if _, ok := cbScratch[id]; !ok {
		cbScratch[id] = make([]byte, 65536)
	}
	cbMu.Unlock()
	return cCode(librns.SetEventCallback(id, func(ev librns.Event) {
		cbMu.Lock()
		fn := cbFn[id]
		ud := cbUser[id]
		buf := cbScratch[id]
		cbMu.Unlock()
		if fn == nil {
			return
		}
		var cev C.rns_event
		if len(ev.AppData) > 0 && len(buf) > 0 {
			cev.app_data = (*C.uint8_t)(unsafe.Pointer(&buf[0]))
			cev.app_data_cap = sizeFromInt(len(buf))
		}
		fillEvent(&cev, ev)
		C.call_rns_event_callback(fn, &cev, ud)
	}))
}

func fillPathEntry(dst *C.rns_path_entry, e librns.PathEntry) {
	dst.hash_len = 0
	dst.via_len = 0
	dst.hops = C.uint8_t(e.Hops)
	dst.timestamp = C.double(e.Timestamp)
	dst.expires = C.double(e.Expires)
	dst.iface[0] = 0
	if len(e.Hash) > 0 {
		n := copyToFixed((*[16]C.uint8_t)(unsafe.Pointer(&dst.hash[0])), e.Hash)
		dst.hash_len = sizeFromInt(n)
	}
	if len(e.Via) > 0 {
		n := copyToFixed((*[16]C.uint8_t)(unsafe.Pointer(&dst.via[0])), e.Via)
		dst.via_len = sizeFromInt(n)
	}
	if e.Interface != "" {
		copyCString(&dst.iface[0], C.size_t(len(dst.iface)), e.Interface)
	}
}

func fillInterfaceEntry(dst *C.rns_interface_entry, e librns.InterfaceEntry) {
	dst.name[0] = 0
	dst.type_name[0] = 0
	dst.online = 0
	dst.enabled = 0
	if e.Online {
		dst.online = 1
	}
	if e.Enabled {
		dst.enabled = 1
	}
	dst.rx_bytes = C.uint64_t(e.RxBytes)
	dst.tx_bytes = C.uint64_t(e.TxBytes)
	dst.rx_packets = C.uint64_t(e.RxPackets)
	dst.tx_packets = C.uint64_t(e.TxPackets)
	if e.Name != "" {
		copyCString(&dst.name[0], C.size_t(len(dst.name)), e.Name)
	}
	if e.Type != "" {
		copyCString(&dst.type_name[0], C.size_t(len(dst.type_name)), e.Type)
	}
}

func fillEvent(dst *C.rns_event, ev librns.Event) {
	dst.kind = cCode(ev.Kind)
	dst.hops = C.uint8_t(ev.Hops)
	dst.link_id_len = 0
	dst.destination_hash_len = 0
	dst.identity_hash_len = 0
	dst.request_id_len = 0
	dst.path_truncated = 0
	dst.error_message_truncated = 0
	dst.app_data_len = 0
	dst.app_data_truncated = 0
	dst.path[0] = 0
	dst.error_message[0] = 0

	if len(ev.LinkID) > 0 {
		n := copyToFixed((*[16]C.uint8_t)(unsafe.Pointer(&dst.link_id[0])), ev.LinkID)
		dst.link_id_len = sizeFromInt(n)
	}
	if len(ev.DestinationHash) > 0 {
		n := copyToFixed((*[16]C.uint8_t)(unsafe.Pointer(&dst.destination_hash[0])), ev.DestinationHash)
		dst.destination_hash_len = sizeFromInt(n)
	}
	if len(ev.IdentityHash) > 0 {
		n := copyToFixed((*[16]C.uint8_t)(unsafe.Pointer(&dst.identity_hash[0])), ev.IdentityHash)
		dst.identity_hash_len = sizeFromInt(n)
	}
	if len(ev.RequestID) > 0 {
		n := copyToFixed((*[16]C.uint8_t)(unsafe.Pointer(&dst.request_id[0])), ev.RequestID)
		dst.request_id_len = sizeFromInt(n)
	}
	if ev.Path != "" {
		n := copyCString(&dst.path[0], C.size_t(len(dst.path)), ev.Path)
		if n < len(ev.Path) {
			dst.path_truncated = 1
		}
	}
	if ev.ErrorMessage != "" {
		n := copyCString(&dst.error_message[0], C.size_t(len(dst.error_message)), ev.ErrorMessage)
		if n < len(ev.ErrorMessage) {
			dst.error_message_truncated = 1
		}
	}
	if len(ev.AppData) > 0 && dst.app_data != nil && dst.app_data_cap > 0 {
		n := copyCBytes(dst.app_data, dst.app_data_cap, ev.AppData)
		dst.app_data_len = sizeFromInt(n)
		if n < len(ev.AppData) {
			dst.app_data_truncated = 1
		}
	} else if len(ev.AppData) > 0 {
		dst.app_data_len = sizeFromInt(len(ev.AppData))
		dst.app_data_truncated = 1
	}
}

func copyToFixed(dst *[16]C.uint8_t, src []byte) int {
	n := len(src)
	if n > 16 {
		n = 16
	}
	for i := 0; i < n; i++ {
		dst[i] = C.uint8_t(src[i])
	}
	return n
}

func copyFixedBytes(out *C.uint8_t, outLen C.size_t, written *C.size_t, src []byte) C.int {
	if out == nil || outLen == 0 {
		if written != nil {
			*written = sizeFromInt(len(src))
		}
		return cCode(librns.ErrTruncated)
	}
	dstLen, ok := sizeToInt(outLen)
	if !ok {
		return cCode(librns.ErrInvalidArg)
	}
	n := copy(unsafe.Slice((*byte)(unsafe.Pointer(out)), dstLen), src)
	if written != nil {
		*written = sizeFromInt(n)
	}
	if n < len(src) {
		return cCode(librns.ErrTruncated)
	}
	return cCode(librns.OK)
}

func copyCString(dst *C.char, dstLen C.size_t, s string) int {
	limit, ok := sizeToInt(dstLen)
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

func cStringOrEmpty(p *C.char) string {
	if p == nil {
		return ""
	}
	return C.GoString(p)
}

func cStringArray(aspects **C.char, count int) []string {
	if aspects == nil || count <= 0 {
		return nil
	}
	slice := unsafe.Slice(aspects, count)
	out := make([]string, 0, count)
	for _, p := range slice {
		if p == nil {
			continue
		}
		out = append(out, C.GoString(p))
	}
	return out
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
	return int(n), true // #nosec G115 -- bounded by maxCGoBytes
}

func sizeFromInt(n int) C.size_t {
	if n < 0 {
		return 0
	}
	return C.size_t(n) // #nosec G115 -- non-negative Go length
}

func goBytesFromC(ptr *C.uint8_t, n C.size_t) ([]byte, int) {
	if n == 0 {
		return nil, librns.OK
	}
	if ptr == nil {
		return nil, librns.ErrInvalidArg
	}
	cint := C.rns_size_as_cint(n)
	if cint < 0 {
		return nil, librns.ErrInvalidArg
	}
	return C.GoBytes(unsafe.Pointer(ptr), cint), librns.OK
}

func cCode(code int) C.int {
	switch code {
	case librns.OK:
		return 0
	case librns.ErrInvalidArg:
		return 1
	case librns.ErrInvalidHandle:
		return 2
	case librns.ErrNotFound:
		return 3
	case librns.ErrState:
		return 4
	case librns.ErrIO:
		return 5
	case librns.ErrInternal:
		return 6
	case librns.ErrTimeout:
		return 7
	case librns.ErrTruncated:
		return 8
	default:
		return 6
	}
}
