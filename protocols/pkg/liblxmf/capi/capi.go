//go:build cgo

package capi

/*
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <limits.h>

static inline int lxmf_size_as_cint(size_t n) {
	if (n > (size_t)INT_MAX) {
		return -1;
	}
	return (int)n;
}
*/
import "C"

import (
	"math"
	"sync"
	"unsafe"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/liblxmf"
)

const maxCGoBytes = math.MaxInt32

var versionCString *C.char

func init() { versionCString = C.CString(liblxmf.Version()) }

//export lxmf_version
func lxmf_version() *C.char { return versionCString }

//export lxmf_last_error
func lxmf_last_error(buf *C.char, bufLen C.size_t, written *C.size_t) C.int {
	msg := liblxmf.LastError()
	if written != nil {
		*written = 0
	}
	if buf == nil || bufLen == 0 {
		if written != nil {
			*written = sizeFromInt(len(msg))
		}
		if len(msg) > 0 {
			return cCode(liblxmf.ErrTruncated)
		}
		return cCode(liblxmf.OK)
	}
	n := copyCString(buf, bufLen, msg)
	if written != nil {
		*written = sizeFromInt(n)
	}
	if n < len(msg) {
		return cCode(liblxmf.ErrTruncated)
	}
	return cCode(liblxmf.OK)
}

//export lxmf_identity_generate
func lxmf_identity_generate() C.uint64_t {
	id, _ := liblxmf.IdentityGenerate()
	return C.uint64_t(id)
}

//export lxmf_identity_load
func lxmf_identity_load(path *C.char) C.uint64_t {
	p := ""
	if path != nil {
		p = C.GoString(path)
	}
	id, _ := liblxmf.IdentityLoad(p)
	return C.uint64_t(id)
}

//export lxmf_identity_save
func lxmf_identity_save(identity C.uint64_t, path *C.char) C.int {
	p := ""
	if path != nil {
		p = C.GoString(path)
	}
	return cCode(liblxmf.IdentitySave(uint64(identity), p))
}

//export lxmf_identity_destroy
func lxmf_identity_destroy(identity C.uint64_t) C.int {
	return cCode(liblxmf.IdentityDestroy(uint64(identity)))
}

//export lxmf_identity_hash
func lxmf_identity_hash(identity C.uint64_t, out *C.uint8_t, outLen C.size_t, written *C.size_t) C.int {
	data, code := liblxmf.IdentityHash(uint64(identity))
	if code != liblxmf.OK {
		return cCode(code)
	}
	return writeBytes(out, outLen, written, data)
}

//export lxmf_identity_public_key
func lxmf_identity_public_key(identity C.uint64_t, out *C.uint8_t, outLen C.size_t, written *C.size_t) C.int {
	data, code := liblxmf.IdentityPublicKey(uint64(identity))
	if code != liblxmf.OK {
		return cCode(code)
	}
	return writeBytes(out, outLen, written, data)
}

//export lxmf_identity_delivery_hash
func lxmf_identity_delivery_hash(identity C.uint64_t, out *C.uint8_t, outLen C.size_t, written *C.size_t) C.int {
	data, code := liblxmf.IdentityDeliveryHash(uint64(identity))
	if code != liblxmf.OK {
		return cCode(code)
	}
	return writeBytes(out, outLen, written, data)
}

//export lxmf_identity_register_recall
func lxmf_identity_register_recall(identity C.uint64_t) C.int {
	return cCode(liblxmf.IdentityRegisterRecall(uint64(identity)))
}

//export lxmf_identity_register_recall_source
func lxmf_identity_register_recall_source(source *C.uint8_t, sourceLen C.size_t, publicKey *C.uint8_t, publicKeyLen C.size_t) C.int {
	src, code := goBytesFromC(source, sourceLen)
	if code != liblxmf.OK {
		return cCode(code)
	}
	pk, code := goBytesFromC(publicKey, publicKeyLen)
	if code != liblxmf.OK {
		return cCode(code)
	}
	return cCode(liblxmf.IdentityRegisterRecallSource(src, pk))
}

//export lxmf_message_create
func lxmf_message_create(dest *C.uint8_t, destLen C.size_t, source *C.uint8_t, sourceLen C.size_t, title, content *C.char) C.uint64_t {
	d, code := goBytesFromC(dest, destLen)
	if code != liblxmf.OK {
		return 0
	}
	s, code := goBytesFromC(source, sourceLen)
	if code != liblxmf.OK {
		return 0
	}
	id, _ := liblxmf.MessageCreate(d, s, C.GoString(title), C.GoString(content))
	return C.uint64_t(id)
}

//export lxmf_message_pack
func lxmf_message_pack(message, identity C.uint64_t, out *C.uint8_t, outLen C.size_t, written *C.size_t) C.int {
	data, code := liblxmf.MessagePack(uint64(message), uint64(identity))
	if code != liblxmf.OK {
		return cCode(code)
	}
	return writeBytes(out, outLen, written, data)
}

//export lxmf_message_encrypted_payload
func lxmf_message_encrypted_payload(message C.uint64_t, out *C.uint8_t, outLen C.size_t, written *C.size_t) C.int {
	data, code := liblxmf.MessageEncryptedPayload(uint64(message))
	if code != liblxmf.OK {
		return cCode(code)
	}
	return writeBytes(out, outLen, written, data)
}

//export lxmf_message_pack_propagated
func lxmf_message_pack_propagated(
	message, identity C.uint64_t,
	recipientHash *C.uint8_t, recipientHashLen C.size_t,
	recipientPublicKey *C.uint8_t, recipientPublicKeyLen C.size_t,
	pnStampCost C.int,
	out *C.uint8_t, outLen C.size_t, written *C.size_t,
) C.int {
	hash, code := goBytesFromC(recipientHash, recipientHashLen)
	if code != liblxmf.OK {
		return cCode(code)
	}
	pk, code := goBytesFromC(recipientPublicKey, recipientPublicKeyLen)
	if code != liblxmf.OK {
		return cCode(code)
	}
	data, code := liblxmf.MessagePackPropagated(
		uint64(message), uint64(identity), hash, pk, int(pnStampCost),
	)
	if code != liblxmf.OK {
		return cCode(code)
	}
	return writeBytes(out, outLen, written, data)
}

//export lxmf_message_unpack
func lxmf_message_unpack(data *C.uint8_t, dataLen C.size_t) C.uint64_t {
	raw, code := goBytesFromC(data, dataLen)
	if code != liblxmf.OK {
		return 0
	}
	id, _ := liblxmf.MessageUnpack(raw)
	return C.uint64_t(id)
}

//export lxmf_message_get_dest
func lxmf_message_get_dest(message C.uint64_t, out *C.uint8_t, outLen C.size_t, written *C.size_t) C.int {
	data, code := liblxmf.MessageGetDest(uint64(message))
	if code != liblxmf.OK {
		return cCode(code)
	}
	return writeBytes(out, outLen, written, data)
}

//export lxmf_message_get_source
func lxmf_message_get_source(message C.uint64_t, out *C.uint8_t, outLen C.size_t, written *C.size_t) C.int {
	data, code := liblxmf.MessageGetSource(uint64(message))
	if code != liblxmf.OK {
		return cCode(code)
	}
	return writeBytes(out, outLen, written, data)
}

//export lxmf_message_get_title
func lxmf_message_get_title(message C.uint64_t, buf *C.char, bufLen C.size_t, written *C.size_t) C.int {
	s, code := liblxmf.MessageGetTitle(uint64(message))
	if code != liblxmf.OK {
		return cCode(code)
	}
	return writeString(buf, bufLen, written, s)
}

//export lxmf_message_get_content
func lxmf_message_get_content(message C.uint64_t, buf *C.char, bufLen C.size_t, written *C.size_t) C.int {
	s, code := liblxmf.MessageGetContent(uint64(message))
	if code != liblxmf.OK {
		return cCode(code)
	}
	return writeString(buf, bufLen, written, s)
}

//export lxmf_message_get_hash
func lxmf_message_get_hash(message C.uint64_t, out *C.uint8_t, outLen C.size_t, written *C.size_t) C.int {
	data, code := liblxmf.MessageGetHash(uint64(message))
	if code != liblxmf.OK {
		return cCode(code)
	}
	return writeBytes(out, outLen, written, data)
}

//export lxmf_message_set_fields_json
func lxmf_message_set_fields_json(message C.uint64_t, json *C.char) C.int {
	if json == nil {
		return cCode(liblxmf.ErrInvalidArg)
	}
	return cCode(liblxmf.MessageSetFieldsJSON(uint64(message), []byte(C.GoString(json))))
}

//export lxmf_message_fields_json
func lxmf_message_fields_json(message C.uint64_t, buf *C.char, bufLen C.size_t, written *C.size_t) C.int {
	data, code := liblxmf.MessageFieldsJSON(uint64(message))
	if code != liblxmf.OK {
		return cCode(code)
	}
	return writeString(buf, bufLen, written, string(data))
}

//export lxmf_message_field_count
func lxmf_message_field_count(message C.uint64_t, count *C.size_t) C.int {
	if count == nil {
		return cCode(liblxmf.ErrInvalidArg)
	}
	n, code := liblxmf.MessageFieldCount(uint64(message))
	if code != liblxmf.OK {
		return cCode(code)
	}
	*count = sizeFromInt(n)
	return cCode(liblxmf.OK)
}

//export lxmf_message_unpack_verified
func lxmf_message_unpack_verified(data *C.uint8_t, dataLen C.size_t, identity C.uint64_t) C.uint64_t {
	raw, code := goBytesFromC(data, dataLen)
	if code != liblxmf.OK {
		return 0
	}
	id, _ := liblxmf.MessageUnpackVerified(raw, uint64(identity))
	return C.uint64_t(id)
}

//export lxmf_message_destroy
func lxmf_message_destroy(message C.uint64_t) C.int {
	return cCode(liblxmf.MessageDestroy(uint64(message)))
}

//export lxmf_stamp_cost_from_app_data
func lxmf_stamp_cost_from_app_data(appData *C.uint8_t, appDataLen C.size_t, costOut *C.int64_t) C.int {
	if costOut == nil {
		return cCode(liblxmf.ErrInvalidArg)
	}
	raw, code := goBytesFromC(appData, appDataLen)
	if code != liblxmf.OK {
		*costOut = 0
		return cCode(code)
	}
	cost, code := liblxmf.StampCostFromAppData(raw)
	*costOut = C.int64_t(cost)
	return cCode(code)
}

//export lxmf_pn_stamp_cost_from_app_data
func lxmf_pn_stamp_cost_from_app_data(appData *C.uint8_t, appDataLen C.size_t, costOut *C.int64_t) C.int {
	if costOut == nil {
		return cCode(liblxmf.ErrInvalidArg)
	}
	raw, code := goBytesFromC(appData, appDataLen)
	if code != liblxmf.OK {
		*costOut = 0
		return cCode(code)
	}
	cost, code := liblxmf.PNStampCostFromAppData(raw)
	*costOut = C.int64_t(cost)
	return cCode(code)
}

//export lxmf_message_apply_stamp
func lxmf_message_apply_stamp(message, identity C.uint64_t, stampCost, timeoutMs C.int) C.int {
	return cCode(liblxmf.MessageApplyStamp(uint64(message), uint64(identity), int(stampCost), int(timeoutMs)))
}

//export lxmf_encode_announce_app_data
func lxmf_encode_announce_app_data(displayName *C.char, stampCost C.int64_t, iconName *C.char, fgRGB3, bgRGB3 *C.uint8_t, out *C.uint8_t, outLen C.size_t, written *C.size_t) C.int {
	name := ""
	if displayName != nil {
		name = C.GoString(displayName)
	}
	icon := ""
	if iconName != nil {
		icon = C.GoString(iconName)
	}
	var fg, bg []byte
	if fgRGB3 != nil {
		fg = C.GoBytes(unsafe.Pointer(fgRGB3), 3)
	}
	if bgRGB3 != nil {
		bg = C.GoBytes(unsafe.Pointer(bgRGB3), 3)
	}
	data, code := liblxmf.EncodeAnnounceAppData(name, int64(stampCost), icon, fg, bg)
	if code != liblxmf.OK {
		return cCode(code)
	}
	return writeBytes(out, outLen, written, data)
}

func goBytesFromC(ptr *C.uint8_t, n C.size_t) ([]byte, int) {
	if n == 0 {
		return nil, liblxmf.OK
	}
	if ptr == nil {
		return nil, liblxmf.ErrInvalidArg
	}
	cint := C.lxmf_size_as_cint(n)
	if cint < 0 {
		return nil, liblxmf.ErrInvalidArg
	}
	return C.GoBytes(unsafe.Pointer(ptr), cint), liblxmf.OK
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
			return cCode(liblxmf.ErrTruncated)
		}
		return cCode(liblxmf.OK)
	}
	n := copyCBytes(out, outLen, src)
	if written != nil {
		*written = sizeFromInt(n)
	}
	if n < len(src) {
		return cCode(liblxmf.ErrTruncated)
	}
	return cCode(liblxmf.OK)
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
			return cCode(liblxmf.ErrTruncated)
		}
		return cCode(liblxmf.OK)
	}
	n := copyCString(buf, bufLen, s)
	if written != nil {
		*written = sizeFromInt(n)
	}
	if n < len(s) {
		return cCode(liblxmf.ErrTruncated)
	}
	return cCode(liblxmf.OK)
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

func cCode(code int) C.int {
	switch code {
	case liblxmf.OK:
		return 0
	case liblxmf.ErrInvalidArg:
		return 1
	case liblxmf.ErrInvalidHandle:
		return 2
	case liblxmf.ErrInternal:
		return 6
	case liblxmf.ErrTruncated:
		return 8
	default:
		return 6
	}
}

var _ = sync.Mutex{}
