package msgpack

import (
	"fmt"
	"reflect"

	"quad4/msgpack/v5/pkg/msgpack/msgpcode"
)

var sliceStringPtrType = reflect.TypeFor[*[]string]()

// DecodeArrayLen decodes array length. Length is -1 when array is nil.
func (d *Decoder) DecodeArrayLen() (int, error) {
	c, err := d.readCode()
	if err != nil {
		return 0, err
	}
	return d.arrayLen(c)
}

func (d *Decoder) arrayLen(c byte) (int, error) {
	if c == msgpcode.Nil {
		return -1, nil
	} else if c >= msgpcode.FixedArrayLow && c <= msgpcode.FixedArrayHigh {
		n := int(c & msgpcode.FixedArrayMask)
		if err := d.rejectOversizedContainer(n, 1, "array"); err != nil {
			return 0, err
		}
		return n, nil
	}
	switch c {
	case msgpcode.Array16:
		n, err := d.uint16()
		if err != nil {
			return 0, err
		}
		if err := d.rejectOversizedContainer(int(n), 1, "array"); err != nil {
			return 0, err
		}
		return int(n), nil
	case msgpcode.Array32:
		n, err := d.uint32()
		if err != nil {
			return 0, err
		}
		size, err := uint32ToInt(n, "array length")
		if err != nil {
			return 0, err
		}
		if err := d.rejectOversizedContainer(size, 1, "array"); err != nil {
			return 0, err
		}
		return size, nil
	}
	return 0, fmt.Errorf("msgpack: invalid code=%x decoding array length", c)
}

func decodeStringSliceValue(d *Decoder, v reflect.Value) error {
	ptr := v.Addr().Convert(sliceStringPtrType).Interface().(*[]string)
	return d.decodeStringSlicePtr(ptr)
}

func (d *Decoder) decodeStringSlicePtr(ptr *[]string) error {
	n, err := d.DecodeArrayLen()
	if err != nil {
		return err
	}
	if n == -1 {
		return nil
	}

	ss := makeStrings(*ptr, n, d.flags&disableAllocLimitFlag != 0)
	for range n {
		s, err := d.DecodeString()
		if err != nil {
			return err
		}
		ss = append(ss, s)
	}
	*ptr = ss

	return nil
}

func makeStrings(s []string, n int, noLimit bool) []string {
	if !noLimit && n > sliceAllocLimit {
		n = sliceAllocLimit
	}

	if s == nil {
		return make([]string, 0, n)
	}

	if cap(s) >= n {
		return s[:0]
	}

	s = s[:cap(s)]
	s = append(s, make([]string, n-len(s))...)
	return s[:0]
}

func decodeSliceValue(d *Decoder, v reflect.Value) error {
	n, err := d.DecodeArrayLen()
	if err != nil {
		return err
	}

	if n == -1 {
		v.Set(reflect.Zero(v.Type()))
		return nil
	}
	if n == 0 && v.IsNil() {
		v.Set(reflect.MakeSlice(v.Type(), 0, 0))
		return nil
	}

	if v.Cap() >= n {
		v.Set(v.Slice(0, n))
	} else if v.Len() < v.Cap() {
		v.Set(v.Slice(0, v.Cap()))
	}

	// noLimit is true only when the caller has explicitly disabled
	// allocation limits via UseAllocLimitDisable. When limits are in
	// effect, growSliceValue caps each grow step to sliceAllocLimit so an
	// oversized array32 length cannot trigger a multi-gigabyte up-front
	// allocation. Real elements still flow through as input arrives.
	noLimit := d.flags&disableAllocLimitFlag != 0

	if noLimit && n > v.Len() {
		v.Set(growSliceValue(v, n, noLimit))
	}

	for i := range n {
		if i >= v.Len() {
			v.Set(growSliceValue(v, n, noLimit))
		}

		elem := v.Index(i)
		if err := d.DecodeValue(elem); err != nil {
			return err
		}
	}

	return nil
}

func growSliceValue(v reflect.Value, n int, noLimit bool) reflect.Value {
	diff := n - v.Len()
	if !noLimit && diff > sliceAllocLimit {
		diff = sliceAllocLimit
	}
	v = reflect.AppendSlice(v, reflect.MakeSlice(v.Type(), diff, diff))
	return v
}

func decodeArrayValue(d *Decoder, v reflect.Value) error {
	n, err := d.DecodeArrayLen()
	if err != nil {
		return err
	}

	if n == -1 {
		return nil
	}
	if n > v.Len() {
		return fmt.Errorf("%s len is %d, but msgpack has %d elements", v.Type(), v.Len(), n)
	}

	for i := range n {
		sv := v.Index(i)
		if err := d.DecodeValue(sv); err != nil {
			return err
		}
	}

	return nil
}

func (d *Decoder) DecodeSlice() ([]any, error) {
	c, err := d.readCode()
	if err != nil {
		return nil, err
	}
	return d.decodeSlice(c)
}

func (d *Decoder) decodeSlice(c byte) ([]any, error) {
	n, err := d.arrayLen(c)
	if err != nil {
		return nil, err
	}
	if n == -1 {
		return nil, nil
	}

	// Clamp the initial backing-array allocation so a truncated or
	// oversized header such as array32 with a huge length cannot request
	// an arbitrarily large slice up front. The decoder still grows the
	// slice via append as real elements arrive, so well-formed input with
	// more than sliceAllocLimit elements continues to round-trip when the
	// limit is disabled.
	initCap := n
	if d.flags&disableAllocLimitFlag == 0 && initCap > sliceAllocLimit {
		initCap = sliceAllocLimit
	}

	s := make([]any, 0, initCap)
	for range n {
		v, err := d.decodeInterfaceCond()
		if err != nil {
			return nil, err
		}
		s = append(s, v)
	}

	return s, nil
}

func (d *Decoder) skipSlice(c byte) error {
	n, err := d.arrayLen(c)
	if err != nil {
		return err
	}

	for range n {
		if err := d.Skip(); err != nil {
			return err
		}
	}

	return nil
}
