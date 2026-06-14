package msgpack

import (
	"io"
	"reflect"
	"sync"
	"time"

	"quad4/msgpack/v5/pkg/msgpack/msgpcode"
)

const (
	sortMapKeysFlag uint32 = 1 << iota
	arrayEncodedStructsFlag
	useCompactIntsFlag
	useCompactFloatsFlag
	useInternedStringsFlag
	omitEmptyFlag
)

type writer interface {
	io.Writer
	WriteByte(byte) error
}

// byteWriter adapts an io.Writer that does not implement io.ByteWriter.
//
// The single-byte buffer is kept on the wrapper so WriteByte does not
// allocate a fresh one-byte slice on every call. Wrappers are created
// per Encoder.Reset, so there is no concurrency concern.
type byteWriter struct {
	io.Writer
	scratch [1]byte
}

func newByteWriter(w io.Writer) *byteWriter {
	return &byteWriter{Writer: w}
}

func (bw *byteWriter) WriteByte(c byte) error {
	bw.scratch[0] = c
	_, err := bw.Write(bw.scratch[:])
	return err
}

type appendWriter struct {
	b []byte
}

func (w *appendWriter) Write(p []byte) (int, error) {
	w.b = append(w.b, p...)
	return len(p), nil
}

func (w *appendWriter) WriteByte(c byte) error {
	w.b = append(w.b, c)
	return nil
}

//------------------------------------------------------------------------------

var encPool = sync.Pool{
	New: func() interface{} {
		return NewEncoder(nil)
	},
}

func GetEncoder() *Encoder {
	return encPool.Get().(*Encoder)
}

func PutEncoder(enc *Encoder) {
	enc.w = nil
	encPool.Put(enc)
}

// marshalInitialBufSize is the initial backing capacity reserved by Marshal.
//
// Pre-growing avoids the first one or two doublings (8/16/32/64) for the
// majority of small payloads (numbers, short strings, small structs) without
// over-allocating for callers that encode large values.
const marshalInitialBufSize = 64

// Marshal returns the MessagePack encoding of v.
func Marshal(v interface{}) ([]byte, error) {
	return AppendMarshal(nil, v)
}

// AppendMarshal appends the MessagePack encoding of v into dst[:0] and
// returns the resulting slice.
//
// The returned bytes may reuse dst's backing array. This allows callers to
// keep a reusable scratch buffer and avoid per-call output allocations in
// hot paths.
func AppendMarshal(dst []byte, v interface{}) ([]byte, error) {
	enc := GetEncoder()
	enc.Reset(nil)
	b, err := enc.Append(dst, v)
	PutEncoder(enc)
	return b, err
}

// Append encodes v into dst[:0] and returns the resulting bytes.
//
// The returned bytes may reuse dst's backing array. Reusing both an Encoder
// and dst allows hot paths to avoid per-call output allocations.
func (e *Encoder) Append(dst []byte, v interface{}) ([]byte, error) {
	aw := &e.appendBuf
	aw.b = dst[:0]
	if cap(dst) == 0 {
		aw.b = make([]byte, 0, marshalInitialBufSize)
	}

	oldWriter := e.w
	e.w = aw
	err := e.Encode(v)
	e.w = oldWriter
	if err != nil {
		return nil, err
	}
	return aw.b, nil
}

type Encoder struct {
	w         writer
	dict      map[string]int
	structTag string
	buf       []byte
	timeBuf   []byte
	appendBuf appendWriter
	flags     uint32
}

// NewEncoder returns a new encoder that writes to w.
func NewEncoder(w io.Writer) *Encoder {
	e := &Encoder{
		buf: make([]byte, 9),
	}
	e.Reset(w)
	return e
}

// Writer returns the Encoder's writer.
func (e *Encoder) Writer() io.Writer {
	return e.w
}

// Reset discards any buffered data, resets all state, and switches the writer to write to w.
func (e *Encoder) Reset(w io.Writer) {
	e.ResetDict(w, nil)
}

// ResetDict is like Reset, but also resets the dict.
func (e *Encoder) ResetDict(w io.Writer, dict map[string]int) {
	e.ResetWriter(w)
	e.flags = 0
	e.structTag = ""
	e.dict = dict
}

func (e *Encoder) WithDict(dict map[string]int, fn func(*Encoder) error) error {
	oldDict := e.dict
	e.dict = dict
	err := fn(e)
	e.dict = oldDict
	return err
}

func (e *Encoder) ResetWriter(w io.Writer) {
	e.dict = nil
	if bw, ok := w.(writer); ok {
		e.w = bw
	} else if w == nil {
		e.w = nil
	} else {
		e.w = newByteWriter(w)
	}
}

// SetSortMapKeys causes the Encoder to encode map keys in increasing order.
// Supported map types are:
//   - map[string]string
//   - map[string]bool
//   - map[string]interface{}
func (e *Encoder) SetSortMapKeys(on bool) *Encoder {
	if on {
		e.flags |= sortMapKeysFlag
	} else {
		e.flags &= ^sortMapKeysFlag
	}
	return e
}

// SetCustomStructTag causes the Encoder to use a custom struct tag as
// fallback option if there is no msgpack tag.
func (e *Encoder) SetCustomStructTag(tag string) {
	e.structTag = tag
}

// SetOmitEmpty causes the Encoder to omit empty values by default.
func (e *Encoder) SetOmitEmpty(on bool) {
	if on {
		e.flags |= omitEmptyFlag
	} else {
		e.flags &= ^omitEmptyFlag
	}
}

// UseArrayEncodedStructs causes the Encoder to encode Go structs as msgpack arrays.
func (e *Encoder) UseArrayEncodedStructs(on bool) {
	if on {
		e.flags |= arrayEncodedStructsFlag
	} else {
		e.flags &= ^arrayEncodedStructsFlag
	}
}

// UseCompactEncoding causes the Encoder to chose the most compact encoding.
// For example, it allows to encode small Go int64 as msgpack int8 saving 7 bytes.
func (e *Encoder) UseCompactInts(on bool) {
	if on {
		e.flags |= useCompactIntsFlag
	} else {
		e.flags &= ^useCompactIntsFlag
	}
}

// UseCompactFloats causes the Encoder to chose a compact integer encoding
// for floats that can be represented as integers.
func (e *Encoder) UseCompactFloats(on bool) {
	if on {
		e.flags |= useCompactFloatsFlag
	} else {
		e.flags &= ^useCompactFloatsFlag
	}
}

// UseInternedStrings causes the Encoder to intern strings.
func (e *Encoder) UseInternedStrings(on bool) {
	if on {
		e.flags |= useInternedStringsFlag
	} else {
		e.flags &= ^useInternedStringsFlag
	}
}

func (e *Encoder) Encode(v interface{}) error {
	switch v := v.(type) {
	case nil:
		return e.EncodeNil()
	case string:
		return e.EncodeString(v)
	case []byte:
		return e.EncodeBytes(v)
	case int:
		return e.EncodeInt(int64(v))
	case int64:
		return e.encodeInt64Cond(v)
	case uint:
		return e.EncodeUint(uint64(v))
	case uint64:
		return e.encodeUint64Cond(v)
	case bool:
		return e.EncodeBool(v)
	case float32:
		return e.EncodeFloat32(v)
	case float64:
		return e.EncodeFloat64(v)
	case time.Duration:
		return e.encodeInt64Cond(int64(v))
	case time.Time:
		return e.EncodeTime(v)
	}
	return e.EncodeValue(reflect.ValueOf(v))
}

func (e *Encoder) EncodeMulti(v ...interface{}) error {
	for _, vv := range v {
		if err := e.Encode(vv); err != nil {
			return err
		}
	}
	return nil
}

func (e *Encoder) EncodeValue(v reflect.Value) error {
	fn := getEncoder(v.Type())
	return fn(e, v)
}

func (e *Encoder) EncodeNil() error {
	return e.writeCode(msgpcode.Nil)
}

func (e *Encoder) EncodeBool(value bool) error {
	if value {
		return e.writeCode(msgpcode.True)
	}
	return e.writeCode(msgpcode.False)
}

func (e *Encoder) EncodeDuration(d time.Duration) error {
	return e.EncodeInt(int64(d))
}

func (e *Encoder) writeCode(c byte) error {
	return e.w.WriteByte(c)
}

func (e *Encoder) write(b []byte) error {
	_, err := e.w.Write(b)
	return err
}

func (e *Encoder) writeString(s string) error {
	_, err := e.w.Write(stringToBytes(s))
	return err
}
